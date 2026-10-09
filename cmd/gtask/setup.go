package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Srindot/cli-Gtasks/internal/gtasks"
)

// The Cloud console is a maze; these links land directly on the page each step needs.
type setupStep struct {
	title, url string
	do         []string
}

var setupSteps = []setupStep{
	{"Create a project", "https://console.cloud.google.com/projectcreate", []string{
		"Project name: gtask (anything works)",
		"leave Organization / Location as they are → Create",
		"wait until the bell (top-right) says the project is ready",
	}},
	{"Turn on the Tasks API", "https://console.cloud.google.com/apis/library/tasks.googleapis.com", []string{
		"the project picker (top-left) must say gtask — click it and pick gtask if not",
		"click Enable",
	}},
	{"Set up the sign-in screen", "https://console.cloud.google.com/auth/overview", []string{
		"click Get started",
		"App name: gtask · User support email: pick yours → Next",
		"Audience: External → Next",
		"Contact information: your email → Next",
		"tick “I agree…” → Continue → Create",
	}},
	{"Add yourself as a test user", "https://console.cloud.google.com/auth/audience", []string{
		"leave Publishing status on Testing (no need to press Publish)",
		"scroll to Test users → + Add users → your Gmail address → Save",
		"Testing apps ask you to sign in again about once a week — gtask reopens the browser for you",
	}},
	{"Create the key", "https://console.cloud.google.com/auth/clients/create", []string{
		"Application type: Desktop app   (not Web application)",
		"Name: anything → Create",
		"in the “OAuth client created” box click Download JSON → OK",
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
			if err := importKey(f); err != nil {
				return err
			}
			return login()
		}
	}

	fmt.Println(subtle.Render("  Google wants every app that reads your tasks to have its own key."))
	fmt.Println(subtle.Render("  You'll make one for yourself. Each step opens the exact page —"))
	fmt.Println(subtle.Render("  do what's listed, come back here, press Enter."))
	fmt.Println(muted.Render("  (already have a Client ID? type ") + rose.Render("k") + muted.Render(" at any step to jump to the end)"))

	began := time.Now()
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
		for _, d := range s.do {
			fmt.Println(pine.Render("       › ") + text.Render(d))
		}
		openBrowser(s.url)
		if ans := strings.ToLower(a.line(muted.Render("       done? Enter to continue  "))); ans == "q" {
			os.Exit(0)
		}
	}
	// Step 5 ends with a download; pick it up instead of asking.
	if f := findDownloadedClient(); f != "" && modTime(f) > began.UnixNano() {
		fmt.Println()
		fmt.Println(pine.Render("  ✓ ") + text.Render("got the key you just downloaded: ") + gold.Render(filepath.Base(f)))
		if err := importKey(f); err == nil {
			return login()
		} else {
			fmt.Println(love.Render("  ✗ ") + text.Render(err.Error()))
		}
	}
	return askKey(a)
}

func importKey(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return saveCredentials(b)
}

func askKey(a asker) error {
	fmt.Println()
	fmt.Println(iris.Bold(true).Render("  ✓  ") + text.Bold(true).Render("Paste the key here"))
	fmt.Println(muted.Render("       downloaded the JSON? just press Enter and gtask finds it in ~/Downloads"))
	fmt.Println(muted.Render("       otherwise paste the Client ID, or the path to the .json file"))
	for {
		id := a.line(text.Render("       Client ID      ") + muted.Render("› "))
		if id == "" {
			f := findDownloadedClient()
			if f == "" {
				fmt.Println(love.Render("       ✗ ") + text.Render("no client_secret_*.json in ~/Downloads yet"))
				continue
			}
			fmt.Println(pine.Render("       ✓ ") + text.Render("found ") + gold.Render(filepath.Base(f)))
			if err := importKey(f); err != nil {
				fmt.Println(love.Render("       ✗ ") + text.Render(err.Error()))
				continue
			}
			return login()
		}
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
