package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

var weekdays = map[string]time.Weekday{
	"sun": time.Sunday, "mon": time.Monday, "tue": time.Tuesday, "wed": time.Wednesday,
	"thu": time.Thursday, "fri": time.Friday, "sat": time.Saturday,
}

func today() time.Time {
	n := time.Now()
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.Local)
}

// ParseDue understands: today, tom(orrow), yesterday, mon..sun, next week,
// +3 / +3d / +2w / +1m, 2026-10-12, 10/12, oct 12, 12 oct, or a bare day-of-month.
// ok=false means unparseable; a zero time with ok=true means "clear".
func ParseDue(s string) (time.Time, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	t := today()
	switch s {
	case "", "none", "no", "-", "clear":
		return time.Time{}, true
	case "today", "tod", "now", "td":
		return t, true
	case "tomorrow", "tom", "tmr", "tm":
		return t.AddDate(0, 0, 1), true
	case "yesterday", "yd":
		return t.AddDate(0, 0, -1), true
	case "next week", "nw":
		return t.AddDate(0, 0, 7), true
	case "weekend", "we":
		s = "sat"
	}
	if strings.HasPrefix(s, "+") {
		n, unit := s[1:], "d"
		if n == "" {
			return time.Time{}, false
		}
		if l := n[len(n)-1:]; l == "d" || l == "w" || l == "m" || l == "y" {
			n, unit = n[:len(n)-1], l
		}
		v, err := strconv.Atoi(n)
		if err != nil {
			return time.Time{}, false
		}
		switch unit {
		case "w":
			return t.AddDate(0, 0, 7*v), true
		case "m":
			return t.AddDate(0, v, 0), true
		case "y":
			return t.AddDate(v, 0, 0), true
		}
		return t.AddDate(0, 0, v), true
	}
	if len(s) >= 3 {
		if wd, ok := weekdays[strings.TrimPrefix(s, "next ")[:3]]; ok && !strings.ContainsAny(s, "0123456789") {
			d := (int(wd) - int(t.Weekday()) + 7) % 7
			if d == 0 {
				d = 7
			}
			return t.AddDate(0, 0, d), true
		}
	}
	for _, layout := range []string{"2006-01-02", "2006/01/02", "2006-1-2", "02/01/2006"} {
		if d, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return d, true
		}
	}
	// Year-less dates roll forward to the next occurrence.
	for _, layout := range []string{"01-02", "1/2", "Jan 2", "January 2", "2 Jan", "2 January", "Jan2", "2Jan"} {
		if d, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			d = time.Date(t.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.Local)
			if d.Before(t) {
				d = d.AddDate(1, 0, 0)
			}
			return d, true
		}
	}
	if v, err := strconv.Atoi(s); err == nil && v >= 1 && v <= 31 {
		d := time.Date(t.Year(), t.Month(), v, 0, 0, 0, 0, time.Local)
		if d.Before(t) {
			d = time.Date(t.Year(), t.Month()+1, v, 0, 0, 0, 0, time.Local)
		}
		return d, true
	}
	return time.Time{}, false
}

// SplitQuickDue pulls a trailing "@tomorrow"-style due date out of a title.
func SplitQuickDue(title string) (string, time.Time, bool) {
	i := strings.LastIndex(title, " @")
	if i < 0 {
		return title, time.Time{}, false
	}
	d, ok := ParseDue(title[i+2:])
	if !ok || d.IsZero() {
		return title, time.Time{}, false
	}
	return strings.TrimSpace(title[:i]), d, true
}

// HumanDue renders a due date relative to today along with its accent color.
func HumanDue(d time.Time) (string, string) {
	days := int(d.Sub(today()).Hours() / 24)
	switch {
	case days < -1:
		return fmt.Sprintf("%dd overdue", -days), cLove
	case days == -1:
		return "yesterday", cLove
	case days == 0:
		return "today", cGold
	case days == 1:
		return "tomorrow", cFoam
	case days < 7:
		return d.Format("Mon"), cIris
	case d.Year() == today().Year():
		return d.Format("Jan 2"), cSubtle
	}
	return d.Format("Jan 2 2006"), cSubtle
}

func ago(ts string) string {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
	return t.Local().Format("Jan 2 2006")
}

// relDays phrases the distance to d: "today", "in 3 days", "2 days ago".
func relDays(d time.Time) string {
	n := int(d.Sub(today()).Hours() / 24)
	switch {
	case n == 0:
		return "today"
	case n == 1:
		return "tomorrow"
	case n == -1:
		return "yesterday"
	case n > 1:
		return fmt.Sprintf("in %d days", n)
	}
	return fmt.Sprintf("%d days ago", -n)
}
