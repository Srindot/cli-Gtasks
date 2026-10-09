package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/srindot/gtask/internal/gtasks"
)

// The Cloud console is a maze; these links land directly on the page each step needs.
type setupStep struct {
	title, url string
	do         []string
}

var setupSteps = []setupStep{
	{"Create a project", "https://console.cloud.google.com/projectcreate", []string{
		"Project name: type " + "gtask" + " (anything works)",
		"leave Location as it is → click Create",
		"wait ~10 seconds until the notification says it's done",
	}},
	{"Turn on the Tasks API", "https://console.cloud.google.com/apis/library/tasks.googleapis.com", []string{
		"check the project picker at the top-left says gtask (switch to it if not)",
		"click the blue Enable button",
	}},
	{"Set up the sign-in screen", "https://console.cloud.google.com/auth/overview", []string{
		"click Get started",
		"App information: name gtask, support email = you → Next",
		"Audience: External → Next",
		"Contact information: your email → Next",
		"tick the agreement → Continue → Create",
	}},
	{"Let yourself sign in", "https://console.cloud.google.com/auth/audience", []string{
		"under Publishing status click Publish app → Confirm",
		"if it says to complete Branding: open https://console.cloud.google.com/auth/branding,",
		"  fill App name, User support email and Developer contact email → Save, then publish",
		"or instead: Test users → + Add users → your Gmail → Save",
		"  (works too, but Google signs you out every 7 days)",
	}},
	{"Create the key gtask uses", "https://console.cloud.google.com/auth/clients/create", []string{
		"Application type: Desktop app",
		"name: anything → click Create",
		"a box appears with a Client ID and Client secret — keep it open",
	}},
}

func isTTY() bool {
	fi, err := os.Stdin.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

type asker struct{ r *bufio.Reader }

func (a asker) line(prompt string) string {
	fmt.Print(prompt)
	s, err := a.r.ReadString('\n')
	if err != nil && s == "" {
		fmt.Println()
		os.Exit(130)
	}
	return strings.TrimSpace(s)
}

func (a asker) yes(prompt string) bool {
	s := strings.ToLower(a.line(prompt + muted.Render(" [Y/n] ")))
	return s == "" || s == "y" || s == "yes"
}

func setup() error {
	a := asker{bufio.NewReader(os.Stdin)}
	fmt.Println()
	fmt.Println(rose.Bold(true).Render("  ✿ gtask setup") + muted.Render("  ·  one time, about 3 minutes"))
	fmt.Println()

	if _, err := gtasks.LoadConfig(configDir); err == nil {
		if a.yes(text.Render("  you already have a Google key set up. just sign in?")) {
			return login()
		}
	}

	// Someone who already downloaded the JSON can skip the whole tour.
	if f := findDownloadedClient(); f != "" {
		fmt.Println(text.Render("  found a downloaded key: ") + gold.Render(f))
		if a.yes(text.Render("  use it?")) {
			b, err := os.ReadFile(f)
			if err != nil {
				return err
			}
			if err := saveCredentials(b); err != nil {
				return err
			}
			return login()
		}
	}

	fmt.Println(subtle.Render("  Google wants every app that reads your tasks to have its own key."))
	fmt.Println(subtle.Render("  You'll make one for yourself. Each step opens the exact page —"))
	fmt.Println(subtle.Render("  do what's listed, come back here, press Enter."))
	fmt.Println(muted.Render("  (already have a Client ID? type ") + rose.Render("k") + muted.Render(" at any step to jump to the end)"))

	for i, s := range setupSteps {
		fmt.Println()
		fmt.Println(iris.Bold(true).Render(fmt.Sprintf("  %d/%d  ", i+1, len(setupSteps))) + text.Bold(true).Render(s.title))
		fmt.Println(muted.Render("       ") + foam.Underline(true).Render(s.url))
		ans := strings.ToLower(a.line(muted.Render("       Enter to open · s skip · k I have the key · q quit  ")))
		switch ans {
		case "q":
			os.Exit(0)
		case "k":
			return askKey(a)
		case "s":
			continue
		}
		openBrowser(s.url)
		for _, d := range s.do {
			fmt.Println(pine.Render("       › ") + text.Render(d))
		}
		if ans := strings.ToLower(a.line(muted.Render("       done? Enter to continue  "))); ans == "q" {
			os.Exit(0)
		}
	}
	return askKey(a)
}

func askKey(a asker) error {
	fmt.Println()
	fmt.Println(iris.Bold(true).Render("  ✓  ") + text.Bold(true).Render("Paste the key here"))
	fmt.Println(muted.Render("       copy the values from that box (or paste the path of the downloaded .json)"))
	for {
		id := a.line(text.Render("       Client ID      ") + muted.Render("› "))
		if strings.HasSuffix(strings.ToLower(id), ".json") {
			b, err := os.ReadFile(expandHome(strings.Trim(id, `'"`)))
			if err != nil {
				fmt.Println(love.Render("       ✗ ") + text.Render(err.Error()))
				continue
			}
			if err := saveCredentials(b); err != nil {
				fmt.Println(love.Render("       ✗ ") + text.Render(err.Error()))
				continue
			}
			return login()
		}
		if !strings.HasSuffix(id, ".apps.googleusercontent.com") {
			fmt.Println(love.Render("       ✗ ") + text.Render("that doesn't look right — it ends in ") + gold.Render(".apps.googleusercontent.com"))
			continue
		}
		secret := a.line(text.Render("       Client secret  ") + muted.Render("› "))
		if secret == "" {
			fmt.Println(love.Render("       ✗ ") + text.Render("the secret is the line under the ID (starts with GOCSPX-)"))
			continue
		}
		b, _ := json.Marshal(map[string]any{"installed": map[string]string{"client_id": id, "client_secret": secret}})
		if err := saveCredentials(b); err != nil {
			return err
		}
		return login()
	}
}

// saveCredentials validates the client JSON and stores it in the config dir.
func saveCredentials(b []byte) error {
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return err
	}
	p := gtasks.CredentialsPath(configDir)
	if err := os.WriteFile(p, b, 0o600); err != nil {
		return err
	}
	if _, err := gtasks.LoadConfig(configDir); err != nil {
		os.Remove(p)
		return err
	}
	fmt.Println(pine.Render("       ✓ ") + text.Render("key saved to ") + muted.Render(p))
	return nil
}

// findDownloadedClient returns the newest client_secret_*.json in ~/Downloads.
func findDownloadedClient() string {
	h, _ := os.UserHomeDir()
	fs, _ := filepath.Glob(filepath.Join(h, "Downloads", "client_secret_*.json"))
	sort.Slice(fs, func(i, j int) bool { return modTime(fs[i]) > modTime(fs[j]) })
	if len(fs) == 0 {
		return ""
	}
	return fs[0]
}

func modTime(p string) int64 {
	fi, err := os.Stat(p)
	if err != nil {
		return 0
	}
	return fi.ModTime().UnixNano()
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		h, _ := os.UserHomeDir()
		return filepath.Join(h, p[2:])
	}
	return p
}
