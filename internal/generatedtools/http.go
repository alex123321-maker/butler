package generatedtools

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/owneridentity"
)

type privateRequest struct {
	Actor   owneridentity.Actor `json:"actor"`
	Control ControlRequest      `json:"control"`
}

func NewHandler(token string, s *Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(token) < 32 || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 {
			http.Error(w, "private converter authentication required", 401)
			return
		}
		if r.Method != "POST" || r.URL.RawQuery != "" {
			http.Error(w, "fixed converter POST required", 405)
			return
		}
		raw, e := io.ReadAll(http.MaxBytesReader(w, r.Body, 8192))
		var req privateRequest
		if e != nil || Decode(raw, &req) != nil || s == nil {
			http.Error(w, "invalid converter request", 400)
			return
		}
		result, e := s.Control(owneridentity.WithActor(r.Context(), req.Actor), req.Control)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		if e != nil {
			w.WriteHeader(409)
			result.Error = SafeError(e)
		}
		_ = json.NewEncoder(w).Encode(result)
	})
}

type Client struct {
	endpoint, token string
	http            *http.Client
}

func NewClient(endpoint, token string) (*Client, error) {
	u, e := url.Parse(endpoint)
	if e != nil || u.Scheme != "http" && u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Path != "/internal/converters" || u.RawPath != "" || u.RawQuery != "" || u.Fragment != "" || len(token) < 32 {
		return nil, ErrAccess
	}
	return &Client{endpoint: endpoint, token: token, http: &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (c *Client) Control(ctx context.Context, req ControlRequest) (ControlResponse, error) {
	result := ControlResponse{}
	a, ok := owneridentity.FromContext(ctx)
	if c == nil || !ok {
		return result, ErrAccess
	}
	raw, e := json.Marshal(privateRequest{Actor: a, Control: req})
	if e != nil || len(raw) > 8192 {
		return result, ErrPackage
	}
	r, e := http.NewRequestWithContext(ctx, "POST", c.endpoint, bytes.NewReader(raw))
	if e != nil {
		return result, ErrAccess
	}
	r.Header.Set("Authorization", "Bearer "+c.token)
	r.Header.Set("Content-Type", "application/json")
	res, e := c.http.Do(r)
	if e != nil {
		return result, ErrAccess
	}
	defer res.Body.Close()
	raw, e = io.ReadAll(io.LimitReader(res.Body, 256*1024+1))
	if e != nil || len(raw) > 256*1024 {
		return result, ErrAccess
	}
	if _, e = actionauth.ArgumentsHash(string(raw)); e != nil {
		return result, ErrAccess
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&result) != nil || d.Decode(new(any)) != io.EOF {
		return ControlResponse{}, ErrAccess
	}
	if res.StatusCode != 200 {
		return result, ErrAccess
	}
	return result, nil
}
