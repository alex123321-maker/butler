package resourceexec

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

type wireRequest struct {
	Operation string         `json:"operation"`
	Request   RuntimeRequest `json:"request"`
	Receipt   *Receipt       `json:"receipt,omitempty"`
}
type wireResponse struct {
	Spec *RuntimeSpec `json:"spec,omitempty"`
}

// NewHTTPHandler is an internal authenticated service endpoint. It does not
// register itself on a public API or accept owner/model policies.
func NewHTTPHandler(authority RuntimeAuthority, token string) (http.Handler, error) {
	if authority == nil || !validServiceToken(token) {
		return nil, ErrDenied
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != http.MethodPost || len(r.Header.Values("Authorization")) != 1 || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 {
			http.Error(w, "private resource authority required", http.StatusUnauthorized)
			return
		}
		var request wireRequest
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
		dec.DisallowUnknownFields()
		if dec.Decode(&request) != nil || dec.Decode(&struct{}{}) != io.EOF {
			http.Error(w, "invalid resource request", http.StatusBadRequest)
			return
		}
		var response wireResponse
		var e error
		switch request.Operation {
		case "inspect":
			if request.Receipt != nil {
				e = ErrDenied
				break
			}
			var spec RuntimeSpec
			spec, e = authority.Inspect(r.Context(), request.Request)
			response.Spec = &spec
		case "begin":
			if request.Receipt != nil {
				e = ErrDenied
				break
			}
			var spec RuntimeSpec
			spec, e = authority.Begin(r.Context(), request.Request)
			response.Spec = &spec
		case "finish":
			if request.Receipt == nil || request.Request != (RuntimeRequest{}) {
				e = ErrDenied
				break
			}
			e = authority.Finish(r.Context(), *request.Receipt)
		default:
			e = ErrDenied
		}
		if e != nil {
			http.Error(w, "current guarded resource dispatch required", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}), nil
}

type HTTPAuthority struct {
	endpoint, token string
	client          *http.Client
}

func NewHTTPAuthority(endpoint, token string) (*HTTPAuthority, error) {
	u, e := url.Parse(endpoint)
	if e != nil || u.Hostname() == "" || u.User != nil || u.Fragment != "" || u.RawQuery != "" || (u.Scheme != "http" && u.Scheme != "https") || !validServiceToken(token) {
		return nil, ErrDenied
	}
	// This private authority hop must not inherit a provider's ambient proxy.
	// The operator fixes the endpoint and the internal network at composition.
	transport := &http.Transport{Proxy: nil, MaxIdleConns: 8, MaxIdleConnsPerHost: 4, IdleConnTimeout: 30 * time.Second, ResponseHeaderTimeout: 3 * time.Second, TLSHandshakeTimeout: 3 * time.Second}
	return &HTTPAuthority{endpoint, token, &http.Client{Transport: transport, Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

func validServiceToken(token string) bool {
	return len(token) >= 32 && len(token) <= 4096 && !strings.ContainsAny(token, "\r\n\t ")
}
func (c *HTTPAuthority) read(ctx context.Context, r wireRequest) (wireResponse, error) {
	var response wireResponse
	data, e := json.Marshal(r)
	if e != nil {
		return response, ErrDenied
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(data))
	if e != nil {
		return response, ErrDenied
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, e := c.client.Do(req)
	if e != nil {
		return response, ErrDenied
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return response, ErrDenied
	}
	data, e = io.ReadAll(io.LimitReader(resp.Body, 4097))
	if e != nil || len(data) > 4096 {
		return response, ErrDenied
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if dec.Decode(&response) != nil || dec.Decode(&struct{}{}) != io.EOF {
		return response, ErrDenied
	}
	return response, nil
}
func (c *HTTPAuthority) Inspect(ctx context.Context, r RuntimeRequest) (RuntimeSpec, error) {
	v, e := c.read(ctx, wireRequest{Operation: "inspect", Request: r})
	if e != nil || v.Spec == nil {
		return RuntimeSpec{}, ErrDenied
	}
	return *v.Spec, nil
}
func (c *HTTPAuthority) Begin(ctx context.Context, r RuntimeRequest) (RuntimeSpec, error) {
	v, e := c.read(ctx, wireRequest{Operation: "begin", Request: r})
	if e != nil || v.Spec == nil {
		return RuntimeSpec{}, ErrDenied
	}
	return *v.Spec, nil
}
func (c *HTTPAuthority) Finish(ctx context.Context, r Receipt) error {
	_, e := c.read(ctx, wireRequest{Operation: "finish", Receipt: &r})
	return e
}
