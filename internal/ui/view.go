package ui

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var spinner = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func spin(now time.Time) string {
	i := int(now.UnixMilli()/80) % len(spinner)
	return paint(gradient(shimmer, float64(now.UnixMilli())/1800), spinner[i])
}

// fit truncates or pads s to exactly w cells.
func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	s = ansi.Truncate(s, w, "…")
	if pad := w - lipgloss.Width(s); pad > 0 {
		s += strings.Repeat(" ", pad)
	}
	return s
}

func (m *Model) layout() (lw, tw, dw int) {
	switch {
	case m.w >= 120:
		lw, dw = 28, max(34, min(52, m.w*28/100))
	case m.w >= 72:
		lw = 26
	case m.focus == paneLists:
		lw = m.w
	}
	return lw, m.w - lw - dw, dw
}

func (m *Model) showDetails() bool { _, _, dw := m.layout(); return dw > 0 }

// bodyHeight is the number of content lines inside a pane.
func (m *Model) bodyHeight() int { return max(m.h-5, 1) }

func (m *Model) sepIndex(rs []row) int {
	if m.filter != "" {
		return -1
	}
	for i, r := range rs {
		if r.done {
			return i
		}
	}
	return -1
}

func (m *Model) lineOf(i int) int {
	if s := m.sepIndex(m.rows()); s >= 0 && i >= s {
		return i + 1
	}
	return i
}

func (m *Model) lineCount() int {
	rs := m.rows()
	if m.sepIndex(rs) >= 0 {
		return len(rs) + 1
	}
	return len(rs)
}

func (m *Model) focusColor(p pane, now time.Time) string {
	t := easeOut(float64(now.Sub(m.focusAt)) / float64(focusLife))
	if m.focus == p {
		return lerp(cHlHigh, cIris, t)
	}
	return lerp(cIris, cHlHigh, t)
}

// box draws a rounded panel with a title in the top border and an optional
// scrollbar thumb on the right edge.
func box(title string, lines []string, w, h int, border string, total, offset int) string {
	if w < 4 || h < 2 {
		return strings.Repeat(strings.Repeat(" ", max(w, 0))+"\n", max(h-1, 0)) + strings.Repeat(" ", max(w, 0))
	}
	b := fg(border)
	inner := w - 4
	title = ansi.Truncate(title, max(w-6, 0), "…")
	fill := w - 4 - lipgloss.Width(title) - 1
	var sb strings.Builder
	sb.WriteString(b.Render("╭─ ") + title + b.Render(" "+strings.Repeat("─", max(fill, 0))+"╮"))
	body := h - 2
	thumbA, thumbB := -1, -1
	if total > body && body > 0 {
		size := max(1, body*body/total)
		thumbA = offset * (body - size) / max(total-body, 1)
		thumbB = thumbA + size
	}
	for i := 0; i < body; i++ {
		l := ""
		if i < len(lines) {
			l = lines[i]
		}
		right := b.Render("│")
		if i >= thumbA && i < thumbB {
			right = paint(lerp(border, cSubtle, .5), "┃")
		}
		sb.WriteString("\n" + b.Render("│") + " " + fit(l, inner) + " " + right)
	}
	sb.WriteString("\n" + b.Render("╰"+strings.Repeat("─", w-2)+"╯"))
	return sb.String()
}

func (m *Model) View() string {
	if m.w == 0 || m.h == 0 {
		return ""
	}
	now := time.Now()
	if len(m.lists) == 0 {
		return m.splash(now)
	}
	var body string
	ph := max(m.h-3, 2)
	if m.mode == modeHelp {
		body = lipgloss.Place(m.w, ph, lipgloss.Center, lipgloss.Center, m.help())
	} else {
		lw, tw, dw := m.layout()
		var cols []string
		if lw > 0 {
			cols = append(cols, m.viewLists(lw, ph, now))
		}
		if tw > 0 {
			cols = append(cols, m.viewTasks(tw, ph, now))
		}
		if dw > 0 {
			cols = append(cols, m.viewDetails(dw, ph, now))
		}
		body = lipgloss.JoinHorizontal(lipgloss.Top, cols...)
	}
	return fit(m.header(now), m.w) + "\n" + body + "\n" + fit(m.statusLine(), m.w) + "\n" + fit(m.cmdLine(now), m.w)
}

func (m *Model) busy() bool { return m.pending+m.fetching > 0 }

func (m *Model) header(now time.Time) string {
	logo := " ✿ gtask"
	var left string
	if m.busy() || now.Sub(m.started) < introLife {
		left = wave(logo, now.Sub(m.started).Seconds()*.9, .09, true)
	} else {
		left = wave(logo, 0, .09, true)
	}
	var right string
	switch {
	case m.quit:
		right = spin(now) + paint(cGold, fmt.Sprintf(" saving %d change%s", m.pending, plural(m.pending)))
	case m.busy():
		right = spin(now) + paint(cFoam, " syncing")
	case m.offline:
		right = paint(cLove, "✗ offline")
	case !m.lastSync.IsZero():
		right = paint(cPine, "●") + paint(cMuted, " synced "+ago(m.lastSync.UTC().Format(time.RFC3339)))
	}
	if m.demo {
		right = paint(cGold, "demo") + paint(cHlHigh, " · ") + right
	}
	gap := m.w - lipgloss.Width(left) - lipgloss.Width(right) - 1
	return left + strings.Repeat(" ", max(gap, 1)) + right
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func (m *Model) viewLists(w, h int, now time.Time) string {
	inner := w - 4
	var lines []string
	for i, l := range m.lists {
		sel := i == m.listIdx
		bar := "  "
		if sel {
			c := cIris
			if m.focus != paneLists {
				c = cHlHigh
			}
			bar = paint(c, "▌") + " "
		}
		name := paint(cSubtle, l.Title)
		if sel {
			name = fg(cRose).Bold(true).Render(l.Title)
		}
		count := ""
		if open, _ := m.counts(l.ID); m.loaded[l.ID] && open > 0 {
			count = paint(cMuted, fmt.Sprint(open))
		} else if !m.loaded[l.ID] {
			count = paint(cHlHigh, "·")
		}
		name = ansi.Truncate(name, inner-2-lipgloss.Width(count)-1, "…")
		gap := inner - 2 - lipgloss.Width(name) - lipgloss.Width(count)
		lines = append(lines, bar+name+strings.Repeat(" ", max(gap, 1))+count)
	}
	border := m.focusColor(paneLists, now)
	return box(paint(cSubtle, "lists"), lines, w, h, border, 0, 0)
}

func (m *Model) viewTasks(w, h int, now time.Time) string {
	inner := w - 4
	rs := m.rows()
	sep := m.sepIndex(rs)
	var lines []string
	for i, r := range rs {
		if i == sep {
			label := fmt.Sprintf(" completed · %d ", len(rs)-sep)
			lines = append(lines, paint(cHlMed, "──")+paint(cMuted, label)+paint(cHlMed, strings.Repeat("─", max(inner-2-lipgloss.Width(label), 0))))
		}
		lines = append(lines, m.row(r, i == m.cursor, inner, now))
	}
	body := h - 2
	if len(lines) == 0 {
		var msg []string
		switch {
		case !m.loaded[m.curID()]:
			msg = []string{spin(now) + paint(cSubtle, " loading")}
		case m.filter != "":
			msg = []string{paint(cMuted, "no matches for “"+m.filter+"”")}
		default:
			_, done := m.counts(m.curID())
			msg = []string{wave("✿  all clear", now.Sub(m.started).Seconds()*.3, .08, false), "",
				paint(cMuted, "press ") + paint(cRose, "o") + paint(cMuted, " to add a task")}
			if done > 0 && !m.showDone {
				msg = append(msg, paint(cMuted, fmt.Sprintf("%d completed hidden · ", done))+paint(cRose, "C")+paint(cMuted, " to show"))
			}
		}
		top := max((body-len(msg))/2, 0)
		lines = make([]string, top)
		for _, l := range msg {
			lines = append(lines, strings.Repeat(" ", max((inner-lipgloss.Width(l))/2, 0))+l)
		}
	}
	end := min(m.offset+body, len(lines))
	if m.offset < end {
		lines = lines[m.offset:end]
	}

	l := m.curList()
	title := ""
	if l != nil {
		// The title types itself in when you switch lists.
		p := easeOut(float64(now.Sub(m.listAt)) / float64(focusLife))
		n := int(p * float64(utf8.RuneCountInString(l.Title)))
		shown := string([]rune(l.Title)[:n])
		if p >= 1 {
			title = fg(cRose).Bold(true).Render(l.Title)
		} else {
			title = wave(shown, now.Sub(m.listAt).Seconds()*3, .1, true)
		}
		open, done := m.counts(l.ID)
		title += paint(cMuted, fmt.Sprintf(" %d", open))
		if done > 0 {
			title += paint(cHlHigh, fmt.Sprintf(" ✓%d", done))
		}
		if m.filter != "" {
			title += paint(cGold, "  /"+m.filter)
		}
	}
	return box(title, lines, w, h, m.focusColor(paneTasks, now), m.lineCount(), m.offset)
}

func (m *Model) row(r row, sel bool, w int, now time.Time) string {
	focused := m.focus == paneTasks
	bar := "  "
	switch {
	case sel && focused:
		bar = paint(cIris, "▌") + " "
	case sel:
		bar = paint(cHlHigh, "▌") + " "
	case r.t != nil:
		// A short comet tail where the cursor just was.
		if at, ok := m.trail[r.t.ID]; ok && focused {
			p := clamp(float64(now.Sub(at)) / float64(trailLife))
			g := []string{"▌", "▍", "▎", "▏"}[min(int(p*4), 3)]
			bar = paint(lerp(cIris, cHlMed, p), g) + " "
		}
	}
	indent := strings.Repeat(paint(cHlMed, "│ "), r.depth)
	if r.editNew {
		return bar + indent + paint(cRose, "○ ") + m.in.View()
	}
	t := r.t
	a := m.anims[t.ID]

	check, checkC := "○", cSubtle
	if sel && focused {
		checkC = cRose
	}
	if t.Done() {
		check, checkC = "✓", cPine
	}

	meta, metaC := "", cMuted
	if t.Done() {
		meta = ago(t.Completed)
	} else if d, ok := t.DueDate(); ok {
		meta, metaC = HumanDue(d)
	}
	extras := ""
	if r.folded {
		extras += paint(cMuted, fmt.Sprintf(" ▸ %d", r.kids))
	}
	if t.Notes != "" && !m.showDetails() {
		extras += paint(cMuted, " ≡")
	}

	if m.mode == modeInsert && m.ed.kind == editRename && m.ed.taskID == t.ID {
		return bar + indent + paint(cRose, check+" ") + m.in.View()
	}

	title := t.Title
	if title == "" {
		title = "(untitled)"
	}
	left := bar + indent
	used := lipgloss.Width(left) + 2 + lipgloss.Width(extras) + lipgloss.Width(meta) + 2
	title = ansi.Truncate(title, max(w-used, 4), "…")
	runes := []rune(title)

	base := fg(cText)
	if t.Done() {
		base = fg(cMuted).Strikethrough(true)
	} else if r.depth > 0 && !sel {
		base = fg(lerp(cText, cSubtle, .5))
	}
	if sel && focused {
		base = base.Bold(true)
	}
	styled := m.highlight(title, base)

	if a != nil {
		p := a.progress(now)
		switch a.kind {
		case animComplete:
			// Checkbox fills in, then a strikethrough sweeps left to right.
			frames := []string{"○", "◔", "◑", "◕", "●"}
			if p < .45 {
				check, checkC = frames[min(int(p/.45*5), 4)], lerp(cSubtle, cFoam, p/.45)
			} else {
				check, checkC = "✓", lerp(cFoam, cPine, (p-.45)/.55)
			}
			k := int(easeInOut((p-.15)/.7) * float64(len(runes)))
			mc := lerp(cText, cMuted, p)
			styled = fg(mc).Strikethrough(true).Render(string(runes[:k])) + base.Render(string(runes[k:]))
			sparkle := []string{"✦", "✧", "⋆", "·", " "}
			if p > .4 {
				meta, metaC = sparkle[min(int((p-.4)/.6*5), 4)], lerp(cGold, cRose, p)
			}
		case animDelete:
			e := easeInOut(p)
			k := int((1 - e) * float64(len(runes)))
			styled = fg(lerp(cText, cLove, p*2)).Render(string(runes[:k]))
			check, checkC = "✕", lerp(cLove, cHlMed, e)
			meta = ""
			if sel {
				bar = paint(cLove, "▌") + " "
			}
		case animFlash:
			c := lerp(cGold, cText, easeOut(p))
			if t.Done() {
				c = lerp(cGold, cMuted, easeOut(p))
			}
			styled = fg(c).Bold(p < .5).Render(title)
		}
	}

	left += paint(checkC, check) + " "
	gap := w - lipgloss.Width(left) - lipgloss.Width(styled) - lipgloss.Width(extras) - lipgloss.Width(meta)
	return left + styled + extras + strings.Repeat(" ", max(gap, 1)) + paint(metaC, meta)
}

// highlight renders the filter match inside s.
func (m *Model) highlight(s string, base lipgloss.Style) string {
	if m.filter == "" {
		return base.Render(s)
	}
	i := strings.Index(strings.ToLower(s), strings.ToLower(m.filter))
	if i < 0 || len(strings.ToLower(s)) != len(s) {
		return base.Render(s)
	}
	j := i + len(m.filter)
	return base.Render(s[:i]) + fg(cGold).Bold(true).Underline(true).Render(s[i:j]) + base.Render(s[j:])
}

func wrap(s string, w int) []string {
	if s == "" {
		return nil
	}
	return strings.Split(lipgloss.NewStyle().Width(w).Render(s), "\n")
}

func (m *Model) viewDetails(w, h int, now time.Time) string {
	inner := w - 4
	t := m.selected()
	var lines []string
	if t != nil {
		ts := fg(cText).Bold(true)
		if t.Done() {
			ts = fg(cMuted).Strikethrough(true)
		}
		for _, l := range wrap(t.Title, inner) {
			lines = append(lines, ts.Render(strings.TrimRight(l, " ")))
		}
		lines = append(lines, "")
		field := func(k, v string) { lines = append(lines, paint(cMuted, fmt.Sprintf("%-9s", k))+v) }
		if d, ok := t.DueDate(); ok {
			hd, c := HumanDue(d)
			field("due", paint(c, hd)+paint(cMuted, "  "+d.Format("Mon, Jan 2")))
		} else {
			field("due", paint(cHlHigh, "—"))
		}
		if t.Done() {
			field("status", paint(cPine, "✓ done ")+paint(cMuted, ago(t.Completed)))
		} else {
			field("status", paint(cSubtle, "○ open"))
		}
		if n := len(m.children(m.curID(), t.ID)); n > 0 {
			field("subtasks", paint(cIris, fmt.Sprint(n)))
		}
		if u := ago(t.Updated); u != "" {
			field("updated", paint(cSubtle, u))
		}
		lines = append(lines, "", paint(cIris, "notes"))
		if t.Notes == "" {
			lines = append(lines, paint(cHlHigh, "none — ")+paint(cMuted, "e")+paint(cHlHigh, " to write some"))
		} else {
			for _, l := range wrap(t.Notes, inner) {
				lines = append(lines, paint(cSubtle, l))
			}
		}
	}
	return box(paint(cSubtle, "details"), lines, w, h, cHlHigh, 0, 0)
}

var modeInfo = map[mode][2]string{
	modeNormal:  {"NORMAL", cIris},
	modeInsert:  {"INSERT", cFoam},
	modeCommand: {"COMMAND", cGold},
	modeSearch:  {"SEARCH", cRose},
	modePrompt:  {"INPUT", cGold},
	modeConfirm: {"CONFIRM", cLove},
	modeHelp:    {"HELP", cPine},
}

func (m *Model) statusLine() string {
	mi := modeInfo[m.mode]
	pill := lipgloss.NewStyle().Background(lipgloss.Color(mi[1])).Foreground(lipgloss.Color(cBase)).Bold(true).Padding(0, 1).Render(mi[0])
	left := pill
	if l := m.curList(); l != nil {
		open, done := m.counts(l.ID)
		left += " " + paint(cRose, l.Title) + paint(cMuted, fmt.Sprintf("  %d open · %d done", open, done))
	}
	if m.pending > 0 {
		left += paint(cGold, fmt.Sprintf("  ↑%d", m.pending))
	}
	var right []string
	if sc := m.showcmd(); sc != "" {
		right = append(right, paint(cGold, sc))
	}
	if m.focus == paneTasks {
		if n := len(m.rows()); n > 0 {
			right = append(right, paint(cSubtle, fmt.Sprintf("%d/%d", m.cursor+1, n)))
		}
	}
	right = append(right, paint(cMuted, "? help "))
	r := strings.Join(right, paint(cHlHigh, "  "))
	return left + strings.Repeat(" ", max(m.w-lipgloss.Width(left)-lipgloss.Width(r), 1)) + r
}

func (m *Model) showcmd() string {
	s := m.keys
	if m.count > 0 {
		s = fmt.Sprint(m.count) + s
	}
	return s
}

func (m *Model) cmdLine(now time.Time) string {
	switch m.mode {
	case modeCommand:
		return paint(cGold, ":") + m.in.View()
	case modeSearch:
		return paint(cRose, "/") + m.in.View()
	case modePrompt:
		return paint(cIris, m.promptLabel) + m.in.View()
	case modeConfirm:
		return paint(cGold, m.confirmText)
	case modeInsert:
		return fg(cFoam).Bold(true).Render("-- INSERT --") + paint(cMuted, "   ⏎ save & next   esc save   ^c cancel")
	}
	if m.quit {
		return spin(now) + paint(cGold, fmt.Sprintf(" waiting for %d change%s to reach Google… ", m.pending, plural(m.pending))) + paint(cMuted, "(q again to force)")
	}
	if t := m.toast; t != nil {
		age := now.Sub(t.at)
		runes := []rune(t.text)
		n := int(easeOut(float64(age)/float64(toastSlide)) * float64(len(runes)))
		c := t.color
		if fade := age - (toastLife - toastFade); fade > 0 {
			c = lerp(t.color, cHlMed, float64(fade)/float64(toastFade))
		}
		return paint(c, string(runes[:n]))
	}
	return ""
}

var logoArt = []string{
	"╔═╗╔╦╗╔═╗╔═╗╦╔═",
	"║ ╦ ║ ╠═╣╚═╗╠╩╗",
	"╚═╝ ╩ ╩ ╩╚═╝╩ ╩",
}

func (m *Model) splash(now time.Time) string {
	t := now.Sub(m.started).Seconds()
	var out []string
	for i, l := range logoArt {
		out = append(out, wave(l, t*.7-float64(i)*.06, .035, true))
	}
	// A row of twinkling stars.
	stars := []rune(" ·⋆✧✦✧⋆· ")
	var tw strings.Builder
	for i := 0; i < 15; i++ {
		ph := t*1.6 + float64(i*7%11)*.37
		k := int(ph*4) % len(stars)
		tw.WriteString(paint(gradient(shimmer, float64(i)/15-t*.2), string(stars[k])))
	}
	out = append(out, "", tw.String(), "")
	if m.offline {
		out = append(out, paint(cLove, "couldn't reach Google Tasks"), paint(cMuted, "R retry · q quit"))
	} else {
		out = append(out, spin(now)+paint(cSubtle, " fetching your lists"))
	}
	if m.toast != nil && m.offline {
		out = append(out, "", paint(cMuted, ansi.Truncate(m.toast.text, m.w-4, "…")))
	}
	return lipgloss.Place(m.w, m.h, lipgloss.Center, lipgloss.Center, lipgloss.JoinVertical(lipgloss.Center, out...))
}

func (m *Model) help() string {
	type kv [2]string
	sections := []struct {
		name  string
		color string
		keys  []kv
	}{
		{"move", cFoam, []kv{{"j k", "down / up (5j works)"}, {"gg G", "top / bottom"}, {"^d ^u", "half page"}, {"h l  tab", "lists ↔ tasks"}, {"H L  gT gt", "prev / next list"}}},
		{"edit", cIris, []kv{{"o O", "new task below / above"}, {"i a r", "rename"}, {"cc S", "replace title"}, {"e ⏎", "title + notes in $EDITOR"}, {"t T", "set / clear due date"}, {"x ␣", "toggle done"}}},
		{"organize", cRose, []kv{{"dd", "delete (into register)"}, {"yy", "yank (+ clipboard)"}, {"p P", "paste below / above"}, {"J K", "move down / up"}, {">> <<", "indent / outdent"}, {"za zR zM", "fold / open / close all"}, {"u", "undo"}}},
		{"more", cGold, []kv{{"/", "filter   esc clears"}, {"C", "show completed"}, {"R", "sync now"}, {":new :rename", "create / rename list"}, {":dellist :clear", "delete list / clear done"}, {"q :q", "quit"}}},
	}
	col := func(idx ...int) string {
		var ls []string
		for _, i := range idx {
			s := sections[i]
			ls = append(ls, fg(s.color).Bold(true).Render(s.name))
			for _, k := range s.keys {
				ls = append(ls, paint(cText, fmt.Sprintf("%-16s", k[0]))+paint(cSubtle, k[1]))
			}
			ls = append(ls, "")
		}
		return strings.Join(ls, "\n")
	}
	cols := lipgloss.JoinHorizontal(lipgloss.Top, col(0, 1), "    ", col(2, 3))
	due := paint(cMuted, "dates: ") + paint(cSubtle, "today tom fri +3d +2w oct 12 2026-10-12") +
		paint(cMuted, "   quick add: ") + paint(cSubtle, "buy milk @tom")
	content := lipgloss.JoinVertical(lipgloss.Left, cols, due)
	w := min(lipgloss.Width(content)+4, m.w)
	lines := strings.Split(content, "\n")
	return box(wave("keys", 0, .1, true), lines, w, min(len(lines)+2, m.h-3), cIris, 0, 0)
}
