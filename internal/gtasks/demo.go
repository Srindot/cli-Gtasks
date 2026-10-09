package gtasks

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Demo is an in-memory Backend with simulated latency, used by `gtask --demo`.
type Demo struct {
	mu    sync.Mutex
	seq   int
	lists []List
	tasks map[string][]Task // ordered; sibling order == slice order
	Delay time.Duration
}

func NewDemo() *Demo {
	d := &Demo{tasks: map[string][]Task{}, Delay: 180 * time.Millisecond}
	day := func(n int) string { return FormatDue(time.Now().AddDate(0, 0, n)) }
	add := func(list string, t Task) string {
		d.seq++
		t.ID = fmt.Sprintf("demo-%d", d.seq)
		if t.Status == "" {
			t.Status = StatusOpen
		}
		if t.Status == StatusDone {
			t.Completed = time.Now().Add(-time.Duration(d.seq) * time.Hour).Format(time.RFC3339)
		}
		d.tasks[list] = append(d.tasks[list], t)
		return t.ID
	}
	d.lists = []List{{ID: "inbox", Title: "Inbox"}, {ID: "work", Title: "Work"}, {ID: "reading", Title: "Reading"}}
	add("inbox", Task{Title: "Water the plants", Due: day(0)})
	trip := add("inbox", Task{Title: "Plan weekend trip", Due: day(3), Notes: "Check train times.\nBook somewhere with a view."})
	add("inbox", Task{Title: "Book train tickets", Parent: trip})
	add("inbox", Task{Title: "Pack the film camera", Parent: trip})
	add("inbox", Task{Title: "Call mom", Due: day(1)})
	add("inbox", Task{Title: "Renew passport", Due: day(-2)})
	add("inbox", Task{Title: "Try the new ramen place"})
	add("inbox", Task{Title: "Buy oat milk", Status: StatusDone})
	add("inbox", Task{Title: "Fix bike brakes", Status: StatusDone})
	add("work", Task{Title: "Review PR #142", Due: day(0)})
	add("work", Task{Title: "Write design doc for sync engine", Due: day(5), Notes: "Cover conflict resolution and offline mode."})
	add("work", Task{Title: "1:1 prep"})
	add("reading", Task{Title: "The Left Hand of Darkness"})
	add("reading", Task{Title: "Designing Data-Intensive Applications", Notes: "Chapters 5–9"})
	return d
}

func (d *Demo) wait(ctx context.Context) error {
	select {
	case <-time.After(d.Delay):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

var errNotFound = errors.New("demo: not found")

func (d *Demo) Lists(ctx context.Context) ([]List, error) {
	if err := d.wait(ctx); err != nil {
		return nil, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]List(nil), d.lists...), nil
}

func (d *Demo) Tasks(ctx context.Context, listID string) ([]Task, error) {
	if err := d.wait(ctx); err != nil {
		return nil, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	out := append([]Task(nil), d.tasks[listID]...)
	for i := range out {
		out[i].Position = fmt.Sprintf("%020d", i)
	}
	return out, nil
}

func (d *Demo) CreateList(ctx context.Context, title string) (List, error) {
	if err := d.wait(ctx); err != nil {
		return List{}, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.seq++
	l := List{ID: fmt.Sprintf("demo-list-%d", d.seq), Title: title}
	d.lists = append(d.lists, l)
	return l, nil
}

func (d *Demo) RenameList(ctx context.Context, listID, title string) (List, error) {
	if err := d.wait(ctx); err != nil {
		return List{}, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for i := range d.lists {
		if d.lists[i].ID == listID {
			d.lists[i].Title = title
			return d.lists[i], nil
		}
	}
	return List{}, errNotFound
}

func (d *Demo) DeleteList(ctx context.Context, listID string) error {
	if err := d.wait(ctx); err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for i := range d.lists {
		if d.lists[i].ID == listID {
			d.lists = append(d.lists[:i], d.lists[i+1:]...)
			delete(d.tasks, listID)
			return nil
		}
	}
	return errNotFound
}

func (d *Demo) index(listID, id string) int {
	for i, t := range d.tasks[listID] {
		if t.ID == id {
			return i
		}
	}
	return -1
}

// place inserts t so that it follows previous, or leads its parent's children.
func (d *Demo) place(listID string, t Task, parent, previous string) {
	ts := d.tasks[listID]
	at := len(ts)
	if previous != "" {
		if i := d.index(listID, previous); i >= 0 {
			at = i + 1
		}
	} else {
		at = -1
		for i, o := range ts {
			if o.Parent == parent {
				at = i
				break
			}
		}
		if at < 0 {
			at = len(ts)
			if i := d.index(listID, parent); i >= 0 {
				at = i + 1
			}
		}
	}
	t.Parent = parent
	ts = append(ts, Task{})
	copy(ts[at+1:], ts[at:])
	ts[at] = t
	d.tasks[listID] = ts
}

func (d *Demo) Insert(ctx context.Context, listID string, t Task, parent, previous string) (Task, error) {
	if err := d.wait(ctx); err != nil {
		return Task{}, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.seq++
	t.ID = fmt.Sprintf("demo-%d", d.seq)
	if t.Status == "" {
		t.Status = StatusOpen
	}
	t.Updated = time.Now().Format(time.RFC3339)
	d.place(listID, t, parent, previous)
	return t, nil
}

func (d *Demo) Patch(ctx context.Context, listID, taskID string, p map[string]any) (Task, error) {
	if err := d.wait(ctx); err != nil {
		return Task{}, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	i := d.index(listID, taskID)
	if i < 0 {
		return Task{}, errNotFound
	}
	t := &d.tasks[listID][i]
	str := func(v any) string { s, _ := v.(string); return s }
	for k, v := range p {
		switch k {
		case "title":
			t.Title = str(v)
		case "notes":
			t.Notes = str(v)
		case "due":
			t.Due = str(v)
		case "status":
			t.Status = str(v)
			if t.Status == StatusDone {
				t.Completed = time.Now().Format(time.RFC3339)
			} else {
				t.Completed = ""
			}
		}
	}
	t.Updated = time.Now().Format(time.RFC3339)
	return *t, nil
}

func (d *Demo) Delete(ctx context.Context, listID, taskID string) error {
	if err := d.wait(ctx); err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	var keep []Task
	for _, t := range d.tasks[listID] {
		if t.ID != taskID && t.Parent != taskID {
			keep = append(keep, t)
		}
	}
	d.tasks[listID] = keep
	return nil
}

func (d *Demo) Move(ctx context.Context, listID, taskID, parent, previous string) (Task, error) {
	if err := d.wait(ctx); err != nil {
		return Task{}, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	i := d.index(listID, taskID)
	if i < 0 {
		return Task{}, errNotFound
	}
	t := d.tasks[listID][i]
	d.tasks[listID] = append(d.tasks[listID][:i], d.tasks[listID][i+1:]...)
	d.place(listID, t, parent, previous)
	return t, nil
}

func (d *Demo) Clear(ctx context.Context, listID string) error {
	if err := d.wait(ctx); err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for i := range d.tasks[listID] {
		if d.tasks[listID][i].Done() {
			d.tasks[listID][i].Hidden = true // like Google: cleared tasks stay, hidden
		}
	}
	return nil
}
