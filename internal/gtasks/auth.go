package gtasks

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/oauth2"
)

const Scope = "https://www.googleapis.com/auth/tasks"

var (
	ErrNoCredentials = errors.New("no OAuth client credentials found")
	ErrNoToken       = errors.New("not logged in")
)

var googleEndpoint = oauth2.Endpoint{
	AuthURL:   "https://accounts.google.com/o/oauth2/auth",
	TokenURL:  "https://oauth2.googleapis.com/token",
	AuthStyle: oauth2.AuthStyleInParams,
}

func CredentialsPath(dir string) string { return filepath.Join(dir, "credentials.json") }
func tokenPath(dir string) string       { return filepath.Join(dir, "token.json") }

// LoadConfig reads the OAuth client from env vars or the credentials.json downloaded from Google Cloud.
func LoadConfig(dir string) (*oauth2.Config, error) {
	id, secret := os.Getenv("GTASK_CLIENT_ID"), os.Getenv("GTASK_CLIENT_SECRET")
	if id == "" {
		b, err := os.ReadFile(CredentialsPath(dir))
		if err != nil {
			return nil, ErrNoCredentials
		}
		type client struct {
			ClientID     string `json:"client_id"`
			ClientSecret string `json:"client_secret"`
		}
		var f struct {
			Installed *client `json:"installed"`
			Web       *client `json:"web"`
		}
		if err := json.Unmarshal(b, &f); err != nil {
			return nil, fmt.Errorf("parse %s: %w", CredentialsPath(dir), err)
		}
		c := f.Installed
		if c == nil {
			c = f.Web
		}
		if c == nil || c.ClientID == "" {
			return nil, fmt.Errorf("%s has no \"installed\" client — create a *Desktop app* OAuth client", CredentialsPath(dir))
		}
		id, secret = c.ClientID, c.ClientSecret
	}
	return &oauth2.Config{ClientID: id, ClientSecret: secret, Endpoint: googleEndpoint, Scopes: []string{Scope}}, nil
}

// Login runs the loopback OAuth flow with PKCE and stores the resulting token in dir.
func Login(ctx context.Context, dir string, open func(url string)) error {
	cfg, err := LoadConfig(dir)
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer ln.Close()
	cfg.RedirectURL = fmt.Sprintf("http://127.0.0.1:%d/", ln.Addr().(*net.TCPAddr).Port)

	sb := make([]byte, 16)
	rand.Read(sb)
	state := hex.EncodeToString(sb)
	verifier := oauth2.GenerateVerifier()

	type result struct {
		code string
		err  error
	}
	done := make(chan result, 1)
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("state") != state {
			http.Error(w, "state mismatch", http.StatusBadRequest)
			return
		}
		if e := q.Get("error"); e != "" {
			fmt.Fprint(w, page("Login failed", e))
			done <- result{err: errors.New(e)}
			return
		}
		fmt.Fprint(w, page("You're in ✿", "gtask is connected to Google Tasks. You can close this tab."))
		done <- result{code: q.Get("code")}
	})}
	go srv.Serve(ln)
	defer srv.Close()

	authURL := cfg.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.S256ChallengeOption(verifier),
		oauth2.SetAuthURLParam("prompt", "consent"))
	open(authURL)

	var res result
	select {
	case res = <-done:
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(5 * time.Minute):
		return errors.New("timed out waiting for the browser")
	}
	if res.err != nil {
		return res.err
	}
	tok, err := cfg.Exchange(ctx, res.code, oauth2.VerifierOption(verifier))
	if err != nil {
		return err
	}
	return saveToken(dir, tok)
}

func page(title, msg string) string {
	return `<!doctype html><meta charset=utf-8><title>gtask</title><body style="background:#191724;color:#e0def4;font-family:system-ui;display:grid;place-items:center;height:100vh;margin:0">` +
		`<div style="text-align:center"><h1 style="color:#ebbcba;font-weight:500">` + title + `</h1><p style="color:#908caa">` + msg + `</p></div>`
}

func saveToken(dir string, tok *oauth2.Token) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(tok, "", "  ")
	return os.WriteFile(tokenPath(dir), b, 0o600)
}

// savingSource persists refreshed tokens so refresh-token rotation survives restarts.
type savingSource struct {
	mu   sync.Mutex
	src  oauth2.TokenSource
	dir  string
	last string
}

func (s *savingSource) Token() (*oauth2.Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, err := s.src.Token()
	if err != nil {
		return nil, err
	}
	if t.AccessToken != s.last {
		s.last = t.AccessToken
		_ = saveToken(s.dir, t)
	}
	return t, nil
}

// HTTPClient returns an authenticated client using the stored token.
func HTTPClient(ctx context.Context, dir string) (*http.Client, error) {
	cfg, err := LoadConfig(dir)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(tokenPath(dir))
	if err != nil {
		return nil, ErrNoToken
	}
	var tok oauth2.Token
	if err := json.Unmarshal(b, &tok); err != nil {
		return nil, ErrNoToken
	}
	ctx = context.WithValue(ctx, oauth2.HTTPClient, &http.Client{Timeout: 20 * time.Second})
	src := &savingSource{src: cfg.TokenSource(ctx, &tok), dir: dir, last: tok.AccessToken}
	return oauth2.NewClient(ctx, oauth2.ReuseTokenSource(&tok, src)), nil
}
