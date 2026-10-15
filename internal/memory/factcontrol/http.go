package factcontrol

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

	"github.com/butler/butler/internal/memory/readcontrol"
)

type Client struct {
	endpoint, token string
	http            *http.Client
}

func validToken(t string) bool {
	return len(t) >= 32 && len(t) <= 4096 && !strings.ContainsAny(t, "\r\n \t")
}
func NewClient(endpoint, token string) (*Client, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, ErrDenied
	}
	ip := net.ParseIP(u.Hostname())
	private := u.Scheme == "http" && (u.Host == "orchestrator:8080" || ip != nil && ip.IsLoopback())
	if u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Opaque != "" || u.Fragment != "" || u.Path != Path || u.RawPath != "" || !validToken(token) || u.Scheme != "https" && !private {
		return nil, ErrDenied
	}
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = nil
	return &Client{endpoint, token, &http.Client{Transport: t, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func decodeEnvelope(raw []byte) (readcontrol.Envelope, error) {
	var e readcontrol.Envelope
	d := json.NewDecoder(bytes.NewReader(raw))
	if unique(d, 0) != nil {
		return e, ErrDenied
	}
	if _, err := d.Token(); err != io.EOF {
		return e, ErrDenied
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&e) != nil || e.Tool != Tool || e.RunID == "" || e.ToolCallID == "" || e.EffectID == "" || len(e.ContractHash) != 64 {
		return e, ErrDenied
	}
	if d.Decode(new(any)) != io.EOF {
		return e, ErrDenied
	}
	_, err := DecodeArgs(e.ArgsJSON)
	return e, err
}
func (c *Client) ReadDispatched(ctx context.Context, e readcontrol.Envelope) (readcontrol.Result, error) {
	if c == nil {
		return readcontrol.Empty("denied"), ErrDenied
	}
	raw, err := json.Marshal(e)
	if err != nil {
		return readcontrol.Empty("denied"), err
	}
	if _, err = decodeEnvelope(raw); err != nil {
		return readcontrol.Empty("denied"), err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(raw))
	if err != nil {
		return readcontrol.Empty("denied"), err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return readcontrol.Empty("denied"), ErrDenied
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return readcontrol.Empty("denied"), ErrDenied
	}
	raw, err = io.ReadAll(io.LimitReader(resp.Body, readcontrol.MaxResponse+1))
	if err != nil {
		return readcontrol.Empty("denied"), ErrDenied
	}
	return readcontrol.DecodeResult(raw)
}
func NewHandler(token string, s readcontrol.Service) (http.Handler, error) {
	if !validToken(token) || s == nil {
		return nil, ErrDenied
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != http.MethodPost || r.URL.Path != Path || r.URL.RawPath != "" || r.URL.RawQuery != "" || r.URL.ForceQuery || len(r.Header.Values("Authorization")) != 1 || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 {
			http.Error(w, "temporal read denied", 403)
			return
		}
		raw, err := io.ReadAll(io.LimitReader(r.Body, 16385))
		if err != nil || len(raw) > 16384 {
			http.Error(w, "temporal read denied", 400)
			return
		}
		e, err := decodeEnvelope(raw)
		if err != nil {
			http.Error(w, "temporal read denied", 400)
			return
		}
		out, err := s.ReadDispatched(r.Context(), e)
		if err != nil {
			http.Error(w, "temporal read denied", 409)
			return
		}
		raw, err = json.Marshal(out)
		if err != nil {
			http.Error(w, "temporal read denied", 409)
			return
		}
		if _, err = readcontrol.DecodeResult(raw); err != nil {
			http.Error(w, "temporal read denied", 409)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(raw)
	}), nil
}
