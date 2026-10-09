package ui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Srindot/cli-Gtasks/internal/gtasks"
)

// popup is a yazi-style floating input that appears next to the cursor.
type popup struct {
	title, color, icon string
	pane               pane
	hint               func(m *Model, v string) []seg
	submit             func(m *Model, v string) tea.Cmd
	tab                func(m *Model)
	confirm            string // when set, this is a y/n question instead of an input
	at                 time.Time
}

func (m *Model) openPopup(p *popup, value string) {
	p.at = time.Now()
	m.pop = p
	if p.confirm != "" {
		m.mode = modeConfirm
		return
	}
	m.beginInput(value, "")
	m.mode = modePopup
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

func dueHint(expr string, quick bool) []seg {
	var d time.Time
	ok := false
	if quick {
		_, d, ok = SplitQuickDue(expr)
	} else {
		d, ok = ParseDue(expr)
		if ok && d.IsZero() {
			return segs(seg{"empty clears the due date", cMuted})
		}
	}
	if ok && !d.IsZero() {
		_, c := HumanDue(d)
		return segs(seg{ic.due + " " + d.Format("Mon, Jan 2"), c}, seg{" · " + relDays(d), cMuted})
	}
	if quick {
		return nil
	}
	return segs(seg{"tom · fri · +3d · +2w · oct 12", cMuted})
}

func (m *Model) duePopup(t *gtasks.Task) {
	cur := ""
	if d, ok := t.DueDate(); ok {
		cur = d.Format("2006-01-02")
	}
	lid, id := m.curID(), t.ID
	m.openPopup(&popup{title: "Due date", icon: ic.due, color: cGold, pane: m.focus,
		hint: func(m *Model, v string) []seg { return dueHint(v, false) },
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

// popupCard renders the open popup as a card of width w.
func (m *Model) popupCard(w int, now time.Time) []string {
	p := m.pop
	title := segs(seg{p.icon + " " + p.title, p.color})
	var body string
	var hint []seg
	if p.confirm != "" {
		body = paint(cText, p.confirm)
		hint = segs(seg{"y", cLove}, seg{" delete  ", cMuted}, seg{"n", cText}, seg{" keep", cMuted})
	} else {
		body = paint(p.color, ic.edit+" ") + m.inputView(w-6, false, "", now)
		if p.hint != nil {
			hint = m.chip(p.hint(m, m.in.Value()), now)
		}
	}
	return frame(w, p.color, title, hint, []string{body}, p.at, now)
}

// cardWidth is how wide a card may be inside a column of width w.
func cardWidth(w int) int { return max(min(w-2, 72), 0) }
