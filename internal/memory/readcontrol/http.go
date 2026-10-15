package readcontrol

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func validToken(token string) bool {
	return len(token) >= 32 && len(token) <= 4096 && !strings.ContainsAny(token, "\r\n \t")
}

type Client struct {
	endpoint, token string
	http            *http.Client
}

func NewClient(endpoint, token string) (*Client, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Opaque != "" || u.Fragment != "" || u.Path != Path || u.RawPath != "" || !validToken(token) || u.Scheme != "https" && (u.Scheme != "http" || net.ParseIP(u.Hostname()) == nil || !net.ParseIP(u.Hostname()).IsLoopback()) {
		return nil, ErrDenied
	}
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = nil
	return &Client{endpoint, token, &http.Client{Transport: t, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (c *Client) ReadDispatched(ctx context.Context, e Envelope) (Result, error) {
	if c == nil {
		return Empty("denied"), ErrDenied
	}
	raw, err := json.Marshal(e)
	if err != nil {
		return Empty("denied"), ErrDenied
	}
	if _, err = decodeEnvelope(raw); err != nil {
		return Empty("denied"), err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(raw))
	if err != nil {
		return Empty("denied"), ErrDenied
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return Empty("denied"), ErrDenied
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Empty("denied"), ErrDenied
	}
	raw, err = io.ReadAll(io.LimitReader(resp.Body, MaxResponse+1))
	if err != nil {
		return Empty("denied"), ErrDenied
	}
	return DecodeResult(raw)
}
func NewHandler(token string, service Service) (http.Handler, error) {
	if !validToken(token) || service == nil {
		return nil, ErrDenied
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != http.MethodPost || r.URL.Path != Path || r.URL.RawPath != "" || r.URL.RawQuery != "" || r.URL.ForceQuery || len(r.Header.Values("Authorization")) != 1 || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 {
			http.Error(w, "memory read denied", http.StatusForbidden)
			return
		}
		raw, err := io.ReadAll(io.LimitReader(r.Body, 8193))
		if err != nil {
			http.Error(w, "memory read denied", http.StatusBadRequest)
			return
		}
		e, err := decodeEnvelope(raw)
		if err != nil {
			http.Error(w, "memory read denied", http.StatusBadRequest)
			return
		}
		receipt, err := service.ReadDispatched(r.Context(), e)
		if err != nil {
			http.Error(w, "memory read denied", http.StatusConflict)
			return
		}
		raw, err = json.Marshal(receipt)
		if err != nil {
			http.Error(w, "memory read denied", http.StatusConflict)
			return
		}
		if _, err = DecodeResult(raw); err != nil {
			http.Error(w, "memory read denied", http.StatusConflict)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(raw)
	}), nil
}
