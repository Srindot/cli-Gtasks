package ui

import (
	"context"
)

type viewReq chan string

func t0() context.Context { return context.Background() }

func init() {
	panicHard = true
	testHook = func(m *Model, msg any) bool {
		if v, ok := msg.(viewReq); ok {
			v <- m.View()
			return true
		}
		return false
	}
}
