// Package gtasks is a small Google Tasks REST client plus an in-memory demo backend.
package gtasks

import (
	"context"
	"time"
)

type List struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Updated string `json:"updated,omitempty"`
}

type Task struct {
	ID        string `json:"id,omitempty"`
	Title     string `json:"title"`
	Notes     string `json:"notes,omitempty"`
	Status    string `json:"status,omitempty"` // "needsAction" | "completed"
	Due       string `json:"due,omitempty"`    // RFC3339, only the date part is meaningful
	Completed string `json:"completed,omitempty"`
	Parent    string `json:"parent,omitempty"`
	Position  string `json:"position,omitempty"`
	Updated   string `json:"updated,omitempty"`
	Deleted   bool   `json:"deleted,omitempty"`
	Hidden    bool   `json:"hidden,omitempty"`
}

const (
	StatusOpen = "needsAction"
	StatusDone = "completed"
)

func (t *Task) Done() bool { return t.Status == StatusDone }

func (t *Task) DueDate() (time.Time, bool) {
	if t.Due == "" {
		return time.Time{}, false
	}
	d, err := time.Parse(time.RFC3339, t.Due)
	if err != nil {
		return time.Time{}, false
	}
	// Google stores due dates as midnight UTC; reinterpret as a local calendar date.
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.Local), true
}

// FormatDue converts a local calendar date into the wire format Google expects.
func FormatDue(d time.Time) string {
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC).Format("2006-01-02T15:04:05.000Z")
}

// Backend is everything the UI needs from a task store.
type Backend interface {
	Lists(ctx context.Context) ([]List, error)
	Tasks(ctx context.Context, listID string) ([]Task, error)
	CreateList(ctx context.Context, title string) (List, error)
	RenameList(ctx context.Context, listID, title string) (List, error)
	DeleteList(ctx context.Context, listID string) error
	Insert(ctx context.Context, listID string, t Task, parent, previous string) (Task, error)
	// Patch sends a partial update; a nil value clears the field.
	Patch(ctx context.Context, listID, taskID string, patch map[string]any) (Task, error)
	Delete(ctx context.Context, listID, taskID string) error
	Move(ctx context.Context, listID, taskID, parent, previous string) (Task, error)
	Clear(ctx context.Context, listID string) error
}
