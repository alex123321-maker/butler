package schedulecontrol

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
			http.Error(w, "schedule control denied", 403)
			return
		}
		raw, err := io.ReadAll(io.LimitReader(r.Body, 128*1024+1))
		var e Envelope
		if err != nil || StrictJSON(raw, &e) != nil || !validEnvelope(e) {
			http.Error(w, "invalid schedule control", 400)
			return
		}
		receipt, err := service.ExecuteDispatched(r.Context(), e)
		if err != nil {
			http.Error(w, "schedule control denied", 409)
			return
		}
		raw, err = json.Marshal(receipt)
		if err != nil || len(raw) > 128*1024 {
			http.Error(w, "schedule result unavailable", 409)
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
	raw, err = io.ReadAll(io.LimitReader(resp.Body, 128*1024+1))
	var out Receipt
	if err != nil || StrictJSON(raw, &out) != nil {
		return Receipt{}, ErrDenied
	}
	if e.Tool == "schedule.list" {
		if out.Schedule != nil || len(out.Schedules) > 10 || out.NextCursor != "" && !validID(out.NextCursor) {
			return Receipt{}, ErrDenied
		}
		for _, v := range out.Schedules {
			if !validView(v) {
				return Receipt{}, ErrDenied
			}
		}
	} else if out.Schedule == nil || len(out.Schedules) != 0 || !validView(*out.Schedule) {
		return Receipt{}, ErrDenied
	}
	return out, nil
}
func validView(v View) bool {
	return validID(v.ID) && v.Revision > 0 && (v.State == "active" || v.State == "paused" || v.State == "blocked" || v.State == "cancelled" || v.State == "finished")
}
