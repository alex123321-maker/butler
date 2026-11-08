package browserspa

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Capabilities struct {
	Profile         string `json:"profile"`
	WorkerTLSGate   bool   `json:"worker_tls_gate"`
	LiveAuthority   bool   `json:"live_authority"`
	TerminalContext bool   `json:"terminal_context"`
}

func capabilities() Capabilities {
	return Capabilities{Profile: Profile, WorkerTLSGate: true, LiveAuthority: true, TerminalContext: true}
}
func CapabilitiesHandler(token string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if len(token) < 32 || r.Method != http.MethodGet || len(r.Header.Values("Authorization")) != 1 || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 {
			http.Error(w, "private runtime capability required", 401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(capabilities())
	})
}
func ProbeCapabilities(ctx context.Context, controlURL, token string) error {
	u, err := url.Parse(controlURL)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") || len(token) < 32 || strings.ContainsAny(token, "\r\n\t ") {
		return ErrDenied
	}
	u.Path, u.RawPath = CapabilityPath, ""
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return ErrDenied
	}
	request.Header.Set("Authorization", "Bearer "+token)
	client := &http.Client{Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true, ResponseHeaderTimeout: 3 * time.Second}, Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	response, err := client.Do(request)
	if err != nil {
		return ErrDenied
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 1025))
	var got Capabilities
	if err != nil || len(raw) > 1024 || response.StatusCode != 200 || json.Unmarshal(raw, &got) != nil || got != capabilities() {
		return ErrDenied
	}
	return nil
}
