package gtasks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

const apiBase = "https://tasks.googleapis.com/tasks/v1"

// Client talks to the Google Tasks REST API using an authenticated http.Client.
type Client struct{ hc *http.Client }

func NewClient(hc *http.Client) *Client { return &Client{hc: hc} }

type apiError struct {
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func (c *Client) do(ctx context.Context, method, path string, q url.Values, body, out any) error {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(b)
	}
	u := apiBase + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, r)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		var ae apiError
		b, _ := io.ReadAll(resp.Body)
		if json.Unmarshal(b, &ae) == nil && ae.Error.Message != "" {
			return fmt.Errorf("google tasks: %s (%d)", ae.Error.Message, resp.StatusCode)
		}
		return fmt.Errorf("google tasks: %s", resp.Status)
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func esc(s string) string { return url.PathEscape(s) }

func (c *Client) Lists(ctx context.Context) ([]List, error) {
	var all []List
	token := ""
	for {
		q := url.Values{"maxResults": {"100"}}
		if token != "" {
			q.Set("pageToken", token)
		}
		var page struct {
			Items         []List `json:"items"`
			NextPageToken string `json:"nextPageToken"`
		}
		if err := c.do(ctx, "GET", "/users/@me/lists", q, nil, &page); err != nil {
			return nil, err
		}
		all = append(all, page.Items...)
		if token = page.NextPageToken; token == "" {
			return all, nil
		}
	}
}

func (c *Client) Tasks(ctx context.Context, listID string) ([]Task, error) {
	var all []Task
	token := ""
	for {
		// showHidden is required to see tasks completed from Google's own apps.
		q := url.Values{"maxResults": {"100"}, "showCompleted": {"true"}, "showHidden": {"true"}}
		if token != "" {
			q.Set("pageToken", token)
		}
		var page struct {
			Items         []Task `json:"items"`
			NextPageToken string `json:"nextPageToken"`
		}
		if err := c.do(ctx, "GET", "/lists/"+esc(listID)+"/tasks", q, nil, &page); err != nil {
			return nil, err
		}
		for _, t := range page.Items {
			if !t.Deleted {
				all = append(all, t)
			}
		}
		if token = page.NextPageToken; token == "" {
			return all, nil
		}
	}
}

func (c *Client) CreateList(ctx context.Context, title string) (List, error) {
	var l List
	err := c.do(ctx, "POST", "/users/@me/lists", nil, map[string]string{"title": title}, &l)
	return l, err
}

func (c *Client) RenameList(ctx context.Context, listID, title string) (List, error) {
	var l List
	err := c.do(ctx, "PATCH", "/users/@me/lists/"+esc(listID), nil, map[string]string{"title": title}, &l)
	return l, err
}

func (c *Client) DeleteList(ctx context.Context, listID string) error {
	return c.do(ctx, "DELETE", "/users/@me/lists/"+esc(listID), nil, nil, nil)
}

func placement(parent, previous string) url.Values {
	q := url.Values{}
	if parent != "" {
		q.Set("parent", parent)
	}
	if previous != "" {
		q.Set("previous", previous)
	}
	return q
}

func (c *Client) Insert(ctx context.Context, listID string, t Task, parent, previous string) (Task, error) {
	body := map[string]any{"title": t.Title}
	if t.Notes != "" {
		body["notes"] = t.Notes
	}
	if t.Due != "" {
		body["due"] = t.Due
	}
	if t.Status != "" {
		body["status"] = t.Status
	}
	var out Task
	err := c.do(ctx, "POST", "/lists/"+esc(listID)+"/tasks", placement(parent, previous), body, &out)
	return out, err
}

func (c *Client) Patch(ctx context.Context, listID, taskID string, patch map[string]any) (Task, error) {
	var out Task
	err := c.do(ctx, "PATCH", "/lists/"+esc(listID)+"/tasks/"+esc(taskID), nil, patch, &out)
	return out, err
}

func (c *Client) Delete(ctx context.Context, listID, taskID string) error {
	return c.do(ctx, "DELETE", "/lists/"+esc(listID)+"/tasks/"+esc(taskID), nil, nil, nil)
}

func (c *Client) Move(ctx context.Context, listID, taskID, parent, previous string) (Task, error) {
	var out Task
	err := c.do(ctx, "POST", "/lists/"+esc(listID)+"/tasks/"+esc(taskID)+"/move", placement(parent, previous), nil, &out)
	return out, err
}

func (c *Client) Clear(ctx context.Context, listID string) error {
	return c.do(ctx, "POST", "/lists/"+esc(listID)+"/clear", nil, nil, nil)
}
