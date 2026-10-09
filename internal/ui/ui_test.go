package ui

import (
	"io"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Srindot/cli-Gtasks/internal/gtasks"
)

type harness struct {
	t *testing.T
	m *Model
	p *tea.Program
	d *gtasks.Demo
}

func start(t *testing.T, w, h int) *harness {
	d := gtasks.NewDemo()
	d.Delay = 5 * time.Millisecond
	m := New(Options{Backend: d, Demo: true})
	p := tea.NewProgram(m, tea.WithInput(nil), tea.WithOutput(io.Discard), tea.WithoutRenderer())
	go p.Run()
	t.Cleanup(p.Kill)
	p.Send(tea.WindowSizeMsg{Width: w, Height: h})
	hs := &harness{t, m, p, d}
	hs.settle()
	return hs
}

// settle waits for animations and sync to finish.
func (h *harness) settle() { time.Sleep(900 * time.Millisecond) }

var keyDelay = 15 * time.Millisecond

func (h *harness) keys(ks ...string) {
	for _, k := range ks {
		var msg tea.KeyMsg
		switch k {
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case "tab":
			msg = tea.KeyMsg{Type: tea.KeyTab}
		case "space":
			msg = tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		h.p.Send(msg)
		time.Sleep(keyDelay)
	}
}

func (h *harness) typ(s string) {
	for _, r := range s {
		h.keys(string(r))
	}
}

func (h *harness) view(label string) string {
	v := make(chan string)
	h.p.Send(viewReq(v))
	var s string
	select {
	case s = <-v:
	case <-time.After(3 * time.Second):
		h.t.Fatalf("%s: program stopped responding (panic?)", label)
	}
	for i, l := range strings.Split(s, "\n") {
		if w := lipgloss.Width(l); w != h.m.w {
			h.t.Errorf("%s: line %d is %d wide, want %d: %q", label, i, w, h.m.w, l)
		}
	}
	if os.Getenv("SHOW") != "" {
		h.t.Logf("── %s ──\n%s", label, s)
	}
	return s
}

func (h *harness) serverTitles(list string) []string {
	ts, _ := h.d.Tasks(t0(), list)
	var out []string
	for _, t := range ts {
		s := t.Title
		if t.Parent != "" {
			s = "  " + s
		}
		if t.Done() {
			s += " ✓"
		}
		out = append(out, s)
	}
	return out
}

func TestFlow(t *testing.T) {
	h := start(t, 130, 24)
	v := h.view("startup")
	if !strings.Contains(v, "Water the plants") || !strings.Contains(v, "Inbox") {
		t.Fatalf("tasks not rendered:\n%s", v)
	}

	// Add two tasks in a row with o + Enter, the second with a quick due date.
	h.keys("o")
	h.typ("first new task")
	h.keys("enter")
	h.typ("second @tom")
	h.keys("esc")
	h.settle()
	h.view("after add")
	got := strings.Join(h.serverTitles("inbox"), "\n")
	if !strings.Contains(got, "Water the plants\nfirst new task\nsecond") {
		t.Fatalf("server order wrong:\n%s", got)
	}

	// Complete it (x), check the strike animation mid-way.
	h.keys("x")
	time.Sleep(200 * time.Millisecond)
	h.view("mid-complete animation")
	h.settle()
	if !strings.Contains(strings.Join(h.serverTitles("inbox"), "\n"), "second ✓") {
		t.Fatalf("not completed on server:\n%s", strings.Join(h.serverTitles("inbox"), "\n"))
	}
	// Undo brings it back.
	h.keys("u")
	h.settle()
	if strings.Contains(strings.Join(h.serverTitles("inbox"), "\n"), "second ✓") {
		t.Fatal("undo did not reopen task")
	}

	// Indent "second" under "first new task", then outdent.
	h.keys(">", ">")
	h.settle()
	h.view("after indent")
	if !strings.Contains(strings.Join(h.serverTitles("inbox"), "\n"), "first new task\n  second") {
		t.Fatalf("indent failed:\n%s", strings.Join(h.serverTitles("inbox"), "\n"))
	}
	h.keys("<", "<")
	h.settle()

	// Delete, then paste at the top; also check J moves.
	h.keys("d", "d")
	time.Sleep(150 * time.Millisecond)
	h.view("mid-delete animation")
	h.settle()
	h.keys("g", "g", "P")
	h.settle()
	h.view("after dd + ggP")
	got = strings.Join(h.serverTitles("inbox"), "\n")
	if !strings.HasPrefix(got, "second") {
		t.Fatalf("paste at top failed:\n%s", got)
	}
	h.keys("J")
	h.settle()
	if got = strings.Join(h.serverTitles("inbox"), "\n"); !strings.HasPrefix(got, "Water the plants\nsecond") {
		t.Fatalf("J failed:\n%s", got)
	}

	// Rename with cc.
	h.keys("c", "c")
	h.typ("renamed!")
	h.keys("esc")
	h.settle()
	if got = strings.Join(h.serverTitles("inbox"), "\n"); !strings.Contains(got, "renamed!") {
		t.Fatalf("rename failed:\n%s", got)
	}

	// Due date prompt.
	h.keys("t")
	h.p.Send(tea.KeyMsg{Type: tea.KeyCtrlU})
	h.typ("+2d")
	h.view("due popup")
	h.keys("enter")
	h.settle()
	if ts, _ := h.d.Tasks(t0(), "inbox"); ts[1].Title != "renamed!" || ts[1].Due != gtasks.FormatDue(today().AddDate(0, 0, 2)) {
		t.Fatalf("due not set: %+v", ts[1])
	}

	// Search.
	h.keys("/")
	h.typ("plan")
	h.view("searching")
	h.keys("enter", "esc")

	// Lists: switch with L, create one with :new.
	h.keys("L")
	h.settle()
	if v = h.view("work list"); !strings.Contains(v, "Review PR #142") {
		t.Fatalf("L did not switch list:\n%s", v)
	}
	h.keys(":")
	h.typ("new Groceries")
	h.keys("enter", "o")
	h.typ("eggs")
	h.keys("esc")
	h.settle()
	h.view("new list")
	ls, _ := h.d.Lists(t0())
	if ls[len(ls)-1].Title != "Groceries" {
		t.Fatal("list not created")
	}
	if got := h.serverTitles(ls[len(ls)-1].ID); len(got) != 1 || got[0] != "eggs" {
		t.Fatalf("task in new list: %v", got)
	}

	// Folding and completed section.
	h.keys("g", "t", "C")
	h.settle()
	h.view("inbox with completed")
	h.keys(":")
	h.typ("clear")
	h.keys("enter", "R")
	h.settle()
	if v := h.view("after clear + resync"); strings.Contains(v, "Buy oat milk") {
		t.Fatalf("cleared task came back:\n%s", v)
	}
	h.keys("?")
	h.view("help")
	h.keys("esc")
}

func TestNarrow(t *testing.T) {
	h := start(t, 60, 16)
	h.view("narrow")
	h.keys("h")
	h.view("narrow lists")
}

func TestParseDue(t *testing.T) {
	for _, s := range []string{"+", "+d", "@", ""} {
		ParseDue(s) // partial input from the live preview must not panic
		SplitQuickDue("x @" + s)
	}
	for _, s := range []string{"today", "tom", "fri", "+3d", "+2w", "oct 12", "2026-10-12", "12", "10/12", "next mon"} {
		if d, ok := ParseDue(s); !ok || d.IsZero() {
			t.Errorf("ParseDue(%q) failed", s)
		}
	}
	if _, ok := ParseDue("blah"); ok {
		t.Error("blah parsed")
	}
	if title, _, ok := SplitQuickDue("buy milk @tom"); !ok || title != "buy milk" {
		t.Errorf("quick due: %q %v", title, ok)
	}
}

func TestCreateAndDetails(t *testing.T) {
	h := start(t, 140, 30)
	// a: new top-level task after the cursor, with a quick due date.
	h.keys("a")
	h.typ("Pay rent @fri")
	h.view("popup new task")
	h.keys("enter")
	h.settle()
	ts, _ := h.d.Tasks(t0(), "inbox")
	if ts[1].Title != "Pay rent" || ts[1].Due == "" || ts[1].Parent != "" {
		t.Fatalf("a did not create a dated top-level task: %+v", ts[1])
	}

	// A: subtask under the selected task.
	h.keys("A")
	h.typ("Set up autopay")
	h.view("popup new subtask")
	h.keys("enter")
	h.settle()
	ts, _ = h.d.Tasks(t0(), "inbox")
	if ts[2].Title != "Set up autopay" || ts[2].Parent != ts[1].ID {
		t.Fatalf("A did not create a subtask: %+v", ts[2])
	}

	// a then tab flips the popup to subtask mode (anchor is now the subtask → same parent).
	h.keys("a")
	h.keys("tab")
	h.typ("Email landlord")
	h.view("popup toggled to subtask")
	h.keys("enter")
	h.settle()
	ts, _ = h.d.Tasks(t0(), "inbox")
	if ts[3].Title != "Email landlord" || ts[3].Parent != ts[1].ID {
		t.Fatalf("tab toggle failed: %+v", ts[3])
	}

	// Details pane: l focuses it, j to the due field, enter edits it.
	h.keys("g", "g", "j", "l")
	h.view("details focused")
	h.keys("j", "enter")
	h.p.Send(tea.KeyMsg{Type: tea.KeyCtrlU})
	h.typ("+3d")
	h.view("due popup in details")
	h.keys("enter")
	h.settle()
	ts, _ = h.d.Tasks(t0(), "inbox")
	if ts[1].Due != gtasks.FormatDue(today().AddDate(0, 0, 3)) {
		t.Fatalf("due edit from details failed: %+v", ts[1])
	}

	// Notes inline editor.
	h.keys("j", "j", "enter")
	h.typ("first line")
	h.p.Send(tea.KeyMsg{Type: tea.KeyEnter})
	h.typ("second line")
	h.view("editing notes")
	h.keys("esc")
	h.settle()
	ts, _ = h.d.Tasks(t0(), "inbox")
	if ts[1].Notes != "first line\nsecond line" {
		t.Fatalf("notes not saved: %q", ts[1].Notes)
	}

	// Subtask rows in details can be toggled with x.
	h.keys("j", "x")
	h.settle()
	h.view("details after toggling subtask")
	ts, _ = h.d.Tasks(t0(), "inbox")
	if !ts[2].Done() {
		t.Fatalf("subtask not toggled from details: %+v", ts[2])
	}
}

// The input card opens where the task will land and pushes rows down instead
// of covering them; esc leaves the list untouched.
func TestCardPushesRows(t *testing.T) {
	h := start(t, 120, 30)
	before := h.serverTitles("inbox")
	for _, k := range []string{"a", "A", "t"} {
		h.keys("g", "g", "j", k)
		h.settle()
		v := h.view("card " + k)
		for _, title := range []string{"Water the plants", "Plan weekend trip", "Book train tickets", "Call mom", "Renew passport", "Try the new ramen place"} {
			if !strings.Contains(v, title) {
				t.Errorf("%s: card hid %q:\n%s", k, title, v)
			}
		}
		if !strings.Contains(v, "╭─") || !strings.Contains(v, "╯") {
			t.Errorf("%s: no card drawn:\n%s", k, v)
		}
		h.keys("esc")
	}
	h.settle()
	if got := h.serverTitles("inbox"); strings.Join(got, "|") != strings.Join(before, "|") {
		t.Fatalf("esc changed tasks: %v -> %v", before, got)
	}
}
