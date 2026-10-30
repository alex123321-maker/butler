// Package browsercontrol is the private owner channel for server Chromium.
// Pixels and input never form model tool arguments, artifacts or transcripts.
package browsercontrol

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/butler/butler/internal/actionauth"
)

const Path = "/internal/server-browser"

var ErrDenied = errors.New("browser control unavailable; reload the session")

type Command struct {
	PageID     string  `json:"page_id,omitempty"`
	DocumentID string  `json:"document_id,omitempty"`
	Operation  string  `json:"operation"`
	SessionID  string  `json:"browser_session_id,omitempty"`
	RunID      string  `json:"run_id,omitempty"`
	OwnerID    string  `json:"owner_id,omitempty"`
	Controller string  `json:"controller,omitempty"`
	Epoch      int64   `json:"epoch,omitempty"`
	FrameID    string  `json:"frame_id,omitempty"`
	X          float64 `json:"x,omitempty"`
	Y          float64 `json:"y,omitempty"`
	DeltaX     float64 `json:"delta_x,omitempty"`
	DeltaY     float64 `json:"delta_y,omitempty"`
	Button     string  `json:"button,omitempty"`
	Key        string  `json:"key,omitempty"`
	Text       string  `json:"text,omitempty"`
	URL        string  `json:"url,omitempty"`
}
type Frame struct {
	PageID     string `json:"page_id,omitempty"`
	DocumentID string `json:"document_id,omitempty"`
	URL        string `json:"url,omitempty"`
	JPEG       string `json:"jpeg,omitempty"`
	ID         string `json:"frame_id,omitempty"`
	Width      int    `json:"width,omitempty"`
	Height     int    `json:"height,omitempty"`
	Epoch      int64  `json:"epoch"`
	Mode       string `json:"mode"`
}
type Session struct {
	ID      string    `json:"browser_session_id"`
	RunID   string    `json:"run_id"`
	TaskID  string    `json:"task_id"`
	Expires time.Time `json:"expires_at"`
	Mode    string    `json:"mode"`
	Epoch   int64     `json:"epoch"`
}
type Reply struct {
	ManagedPages bool        `json:"managed_pages,omitempty"`
	Pages        []OwnerPage `json:"pages,omitempty"`
	Sessions     []Session   `json:"sessions,omitempty"`
	Frame        *Frame      `json:"frame,omitempty"`
	OK           bool        `json:"ok"`
}

type OwnerPage struct {
	ID         string `json:"page_id"`
	DocumentID string `json:"document_id"`
	URL        string `json:"url"`
	State      string `json:"state"`
}
type Controller interface {
	Control(context.Context, Command) (Reply, error)
}

func Handler(token string, controller Controller) http.Handler {
	want := sha256.Sum256([]byte("Bearer " + token))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		got := sha256.Sum256([]byte(r.Header.Get("Authorization")))
		if len(token) < 32 || len(r.Header.Values("Authorization")) != 1 || subtle.ConstantTimeCompare(want[:], got[:]) != 1 {
			w.WriteHeader(401)
			return
		}
		if r.Method != http.MethodPost || r.URL.Path != Path || r.URL.RawQuery != "" {
			w.WriteHeader(400)
			return
		}
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 16384))
		if err != nil {
			w.WriteHeader(400)
			return
		}
		cmd, err := Decode(raw)
		if err != nil {
			w.WriteHeader(400)
			return
		}
		reply, err := controller.Control(r.Context(), cmd)
		if err != nil {
			w.WriteHeader(409)
			_, _ = w.Write([]byte(`{"error":"browser control unavailable; reload the session"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(reply)
	})
}
func Decode(raw []byte) (Command, error) {
	var command Command
	if _, err := actionauth.ArgumentsHash(string(raw)); err != nil {
		return command, ErrDenied
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&command) != nil {
		return command, ErrDenied
	}
	return command, nil
}

type Client struct {
	endpoint, token string
	http            *http.Client
}

func NewClient(endpoint, token string) (*Client, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Hostname() == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Path != Path || u.RawQuery != "" || u.Fragment != "" || len(token) < 32 {
		return nil, ErrDenied
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &Client{endpoint, token, &http.Client{Transport: transport, Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (c *Client) Control(ctx context.Context, command Command) (Reply, error) {
	var reply Reply
	if c == nil {
		return reply, ErrDenied
	}
	raw, _ := json.Marshal(command)
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(raw))
	if err != nil {
		return reply, ErrDenied
	}
	r.Header.Set("Authorization", "Bearer "+c.token)
	r.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(r)
	if err != nil {
		return reply, ErrDenied
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return reply, ErrDenied
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024+1))
	if err != nil || len(data) > 2*1024*1024 || json.Unmarshal(data, &reply) != nil || !reply.OK {
		return Reply{}, ErrDenied
	}
	return reply, nil
}
