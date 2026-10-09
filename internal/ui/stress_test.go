package ui

import (
	"math/rand"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// TestStress mashes random keys and resizes; every frame must fill the
// terminal exactly (a taller or wider frame scrolls the screen away).
func TestStress(t *testing.T) {
	seed := time.Now().UnixNano()
	t.Logf("seed %d", seed)
	r := rand.New(rand.NewSource(seed))
	h := start(t, 120, 30)
	keys := []string{"j", "k", "h", "l", "j", "k", "x", "u", "a", "A", "o", "O", "enter", "esc", "esc", "tab",
		"D", "y", "n", "dd", "p", "P", "J", "K", ">", "<", "za", "zR", "zM", "G", "gg", "gt", "gT", "H", "L",
		"t", "T", "N", "r", "i", "C", "/", ":", "?", "q", "space", "@", "+", "1", "Y", "S", "c", "e"}
	sizes := [][2]int{{120, 30}, {80, 20}, {50, 12}, {200, 50}, {30, 6}, {96, 24}, {10, 3}, {64, 8}, {1, 1}}
	for i := 0; i < 1500; i++ {
		if r.Intn(40) == 0 {
			s := sizes[r.Intn(len(sizes))]
			h.p.Send(tea.WindowSizeMsg{Width: s[0], Height: s[1]})
		}
		k := keys[r.Intn(len(keys))]
		if k == "q" || k == "e" { // quitting / spawning an editor ends the run
			continue
		}
		for _, c := range strings.Split(k, "") {
			if k == "enter" || k == "esc" || k == "tab" || k == "space" {
				c = k
			}
			h.keys(c)
			if k == "enter" || k == "esc" || k == "tab" || k == "space" {
				break
			}
		}
		if i%10 == 0 {
			v := h.view("step")
			if n := len(strings.Split(v, "\n")); h.m.h > 0 && n != h.m.h {
				t.Fatalf("step %d (%dx%d) mode %d lists %d: frame has %d lines, want %d\n%s", i, h.m.w, h.m.h, h.m.mode, len(h.m.lists), n, h.m.h, v)
			}
			if t.Failed() {
				t.FailNow()
			}
		}
	}
}
