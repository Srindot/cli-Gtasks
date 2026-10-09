package ui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/srindot/gtask/internal/gtasks"
)

// popup is a yazi-style floating input that appears next to the cursor.
type popup struct {
	title, color, icon string
	pane               pane
	hint               func(m *Model, v string) string
	submit             func(m *Model, v string) tea.Cmd
	tab                func(m *Model)
	confirm            string // when set, this is a y/n question instead of an input
	at                 time.Time
}

const popupH = 4

func (m *Model) openPopup(p *popup, value string) {
	p.at = time.Now()
	m.pop = p
	if p.confirm != "" {
		m.mode = modeConfirm
		return
	}
	m.mode = modePopup
	m.in.Placeholder = ""
	m.in.SetValue(value)
	m.in.CursorEnd()
	m.in.Focus()
	m.in.Width = max(m.popupWidth(p.pane)-7, 8)
}

func (m *Model) closePopup() {
	m.pop, m.mode = nil, modeNormal
	m.in.Blur()
}

func (m *Model) paneWidth(p pane) int {
	lw, tw, dw := m.layout()
	return [...]int{lw, tw, dw}[p]
}

func (m *Model) popupWidth(p pane) int {
	w := m.paneWidth(p)
	if w == 0 {
		w = m.w
	}
	return max(min(w-2, 64), 16)
}

func (m *Model) prompt(title, value string, fn func(m *Model, s string) tea.Cmd) {
	m.openPopup(&popup{title: title, icon: ic.edit, color: cGold, pane: m.focus, submit: fn}, value)
}

func (m *Model) confirm(text string, fn func(m *Model) tea.Cmd) {
	m.openPopup(&popup{title: "Are you sure?", icon: ic.warn, color: cLove, pane: m.focus, confirm: text,
		submit: func(m *Model, _ string) tea.Cmd { return fn(m) }}, "")
}

func dueHint(expr string, quick bool) string {
	var d time.Time
	ok := false
	if quick {
		_, d, ok = SplitQuickDue(expr)
	} else {
		d, ok = ParseDue(expr)
		if ok && d.IsZero() {
			return paint(cMuted, "empty clears the due date")
		}
	}
	if ok && !d.IsZero() {
		h, c := HumanDue(d)
		return paint(c, ic.due+"  "+d.Format("Mon, Jan 2")) + paint(cMuted, " · "+h)
	}
	if quick {
		return ""
	}
	return paint(cMuted, "tom · fri · +3d · +2w · oct 12 · 2026-10-12")
}

// newTaskPopup opens the "a" (task) / "A" (subtask) creator. Tab flips between them.
func (m *Model) newTaskPopup(sub bool) {
	if m.curList() == nil {
		return
	}
	lid := m.curID()
	anchor := m.selected()
	if m.focus == paneDetails {
		sub = true
	}
	canSub := anchor != nil && !anchor.Done()
	anchorID := ""
	if anchor != nil {
		anchorID = anchor.ID
	}
	p := &popup{pane: paneTasks}
	isSub := false
	set := func(s bool) {
		isSub = s && canSub
		if isSub {
			p.title, p.icon, p.color = "New subtask of “"+m.rootOf(anchor).Title+"”", ic.sub, cIris
		} else {
			p.title, p.icon, p.color = "New task", ic.add, cRose
		}
	}
	set(sub)
	if canSub {
		p.tab = func(m *Model) { set(!isSub); p.at = time.Now().Add(-time.Second) }
	}
	p.hint = func(m *Model, v string) string {
		if h := dueHint(v, true); h != "" {
			return h
		}
		s := paint(cMuted, "end with ") + paint(cSubtle, "@tom") + paint(cMuted, " for a due date")
		if canSub {
			s += paint(cMuted, " · ") + paint(cSubtle, "tab") + paint(cMuted, " ⇄ subtask")
		}
		return s
	}
	p.submit = func(m *Model, v string) tea.Cmd {
		if strings.TrimSpace(v) == "" {
			return nil
		}
		parent, prev := m.newPlacement(m.find(lid, anchorID), isSub)
		t := m.addTask(lid, v, parent, prev)
		if isSub {
			m.say(cIris, "%s added under %s", t.Title, m.rootOf(t).Title)
		} else {
			m.say(cRose, "%s added", t.Title)
		}
		if m.focus == paneDetails {
			if p := m.rootOf(t); p != nil && p != t {
				m.selectID(p.ID)
				m.dfield = fSubs + len(m.children(lid, p.ID)) - 1
			}
		}
		return nil
	}
	m.openPopup(p, "")
}

func (m *Model) duePopup(t *gtasks.Task) {
	cur := ""
	if d, ok := t.DueDate(); ok {
		cur = d.Format("2006-01-02")
	}
	lid, id := m.curID(), t.ID
	m.openPopup(&popup{title: "Due date", icon: ic.due, color: cGold, pane: m.focus,
		hint: func(m *Model, v string) string { return dueHint(v, false) },
		submit: func(m *Model, v string) tea.Cmd {
			if t := m.find(lid, id); t != nil {
				return m.setDue(t, v)
			}
			return nil
		}}, cur)
}

func (m *Model) renamePopup(t *gtasks.Task) {
	lid, id := m.curID(), t.ID
	m.openPopup(&popup{title: "Rename", icon: ic.edit, color: cFoam, pane: m.focus,
		submit: func(m *Model, v string) tea.Cmd { return m.rename(lid, id, v) }}, t.Title)
}

// popupLines renders the floating box, growing open line by line.
func (m *Model) popupLines(pw int, now time.Time) []string {
	p := m.pop
	b := fg(p.color)
	title := ansiTrunc(p.icon+" "+p.title, pw-6)
	top := b.Render("╭─ ") + fg(p.color).Bold(true).Render(title) + b.Render(" "+strings.Repeat("─", max(pw-5-lipgloss.Width(title), 0))+"╮")
	var l1, l2 string
	if p.confirm != "" {
		l1 = paint(cText, p.confirm)
		l2 = paint(cLove, "y") + paint(cMuted, " yes   ") + paint(cText, "n") + paint(cMuted, " no")
	} else {
		l1 = paint(p.color, "› ") + m.in.View()
		if p.hint != nil {
			l2 = "  " + p.hint(m, m.in.Value())
		}
	}
	row := func(s string) string { return b.Render("│") + " " + fit(s, pw-4) + " " + b.Render("│") }
	lines := []string{top, row(l1), row(l2), b.Render("╰" + strings.Repeat("─", pw-2) + "╯")}
	k := int(easeOut(float64(now.Sub(p.at))/float64(140*time.Millisecond))*popupH + .999)
	return lines[:max(min(k, popupH), 1)]
}

// overlayPopup draws the popup over a column's lines, just below line y.
func (m *Model) overlayPopup(lines []string, w, y int, now time.Time) {
	if m.pop == nil {
		return
	}
	pw := min(m.popupWidth(m.pop.pane), w-1)
	if pw < 8 {
		return
	}
	if y+popupH > len(lines) {
		y -= popupH + 1
	}
	y = max(min(y, len(lines)-popupH), 0)
	for i, l := range m.popupLines(pw, now) {
		if y+i < len(lines) {
			lines[y+i] = " " + l + strings.Repeat(" ", max(w-1-pw, 0))
		}
	}
}
