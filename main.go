// gtask — a fast, vim-flavoured Google Tasks client for the terminal.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/srindot/gtask/internal/gtasks"
	"github.com/srindot/gtask/internal/ui"
)

var (
	rose   = lipgloss.NewStyle().Foreground(lipgloss.Color("#ebbcba"))
	iris   = lipgloss.NewStyle().Foreground(lipgloss.Color("#c4a7e7"))
	foam   = lipgloss.NewStyle().Foreground(lipgloss.Color("#9ccfd8"))
	pine   = lipgloss.NewStyle().Foreground(lipgloss.Color("#31748f"))
	gold   = lipgloss.NewStyle().Foreground(lipgloss.Color("#f6c177"))
	love   = lipgloss.NewStyle().Foreground(lipgloss.Color("#eb6f92"))
	muted  = lipgloss.NewStyle().Foreground(lipgloss.Color("#6e6a86"))
	subtle = lipgloss.NewStyle().Foreground(lipgloss.Color("#908caa"))
	text   = lipgloss.NewStyle().Foreground(lipgloss.Color("#e0def4"))
)

func dir(env, fallback string) string {
	if d := os.Getenv(env); d != "" {
		return filepath.Join(d, "gtask")
	}
	h, _ := os.UserHomeDir()
	return filepath.Join(h, fallback, "gtask")
}

var (
	configDir = dir("XDG_CONFIG_HOME", ".config")
	cacheDir  = dir("XDG_CACHE_HOME", ".cache")
)

const usage = `gtask — Google Tasks in your terminal

usage
  gtask                       open the TUI
  gtask --demo                try the TUI with sample data (no account needed)
  gtask auth                  connect your Google account
  gtask ls [list]             print open tasks
  gtask lists                 print your task lists
  gtask add [-l list] text    add a task  (end with @tomorrow, @fri, @+3d … for a due date)
  gtask logout                forget the stored token

press ? inside the TUI for keys`

func main() {
	demo := flag.Bool("demo", false, "use sample data")
	flag.Usage = func() { fmt.Fprintln(os.Stderr, usage) }
	flag.Parse()
	args := flag.Args()

	if *demo {
		run(gtasks.NewDemo(), "", true)
		return
	}
	cmd := ""
	if len(args) > 0 {
		cmd, args = args[0], args[1:]
	}
	switch cmd {
	case "auth", "login":
		must(login())
		return
	case "logout":
		os.Remove(filepath.Join(configDir, "token.json"))
		os.Remove(filepath.Join(cacheDir, "cache.json"))
		fmt.Println(muted.Render("logged out"))
		return
	case "help", "-h", "--help":
		fmt.Println(usage)
		return
	}

	be, err := backend()
	if err != nil {
		explain(err)
		os.Exit(1)
	}
	switch cmd {
	case "":
		run(be, filepath.Join(cacheDir, "cache.json"), false)
	case "ls", "list":
		must(cmdLs(be, strings.Join(args, " ")))
	case "lists":
		must(cmdLists(be))
	case "add", "a":
		must(cmdAdd(be, args))
	default:
		fmt.Fprintln(os.Stderr, love.Render("unknown command: "+cmd)+"\n\n"+usage)
		os.Exit(2)
	}
}

func run(be gtasks.Backend, cache string, demo bool) {
	m := ui.New(ui.Options{Backend: be, CachePath: cache, Demo: demo})
	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		must(err)
	}
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, love.Render("✗ ")+err.Error())
		os.Exit(1)
	}
}

func backend() (gtasks.Backend, error) {
	hc, err := gtasks.HTTPClient(context.Background(), configDir)
	if err != nil {
		return nil, err
	}
	return gtasks.NewClient(hc), nil
}

func openBrowser(url string) {
	name := "xdg-open"
	switch runtime.GOOS {
	case "darwin":
		name = "open"
	case "windows":
		name = "explorer"
	}
	_ = exec.Command(name, url).Start()
}

func login() error {
	if _, err := gtasks.LoadConfig(configDir); err != nil {
		explain(err)
		os.Exit(1)
	}
	fmt.Println(rose.Render("✿ ") + text.Render("opening your browser to sign in to Google…"))
	err := gtasks.Login(context.Background(), configDir, func(url string) {
		openBrowser(url)
		fmt.Println(muted.Render("  if nothing opened, visit:\n  ") + subtle.Render(url))
	})
	if err != nil {
		return err
	}
	fmt.Println(pine.Render("✓ ") + text.Render("connected. run ") + rose.Render("gtask") + text.Render(" to begin."))
	return nil
}

func explain(err error) {
	switch {
	case errors.Is(err, gtasks.ErrNoCredentials):
		step := func(n, s string) string { return iris.Render("  "+n+" ") + text.Render(s) }
		fmt.Println(rose.Render("✿ gtask needs a Google OAuth client (one-time, ~3 minutes)") + "\n")
		fmt.Println(step("1", "open ") + foam.Render("https://console.cloud.google.com/projectcreate") + text.Render(" and create a project"))
		fmt.Println(step("2", "enable the Tasks API: ") + foam.Render("https://console.cloud.google.com/apis/library/tasks.googleapis.com"))
		fmt.Println(step("3", "Google Auth Platform → Branding: fill in an app name + your email"))
		fmt.Println(step("4", "Audience: External, then add yourself as a test user ") + muted.Render("(or Publish the app so tokens don't expire after 7 days)"))
		fmt.Println(step("5", "Clients → Create client → ") + gold.Render("Desktop app") + text.Render(" → download the JSON"))
		fmt.Println(step("6", "save it as ") + gold.Render(gtasks.CredentialsPath(configDir)))
		fmt.Println(step("7", "run ") + rose.Render("gtask auth"))
		fmt.Println("\n" + muted.Render("  want a look first? ") + rose.Render("gtask --demo"))
	case errors.Is(err, gtasks.ErrNoToken):
		fmt.Println(rose.Render("✿ ") + text.Render("not signed in yet — run ") + rose.Render("gtask auth"))
	default:
		fmt.Fprintln(os.Stderr, love.Render("✗ ")+err.Error())
	}
}

func ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 30*time.Second)
}

func pickList(c context.Context, be gtasks.Backend, name string) (gtasks.List, error) {
	ls, err := be.Lists(c)
	if err != nil {
		return gtasks.List{}, err
	}
	if len(ls) == 0 {
		return gtasks.List{}, errors.New("you have no task lists")
	}
	if name == "" {
		return ls[0], nil
	}
	for _, l := range ls {
		if strings.EqualFold(l.Title, name) {
			return l, nil
		}
	}
	for _, l := range ls {
		if strings.HasPrefix(strings.ToLower(l.Title), strings.ToLower(name)) {
			return l, nil
		}
	}
	return gtasks.List{}, fmt.Errorf("no list called %q", name)
}

func cmdLists(be gtasks.Backend) error {
	c, cancel := ctx()
	defer cancel()
	ls, err := be.Lists(c)
	if err != nil {
		return err
	}
	for _, l := range ls {
		fmt.Println(iris.Render("▌ ") + text.Render(l.Title))
	}
	return nil
}

func cmdLs(be gtasks.Backend, name string) error {
	c, cancel := ctx()
	defer cancel()
	l, err := pickList(c, be, name)
	if err != nil {
		return err
	}
	ts, err := be.Tasks(c, l.ID)
	if err != nil {
		return err
	}
	fmt.Println(rose.Bold(true).Render(l.Title))
	open := map[string]bool{}
	for _, t := range ts {
		if !t.Done() {
			open[t.ID] = true
		}
	}
	print := func(t gtasks.Task, indent string) {
		line := indent + subtle.Render("○ ") + text.Render(t.Title)
		if d, ok := t.DueDate(); ok {
			h, col := ui.HumanDue(d)
			line += "  " + lipgloss.NewStyle().Foreground(lipgloss.Color(col)).Render(h)
		}
		fmt.Println(line)
	}
	sortByPos(ts)
	n := 0
	for _, t := range ts {
		if t.Done() || open[t.Parent] {
			continue
		}
		print(t, "  ")
		n++
		for _, k := range ts {
			if k.Parent == t.ID && !k.Done() {
				print(k, "  "+muted.Render("│ "))
			}
		}
	}
	if n == 0 {
		fmt.Println(muted.Render("  ✿ all clear"))
	}
	return nil
}

func sortByPos(ts []gtasks.Task) {
	for i := 1; i < len(ts); i++ {
		for j := i; j > 0 && ts[j].Position < ts[j-1].Position; j-- {
			ts[j], ts[j-1] = ts[j-1], ts[j]
		}
	}
}

func cmdAdd(be gtasks.Backend, args []string) error {
	fs := flag.NewFlagSet("add", flag.ExitOnError)
	list := fs.String("l", "", "list name")
	due := fs.String("d", "", "due date")
	notes := fs.String("n", "", "notes")
	fs.Parse(args)
	title := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if title == "" {
		return errors.New("usage: gtask add [-l list] [-d due] [-n notes] title")
	}
	t := gtasks.Task{Notes: *notes}
	if *due != "" {
		d, ok := ui.ParseDue(*due)
		if !ok {
			return fmt.Errorf("can't read %q as a date", *due)
		}
		if !d.IsZero() {
			t.Due = gtasks.FormatDue(d)
		}
	}
	if tt, d, ok := ui.SplitQuickDue(title); ok {
		title, t.Due = tt, gtasks.FormatDue(d)
	}
	t.Title = title
	c, cancel := ctx()
	defer cancel()
	l, err := pickList(c, be, *list)
	if err != nil {
		return err
	}
	if _, err := be.Insert(c, l.ID, t, "", ""); err != nil {
		return err
	}
	msg := pine.Render("✓ ") + text.Render(title) + muted.Render(" → "+l.Title)
	if d, ok := t.DueDate(); ok {
		h, col := ui.HumanDue(d)
		msg += muted.Render(" · ") + lipgloss.NewStyle().Foreground(lipgloss.Color(col)).Render(h)
	}
	fmt.Println(msg)
	return nil
}
