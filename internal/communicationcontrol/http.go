package communicationcontrol

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func validToken(t string) bool {
	return len(t) >= 32 && len(t) <= 4096 && !strings.ContainsAny(t, "\r\n \t")
}
func validEnvelope(e Envelope) bool {
	if !IsTool(e.Tool) || e.RunID == "" || e.ToolCallID == "" || e.EffectID == "" || len(e.RunID) > 512 || len(e.ToolCallID) > 512 || len(e.EffectID) > 512 || len(e.ContractHash) != 64 {
		return false
	}
	_, err := DecodeArgs(e.Tool, e.ArgsJSON)
	return err == nil
}
func NewHandler(token string, service Service) (http.Handler, error) {
	if !validToken(token) || service == nil {
		return nil, ErrDenied
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != "POST" || r.URL.Path != Path || r.URL.RawPath != "" || r.URL.RawQuery != "" || r.URL.ForceQuery || len(r.Header.Values("Authorization")) != 1 || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 {
			http.Error(w, "communication control denied", 403)
			return
		}
		raw, err := io.ReadAll(io.LimitReader(r.Body, 256*1024+1))
		var e Envelope
		if err != nil || StrictJSON(raw, &e) != nil || !validEnvelope(e) {
			http.Error(w, "invalid communication control", 400)
			return
		}
		receipt, err := service.ExecuteDispatched(r.Context(), e)
		if err != nil {
			http.Error(w, "communication control denied", 409)
			return
		}
		raw, err = json.Marshal(receipt)
		if err != nil || len(raw) > 256*1024 {
			http.Error(w, "communication result unavailable", 409)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(raw)
	}), nil
}

type Client struct {
	endpoint, token string
	http            *http.Client
}

func NewClient(endpoint, token string) (*Client, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Path != Path || u.RawPath != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || (u.Scheme != "http" && u.Scheme != "https") || !validToken(token) {
		return nil, ErrDenied
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &Client{endpoint, token, &http.Client{Transport: transport, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (c *Client) ExecuteDispatched(ctx context.Context, e Envelope) (Receipt, error) {
	if c == nil || !validEnvelope(e) {
		return Receipt{}, ErrDenied
	}
	raw, err := json.Marshal(e)
	if err != nil {
		return Receipt{}, ErrDenied
	}
	req, err := http.NewRequestWithContext(ctx, "POST", c.endpoint, bytes.NewReader(raw))
	if err != nil {
		return Receipt{}, ErrDenied
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return Receipt{}, ErrDenied
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return Receipt{}, ErrDenied
	}
	raw, err = io.ReadAll(io.LimitReader(resp.Body, 256*1024+1))
	var out Receipt
	if err != nil || StrictJSON(raw, &out) != nil {
		return Receipt{}, ErrDenied
	}
	if out.Status != "accepted" || !strings.HasPrefix(out.PublicationRef, "publication:") || out.InteractionRef != "" && !strings.HasPrefix(out.InteractionRef, "interaction:") {
		return Receipt{}, ErrDenied
	}
	return out, nil
}
