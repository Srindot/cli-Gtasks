package ui

import (
	"math"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Every text input in the app lives in a card: a rounded box that draws its
// own border in, holds a custom-rendered input line and carries a hint chip
// in its bottom edge.

type seg struct{ s, c string }

func segs(ss ...seg) []seg { return ss }

func (s seg) render() string { return paint(s.c, s.s) }

func renderSegs(ss []seg) string {
	var b strings.Builder
	for _, s := range ss {
		b.WriteString(s.render())
	}
	return b.String()
}

func plainSegs(ss []seg) string {
	var b strings.Builder
	for _, s := range ss {
		b.WriteString(s.s)
	}
	return b.String()
}

const (
	traceLife = 360 * time.Millisecond // border drawing itself in
	glideLife = 220 * time.Millisecond // color / indent changes
	chipLife  = 260 * time.Millisecond // due chip fading in
	typedLife = 240 * time.Millisecond // freshly typed character glow
)

type cell struct {
	r    string
	c    string
	bold bool
}

func cellsOf(ss []seg, bold bool, max int) []cell {
	var out []cell
	for _, s := range ss {
		for _, r := range s.s {
			if len(out) >= max {
				return out
			}
			if ansi.StringWidth(string(r)) != 1 {
				r = '·' // keep the border math one-cell-per-rune
			}
			out = append(out, cell{string(r), s.c, bold})
		}
	}
	return out
}

// edge builds one horizontal border line: corner, label, rule, corner.
func edge(w int, color, left, right string, label []seg, bold bool) []cell {
	cs := []cell{{left, color, false}, {"─", color, false}}
	if len(label) > 0 {
		cs = append(cs, cell{" ", "", false})
		cs = append(cs, cellsOf(label, bold, max(w-6, 0))...)
		cs = append(cs, cell{" ", "", false})
	}
	for len(cs) < w-1 {
		cs = append(cs, cell{"─", color, false})
	}
	return append(cs[:min(len(cs), w-1)], cell{right, color, false})
}

// frame draws a card of width w around body lines. The border traces itself
// clockwise from the top-left corner, led by a gold spark.
func frame(w int, color string, title, hint []seg, body []string, at, now time.Time) []string {
	if w < 8 {
		out := []string{""}
		for _, b := range body {
			out = append(out, fit(b, max(w, 0)))
		}
		return append(out, "")
	}
	n := len(body)
	top := edge(w, color, "╭", "╮", title, true)
	bot := edge(w, color, "╰", "╯", hint, false)
	total := 2*w + 2*n
	p := easeInOut(float64(now.Sub(at)) / float64(traceLife))
	k := int(math.Round(p * float64(total)))
	tracing := p < 1

	draw := func(c cell, idx int) string {
		if idx >= k {
			return " "
		}
		col := c.c
		if tracing && c.r != " " {
			if d := k - 1 - idx; d < 4 && col != "" {
				col = lerp(cGold, col, float64(d)/4)
			}
		}
		if col == "" {
			return c.r
		}
		st := fg(col)
		if c.bold {
			st = st.Bold(true)
		}
		return st.Render(c.r)
	}
	out := make([]string, 0, n+2)
	var b strings.Builder
	for i, c := range top {
		b.WriteString(draw(c, i))
	}
	out = append(out, b.String())
	for j, l := range body {
		left := draw(cell{"│", color, false}, 2*w+n+(n-1-j))
		right := draw(cell{"│", color, false}, w+j)
		out = append(out, left+" "+fit(l, w-4)+" "+right)
	}
	b.Reset()
	for i, c := range bot {
		b.WriteString(draw(c, w+n+(w-1-i)))
	}
	out = append(out, b.String())
	return out
}

// breathe is the caret color: a slow pulse between rose and iris.
func breathe(now time.Time) string {
	t := (math.Sin(float64(now.UnixMilli())/1000*2*math.Pi/1.6) + 1) / 2
	return lerp(cRose, cIris, t)
}

// quickStart returns the rune index where a trailing "@due" begins, or -1.
func quickStart(rs []rune) int {
	if _, _, ok := SplitQuickDue(string(rs)); !ok {
		return -1
	}
	for i := len(rs) - 1; i > 0; i-- {
		if rs[i] == '@' && rs[i-1] == ' ' {
			return i
		}
	}
	return -1
}

// inputView renders m.in in w cells: a breathing block caret, a gold glow on
// the character just typed, and the quick-due suffix highlighted in gold.
func (m *Model) inputView(w int, quick bool, placeholder string, now time.Time) string {
	if w < 2 {
		return ""
	}
	rs := []rune(m.in.Value())
	pos := min(m.in.Position(), len(rs))
	caret := lipgloss.NewStyle().Background(lipgloss.Color(breathe(now))).Foreground(lipgloss.Color(cBase))
	if len(rs) == 0 {
		return caret.Render(" ") + paint(cMuted, ansiTrunc(placeholder, w-1))
	}
	hl := -1
	if quick {
		hl = quickStart(rs)
	}
	start := 0
	if pos > w-2 {
		start = pos - (w - 2)
	}
	typed := clamp(float64(now.Sub(m.typedAt)) / float64(typedLife))
	var b strings.Builder
	used := 0
	if start > 0 {
		b.WriteString(paint(cMuted, "…"))
		used++
		start++
	}
	for i := start; i <= len(rs) && used < w; i++ {
		ch := " "
		if i < len(rs) {
			ch = string(rs[i])
		}
		if i == pos {
			b.WriteString(caret.Render(ch))
			used += ansi.StringWidth(ch)
			continue
		}
		if i == len(rs) {
			break
		}
		c := cText
		if hl >= 0 && i >= hl {
			c = cGold
		}
		if i == pos-1 && typed < 1 {
			c = lerp(cGold, c, easeOut(typed))
		}
		b.WriteString(paint(c, ch))
		used += ansi.StringWidth(ch)
	}
	return b.String()
}

// chip fades a hint in whenever its text changes.
func (m *Model) chip(ss []seg, now time.Time) []seg {
	p := easeOut(float64(now.Sub(m.hintAt)) / float64(chipLife))
	if p >= 1 {
		return ss
	}
	out := make([]seg, len(ss))
	for i, s := range ss {
		out[i] = seg{s.s, lerp(cHlMed, s.c, p)}
	}
	return out
}

// glide blends from the previous card color to the current one.
func glide(from, to string, at, now time.Time) string {
	if from == "" {
		return to
	}
	return lerp(from, to, easeOut(float64(now.Sub(at))/float64(glideLife)))
}

// splice inserts a card below line y of a column, pushing the rest down and
// scrolling just enough to keep the whole card on screen.
func splice(lines []string, y int, card []string, w, h int) []string {
	y = max(min(y, len(lines)), 0)
	pad := func(s string) string { return fit(" "+s, w) }
	out := make([]string, 0, len(lines)+len(card))
	out = append(out, lines[:y]...)
	for _, c := range card {
		out = append(out, pad(c))
	}
	out = append(out, lines[y:]...)
	if end := y + len(card); end > h {
		out = out[end-h:]
	}
	if len(out) > h {
		out = out[:h]
	}
	return out
}

// feed passes a key to the input and notes what changed, for the animations.
func (m *Model) feed(k tea.KeyMsg) tea.Cmd {
	before := m.in.Value()
	var c tea.Cmd
	m.in, c = m.in.Update(k)
	if v := m.in.Value(); len(v) > len(before) {
		m.typedAt = time.Now()
	}
	if key := plainSegs(m.cardHint()); key != m.hintKey {
		m.hintKey, m.hintAt = key, time.Now()
	}
	return c
}

// cardHint is the chip shown in the bottom edge of whichever card is open.
func (m *Model) cardHint() []seg {
	v := m.in.Value()
	switch {
	case m.mode == modePopup && m.pop != nil && m.pop.hint != nil:
		return m.pop.hint(m, v)
	case m.mode == modeInsert && m.ed.kind == editNew:
		if h := dueHint(v, true); h != nil {
			return h
		}
		if m.ed.more {
			return segs(seg{"⏎", cSubtle}, seg{" next  ", cMuted}, seg{"esc", cSubtle}, seg{" done", cMuted})
		}
		out := segs(seg{"@tom", cSubtle}, seg{" due  ", cMuted})
		if m.ed.canSub {
			out = append(out, seg{"tab", cSubtle}, seg{" ⇄ subtask  ", cMuted})
		}
		return append(out, seg{"⏎", cSubtle}, seg{" add", cMuted})
	case m.mode == modeInsert && m.ed.kind == editRename:
		if h := dueHint(v, true); h != nil {
			return h
		}
		return segs(seg{"⏎", cSubtle}, seg{" save  ", cMuted}, seg{"^c", cSubtle}, seg{" cancel", cMuted})
	}
	return nil
}
