package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"
)

// panicHard makes tests see panics instead of a toast.
var panicHard bool

// guard turns a panic into a logged error so a bug never tears the UI down
// and drops you back at the shell. Stack traces go next to the cache.
func (m *Model) guard(where string) {
	r := recover()
	if r == nil {
		return
	}
	if panicHard {
		panic(r)
	}
	if m.cachePath != "" {
		p := filepath.Join(filepath.Dir(m.cachePath), "crash.log")
		if f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); err == nil {
			fmt.Fprintf(f, "%s %s: %v\n%s\n", time.Now().Format(time.RFC3339), where, r, debug.Stack())
			f.Close()
		}
	}
	m.mode = modeNormal
	m.pop = nil
	m.say(cLove, "✗ internal error (%v) — logged to crash.log", r)
}
