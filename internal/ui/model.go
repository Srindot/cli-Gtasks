package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/srindot/gtask/internal/gtasks"
)

type mode int

const (
	modeNormal mode = iota
	modeInsert
	modeCommand
	modeSearch
	modePopup
	modeConfirm
	modeNotes
	modeHelp
)

type pane int

const (
	paneLists pane = iota
	paneTasks
	paneDetails
)

// Focusable fields in the details pane; subtasks follow from fSubs on.
const (
	fTitle = iota
	fDue
	fStatus
	fNotes
	fSubs
)

type editKind int

const (
	editNone editKind = iota
	editNew
	editRename
)

type edit struct {
	kind             editKind
	listID, taskID   string // rename
	parent, previous string // new
}

type undoEntry struct {
	desc string
	do   func(m *Model) tea.Cmd
}

type toast struct {
	text, color string
	at          time.Time
}

// ---- sync plumbing -------------------------------------------------------

// idMap resolves optimistic local IDs to the IDs Google assigns later.
type idMap struct {
	mu sync.Mutex
	m  map[string]string
}

func (i *idMap) get(id string) string {
	i.mu.Lock()
	defer i.mu.Unlock()
	for {
		r, ok := i.m[id]
		if !ok {
			return id
		}
		id = r
	}
}

func (i *idMap) set(local, real string) {
	i.mu.Lock()
	i.m[local] = real
	i.mu.Unlock()
}

type op struct {
	desc string
	run  func(ctx context.Context, be gtasks.Backend, id func(string) string) (tea.Msg, error)
}

type (
	opDoneMsg struct {
		msg  tea.Msg
		desc string
		err  error
	}
	listsMsg struct {
		lists []gtasks.List
		err   error
	}
	tasksMsg struct {
		listID string
		tasks  []gtasks.Task
		err    error
	}
	createdMsg struct {
		listID, local string
		task          gtasks.Task
	}
	listCreatedMsg struct {
		local string
		list  gtasks.List
	}
	editorMsg struct {
		listID, taskID, path string
		err                  error
	}
	tickMsg     time.Time
	autoSyncMsg struct{}
)

func worker(be gtasks.Backend, ops <-chan op, out chan<- opDoneMsg, ids *idMap) {
	for o := range ops {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		msg, err := o.run(ctx, be, ids.get)
		cancel()
		out <- opDoneMsg{msg: msg, desc: o.desc, err: err}
	}
}

func waitOp(ch <-chan opDoneMsg) tea.Cmd { return func() tea.Msg { return <-ch } }

// ---- model ---------------------------------------------------------------

type Options struct {
	Backend   gtasks.Backend
	CachePath string
	Demo      bool
}

type Model struct {
	be      gtasks.Backend
	ops     chan op
	results chan opDoneMsg
	ids     *idMap

	pending     int // ops queued but not yet acknowledged by the server
	fetching    int
	needRefresh bool
	lastSync    time.Time
	offline     bool

	lists   []gtasks.List
	tasks   map[string][]*gtasks.Task
	loaded  map[string]bool
	listIdx int
	cursors map[string]int
	cursor  int
	offset  int

	focus       pane
	focusAt     time.Time
	listAt      time.Time
	mode        mode
	in          textinput.Model
	ed          edit
	promptLabel string
	pop         *popup
	ta          textarea.Model
	notesFor    [2]string // list, task being edited in modeNotes
	dfield      int       // focused field in the details pane
	selAt       time.Time // when the selection last changed (details fade)
	keys        string    // pending multi-key prefix (g, d, y, c, z, >, <)
	count       int

	showDone bool
	folded   map[string]bool
	cleared  map[string]bool // completed tasks hidden with :clear
	filter   string
	undo     []undoEntry
	reg      []gtasks.Task // yank register: root first, then descendants

	anims   map[string]*anim
	trail   map[string]time.Time
	toast   *toast
	started time.Time
	ticking bool
	quit    bool

	w, h      int
	cachePath string
	demo      bool
	seq       int
}

type cacheFile struct {
	Lists    []gtasks.List             `json:"lists"`
	Tasks    map[string][]*gtasks.Task `json:"tasks"`
	Current  string                    `json:"current"`
	ShowDone bool                      `json:"showDone"`
	Folded   map[string]bool           `json:"folded"`
	Cleared  map[string]bool           `json:"cleared"`
}

func New(o Options) *Model {
	in := textinput.New()
	in.Prompt = ""
	in.Cursor.SetMode(cursor.CursorStatic)
	in.Cursor.Style = fg(cRose).Reverse(true)
	in.TextStyle = fg(cText)
	in.PlaceholderStyle = fg(cMuted)

	ta := textarea.New()
	ta.ShowLineNumbers = false
	ta.Prompt = ""
	ta.CharLimit = 8000
	ta.EndOfBufferCharacter = ' '
	ta.Placeholder = "write anything…"
	taStyle := textarea.Style{Base: lipgloss.NewStyle(), Text: fg(cText), CursorLine: fg(cText),
		Placeholder: fg(cMuted), EndOfBuffer: fg(cHlLow), Prompt: fg(cHlMed)}
	ta.FocusedStyle, ta.BlurredStyle = taStyle, taStyle
	ta.Cursor.SetMode(cursor.CursorStatic)
	ta.Cursor.Style = fg(cRose).Reverse(true)
	m := &Model{
		be: o.Backend, ops: make(chan op, 4096), results: make(chan opDoneMsg, 64),
		ids:       &idMap{m: map[string]string{}},
		tasks:     map[string][]*gtasks.Task{},
		loaded:    map[string]bool{},
		cursors:   map[string]int{},
		folded:    map[string]bool{},
		cleared:   map[string]bool{},
		anims:     map[string]*anim{},
		trail:     map[string]time.Time{},
		focus:     paneTasks,
		in:        in,
		ta:        ta,
		started:   time.Now(),
		cachePath: o.CachePath, demo: o.Demo,
	}
	m.loadCache()
	go worker(m.be, m.ops, m.results, m.ids)
	return m
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(waitOp(m.results), m.fetchLists(), tick(), autoSync())
}

func autoSync() tea.Cmd {
	return tea.Tick(90*time.Second, func(time.Time) tea.Msg { return autoSyncMsg{} })
}

func (m *Model) loadCache() {
	if m.cachePath == "" {
		return
	}
	b, err := os.ReadFile(m.cachePath)
	if err != nil {
		return
	}
	var c cacheFile
	if json.Unmarshal(b, &c) != nil {
		return
	}
	m.lists, m.showDone = c.Lists, c.ShowDone
	if c.Tasks != nil {
		m.tasks = c.Tasks
		for id := range c.Tasks {
			m.loaded[id] = true
		}
	}
	if c.Folded != nil {
		m.folded = c.Folded
	}
	if c.Cleared != nil {
		m.cleared = c.Cleared
	}
	for i, l := range m.lists {
		if l.ID == c.Current {
			m.listIdx = i
		}
	}
}

func (m *Model) saveCache() {
	if m.cachePath == "" {
		return
	}
	c := cacheFile{Lists: m.lists, Tasks: m.tasks, Current: m.curID(), ShowDone: m.showDone, Folded: m.folded, Cleared: m.cleared}
	b, err := json.Marshal(c)
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(m.cachePath), 0o700)
	tmp := m.cachePath + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		_ = os.Rename(tmp, m.cachePath)
	}
}

// ---- lookups -------------------------------------------------------------

func (m *Model) curList() *gtasks.List {
	if m.listIdx < 0 || m.listIdx >= len(m.lists) {
		return nil
	}
	return &m.lists[m.listIdx]
}

func (m *Model) curID() string {
	if l := m.curList(); l != nil {
		return l.ID
	}
	return ""
}

func (m *Model) listIndex(id string) int {
	id = m.ids.get(id)
	for i, l := range m.lists {
		if l.ID == id {
			return i
		}
	}
	return -1
}

func (m *Model) idx(listID, id string) int {
	id = m.ids.get(id)
	for i, t := range m.tasks[m.ids.get(listID)] {
		if t.ID == id {
			return i
		}
	}
	return -1
}

func (m *Model) find(listID, id string) *gtasks.Task {
	if i := m.idx(listID, id); i >= 0 {
		return m.tasks[m.ids.get(listID)][i]
	}
	return nil
}

func (m *Model) children(listID, id string) []*gtasks.Task {
	var out []*gtasks.Task
	for _, t := range m.tasks[listID] {
		if t.Parent == id {
			out = append(out, t)
		}
	}
	return out
}

// subtree returns t and all its descendants, parents before children.
func (m *Model) subtree(listID string, t *gtasks.Task) []*gtasks.Task {
	out := []*gtasks.Task{t}
	for _, c := range m.children(listID, t.ID) {
		out = append(out, m.subtree(listID, c)...)
	}
	return out
}

func (m *Model) prevSibling(listID string, t *gtasks.Task) string {
	prev := ""
	for _, o := range m.tasks[listID] {
		if o == t {
			return prev
		}
		if o.Parent == t.Parent {
			prev = o.ID
		}
	}
	return prev
}

func (m *Model) lastChild(listID, parent string) string {
	last := ""
	for _, o := range m.tasks[listID] {
		if o.Parent == parent {
			last = o.ID
		}
	}
	return last
}

func (m *Model) depth(listID string, t *gtasks.Task) int {
	d := 0
	for t != nil && t.Parent != "" && d < 8 {
		t = m.find(listID, t.Parent)
		d++
	}
	return d
}

func (m *Model) localID() string {
	m.seq++
	return fmt.Sprintf("local-%d-%d", time.Now().UnixNano(), m.seq)
}

// placeLocal mirrors the API's parent/previous semantics on the local slice:
// sibling order is slice order, so t goes right after previous or in front of
// its first sibling.
func (m *Model) placeLocal(listID string, t *gtasks.Task, parent, previous string) {
	listID = m.ids.get(listID)
	ts := m.tasks[listID]
	for i, o := range ts {
		if o == t {
			ts = append(ts[:i:i], ts[i+1:]...)
			break
		}
	}
	t.Parent = m.ids.get(parent)
	at := len(ts)
	if previous != "" {
		p := m.ids.get(previous)
		for i, o := range ts {
			if o.ID == p {
				at = i + 1
				break
			}
		}
	} else {
		for i, o := range ts {
			if o.Parent == t.Parent {
				at = i
				break
			}
		}
		if t.Parent == "" && at == len(ts) {
			at = 0
		}
	}
	ts = append(ts, nil)
	copy(ts[at+1:], ts[at:])
	ts[at] = t
	m.tasks[listID] = ts
}

func (m *Model) removeLocal(listID string, ids ...string) {
	listID = m.ids.get(listID)
	drop := map[string]bool{}
	for _, id := range ids {
		drop[m.ids.get(id)] = true
	}
	var keep []*gtasks.Task
	for _, t := range m.tasks[listID] {
		if !drop[t.ID] {
			keep = append(keep, t)
		}
	}
	m.tasks[listID] = keep
}

// ---- server sync ---------------------------------------------------------

func (m *Model) enqueue(o op) tea.Cmd {
	m.pending++
	m.ops <- o
	return nil
}

func (m *Model) fetchLists() tea.Cmd {
	m.fetching++
	be := m.be
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		ls, err := be.Lists(ctx)
		return listsMsg{ls, err}
	}
}

func (m *Model) fetchTasks(listID string) tea.Cmd {
	m.fetching++
	be := m.be
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		ts, err := be.Tasks(ctx, listID)
		return tasksMsg{listID, ts, err}
	}
}

// stable reports whether it's safe to replace local state with server state.
func (m *Model) stable() bool { return m.pending == 0 && len(m.anims) == 0 && m.ed.kind == editNone }

func (m *Model) maybeRefresh() tea.Cmd {
	if m.needRefresh && m.stable() {
		m.needRefresh = false
		return m.fetchLists()
	}
	return nil
}

func (m *Model) applyLists(ls []gtasks.List) tea.Cmd {
	cur := m.curID()
	m.lists = ls
	m.listIdx = 0
	if i := m.listIndex(cur); i >= 0 {
		m.listIdx = i
	}
	// Current list first so it lands before the rest.
	cmds := []tea.Cmd{}
	if id := m.curID(); id != "" {
		cmds = append(cmds, m.fetchTasks(id))
	}
	for _, l := range ls {
		if l.ID != m.curID() {
			cmds = append(cmds, m.fetchTasks(l.ID))
		}
	}
	return tea.Batch(cmds...)
}

func (m *Model) applyTasks(listID string, in []gtasks.Task) {
	sort.SliceStable(in, func(i, j int) bool { return in[i].Position < in[j].Position })
	sel := ""
	if listID == m.curID() {
		if t := m.selected(); t != nil {
			sel = t.ID
		}
	}
	// Google keeps cleared tasks around as hidden; only we remember which ones were cleared.
	ts := make([]*gtasks.Task, 0, len(in))
	for i := range in {
		if !(in[i].Done() && m.cleared[in[i].ID]) {
			ts = append(ts, &in[i])
		}
	}
	m.tasks[listID] = ts
	m.loaded[listID] = true
	if sel != "" {
		m.selectID(sel)
	}
}

func (m *Model) onCreated(msg createdMsg) {
	m.ids.set(msg.local, msg.task.ID)
	lid := m.ids.get(msg.listID)
	for _, t := range m.tasks[lid] {
		if t.ID == msg.local {
			t.ID, t.Position, t.Updated = msg.task.ID, msg.task.Position, msg.task.Updated
		}
		if t.Parent == msg.local {
			t.Parent = msg.task.ID
		}
	}
	rekey := func(id string) string {
		if id == msg.local {
			return msg.task.ID
		}
		return id
	}
	if a, ok := m.anims[msg.local]; ok {
		delete(m.anims, msg.local)
		m.anims[msg.task.ID] = a
	}
	if m.folded[msg.local] {
		delete(m.folded, msg.local)
		m.folded[msg.task.ID] = true
	}
	m.ed.taskID, m.ed.parent, m.ed.previous = rekey(m.ed.taskID), rekey(m.ed.parent), rekey(m.ed.previous)
}

func (m *Model) onListCreated(msg listCreatedMsg) {
	m.ids.set(msg.local, msg.list.ID)
	for i := range m.lists {
		if m.lists[i].ID == msg.local {
			m.lists[i] = msg.list
		}
	}
	if ts, ok := m.tasks[msg.local]; ok {
		delete(m.tasks, msg.local)
		m.tasks[msg.list.ID] = ts
		m.loaded[msg.list.ID] = true
	}
	if c, ok := m.cursors[msg.local]; ok {
		m.cursors[msg.list.ID] = c
	}
	if m.ed.listID == msg.local {
		m.ed.listID = msg.list.ID
	}
}

// ---- ops -----------------------------------------------------------------

func opInsert(listID, local string, t gtasks.Task, parent, previous string) op {
	return op{"add “" + t.Title + "”", func(ctx context.Context, be gtasks.Backend, id func(string) string) (tea.Msg, error) {
		t.ID = ""
		got, err := be.Insert(ctx, id(listID), t, id(parent), id(previous))
		if err != nil {
			return nil, err
		}
		return createdMsg{listID, local, got}, nil
	}}
}

func opPatch(listID, taskID string, patch map[string]any) op {
	return op{"update task", func(ctx context.Context, be gtasks.Backend, id func(string) string) (tea.Msg, error) {
		_, err := be.Patch(ctx, id(listID), id(taskID), patch)
		return nil, err
	}}
}

func opDelete(listID, taskID string) op {
	return op{"delete task", func(ctx context.Context, be gtasks.Backend, id func(string) string) (tea.Msg, error) {
		return nil, be.Delete(ctx, id(listID), id(taskID))
	}}
}

func opMove(listID, taskID, parent, previous string) op {
	return op{"move task", func(ctx context.Context, be gtasks.Backend, id func(string) string) (tea.Msg, error) {
		_, err := be.Move(ctx, id(listID), id(taskID), id(parent), id(previous))
		return nil, err
	}}
}
