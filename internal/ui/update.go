package ui

import (
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Srindot/cli-Gtasks/internal/gtasks"
)

// testHook lets tests read the view from inside the event loop.
var testHook func(m *Model, msg any) bool

func (m *Model) Update(msg tea.Msg) (tm tea.Model, cmd tea.Cmd) {
	defer func() {
		if tm == nil {
			tm = m
		}
	}()
	defer m.guard("update")
	return m.update(msg)
}

func (m *Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if testHook != nil && testHook(m, msg) {
		return m, nil
	}
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height

	case tickMsg:
		m.ticking = false
		m.step(time.Time(msg))
		cmds = append(cmds, m.maybeRefresh())

	case autoSyncMsg:
		if m.mode == modeNormal && m.stable() {
			cmds = append(cmds, m.fetchLists())
		}
		cmds = append(cmds, autoSync())

	case tea.KeyMsg:
		cmds = append(cmds, m.key(msg))

	case listsMsg:
		m.fetching--
		if msg.err != nil {
			m.offline = true
			m.say(cLove, "✗ %v", msg.err)
		} else if !m.stable() {
			m.needRefresh = true
		} else {
			m.offline = false
			cmds = append(cmds, m.applyLists(msg.lists))
		}

	case tasksMsg:
		m.fetching--
		if msg.err != nil {
			m.offline = true
			m.say(cLove, "✗ %v", msg.err)
		} else if !m.stable() {
			m.needRefresh = true
		} else {
			m.applyTasks(msg.listID, msg.tasks)
			if m.fetching == 0 {
				m.lastSync = time.Now()
				m.saveCache()
			}
		}

	case opDoneMsg:
		m.pending--
		cmds = append(cmds, waitOp(m.results))
		switch r := msg.msg.(type) {
		case createdMsg:
			m.onCreated(r)
		case listCreatedMsg:
			m.onListCreated(r)
		}
		if msg.err != nil {
			m.say(cLove, "✗ %s failed: %v", msg.desc, msg.err)
			m.needRefresh = true
		}
		if m.pending == 0 {
			m.lastSync = time.Now()
			if m.quit {
				m.saveCache()
				return m, tea.Quit
			}
		}
		cmds = append(cmds, m.maybeRefresh())

	case editorMsg:
		cmds = append(cmds, m.onEditor(msg))

	default:
		switch m.mode {
		case modeInsert, modeCommand, modeSearch, modePopup:
			var c tea.Cmd
			m.in, c = m.in.Update(msg)
			cmds = append(cmds, c)
		case modeNotes:
			var c tea.Cmd
			m.ta, c = m.ta.Update(msg)
			cmds = append(cmds, c)
		}
	}

	m.clamp()
	if !m.ticking && m.animating() {
		m.ticking = true
		cmds = append(cmds, tick())
	}
	return m, tea.Batch(cmds...)
}

func (m *Model) clamp() {
	n := len(m.rows())
	if m.cursor >= n {
		m.cursor = n - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.listIdx >= len(m.lists) {
		m.listIdx = len(m.lists) - 1
	}
	if m.listIdx < 0 {
		m.listIdx = 0
	}
	if nf := fSubs + len(m.detailSubs()); m.dfield >= nf {
		m.dfield = nf - 1
	}
	if m.dfield < 0 {
		m.dfield = 0
	}
	// Vim-style scrolloff.
	h := m.bodyHeight()
	so := min(3, max(0, (h-1)/2))
	line := m.lineOf(m.cursor)
	end := line
	if rs := m.rows(); m.cursor < len(rs) {
		end += m.span(rs[m.cursor]) - 1
	}
	if end > m.offset+h-1-so {
		m.offset = end - h + 1 + so
	}
	if line < m.offset+so {
		m.offset = line - so
	}
	if total := m.lineCount(); m.offset > total-h {
		m.offset = total - h
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

func (m *Model) quitCmd(force bool) tea.Cmd {
	if m.pending > 0 && !force {
		m.quit = true
		return nil
	}
	m.saveCache()
	return tea.Quit
}

func (m *Model) key(k tea.KeyMsg) tea.Cmd {
	s := k.String()
	switch m.mode {
	case modeHelp:
		if s == "?" || s == "esc" || s == "q" || s == "enter" {
			m.mode = modeNormal
		}
		return nil
	case modeConfirm:
		p := m.pop
		m.closePopup()
		if s == "y" || s == "Y" || s == "enter" {
			return p.submit(m, "")
		}
		m.say(cMuted, "cancelled")
		return nil
	case modePopup:
		switch s {
		case "esc", "ctrl+c":
			m.closePopup()
			return nil
		case "enter":
			p, v := m.pop, m.in.Value()
			m.closePopup()
			return p.submit(m, v)
		case "tab", "shift+tab":
			if m.pop.tab != nil {
				m.pop.tab(m)
			}
			return nil
		}
		return m.feed(k)
	case modeNotes:
		switch s {
		case "esc":
			return m.commitNotes(true)
		case "ctrl+c":
			return m.commitNotes(false)
		}
		var c tea.Cmd
		m.ta, c = m.ta.Update(k)
		return c
	case modeInsert:
		composer := m.ed.kind == editNew && !m.ed.more
		switch s {
		case "esc":
			if composer {
				m.cancelEdit()
				return nil
			}
			return m.commitEdit(false)
		case "enter":
			return m.commitEdit(m.ed.kind == editNew)
		case "ctrl+c":
			m.cancelEdit()
			return nil
		case "tab", "shift+tab":
			if composer {
				m.flipDraft()
			}
			return nil
		}
		return m.feed(k)
	case modeCommand, modeSearch:
		return m.lineKey(k)
	}
	if m.quit && (s == "ctrl+c" || s == "q") {
		return m.quitCmd(true)
	}
	return m.normalKey(k)
}

func (m *Model) openLine(md mode, label, value string) {
	m.mode, m.promptLabel = md, label
	m.in.Width = max(m.w-len(label)-2, 8)
	m.in.Placeholder = ""
	m.in.SetValue(value)
	m.in.CursorEnd()
	m.in.Focus()
}

func (m *Model) lineKey(k tea.KeyMsg) tea.Cmd {
	s := k.String()
	md := m.mode
	switch {
	case s == "esc" || s == "ctrl+c" || (s == "backspace" && m.in.Value() == ""):
		m.mode = modeNormal
		m.in.Blur()
		if md == modeSearch {
			m.filter = ""
		}
		return nil
	case s == "enter":
		m.mode = modeNormal
		m.in.Blur()
		v := m.in.Value()
		if md == modeCommand {
			return m.command(v)
		}
		m.filter = v
		if v != "" {
			m.setFocus(paneTasks)
			n := len(m.rows())
			m.say(cRose, "%d match%s · esc to clear", n, map[bool]string{true: "", false: "es"}[n == 1])
		}
		return nil
	}
	c := m.feed(k)
	if md == modeSearch {
		m.filter = m.in.Value()
		m.cursor, m.offset = 0, 0
	}
	return c
}

func (m *Model) newListPopup() {
	m.openPopup(&popup{title: "New list", icon: ic.list, color: cIris, pane: paneLists,
		submit: func(m *Model, s string) tea.Cmd { return m.createList(s) }}, "")
}

func (m *Model) renameListPopup() {
	if l := m.curList(); l != nil {
		m.openPopup(&popup{title: "Rename list", icon: ic.edit, color: cFoam, pane: paneLists,
			submit: func(m *Model, s string) tea.Cmd { return m.renameList(s) }}, l.Title)
	}
}

func (m *Model) deleteListPopup() {
	if l := m.curList(); l != nil {
		m.focus = paneLists
		m.confirm("Delete “"+l.Title+"” and all its tasks?", func(m *Model) tea.Cmd { return m.deleteList() })
	}
}

func (m *Model) command(line string) tea.Cmd {
	line = strings.TrimSpace(line)
	if n, err := strconv.Atoi(line); err == nil {
		m.setFocus(paneTasks)
		m.setCursor(n - 1)
		return nil
	}
	name, arg, _ := strings.Cut(line, " ")
	arg = strings.TrimSpace(arg)
	switch name {
	case "":
		return nil
	case "q", "quit", "qa", "wq", "x", "wqa", "xa":
		return m.quitCmd(false)
	case "q!", "qa!":
		return m.quitCmd(true)
	case "w", "e", "e!", "sync", "refresh":
		m.say(cFoam, "syncing…")
		return m.fetchLists()
	case "new", "newlist", "nl", "tabnew":
		if arg == "" {
			m.newListPopup()
			return nil
		}
		return m.createList(arg)
	case "rename", "ren", "rl":
		if arg == "" {
			m.renameListPopup()
			return nil
		}
		return m.renameList(arg)
	case "dellist", "dl", "tabclose":
		m.deleteListPopup()
	case "clear", "clean":
		return m.clearDone()
	case "done", "completed", "c":
		m.showDone = !m.showDone
	case "due":
		if t := m.selected(); t != nil {
			return m.setDue(t, arg)
		}
	case "noh", "nohl", "nohlsearch":
		m.filter = ""
	case "help", "h":
		m.mode = modeHelp
	case "undo", "u":
		return m.doUndo()
	default:
		m.say(cLove, "E492: Not an editor command: %s", line)
	}
	return nil
}

func (m *Model) setFocus(p pane) {
	if m.focus != p {
		m.focus, m.focusAt = p, time.Now()
	}
}

// target is the task an action applies to: a focused subtask in the
// details pane, otherwise the selected row.
func (m *Model) target() *target {
	t := m.selected()
	if m.focus == paneDetails && m.dfield >= fSubs {
		if subs := m.detailSubs(); m.dfield-fSubs < len(subs) {
			return &target{subs[m.dfield-fSubs], true}
		}
	}
	return &target{t, false}
}

type target struct {
	t   *gtasks.Task
	sub bool
}

func (m *Model) normalKey(k tea.KeyMsg) tea.Cmd {
	s := k.String()

	if len(s) == 1 && s[0] >= '0' && s[0] <= '9' && (s != "0" || m.count > 0) && m.keys == "" {
		m.count = min(m.count*10+int(s[0]-'0'), 9999)
		return nil
	}
	n := max(m.count, 1)
	hadCount := m.count > 0

	if m.keys != "" {
		combo := m.keys + s
		m.keys, m.count = "", 0
		return m.combo(combo, n, hadCount)
	}
	switch s {
	case "g", "d", "y", "c", "z", ">", "<":
		m.keys = s
		return nil
	}
	m.count = 0

	switch s {
	case "ctrl+c", "q":
		return m.quitCmd(false)
	case "esc":
		if m.focus == paneDetails {
			m.setFocus(paneTasks)
		}
		m.filter = ""
		return nil
	case ":":
		m.openLine(modeCommand, ":", "")
		return nil
	case "/":
		m.openLine(modeSearch, "/", m.filter)
		m.setFocus(paneTasks)
		return nil
	case "?":
		m.mode = modeHelp
		return nil
	case "R", "ctrl+r":
		m.say(cFoam, "syncing…")
		return m.fetchLists()
	case "C":
		m.showDone = !m.showDone
		m.say(cSubtle, "%s", map[bool]string{true: "showing completed", false: "hiding completed"}[m.showDone])
		return nil
	case "H":
		return m.setList(m.listIdx - n)
	case "L":
		return m.setList(m.listIdx + n)
	case "tab":
		m.setFocus((m.focus + 1) % 3)
		return nil
	case "shift+tab":
		m.setFocus((m.focus + 2) % 3)
		return nil
	case "u":
		return m.doUndo()
	}

	switch m.focus {
	case paneLists:
		return m.listsKey(s, n)
	case paneDetails:
		return m.detailsKey(s, n)
	}
	return m.tasksKey(s, n, hadCount)
}

func (m *Model) combo(c string, n int, hadCount bool) tea.Cmd {
	tg := m.target()
	t := tg.t
	switch c {
	case "gg":
		switch m.focus {
		case paneLists:
			return m.setList(map[bool]int{true: n - 1, false: 0}[hadCount])
		case paneDetails:
			m.dfield = 0
		default:
			m.setCursor(map[bool]int{true: n - 1, false: 0}[hadCount])
		}
	case "gt":
		if hadCount {
			return m.setList(n - 1)
		}
		return m.setList(m.listIdx + 1)
	case "gT":
		return m.setList(m.listIdx - n)
	case "dd":
		if m.focus == paneLists {
			m.deleteListPopup()
			return nil
		}
		if m.focus == paneDetails && !tg.sub {
			return nil
		}
		if t != nil {
			var cmds []tea.Cmd
			for i := 0; i < n && t != nil; i++ {
				cmds = append(cmds, m.remove(m.curID(), t, true))
				if tg.sub {
					break
				}
				// Skip rows already dissolving for counted deletes.
				rs := m.rows()
				t = nil
				for j := m.cursor; j < len(rs); j++ {
					if rs[j].t == nil {
						continue
					}
					if a := m.anims[rs[j].t.ID]; a == nil || a.kind != animDelete {
						t = rs[j].t
						break
					}
				}
			}
			return tea.Batch(cmds...)
		}
	case "yy":
		if t != nil {
			m.yank(t)
		}
	case "cc":
		if t != nil && m.focus == paneTasks {
			m.startRename(t, "", false)
		}
	case "za", "zo", "zc", "zA":
		if t != nil {
			id := t.ID
			if t.Parent != "" && (c == "zc" || c == "za") && !m.folded[t.Parent] && len(m.children(m.curID(), id)) == 0 {
				id = t.Parent // folding from inside a child folds its parent
			}
			switch c {
			case "zo":
				delete(m.folded, id)
			case "zc":
				m.folded[id] = true
			default:
				m.folded[id] = !m.folded[id]
			}
			m.selectID(id)
		}
	case "zR":
		m.folded = map[string]bool{}
	case "zM":
		for _, t := range m.tasks[m.curID()] {
			if t.Parent != "" {
				m.folded[t.Parent] = true
			}
		}
		if t != nil && t.Parent != "" {
			m.selectID(t.Parent)
		}
	case ">>":
		if t != nil && m.focus == paneTasks {
			return m.indent(t)
		}
	case "<<":
		if t != nil && m.focus == paneTasks {
			return m.outdent(t)
		}
	}
	return nil
}

func (m *Model) listsKey(s string, n int) tea.Cmd {
	switch s {
	case "j", "down":
		return m.setList(min(m.listIdx+n, len(m.lists)-1))
	case "k", "up":
		return m.setList(max(m.listIdx-n, 0))
	case "G":
		return m.setList(len(m.lists) - 1)
	case "l", "right", "enter", "space":
		m.setFocus(paneTasks)
	case "a", "o", "O":
		m.newListPopup()
	case "r", "i", "A", "cw":
		m.renameListPopup()
	case "D":
		m.deleteListPopup()
	}
	return nil
}

func (m *Model) tasksKey(s string, n int, hadCount bool) tea.Cmd {
	t := m.selected()
	half := max(m.bodyHeight()/2, 1)
	switch s {
	case "j", "down":
		m.setCursor(m.cursor + n)
	case "k", "up":
		m.setCursor(m.cursor - n)
	case "G":
		if hadCount {
			m.setCursor(n - 1)
		} else {
			m.setCursor(len(m.rows()) - 1)
		}
	case "ctrl+d":
		m.setCursor(m.cursor + half)
	case "ctrl+u":
		m.setCursor(m.cursor - half)
	case "ctrl+f", "pgdown":
		m.setCursor(m.cursor + m.bodyHeight())
	case "ctrl+b", "pgup":
		m.setCursor(m.cursor - m.bodyHeight())
	case "h", "left":
		m.setFocus(paneLists)
	case "l", "right", "enter":
		if t != nil {
			m.setFocus(paneDetails)
			m.dfield = fTitle
		}
	case "a":
		m.compose(false)
	case "A":
		m.compose(true)
	case "o", "O":
		if m.curList() == nil {
			return nil
		}
		m.filter = ""
		m.startNew(m.placement(s == "O"))
	case "x", " ", "space":
		if t != nil {
			return m.toggle(t)
		}
	case "r":
		if t != nil {
			m.startRename(t, t.Title, false)
		}
	case "i", "I":
		if t != nil {
			m.startRename(t, t.Title, s == "I")
		}
	case "S":
		if t != nil {
			m.startRename(t, "", false)
		}
	case "e":
		if t != nil {
			return m.openEditor(t)
		}
	case "N":
		if t != nil {
			m.startNotes(t)
		}
	case "t":
		if t != nil {
			m.duePopup(t)
		}
	case "T":
		if t != nil {
			return m.setDue(t, "")
		}
	case "J":
		if t != nil {
			var cmd tea.Cmd
			for i := 0; i < n; i++ {
				cmd = m.shift(t, true)
			}
			return cmd
		}
	case "K":
		if t != nil {
			var cmd tea.Cmd
			for i := 0; i < n; i++ {
				cmd = m.shift(t, false)
			}
			return cmd
		}
	case "p":
		return m.paste(false)
	case "P":
		return m.paste(true)
	case "Y":
		if t != nil {
			m.yank(t)
		}
	case "D":
		if t != nil {
			return m.remove(m.curID(), t, true)
		}
	case "n":
		m.setCursor(m.cursor + 1)
	}
	return nil
}

func (m *Model) detailsKey(s string, n int) tea.Cmd {
	t := m.selected()
	if t == nil {
		m.setFocus(paneTasks)
		return nil
	}
	tg := m.target()
	last := fSubs + len(m.detailSubs()) - 1
	switch s {
	case "j", "down":
		m.dfield = min(m.dfield+n, last)
	case "k", "up":
		m.dfield = max(m.dfield-n, 0)
	case "G":
		m.dfield = last
	case "h", "left":
		m.setFocus(paneTasks)
	case "enter", "l", "right", "i":
		switch {
		case tg.sub:
			m.selectID(tg.t.ID)
			m.setFocus(paneTasks)
		case m.dfield == fTitle:
			m.renamePopup(t)
		case m.dfield == fDue:
			m.duePopup(t)
		case m.dfield == fStatus:
			return m.toggle(t)
		case m.dfield == fNotes:
			m.startNotes(t)
		}
	case "x", " ", "space":
		return m.toggle(tg.t)
	case "a", "A", "o", "O":
		m.compose(true)
	case "r":
		m.renamePopup(tg.t)
	case "t":
		m.duePopup(tg.t)
	case "T":
		return m.setDue(tg.t, "")
	case "e":
		return m.openEditor(t)
	case "N":
		m.startNotes(t)
	case "D":
		if tg.sub {
			return m.remove(m.curID(), tg.t, true)
		}
	}
	return nil
}
