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

func validRequestToken(token string) bool {
	return len(token) >= 32 && len(token) <= 4096 && !strings.ContainsAny(token, "\r\n \t")
}

type RequestClient struct {
	endpoint, token string
	client          *http.Client
}

func NewRequestClient(endpoint, token string) (*RequestClient, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || !validRequestToken(token) || u.Scheme != "https" && (u.Scheme != "http" || net.ParseIP(u.Hostname()) == nil || !net.ParseIP(u.Hostname()).IsLoopback()) {
		return nil, ErrDenied
	}
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = nil
	return &RequestClient{endpoint, token, &http.Client{Transport: t, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (c *RequestClient) RequestDispatched(ctx context.Context, e RequestEnvelope) (RequestReceipt, error) {
	if c == nil {
		return RequestReceipt{}, ErrDenied
	}
	raw, err := json.Marshal(e)
	if err != nil {
		return RequestReceipt{}, ErrDenied
	}
	if _, err = decodeRequestEnvelope(raw); err != nil {
		return RequestReceipt{}, ErrDenied
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(raw))
	if err != nil {
		return RequestReceipt{}, ErrDenied
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return RequestReceipt{}, ErrDenied
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return RequestReceipt{}, ErrDenied
	}
	raw, err = io.ReadAll(io.LimitReader(resp.Body, 8193))
	if err != nil {
		return RequestReceipt{}, ErrDenied
	}
	return decodeRequestReceipt(raw)
}
func NewRequestHandler(token string, service RequestService) (http.Handler, error) {
	if !validRequestToken(token) || service == nil {
		return nil, ErrDenied
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != http.MethodPost || r.URL.RawQuery != "" || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 {
			http.Error(w, "guarded request denied", http.StatusForbidden)
			return
		}
		raw, err := io.ReadAll(io.LimitReader(r.Body, 8193))
		if err != nil {
			http.Error(w, "guarded request denied", http.StatusBadRequest)
			return
		}
		e, err := decodeRequestEnvelope(raw)
		if err != nil {
			http.Error(w, "guarded request denied", http.StatusBadRequest)
			return
		}
		receipt, err := service.RequestDispatched(r.Context(), e)
		if err != nil {
			http.Error(w, "guarded request denied", http.StatusConflict)
			return
		}
		raw, err = json.Marshal(receipt)
		if err != nil {
			http.Error(w, "guarded request denied", http.StatusConflict)
			return
		}
		if _, err = decodeRequestReceipt(raw); err != nil {
			http.Error(w, "guarded request denied", http.StatusConflict)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(raw)
	}), nil
}
