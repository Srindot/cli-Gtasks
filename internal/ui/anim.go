package ui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type animKind int

const (
	animComplete animKind = iota // checkbox fills, strike sweeps across
	animDelete                   // row dissolves right-to-left in red
	animFlash                    // gold glow that fades back to normal
)

type anim struct {
	kind  animKind
	start time.Time
	dur   time.Duration
	done  func(m *Model)
}

func (a *anim) progress(now time.Time) float64 {
	return clamp(float64(now.Sub(a.start)) / float64(a.dur))
}

const (
	fps        = 60
	trailLife  = 220 * time.Millisecond
	focusLife  = 260 * time.Millisecond
	introLife  = 1600 * time.Millisecond
	toastLife  = 3200 * time.Millisecond
	toastFade  = 700 * time.Millisecond
	toastSlide = 180 * time.Millisecond
)

func tick() tea.Cmd {
	return tea.Tick(time.Second/fps, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m *Model) startAnim(id string, k animKind, d time.Duration, done func(*Model)) {
	if a := m.anims[id]; a != nil {
		if k == animFlash && a.kind != animFlash {
			return // never let a flash interrupt something meaningful
		}
		if a.done != nil {
			a.done(m)
		}
	}
	m.anims[id] = &anim{kind: k, start: time.Now(), dur: d, done: done}
}

func (m *Model) step(now time.Time) {
	for id, a := range m.anims {
		if a.progress(now) >= 1 {
			delete(m.anims, id)
			if a.done != nil {
				a.done(m)
			}
		}
	}
	for id, t := range m.trail {
		if now.Sub(t) > trailLife {
			delete(m.trail, id)
		}
	}
	if m.toast != nil && now.Sub(m.toast.at) > toastLife {
		m.toast = nil
	}
}

// animating decides whether the 60fps ticker keeps running; when nothing
// moves the app is completely idle.
func (m *Model) animating() bool {
	now := time.Now()
	return len(m.anims) > 0 || len(m.trail) > 0 || m.toast != nil ||
		now.Sub(m.focusAt) < focusLife || now.Sub(m.listAt) < focusLife || len(m.lists) == 0 || now.Sub(m.started) < introLife ||
		m.pending+m.fetching > 0
}

func sprintf(format string, a ...any) string {
	if len(a) == 0 {
		return format
	}
	return fmt.Sprintf(format, a...)
}
