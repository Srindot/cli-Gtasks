package ui

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/muesli/termenv"

	"github.com/Srindot/cli-Gtasks/internal/gtasks"
)

type row struct {
	t       *gtasks.Task
	depth   int
	last    bool // last visible child of its parent (tree guide ╰)
	kids    int  // direct open children
	folded  bool // has children that are hidden
	editNew bool // virtual row for a task being typed
	done    bool // lives in the "completed" section
}

// rows is the flattened, visible view of the current list.
func (m *Model) rows() []row {
	lid := m.curID()
	ts := m.tasks[lid]
	var out []row

	if m.filter != "" {
		q := strings.ToLower(m.filter)
		for _, t := range ts {
			if (!t.Done() || m.showDone) && strings.Contains(strings.ToLower(t.Title+" "+t.Notes), q) {
				out = append(out, row{t: t, done: t.Done()})
			}
		}
		return out
	}

	byID := make(map[string]*gtasks.Task, len(ts))
	for _, t := range ts {
		byID[t.ID] = t
	}
	kids := map[string][]*gtasks.Task{}
	var roots []*gtasks.Task
	for _, t := range ts {
		if t.Done() {
			continue
		}
		if p := byID[t.Parent]; p != nil && !p.Done() {
			kids[t.Parent] = append(kids[t.Parent], t)
		} else {
			roots = append(roots, t)
		}
	}
	var walk func(t *gtasks.Task, d int, last bool)
	walk = func(t *gtasks.Task, d int, last bool) {
		f := m.folded[t.ID] && len(kids[t.ID]) > 0
		out = append(out, row{t: t, depth: d, last: last, kids: len(kids[t.ID]), folded: f})
		if !f {
			for i, c := range kids[t.ID] {
				walk(c, d+1, i == len(kids[t.ID])-1)
			}
		}
	}
	for _, r := range roots {
		walk(r, 0, false)
	}

	if m.ed.kind == editNew && m.ed.listID == lid {
		at, d := 0, 0
		if m.ed.previous != "" {
			for i, r := range out {
				if r.t.ID == m.ed.previous {
					at, d = i+1, r.depth
					for at < len(out) && out[at].depth > d {
						at++
					}
					break
				}
			}
		} else if m.ed.parent != "" {
			for i, r := range out {
				if r.t.ID == m.ed.parent {
					at, d = i+1, r.depth+1
					break
				}
			}
		}
		if at > 0 && out[at-1].depth == d && d > 0 {
			out[at-1].last = false
		}
		out = append(out[:at], append([]row{{editNew: true, depth: d, last: at >= len(out) || out[at].depth < d}}, out[at:]...)...)
	}

	if m.showDone {
		var done []*gtasks.Task
		for _, t := range ts {
			if t.Done() {
				done = append(done, t)
			}
		}
		// Most recently completed first.
		for i := 1; i < len(done); i++ {
			for j := i; j > 0 && done[j].Completed > done[j-1].Completed; j-- {
				done[j], done[j-1] = done[j-1], done[j]
			}
		}
		for _, t := range done {
			out = append(out, row{t: t, done: true})
		}
	}
	return out
}

func (m *Model) counts(listID string) (open, done int) {
	for _, t := range m.tasks[listID] {
		if t.Done() {
			done++
		} else {
			open++
		}
	}
	return
}

func (m *Model) selected() *gtasks.Task {
	rs := m.rows()
	if m.cursor >= 0 && m.cursor < len(rs) {
		return rs[m.cursor].t
	}
	return nil
}

func (m *Model) selectID(id string) {
	id = m.ids.get(id)
	for i, r := range m.rows() {
		if r.t != nil && r.t.ID == id {
			m.cursor = i
			return
		}
	}
}

func (m *Model) setCursor(i int) {
	n := len(m.rows())
	if i >= n {
		i = n - 1
	}
	if i < 0 {
		i = 0
	}
	if i != m.cursor {
		if t := m.selected(); t != nil {
			m.trail[t.ID] = time.Now()
		}
		m.selAt = time.Now()
		if m.focus != paneDetails {
			m.dfield = fTitle
		}
	}
	m.cursor = i
}

func (m *Model) setList(i int) tea.Cmd {
	if len(m.lists) == 0 {
		return nil
	}
	i = (i%len(m.lists) + len(m.lists)) % len(m.lists)
	if i == m.listIdx {
		return nil
	}
	m.cursors[m.curID()] = m.cursor
	m.listIdx, m.filter = i, ""
	m.cursor, m.offset = m.cursors[m.curID()], 0
	m.listAt = time.Now()
	if !m.loaded[m.curID()] && !strings.HasPrefix(m.curID(), "local-") {
		return m.fetchTasks(m.curID())
	}
	return nil
}

func (m *Model) pushUndo(desc string, do func(m *Model) tea.Cmd) {
	m.undo = append(m.undo, undoEntry{desc, do})
	if len(m.undo) > 200 {
		m.undo = m.undo[1:]
	}
}

func (m *Model) say(color, format string, a ...any) {
	m.toast = &toast{text: sprintf(format, a...), color: color, at: time.Now()}
}

// ---- task actions --------------------------------------------------------

func (m *Model) setDone(lid, id string, done bool) tea.Cmd {
	t := m.find(lid, id)
	if t == nil {
		return nil
	}
	if a := m.anims[t.ID]; a != nil && a.kind == animComplete {
		delete(m.anims, t.ID) // un-toggled mid-animation
	}
	if done {
		patch := map[string]any{"status": gtasks.StatusDone}
		m.startAnim(t.ID, animComplete, 520*time.Millisecond, func(m *Model) {
			if t := m.find(lid, id); t != nil {
				t.Status, t.Completed = gtasks.StatusDone, time.Now().UTC().Format(time.RFC3339)
			}
		})
		return m.enqueue(opPatch(lid, id, patch))
	}
	t.Status, t.Completed = gtasks.StatusOpen, ""
	m.startAnim(t.ID, animFlash, 700*time.Millisecond, nil)
	return m.enqueue(opPatch(lid, id, map[string]any{"status": gtasks.StatusOpen, "completed": nil}))
}

func (m *Model) toggle(t *gtasks.Task) tea.Cmd {
	lid, id := m.curID(), t.ID
	done := !t.Done()
	if a := m.anims[id]; a != nil && a.kind == animComplete {
		done = false
	}
	m.pushUndo("toggle", func(m *Model) tea.Cmd { return m.setDone(lid, id, !done) })
	if done {
		m.say(cPine, "✓ %s", t.Title)
	}
	return m.setDone(lid, id, done)
}

func snapshot(ts []*gtasks.Task) []gtasks.Task {
	out := make([]gtasks.Task, len(ts))
	for i, t := range ts {
		out[i] = *t
	}
	return out
}

func (m *Model) remove(lid string, t *gtasks.Task, withUndo bool) tea.Cmd {
	if a := m.anims[t.ID]; a != nil && a.kind == animDelete {
		return nil
	}
	sub := m.subtree(lid, t)
	saved := snapshot(sub)
	parent, prev := t.Parent, m.prevSibling(lid, t)
	ids := make([]string, len(sub))
	for i, s := range sub {
		ids[i] = s.ID
	}
	done := func(m *Model) { m.removeLocal(lid, ids...) }
	for _, id := range ids[1:] {
		m.startAnim(id, animDelete, 320*time.Millisecond, nil)
	}
	m.startAnim(t.ID, animDelete, 320*time.Millisecond, done)
	if withUndo {
		m.reg = saved
		m.pushUndo("delete", func(m *Model) tea.Cmd {
			_, cmd := m.insertSnap(lid, saved, parent, prev)
			return cmd
		})
	}
	return m.enqueue(opDelete(lid, t.ID))
}

// insertSnap recreates a snapshot (root first) under parent after previous.
func (m *Model) insertSnap(lid string, snap []gtasks.Task, parent, previous string) (string, tea.Cmd) {
	if len(snap) == 0 {
		return "", nil
	}
	newID := map[string]string{}
	last := map[string]string{}
	for i, s := range snap {
		nt := s
		nt.ID, nt.Position, nt.Hidden = m.localID(), "", false
		p, prev := parent, previous
		if i > 0 {
			p = newID[s.Parent]
			prev = last[p]
		}
		newID[s.ID] = nt.ID
		last[p] = nt.ID
		m.placeLocal(lid, &nt, p, prev)
		m.startAnim(nt.ID, animFlash, 800*time.Millisecond, nil)
		m.enqueue(opInsert(lid, nt.ID, nt, p, prev))
	}
	root := newID[snap[0].ID]
	m.selectID(root)
	return root, nil
}

// placement returns where a new task goes relative to the cursor (o / O).
func (m *Model) placement(above bool) (parent, previous string) {
	lid := m.curID()
	rs := m.rows()
	if m.cursor >= len(rs) || rs[m.cursor].t == nil {
		return "", ""
	}
	r := rs[m.cursor]
	t := r.t
	if r.done {
		// From the completed section: append to the end of the open roots.
		last := ""
		for _, x := range rs {
			if !x.done && x.t != nil && x.depth == 0 {
				last = x.t.ID
			}
		}
		return "", last
	}
	if above {
		return t.Parent, m.prevSibling(lid, t)
	}
	if r.kids > 0 && !r.folded {
		return t.ID, ""
	}
	return t.Parent, t.ID
}

func (m *Model) startNew(parent, previous string) {
	m.ed = edit{kind: editNew, listID: m.curID(), parent: parent, previous: previous, more: true, at: time.Now()}
	m.beginInput("", "new task  ·  end with @tom for a due date")
	m.cursorToDraft()
}

// compose opens the "a" (task) / "A" (subtask) card right where the task
// will land; tab moves it between the two spots.
func (m *Model) compose(sub bool) {
	if m.curList() == nil {
		return
	}
	anchor := m.selected()
	ed := edit{kind: editNew, listID: m.curID(), at: time.Now(), back: m.focus}
	if m.focus == paneDetails {
		sub = true
	}
	if anchor != nil {
		ed.anchor = anchor.ID
		ed.canSub = !anchor.Done() && !m.rootOf(anchor).Done()
	}
	ed.sub = sub && ed.canSub
	m.ed = ed
	m.placeDraft()
	m.setFocus(paneTasks)
	m.beginInput("", "what needs doing?")
	m.cursorToDraft()
}

func (m *Model) placeDraft() {
	if a := m.find(m.ed.listID, m.ed.anchor); a != nil {
		m.ed.parent, m.ed.previous = m.newPlacement(a, m.ed.sub)
	}
}

// flipDraft toggles the composer between task and subtask, gliding the card.
func (m *Model) flipDraft() {
	if !m.ed.canSub {
		return
	}
	m.ed.from, m.ed.fromAt, m.ed.fromDepth = m.draftColor(), time.Now(), m.draftDepth()
	m.ed.sub = !m.ed.sub
	m.placeDraft()
	m.cursorToDraft()
}

func (m *Model) draftColor() string {
	switch {
	case m.ed.kind == editRename:
		return cFoam
	case m.ed.sub:
		return cIris
	}
	return cRose
}

func (m *Model) draftDepth() int {
	for _, r := range m.rows() {
		if r.editNew {
			return r.depth
		}
	}
	return 0
}

func (m *Model) cursorToDraft() {
	for i, r := range m.rows() {
		if r.editNew {
			m.cursor = i
		}
	}
}

func (m *Model) beginInput(value, placeholder string) {
	m.mode = modeInsert
	m.in.Placeholder = placeholder
	m.in.SetValue(value)
	m.in.Width = 1 << 12 // inputView does the scrolling
	m.in.Focus()
	m.in.CursorEnd()
	m.typedAt, m.hintKey, m.hintAt = time.Time{}, "", time.Now()
}

func (m *Model) startRename(t *gtasks.Task, value string, atStart bool) {
	m.ed = edit{kind: editRename, listID: m.curID(), taskID: t.ID, at: time.Now()}
	m.beginInput(value, "new title")
	if atStart {
		m.in.CursorStart()
	}
}

// restoreDraftCursor puts the cursor back where it was before composing.
func (m *Model) restoreDraftCursor(ed edit) {
	switch {
	case ed.anchor != "":
		m.selectID(ed.anchor)
	case ed.previous != "":
		m.selectID(ed.previous)
	default:
		m.setCursor(m.cursor)
	}
}

// commitEdit finishes insert mode; more=true keeps typing new tasks (Enter).
func (m *Model) commitEdit(more bool) tea.Cmd {
	ed, val := m.ed, strings.TrimSpace(m.in.Value())
	m.ed, m.mode = edit{}, modeNormal
	m.in.Blur()
	switch ed.kind {
	case editNew:
		if val == "" {
			m.restoreDraftCursor(ed)
			return nil
		}
		t := m.addTask(ed.listID, val, ed.parent, ed.previous)
		switch {
		case more && ed.more:
			m.startNew(t.Parent, t.ID)
		case ed.sub:
			m.say(cIris, "%s added under %s", t.Title, m.rootOf(t).Title)
		default:
			m.say(cRose, "%s added", t.Title)
		}
		if !ed.more && ed.back == paneDetails {
			m.setFocus(paneDetails)
			if p := m.rootOf(t); p != nil && p != t {
				m.selectID(p.ID)
				m.dfield = fSubs + len(m.children(ed.listID, p.ID)) - 1
			}
		}
	case editRename:
		return m.rename(ed.listID, ed.taskID, val)
	}
	return nil
}

// addTask creates a task locally (instantly) and queues it for Google.
// A trailing "@tomorrow" in the title becomes the due date.
func (m *Model) addTask(lid, val, parent, previous string) *gtasks.Task {
	title, due, hasDue := SplitQuickDue(val)
	t := &gtasks.Task{ID: m.localID(), Title: title, Status: gtasks.StatusOpen, Updated: time.Now().UTC().Format(time.RFC3339)}
	if hasDue {
		t.Due = gtasks.FormatDue(due)
	}
	if parent != "" {
		delete(m.folded, m.ids.get(parent))
	}
	m.placeLocal(lid, t, parent, previous)
	m.startAnim(t.ID, animFlash, 900*time.Millisecond, nil)
	m.enqueue(opInsert(lid, t.ID, *t, parent, previous))
	id := t.ID
	m.pushUndo("add", func(m *Model) tea.Cmd {
		if t := m.find(lid, id); t != nil {
			return m.remove(lid, t, false)
		}
		return nil
	})
	m.selectID(t.ID)
	return t
}

func (m *Model) rename(lid, id, val string) tea.Cmd {
	t := m.find(lid, id)
	val = strings.TrimSpace(val)
	if t == nil || val == "" || val == t.Title {
		return nil
	}
	old := t.Title
	m.pushUndo("rename", func(m *Model) tea.Cmd { return m.patchLocal(lid, id, "title", old) })
	return m.patchLocal(lid, id, "title", val)
}

// newPlacement decides where a task created with a / A goes, relative to anchor.
func (m *Model) newPlacement(anchor *gtasks.Task, sub bool) (parent, previous string) {
	lid := m.curID()
	if anchor == nil {
		return "", ""
	}
	root := anchor
	if anchor.Parent != "" {
		if p := m.find(lid, anchor.Parent); p != nil {
			root = p
		}
	}
	if anchor.Done() || root.Done() {
		last := ""
		for _, r := range m.rows() {
			if r.t != nil && !r.done && r.depth == 0 {
				last = r.t.ID
			}
		}
		return "", last
	}
	if sub {
		if anchor != root {
			return root.ID, anchor.ID
		}
		return root.ID, m.lastChild(lid, root.ID)
	}
	return "", root.ID
}

// rootOf returns the top-level task that t lives under (or t itself).
func (m *Model) rootOf(t *gtasks.Task) *gtasks.Task {
	if t != nil && t.Parent != "" {
		if p := m.find(m.curID(), t.Parent); p != nil {
			return p
		}
	}
	return t
}

func (m *Model) cancelEdit() {
	ed := m.ed
	m.ed, m.mode = edit{}, modeNormal
	m.in.Blur()
	if ed.kind == editNew {
		m.restoreDraftCursor(ed)
		if ed.back == paneDetails {
			m.setFocus(paneDetails)
		}
	} else {
		m.setCursor(m.cursor)
	}
}

// patchLocal sets a string field locally and on the server; "" clears it.
func (m *Model) patchLocal(lid, id, field, val string) tea.Cmd {
	t := m.find(lid, id)
	if t == nil {
		return nil
	}
	switch field {
	case "title":
		t.Title = val
	case "notes":
		t.Notes = val
	case "due":
		t.Due = val
	}
	var v any = val
	if val == "" && field != "title" {
		v = nil
	}
	m.startAnim(t.ID, animFlash, 700*time.Millisecond, nil)
	return m.enqueue(opPatch(lid, id, map[string]any{field: v}))
}

func (m *Model) setDue(t *gtasks.Task, expr string) tea.Cmd {
	d, ok := ParseDue(expr)
	if !ok {
		m.say(cLove, "can't read “%s” as a date — try tom, fri, +3d, oct 12", expr)
		return nil
	}
	val := ""
	if !d.IsZero() {
		val = gtasks.FormatDue(d)
	}
	lid, id, old := m.curID(), t.ID, t.Due
	m.pushUndo("due", func(m *Model) tea.Cmd { return m.patchLocal(lid, id, "due", old) })
	if val == "" {
		m.say(cSubtle, "due date cleared")
	} else {
		h, c := HumanDue(d)
		m.say(c, "due %s", h)
	}
	return m.patchLocal(lid, id, "due", val)
}

func (m *Model) move(lid, id, parent, previous string, withUndo bool) tea.Cmd {
	t := m.find(lid, id)
	if t == nil {
		return nil
	}
	if withUndo {
		op, oprev := t.Parent, m.prevSibling(lid, t)
		m.pushUndo("move", func(m *Model) tea.Cmd { return m.move(lid, id, op, oprev, false) })
	}
	m.placeLocal(lid, t, parent, previous)
	m.startAnim(t.ID, animFlash, 500*time.Millisecond, nil)
	m.selectID(t.ID)
	return m.enqueue(opMove(lid, t.ID, parent, previous))
}

// siblings returns the open, visible siblings of the cursor row.
func (m *Model) siblings(t *gtasks.Task) []*gtasks.Task {
	var out []*gtasks.Task
	for _, r := range m.rows() {
		if r.t != nil && !r.done && r.t.Parent == t.Parent {
			out = append(out, r.t)
		}
	}
	return out
}

func (m *Model) shift(t *gtasks.Task, down bool) tea.Cmd {
	if t.Done() || m.filter != "" {
		return nil
	}
	lid := m.curID()
	sib := m.siblings(t)
	for i, s := range sib {
		if s != t {
			continue
		}
		if down && i+1 < len(sib) {
			return m.move(lid, t.ID, t.Parent, sib[i+1].ID, true)
		}
		if !down && i > 0 {
			return m.move(lid, t.ID, t.Parent, m.prevSibling(lid, sib[i-1]), true)
		}
	}
	return nil
}

func (m *Model) indent(t *gtasks.Task) tea.Cmd {
	lid := m.curID()
	if t.Done() || m.filter != "" {
		return nil
	}
	if t.Parent != "" {
		m.say(cGold, "Google Tasks only supports one level of subtasks")
		return nil
	}
	if len(m.children(lid, t.ID)) > 0 {
		m.say(cGold, "a task with subtasks can't become a subtask")
		return nil
	}
	sib := m.siblings(t)
	for i, s := range sib {
		if s == t && i > 0 {
			p := sib[i-1]
			delete(m.folded, p.ID)
			return m.move(lid, t.ID, p.ID, m.lastChild(lid, p.ID), true)
		}
	}
	return nil
}

func (m *Model) outdent(t *gtasks.Task) tea.Cmd {
	if t.Parent == "" || t.Done() {
		return nil
	}
	return m.move(m.curID(), t.ID, "", t.Parent, true)
}

func (m *Model) yank(t *gtasks.Task) {
	m.reg = snapshot(m.subtree(m.curID(), t))
	termenv.DefaultOutput().Copy(t.Title)
	m.say(cFoam, "yanked “%s”", t.Title)
}

func (m *Model) paste(above bool) tea.Cmd {
	if len(m.reg) == 0 {
		m.say(cMuted, "register is empty")
		return nil
	}
	if m.curList() == nil {
		return nil
	}
	parent, prev := m.placement(above)
	snap := m.reg
	if parent != "" && len(snap) > 1 {
		parent, prev = "", "" // keep the tree one level deep
		if t := m.selected(); t != nil {
			prev = t.Parent
		}
	}
	lid := m.curID()
	root, cmd := m.insertSnap(lid, snap, parent, prev)
	m.pushUndo("paste", func(m *Model) tea.Cmd {
		if t := m.find(lid, root); t != nil {
			return m.remove(lid, t, false)
		}
		return nil
	})
	return cmd
}

func (m *Model) doUndo() tea.Cmd {
	if len(m.undo) == 0 {
		m.say(cMuted, "already at oldest change")
		return nil
	}
	u := m.undo[len(m.undo)-1]
	m.undo = m.undo[:len(m.undo)-1]
	m.say(cIris, "↶ undo %s", u.desc)
	return u.do(m)
}

func (m *Model) clearDone() tea.Cmd {
	lid := m.curID()
	var ids []string
	for _, t := range m.tasks[lid] {
		if t.Done() {
			ids = append(ids, t.ID)
			m.cleared[t.ID] = true
		}
	}
	if len(ids) == 0 {
		m.say(cMuted, "no completed tasks")
		return nil
	}
	m.removeLocal(lid, ids...)
	m.say(cPine, "cleared %d completed", len(ids))
	return m.enqueue(op{"clear completed", func(ctx context.Context, be gtasks.Backend, id func(string) string) (tea.Msg, error) {
		return nil, be.Clear(ctx, id(lid))
	}})
}

// ---- lists ---------------------------------------------------------------

func (m *Model) createList(title string) tea.Cmd {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil
	}
	local := m.localID()
	m.lists = append(m.lists, gtasks.List{ID: local, Title: title})
	m.tasks[local], m.loaded[local] = nil, true
	m.setList(len(m.lists) - 1)
	m.focus, m.focusAt = paneTasks, time.Now()
	m.say(cFoam, "created list %s", title)
	return m.enqueue(op{"create list", func(ctx context.Context, be gtasks.Backend, id func(string) string) (tea.Msg, error) {
		l, err := be.CreateList(ctx, title)
		if err != nil {
			return nil, err
		}
		return listCreatedMsg{local, l}, nil
	}})
}

func (m *Model) renameList(title string) tea.Cmd {
	title = strings.TrimSpace(title)
	l := m.curList()
	if l == nil || title == "" || title == l.Title {
		return nil
	}
	l.Title = title
	lid := l.ID
	return m.enqueue(op{"rename list", func(ctx context.Context, be gtasks.Backend, id func(string) string) (tea.Msg, error) {
		_, err := be.RenameList(ctx, id(lid), title)
		return nil, err
	}})
}

func (m *Model) deleteList() tea.Cmd {
	l := m.curList()
	if l == nil {
		return nil
	}
	lid, title := l.ID, l.Title
	m.lists = append(m.lists[:m.listIdx], m.lists[m.listIdx+1:]...)
	delete(m.tasks, lid)
	if m.listIdx >= len(m.lists) {
		m.listIdx = len(m.lists) - 1
	}
	if m.listIdx < 0 {
		m.listIdx = 0
	}
	m.cursor, m.offset = 0, 0
	m.say(cLove, "deleted list %s", title)
	return m.enqueue(op{"delete list", func(ctx context.Context, be gtasks.Backend, id func(string) string) (tea.Msg, error) {
		return nil, be.DeleteList(ctx, id(lid))
	}})
}

// ---- $EDITOR -------------------------------------------------------------

func (m *Model) openEditor(t *gtasks.Task) tea.Cmd {
	f, err := os.CreateTemp("", "gtask-*.md")
	if err != nil {
		m.say(cLove, "%v", err)
		return nil
	}
	f.WriteString(t.Title + "\n\n" + t.Notes)
	f.Close()
	ed := os.Getenv("VISUAL")
	if ed == "" {
		ed = os.Getenv("EDITOR")
	}
	if ed == "" {
		ed = "nvim"
	}
	lid, id, path := m.curID(), t.ID, f.Name()
	c := exec.Command("sh", "-c", ed+` "$1"`, "gtask", path)
	return tea.ExecProcess(c, func(err error) tea.Msg { return editorMsg{lid, id, path, err} })
}

func (m *Model) onEditor(msg editorMsg) tea.Cmd {
	defer os.Remove(msg.path)
	if msg.err != nil {
		m.say(cLove, "editor: %v", msg.err)
		return nil
	}
	b, err := os.ReadFile(msg.path)
	t := m.find(msg.listID, msg.taskID)
	if err != nil || t == nil {
		return nil
	}
	title, notes, _ := strings.Cut(strings.TrimRight(string(b), "\n"), "\n")
	title, notes = strings.TrimSpace(title), strings.Trim(notes, "\n")
	if title == "" {
		m.say(cGold, "empty title — nothing changed")
		return nil
	}
	if title == t.Title && notes == t.Notes {
		return nil
	}
	lid, id, ot, on := msg.listID, t.ID, t.Title, t.Notes
	m.pushUndo("edit", func(m *Model) tea.Cmd {
		m.patchLocal(lid, id, "title", ot)
		return m.patchLocal(lid, id, "notes", on)
	})
	m.patchLocal(lid, id, "title", title)
	m.patchLocal(lid, id, "notes", notes)
	m.say(cFoam, "saved")
	return nil
}

// ---- inline notes editor -------------------------------------------------

func (m *Model) startNotes(t *gtasks.Task) {
	m.notesFor = [2]string{m.curID(), t.ID}
	m.mode, m.focus, m.dfield = modeNotes, paneDetails, fNotes
	m.ta.SetValue(t.Notes)
	m.ta.Focus()
	m.ta.SetWidth(max(cardWidth(m.paneWidth(paneDetails))-4, 10))
	m.ta.SetHeight(max(m.bodyHeight()-12, 3))
	m.notesAt = time.Now() // the notes card traces in
}

func (m *Model) commitNotes(save bool) tea.Cmd {
	m.mode = modeNormal
	m.ta.Blur()
	lid, id := m.notesFor[0], m.notesFor[1]
	t := m.find(lid, id)
	if !save || t == nil {
		return nil
	}
	val := strings.TrimRight(m.ta.Value(), "\n ")
	if val == t.Notes {
		return nil
	}
	old := t.Notes
	m.pushUndo("notes", func(m *Model) tea.Cmd { return m.patchLocal(lid, id, "notes", old) })
	m.say(cFoam, "notes saved")
	return m.patchLocal(lid, id, "notes", val)
}

// detailSubs are the subtasks shown in the details pane for the selection.
func (m *Model) detailSubs() []*gtasks.Task {
	if t := m.selected(); t != nil {
		return m.children(m.curID(), t.ID)
	}
	return nil
}
