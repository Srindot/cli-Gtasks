package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var spinner = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func spin(now time.Time) string {
	i := int(now.UnixMilli()/80) % len(spinner)
	return paint(gradient(shimmer, float64(now.UnixMilli())/1800), spinner[i])
}

func ansiTrunc(s string, w int) string { return ansi.Truncate(s, max(w, 0), "…") }

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

// line builds one row of a column. When bg is set, every segment (and the
// padding) carries it, so the highlight spans the full width like yazi's.
type line struct {
	bg string
	sb strings.Builder
	w  int
}

func (l *line) put(s string, st lipgloss.Style) {
	if s == "" {
		return
	}
	if l.bg != "" {
		st = st.Background(lipgloss.Color(l.bg))
	}
	l.sb.WriteString(st.Render(s))
	l.w += lipgloss.Width(s)
}

func (l *line) raw(s string) {
	l.sb.WriteString(s)
	l.w += lipgloss.Width(s)
}

func (l *line) done(w int) string {
	if l.w < w {
		l.put(strings.Repeat(" ", w-l.w), lipgloss.NewStyle())
	}
	return ansi.Truncate(l.sb.String(), w, "")
}

// ---- layout --------------------------------------------------------------

// layout splits the width 2:4:3 like yazi's parent/current/preview columns.
func (m *Model) layout() (lw, tw, dw int) {
	switch {
	case m.w >= 96:
		u := m.w - 2
		lw = max(18, u*2/9)
		dw = max(30, u*3/9)
		tw = u - lw - dw
	case m.w >= 64:
		lw = 20
		tw = m.w - lw - 1
		if m.focus == paneDetails {
			dw, tw = tw, 0
		}
	default:
		switch m.focus {
		case paneLists:
			lw = m.w
		case paneDetails:
			dw = m.w
		default:
			tw = m.w
		}
	}
	return
}

func (m *Model) showDetails() bool { _, _, dw := m.layout(); return dw > 0 }

// bodyHeight is the number of rows in the columns.
func (m *Model) bodyHeight() int { return max(m.h-3, 1) }

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

// lineOf is the line row i starts on; cards take more than one line.
func (m *Model) lineOf(i int) int {
	rs := m.rows()
	n := 0
	for j := 0; j < i && j < len(rs); j++ {
		n += m.span(rs[j])
	}
	if s := m.sepIndex(rs); s >= 0 && i >= s {
		n++
	}
	return n
}

func (m *Model) lineCount() int { return m.lineOf(len(m.rows())) }

func (m *Model) span(r row) int {
	if m.isCard(r) {
		return 3
	}
	return 1
}

// isCard reports whether a row is being typed into (and so drawn as a card).
func (m *Model) isCard(r row) bool {
	return r.editNew || (m.mode == modeInsert && m.ed.kind == editRename && r.t != nil && r.t.ID == m.ed.taskID)
}

// taskCard draws the input card for a new task or a rename, in place in the list.
func (m *Model) taskCard(r row, w int, now time.Time) []string {
	color, depth := m.draftColor(), float64(r.depth)
	if m.ed.from != "" {
		color = glide(m.ed.from, color, m.ed.fromAt, now)
		g := easeOut(float64(now.Sub(m.ed.fromAt)) / float64(glideLife))
		depth = float64(m.ed.fromDepth) + (depth-float64(m.ed.fromDepth))*g
	}
	indent := 1 + int(depth*3+.5)
	cw := w - indent - 1
	var title []seg
	check := ic.open
	switch {
	case m.ed.kind == editRename:
		title = segs(seg{ic.edit + " rename", color})
		if r.t != nil && r.t.Done() {
			check = ic.done
		}
	case m.ed.sub:
		under := ""
		if p := m.find(m.ed.listID, m.ed.parent); p != nil {
			under = " of " + p.Title
		}
		title = segs(seg{ic.sub + " new subtask" + under, color})
	default:
		title = segs(seg{ic.add + " new task", color})
	}
	body := paint(color, check+" ") + m.inputView(cw-6, true, m.in.Placeholder, now)
	out := frame(cw, color, title, m.chip(m.cardHint(), now), []string{body}, m.ed.at, now)
	pad := strings.Repeat(" ", indent)
	for i := range out {
		out[i] = fit(pad+out[i], w)
	}
	return out
}

// selBg is the highlight behind the hovered row; it brightens as a pane gains focus.
func (m *Model) selBg(p pane, now time.Time) string {
	t := easeOut(float64(now.Sub(m.focusAt)) / float64(focusLife))
	if m.focus == p {
		return lerp(cHlLow, cHlMed, t)
	}
	return lerp(cHlMed, cHlLow, t)
}

// cascade fades + slides rows in one after another after a list switch.
func cascade(s string, i int, since time.Duration, w int) string {
	p := clamp(float64(since-time.Duration(min(i, 24))*16*time.Millisecond) / float64(200*time.Millisecond))
	if p >= 1 {
		return s
	}
	shift := int((1 - easeOut(p)) * 6)
	plain := ansi.Strip(s)
	return fit(strings.Repeat(" ", shift)+paint(lerp(cHlMed, cSubtle, p), plain), w)
}

// View always returns exactly m.h lines of exactly m.w cells. Anything taller
// makes the terminal scroll and the top of the UI vanishes, so clamp here no
// matter what the panes produced.
func (m *Model) View() (out string) {
	if m.w == 0 || m.h == 0 {
		return ""
	}
	s := func() (s string) {
		defer m.guard("view")
		return m.render()
	}()
	ls := strings.Split(s, "\n")
	if len(ls) > m.h {
		ls = ls[:m.h]
	}
	for len(ls) < m.h {
		ls = append(ls, "")
	}
	for i, l := range ls {
		ls[i] = fit(l, m.w)
	}
	return strings.Join(ls, "\n")
}

func (m *Model) render() string {
	now := time.Now()
	if len(m.lists) == 0 {
		return m.splash(now)
	}
	h := m.bodyHeight()
	var body []string
	if m.mode == modeHelp {
		body = strings.Split(lipgloss.Place(m.w, h, lipgloss.Center, lipgloss.Center, m.help()), "\n")
	} else {
		lw, tw, dw := m.layout()
		var cols [][]string
		if lw > 0 {
			cols = append(cols, m.colLists(lw, h, now))
		}
		if tw > 0 {
			cols = append(cols, m.colTasks(tw, h, now))
		}
		if dw > 0 {
			cols = append(cols, m.colDetails(dw, h, now))
		}
		sep := paint(cHlMed, "│")
		for i := 0; i < h; i++ {
			parts := make([]string, len(cols))
			for c := range cols {
				parts[c] = cols[c][i]
			}
			body = append(body, strings.Join(parts, sep))
		}
	}
	return fit(m.header(now), m.w) + "\n" + strings.Join(body, "\n") + "\n" + fit(m.statusLine(), m.w) + "\n" + fit(m.cmdLine(now), m.w)
}

func (m *Model) busy() bool { return m.pending+m.fetching > 0 }

// header: logo, then a yazi-like breadcrumb, then sync state on the right.
func (m *Model) header(now time.Time) string {
	logo := " ✿ gtask"
	phase := 0.0
	if m.busy() || now.Sub(m.started) < introLife {
		phase = now.Sub(m.started).Seconds() * .9
	}
	left := wave(logo, phase, .09, true)
	if l := m.curList(); l != nil {
		left += paint(cHlHigh, "  ") + fg(cIris).Bold(true).Render(l.Title)
		if t := m.selected(); t != nil && m.focus != paneLists {
			if r := m.rootOf(t); r != t {
				left += paint(cHlHigh, " / ") + paint(cSubtle, ansiTrunc(r.Title, 28))
			}
			left += paint(cHlHigh, " / ") + paint(cText, ansiTrunc(t.Title, 40))
		}
	}
	var right string
	switch {
	case m.quit:
		right = spin(now) + paint(cGold, fmt.Sprintf(" saving %d change%s", m.pending, plural(m.pending)))
	case m.busy():
		right = spin(now) + paint(cFoam, " syncing")
	case m.offline:
		right = paint(cLove, ic.off+" offline")
	case !m.lastSync.IsZero():
		right = paint(cPine, ic.ok) + paint(cMuted, " synced "+ago(m.lastSync.UTC().Format(time.RFC3339)))
	}
	if m.demo {
		right = paint(cGold, "demo") + paint(cHlHigh, " · ") + right
	}
	right += " "
	left = ansiTrunc(left, m.w-lipgloss.Width(right)-1)
	return left + strings.Repeat(" ", max(m.w-lipgloss.Width(left)-lipgloss.Width(right), 1)) + right
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// ---- lists column --------------------------------------------------------

func (m *Model) colLists(w, h int, now time.Time) []string {
	lines := make([]string, 0, h)
	for i, l := range m.lists {
		sel := i == m.listIdx
		ln := &line{}
		if sel {
			ln.bg = m.selBg(paneLists, now)
		}
		icon, iconC, name := ic.list, cMuted, fg(cSubtle)
		if sel {
			icon, iconC, name = ic.listOpen, cIris, fg(cText).Bold(true)
		}
		count := ""
		if open, _ := m.counts(l.ID); m.loaded[l.ID] && open > 0 {
			count = fmt.Sprint(open)
		}
		ln.put(" "+icon+" ", fg(iconC))
		ln.put(ansiTrunc(l.Title, w-6-len(count)), name)
		ln.put(strings.Repeat(" ", max(w-ln.w-len(count)-1, 1)), lipgloss.NewStyle())
		ln.put(count+" ", fg(cMuted))
		lines = append(lines, ln.done(w))
	}
	for len(lines) < h {
		lines = append(lines, strings.Repeat(" ", w))
	}
	lines = lines[:h]
	if m.pop != nil && m.pop.pane == paneLists {
		lines = splice(lines, m.listIdx+1, m.popupCard(cardWidth(w), now), w, h)
	}
	return lines
}

// ---- tasks column --------------------------------------------------------

func (m *Model) colTasks(w, h int, now time.Time) []string {
	rs := m.rows()
	sep := m.sepIndex(rs)
	var all []string
	for i, r := range rs {
		if i == sep {
			label := fmt.Sprintf(" %s completed · %d ", ic.done, len(rs)-sep)
			all = append(all, " "+paint(cHlMed, "─")+paint(cMuted, label)+paint(cHlMed, strings.Repeat("─", max(w-2-lipgloss.Width(label), 0))))
		}
		if m.isCard(r) {
			all = append(all, m.taskCard(r, w, now)...)
			continue
		}
		all = append(all, m.taskRow(r, i == m.cursor, w, now))
	}
	if len(all) == 0 {
		all = m.emptyTasks(w, h, now)
	}
	lines := make([]string, 0, h)
	since := now.Sub(m.listAt)
	for i := m.offset; i < len(all) && len(lines) < h; i++ {
		lines = append(lines, cascade(all[i], i-m.offset, since, w))
	}
	for len(lines) < h {
		lines = append(lines, strings.Repeat(" ", w))
	}
	// Scrollbar thumb on the right edge when the list overflows.
	if total := len(all); total > h {
		size := max(1, h*h/total)
		top := m.offset * (h - size) / max(total-h, 1)
		for i := top; i < top+size && i < h; i++ {
			lines[i] = ansi.Truncate(lines[i], w-1, "") + paint(cHlHigh, "▐")
		}
	}
	if m.pop != nil && m.pop.pane == paneTasks {
		y := m.lineOf(m.cursor) - m.offset + 1
		lines = splice(lines, y, m.popupCard(cardWidth(w), now), w, h)
	}
	return lines
}

func (m *Model) emptyTasks(w, h int, now time.Time) []string {
	var msg []string
	switch {
	case !m.loaded[m.curID()]:
		msg = []string{spin(now) + paint(cSubtle, " loading")}
	case m.filter != "":
		msg = []string{paint(cMuted, "no matches for “"+m.filter+"”")}
	default:
		_, done := m.counts(m.curID())
		msg = []string{wave("✿  all clear", now.Sub(m.started).Seconds()*.3, .08, false), "",
			paint(cRose, "a") + paint(cMuted, "  new task")}
		if done > 0 && !m.showDone {
			msg = append(msg, paint(cRose, "C")+paint(cMuted, fmt.Sprintf("  show %d completed", done)))
		}
	}
	out := make([]string, max((h-len(msg))/2-1, 0))
	for _, l := range msg {
		out = append(out, strings.Repeat(" ", max((w-lipgloss.Width(l))/2, 0))+l)
	}
	for i := range out {
		out[i] = fit(out[i], w)
	}
	return out
}

func (m *Model) taskRow(r row, sel bool, w int, now time.Time) string {
	focused := m.focus == paneTasks
	ln := &line{}
	if sel && !r.editNew {
		ln.bg = m.selBg(paneTasks, now)
	}

	// Left accent: solid on the cursor, a fading comet tail where it just was.
	switch {
	case sel && focused:
		ln.put("▎", fg(cIris))
	case r.t != nil && m.trail[r.t.ID] != (time.Time{}) && focused:
		p := clamp(float64(now.Sub(m.trail[r.t.ID])) / float64(trailLife))
		ln.put([]string{"▎", "▏", "▏", " "}[min(int(p*4), 3)], fg(lerp(cIris, cHlLow, p)))
	default:
		ln.put(" ", lipgloss.NewStyle())
	}
	ln.put(" ", lipgloss.NewStyle())
	if r.depth > 0 {
		branch := "├─ "
		if r.last {
			branch = "╰─ "
		}
		ln.put(strings.Repeat("   ", r.depth-1)+" "+branch, fg(cHlHigh))
	}

	if r.editNew {
		ln.put(ic.open+" ", fg(cRose))
		ln.raw(m.in.View())
		return ln.done(w)
	}
	t := r.t
	a := m.anims[t.ID]

	check, checkC := ic.open, cSubtle
	if r.depth > 0 {
		checkC = cMuted
	}
	if sel && focused {
		checkC = cRose
	}
	if t.Done() {
		check, checkC = ic.done, cPine
	}

	if m.mode == modeInsert && m.ed.kind == editRename && m.ed.taskID == t.ID {
		ln.bg = ""
		ln.put(check+" ", fg(cRose))
		ln.raw(m.in.View())
		return ln.done(w)
	}

	// Right side: due date (or completion time), notes + folded markers.
	meta, metaC := "", cMuted
	if t.Done() {
		meta = ago(t.Completed)
	} else if d, ok := t.DueDate(); ok {
		meta, metaC = HumanDue(d)
		meta = ic.due + " " + meta
	}
	var extras []string
	if r.folded {
		extras = append(extras, fmt.Sprintf("+%d", r.kids))
	}
	if t.Notes != "" {
		extras = append(extras, ic.notes)
	}
	extra := ""
	if len(extras) > 0 {
		extra = " " + strings.Join(extras, " ")
	}

	title := t.Title
	if title == "" {
		title = "(untitled)"
	}
	avail := w - ln.w - 2 - lipgloss.Width(extra) - lipgloss.Width(meta) - 2
	title = ansiTrunc(title, max(avail, 4))
	runes := []rune(title)

	base := fg(cText)
	switch {
	case t.Done():
		base = fg(cMuted).Strikethrough(true)
	case r.depth > 0 && !sel:
		base = fg(lerp(cText, cSubtle, .6))
	}
	if sel && focused {
		base = base.Bold(true)
	}

	segs := m.highlight(title, base)

	if a != nil {
		p := a.progress(now)
		switch a.kind {
		case animComplete:
			// The ring fills, then a strikethrough sweeps across, then a sparkle.
			frames := []string{"○", "◔", "◑", "◕", "●"}
			if p < .45 {
				check, checkC = frames[min(int(p/.45*5), 4)], lerp(cSubtle, cFoam, p/.45)
			} else {
				check, checkC = ic.done, lerp(cFoam, cPine, (p-.45)/.55)
			}
			k := int(easeInOut((p-.15)/.7) * float64(len(runes)))
			segs = []segment{
				{string(runes[:k]), fg(lerp(cText, cMuted, p)).Strikethrough(true)},
				{string(runes[k:]), base},
			}
			if p > .4 {
				meta, metaC = []string{"✦", "✧", "⋆", "·", " "}[min(int((p-.4)/.6*5), 4)], lerp(cGold, cRose, p)
			}
		case animDelete:
			e := easeInOut(p)
			k := int((1 - e) * float64(len(runes)))
			segs = []segment{{string(runes[:k]), fg(lerp(cText, cLove, p*2))}}
			check, checkC, meta = ic.trash, lerp(cLove, cHlMed, e), ""
		case animFlash:
			c := lerp(cGold, cText, easeOut(p))
			if t.Done() {
				c = lerp(cGold, cMuted, easeOut(p))
			}
			segs = []segment{{title, fg(c).Bold(p < .5)}}
		}
	}

	ln.put(check+" ", fg(checkC))
	for _, s := range segs {
		ln.put(s.s, s.st)
	}
	ln.put(extra, fg(cMuted))
	ln.put(strings.Repeat(" ", max(w-ln.w-lipgloss.Width(meta)-1, 1)), lipgloss.NewStyle())
	ln.put(meta+" ", fg(metaC))
	return ln.done(w)
}

type segment struct {
	s  string
	st lipgloss.Style
}

// highlight splits s around the filter match so it can be underlined.
func (m *Model) highlight(s string, base lipgloss.Style) []segment {
	if m.filter == "" {
		return []segment{{s, base}}
	}
	i := strings.Index(strings.ToLower(s), strings.ToLower(m.filter))
	if i < 0 || len(strings.ToLower(s)) != len(s) {
		return []segment{{s, base}}
	}
	j := i + len(m.filter)
	return []segment{{s[:i], base}, {s[i:j], fg(cGold).Bold(true).Underline(true)}, {s[j:], base}}
}

// ---- details column ------------------------------------------------------

func wrap(s string, w int) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, l := range strings.Split(lipgloss.NewStyle().Width(w).Render(s), "\n") {
		out = append(out, strings.TrimRight(l, " "))
	}
	return out
}

func (m *Model) colDetails(w, h int, now time.Time) []string {
	t := m.selected()
	focused := m.focus == paneDetails
	inner := w - 4
	var lines []string
	popupAt := 0

	// field renders a group of lines as one focusable item.
	field := func(id int, ls ...func(l *line)) {
		for _, f := range ls {
			ln := &line{}
			on := focused && m.dfield == id
			if on {
				ln.bg = m.selBg(paneDetails, now)
				ln.put("▎", fg(cIris))
				popupAt = len(lines) + 1
			} else {
				ln.put(" ", lipgloss.NewStyle())
			}
			ln.put(" ", lipgloss.NewStyle())
			f(ln)
			lines = append(lines, ln.done(w))
		}
	}
	blank := func() { lines = append(lines, strings.Repeat(" ", w)) }
	heading := func(name, extra string) {
		ln := &line{}
		ln.put("  "+name, fg(cIris).Bold(true))
		ln.put(extra, fg(cMuted))
		ln.put(" "+strings.Repeat("─", max(w-ln.w-3, 0)), fg(cHlMed))
		lines = append(lines, ln.done(w))
	}
	label := func(l *line, icon, k string) { l.put(icon+" ", fg(cMuted)); l.put(fmt.Sprintf("%-8s", k), fg(cMuted)) }

	if t == nil && m.mode == modeInsert && m.ed.kind == editNew {
		lines = append(lines, m.draftDetails(w, now)...)
	} else if t == nil {
		blank()
		lines = append(lines, "  "+paint(cMuted, "nothing selected"))
	} else {
		blank()
		if r := m.rootOf(t); r != t {
			lines = append(lines, (&line{}).doneWith(w, func(l *line) {
				l.put("  "+ic.sub+" ", fg(cMuted))
				l.put(ansiTrunc(r.Title, inner-3), fg(cMuted))
			}))
		}
		// Title
		ts := fg(cText).Bold(true)
		if t.Done() {
			ts = fg(cMuted).Strikethrough(true)
		}
		var tl []func(l *line)
		for _, s := range wrap(t.Title, inner) {
			s := s
			tl = append(tl, func(l *line) { l.put(s, ts) })
		}
		field(fTitle, tl...)
		blank()

		// Due
		field(fDue, func(l *line) {
			label(l, ic.due, "due")
			if d, ok := t.DueDate(); ok {
				_, c := HumanDue(d)
				l.put(d.Format("Mon, Jan 2"), fg(c))
				l.put("  "+relDays(d), fg(cMuted))
			} else {
				l.put("none", fg(cHlHigh))
				if focused && m.dfield == fDue {
					l.put("  ⏎ to set", fg(cMuted))
				}
			}
		})
		// Status
		field(fStatus, func(l *line) {
			label(l, ic.ok, "status")
			if t.Done() {
				l.put(ic.done+" done", fg(cPine))
				l.put("  "+ago(t.Completed), fg(cMuted))
			} else {
				l.put(ic.open+" open", fg(cSubtle))
			}
		})
		if u := ago(t.Updated); u != "" {
			field(-1, func(l *line) { label(l, ic.sync, "updated"); l.put(u, fg(cSubtle)) })
		}
		blank()

		// Notes
		heading(ic.notes+" notes", "")
		switch {
		case m.mode == modeNotes:
			hint := segs(seg{"esc", cSubtle}, seg{" save  ", cMuted}, seg{"^c", cSubtle}, seg{" cancel", cMuted})
			card := frame(cardWidth(w), cIris, segs(seg{"editing", cIris}), hint,
				strings.Split(m.ta.View(), "\n"), m.notesAt, now)
			for _, c := range card {
				lines = append(lines, " "+c)
			}
		case t.Notes == "":
			field(fNotes, func(l *line) { l.put("empty — ⏎ to write", fg(cHlHigh)) })
		default:
			var nl []func(l *line)
			for _, s := range wrap(t.Notes, inner) {
				s := s
				nl = append(nl, func(l *line) { l.put(s, fg(cSubtle)) })
			}
			field(fNotes, nl...)
		}
		blank()

		// Subtasks
		subs := m.detailSubs()
		done := 0
		for _, s := range subs {
			if s.Done() {
				done++
			}
		}
		if t.Parent == "" {
			heading(ic.sub+" subtasks", fmt.Sprintf("  %d/%d", done, len(subs)))
			if len(subs) > 0 {
				// A tiny progress bar.
				bw := max(inner-2, 4)
				fill := bw * done / len(subs)
				lines = append(lines, "  "+paint(cPine, strings.Repeat("━", fill))+paint(cHlMed, strings.Repeat("━", bw-fill)))
			}
			for i, s := range subs {
				s := s
				field(fSubs+i, func(l *line) {
					if s.Done() {
						l.put(ic.done+" ", fg(cPine))
						l.put(ansiTrunc(s.Title, inner-2), fg(cMuted).Strikethrough(true))
					} else {
						l.put(ic.open+" ", fg(cSubtle))
						l.put(ansiTrunc(s.Title, inner-2), fg(cText))
					}
				})
			}
			lines = append(lines, "  "+paint(cMuted, "A")+paint(cHlHigh, "  add subtask"))
		}
	}

	// Fade the preview in when the selection changes.
	since := now.Sub(m.selAt)
	if since < 160*time.Millisecond && m.mode != modeNotes {
		p := easeOut(float64(since) / float64(160*time.Millisecond))
		for i := range lines {
			lines[i] = fit(paint(lerp(cHlMed, cSubtle, p), ansi.Strip(lines[i])), w)
		}
	}
	for i := range lines {
		lines[i] = fit(lines[i], w)
	}
	for len(lines) < h {
		lines = append(lines, strings.Repeat(" ", w))
	}
	lines = lines[:h]
	if m.pop != nil && m.pop.pane == paneDetails {
		lines = splice(lines, popupAt, m.popupCard(cardWidth(w), now), w, h)
	}
	return lines
}

// draftDetails previews the task being typed: title, due date and where it lands.
func (m *Model) draftDetails(w int, now time.Time) []string {
	title, due, hasDue := SplitQuickDue(m.in.Value())
	color := m.draftColor()
	if m.ed.from != "" {
		color = glide(m.ed.from, color, m.ed.fromAt, now)
	}
	inner := w - 4
	var out []string
	add := func(f func(l *line)) { out = append(out, (&line{}).doneWith(w, f)) }
	out = append(out, "")
	kind := "new task"
	if m.ed.sub {
		kind = "new subtask"
	}
	add(func(l *line) { l.put("  "+ic.add+" ", fg(color)); l.put(kind, fg(color).Bold(true)) })
	out = append(out, "")
	if strings.TrimSpace(title) == "" {
		add(func(l *line) { l.put("  untitled", fg(cHlHigh).Italic(true)) })
	} else {
		for _, s := range wrap(title, inner) {
			s := s
			add(func(l *line) { l.put("  "+s, fg(cText).Bold(true)) })
		}
	}
	out = append(out, "")
	label := func(l *line, icon, k string) {
		l.put("  "+icon+" ", fg(cMuted))
		l.put(fmt.Sprintf("%-8s", k), fg(cMuted))
	}
	add(func(l *line) {
		label(l, ic.due, "due")
		if hasDue {
			_, c := HumanDue(due)
			l.put(due.Format("Mon, Jan 2"), m.chipStyle(c, now))
			l.put("  "+relDays(due), fg(cMuted))
		} else {
			l.put("none", fg(cHlHigh))
		}
	})
	if lst := m.curList(); lst != nil {
		add(func(l *line) { label(l, ic.list, "list"); l.put(ansiTrunc(lst.Title, inner-11), fg(cSubtle)) })
	}
	if p := m.find(m.ed.listID, m.ed.parent); p != nil {
		add(func(l *line) { label(l, ic.sub, "under"); l.put(ansiTrunc(p.Title, inner-11), fg(cIris)) })
	}
	return out
}

// chipStyle fades a value in alongside the card's hint chip.
func (m *Model) chipStyle(c string, now time.Time) lipgloss.Style {
	return fg(lerp(cHlMed, c, easeOut(float64(now.Sub(m.hintAt))/float64(chipLife))))
}

func (l *line) doneWith(w int, f func(l *line)) string { f(l); return l.done(w) }

// ---- status + command line -----------------------------------------------

var modeInfo = map[mode][2]string{
	modeNormal:  {"NOR", cIris},
	modeInsert:  {"INS", cFoam},
	modeCommand: {"CMD", cGold},
	modeSearch:  {"FIND", cRose},
	modePopup:   {"INPUT", cRose},
	modeConfirm: {"CONFIRM", cLove},
	modeNotes:   {"NOTES", cFoam},
	modeHelp:    {"HELP", cPine},
}

func (m *Model) statusLine() string {
	mi := modeInfo[m.mode]
	c := mi[1]
	if m.mode == modePopup && m.pop != nil {
		c = m.pop.color
	}
	on := func(fgc, bgc string) lipgloss.Style {
		return lipgloss.NewStyle().Foreground(lipgloss.Color(fgc)).Background(lipgloss.Color(bgc))
	}
	pill := on(cBase, c).Bold(true).Render(" " + mi[0] + " ")
	left := paint(c, ic.capL) + pill
	if l := m.curList(); l != nil {
		open, done := m.counts(l.ID)
		left += on(c, cHlMed).Render(ic.capR) + on(cText, cHlMed).Render(" "+l.Title+" ") + paint(cHlMed, ic.capR) +
			paint(cMuted, fmt.Sprintf(" %d open · %d done", open, done))
	} else {
		left += paint(c, ic.capR)
	}
	if m.pending > 0 {
		left += paint(cGold, fmt.Sprintf("  ↑%d", m.pending))
	}
	if m.filter != "" {
		left += paint(cRose, "  /"+m.filter)
	}

	var right string
	if sc := m.showcmd(); sc != "" {
		right += paint(cGold, sc) + "  "
	}
	right += paint(cMuted, "? help  ")
	n := len(m.rows())
	pos := "0/0"
	pct := "All"
	if n > 0 {
		pos = fmt.Sprintf("%d/%d", m.cursor+1, n)
		h := m.bodyHeight()
		switch total := m.lineCount(); {
		case total <= h:
			pct = "All"
		case m.offset == 0:
			pct = "Top"
		case m.offset >= total-h:
			pct = "Bot"
		default:
			pct = fmt.Sprintf("%d%%", m.offset*100/max(total-h, 1))
		}
	}
	right += paint(cHlMed, ic.capL) + on(cText, cHlMed).Render(" "+pos+" ") + on(c, cHlMed).Render(ic.capL) +
		on(cBase, c).Bold(true).Render(" "+pct+" ") + paint(c, ic.capR)
	return left + strings.Repeat(" ", max(m.w-lipgloss.Width(left)-lipgloss.Width(right), 1)) + right
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
		return paint(cGold, ":") + m.inputView(m.w-2, false, "", now)
	case modeSearch:
		return paint(cRose, "/") + m.inputView(m.w-2, false, "", now)
	case modeInsert:
		hint := "   ⏎ save & next   esc done   ^c cancel"
		switch {
		case m.ed.kind == editRename:
			hint = "   ⏎ save   esc save   ^c cancel"
		case !m.ed.more && m.ed.canSub:
			hint = "   ⏎ add   esc cancel   tab task ⇄ subtask"
		case !m.ed.more:
			hint = "   ⏎ add   esc cancel"
		}
		return fg(cFoam).Bold(true).Render("-- INSERT --") + paint(cMuted, hint)
	case modePopup:
		return paint(cMuted, "⏎ confirm   esc cancel")
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
		return " " + paint(c, string(runes[:n]))
	}
	return ""
}

// ---- splash + help -------------------------------------------------------

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
		if m.toast != nil {
			out = append(out, "", paint(cMuted, ansiTrunc(m.toast.text, m.w-4)))
		}
	} else {
		out = append(out, spin(now)+paint(cSubtle, " fetching your lists"))
	}
	return lipgloss.Place(m.w, m.h, lipgloss.Center, lipgloss.Center, lipgloss.JoinVertical(lipgloss.Center, out...))
}

func (m *Model) help() string {
	type kv [2]string
	sections := []struct {
		name, color string
		keys        []kv
	}{
		{"move", cFoam, []kv{{"j k", "down / up (5j works)"}, {"gg G", "top / bottom"}, {"^d ^u", "half page"}, {"h l", "lists ← tasks → details"}, {"tab", "cycle panes"}, {"H L  gT gt", "prev / next list"}}},
		{"create", cRose, []kv{{"a", "new task"}, {"A", "new subtask"}, {"tab", "(while typing) task ⇄ subtask"}, {"o O", "inline below / above, ⏎ keeps going"}, {"…@tom", "due date while typing"}}},
		{"edit", cIris, []kv{{"x ␣", "toggle done"}, {"r  i  S", "rename / inline / replace"}, {"t T", "set / clear due date"}, {"N", "edit notes inline"}, {"e", "title + notes in $EDITOR"}, {"l ⏎", "details: edit any field"}}},
		{"organize", cGold, []kv{{"dd D", "delete"}, {"yy p P", "yank / paste (moves across lists)"}, {"J K", "move down / up"}, {">> <<", "make / unmake subtask"}, {"za zR zM", "fold"}, {"u", "undo"}}},
		{"lists", cPine, []kv{{"a r dd", "new / rename / delete (lists pane)"}, {"/  esc", "filter / clear"}, {"C  R", "show completed / sync"}, {":q", "quit"}}},
	}
	col := func(idx ...int) string {
		var ls []string
		for _, i := range idx {
			s := sections[i]
			ls = append(ls, fg(s.color).Bold(true).Render(s.name))
			for _, k := range s.keys {
				ls = append(ls, paint(cText, fmt.Sprintf("%-12s", k[0]))+paint(cSubtle, k[1]))
			}
			ls = append(ls, "")
		}
		return strings.Join(ls, "\n")
	}
	content := lipgloss.JoinHorizontal(lipgloss.Top, col(0, 1, 4), "    ", col(2, 3))
	lines := strings.Split(strings.TrimRight(content, "\n "), "\n")
	w := min(lipgloss.Width(content)+4, m.w)
	b := fg(cIris)
	title := wave(" keys ", 0, .1, true)
	out := []string{b.Render("╭─") + title + b.Render(strings.Repeat("─", max(w-3-lipgloss.Width(title), 0))+"╮")}
	for _, l := range lines {
		out = append(out, b.Render("│")+" "+fit(l, w-4)+" "+b.Render("│"))
	}
	out = append(out, b.Render("╰"+strings.Repeat("─", max(w-2, 0))+"╯"))
	return strings.Join(out, "\n")
}
