package singletabcontrol

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

type metadataRequest struct {
	Operation string `json:"operation"`
	Tab       *Tab   `json:"tab,omitempty"`
}
type metadataResponse struct {
	Tabs    []Tab `json:"tabs,omitempty"`
	Current bool  `json:"current,omitempty"`
}

func validToken(token string) bool {
	return len(token) >= 32 && len(token) <= 4096 && !strings.ContainsAny(token, "\r\n\t ")
}

// This fixed metadata endpoint cannot dispatch native methods or browser actions.
func NewMetadataHandler(backend Metadata, token string) (http.Handler, error) {
	if backend == nil || !validToken(token) {
		return nil, ErrDenied
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != http.MethodPost || len(r.Header.Values("Authorization")) != 1 || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 {
			http.Error(w, "private metadata authentication required", http.StatusUnauthorized)
			return
		}
		var request metadataRequest
		d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048))
		d.DisallowUnknownFields()
		if d.Decode(&request) != nil || d.Decode(&struct{}{}) != io.EOF {
			http.Error(w, "fixed metadata request required", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		var out metadataResponse
		var e error
		switch request.Operation {
		case "list":
			if request.Tab != nil {
				e = ErrDenied
				break
			}
			out.Tabs, e = backend.List(ctx)
			if len(out.Tabs) > MaxTabs {
				e = ErrDenied
			}
			for _, t := range out.Tabs {
				if !validTab(t, t.BrowserInstance) {
					e = ErrDenied
				}
			}
		case "check":
			if request.Tab == nil || !validTab(*request.Tab, request.Tab.BrowserInstance) {
				e = ErrDenied
				break
			}
			e = backend.Check(ctx, *request.Tab)
			out.Current = e == nil
		default:
			e = ErrDenied
		}
		if e != nil {
			http.Error(w, "current fixed metadata required", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	}), nil
}

type MetadataClient struct {
	endpoint, token string
	client          *http.Client
}

// Plain HTTP is loopback-only; remote service requires a trusted TLS gateway.
func NewMetadataClient(endpoint, token string) (*MetadataClient, error) {
	u, e := url.Parse(endpoint)
	if e != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || !validToken(token) {
		return nil, ErrDenied
	}
	if u.Scheme != "https" && (u.Scheme != "http" || net.ParseIP(u.Hostname()) == nil || !net.ParseIP(u.Hostname()).IsLoopback()) {
		return nil, ErrDenied
	}
	tr := &http.Transport{Proxy: nil, MaxIdleConns: 4, MaxIdleConnsPerHost: 2, IdleConnTimeout: 30 * time.Second, ResponseHeaderTimeout: 2 * time.Second, TLSHandshakeTimeout: 2 * time.Second}
	return &MetadataClient{endpoint, token, &http.Client{Transport: tr, Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (c *MetadataClient) call(ctx context.Context, q metadataRequest) (metadataResponse, error) {
	var out metadataResponse
	raw, e := json.Marshal(q)
	if e != nil {
		return out, ErrDenied
	}
	r, e := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(raw))
	if e != nil {
		return out, ErrDenied
	}
	r.Header.Set("Authorization", "Bearer "+c.token)
	r.Header.Set("Content-Type", "application/json")
	response, e := c.client.Do(r)
	if e != nil {
		return out, ErrDenied
	}
	defer response.Body.Close()
	data, e := io.ReadAll(io.LimitReader(response.Body, 16385))
	if e != nil || response.StatusCode != http.StatusOK || len(data) > 16384 {
		return out, ErrDenied
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if d.Decode(&out) != nil || d.Decode(&struct{}{}) != io.EOF {
		return metadataResponse{}, ErrDenied
	}
	return out, nil
}
func (c *MetadataClient) List(ctx context.Context) ([]Tab, error) {
	out, e := c.call(ctx, metadataRequest{Operation: "list"})
	if e != nil || out.Current || len(out.Tabs) < 1 || len(out.Tabs) > MaxTabs {
		return nil, ErrDenied
	}
	for _, t := range out.Tabs {
		if !validTab(t, t.BrowserInstance) {
			return nil, ErrDenied
		}
	}
	return out.Tabs, nil
}
func (c *MetadataClient) Check(ctx context.Context, t Tab) error {
	out, e := c.call(ctx, metadataRequest{Operation: "check", Tab: &t})
	if e != nil || !out.Current || len(out.Tabs) != 0 {
		return ErrDenied
	}
	return nil
}
