package delegation

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type AdmissionService interface {
	AdmitDispatched(context.Context, AdmissionEnvelope) (AdmissionReceipt, error)
}

func NewAdmissionHandler(token string, service AdmissionService) (http.Handler, error) {
	if !validAdmissionToken(token) || service == nil {
		return nil, ErrDenied
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		header := r.Header.Get("Authorization")
		provided := strings.TrimPrefix(header, "Bearer ")
		if !strings.HasPrefix(header, "Bearer ") || subtle.ConstantTimeCompare([]byte(token), []byte(provided)) != 1 {
			http.Error(w, "private admission authentication required", http.StatusUnauthorized)
			return
		}
		var request AdmissionEnvelope
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 256*1024))
		if err != nil || strictAdmissionJSON(raw, &request) != nil {
			http.Error(w, "invalid admission request", http.StatusBadRequest)
			return
		}
		receipt, err := service.AdmitDispatched(r.Context(), request)
		if err != nil {
			http.Error(w, "current claimed admission denied", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(receipt)
	}), nil
}

type AdmissionClient struct {
	endpoint, token string
	client          *http.Client
}

func NewAdmissionClient(endpoint, token string) (*AdmissionClient, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") || !validAdmissionToken(token) {
		return nil, ErrDenied
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil // provider proxy configuration cannot receive private service credentials
	return &AdmissionClient{endpoint: endpoint, token: token, client: &http.Client{Transport: transport, Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func validAdmissionToken(t string) bool {
	return len(t) >= 32 && len(t) <= 4096 && !strings.ContainsAny(t, "\r\n \t")
}
func (c *AdmissionClient) AdmitDispatched(ctx context.Context, e AdmissionEnvelope) (AdmissionReceipt, error) {
	if c == nil || c.client == nil {
		return AdmissionReceipt{}, ErrDenied
	}
	raw, err := json.Marshal(e)
	if err != nil || len(raw) > 256*1024 {
		return AdmissionReceipt{}, ErrDenied
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(raw))
	if err != nil {
		return AdmissionReceipt{}, ErrDenied
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return AdmissionReceipt{}, errors.New("private delegation admission unavailable; outcome may be unknown")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return AdmissionReceipt{}, ErrDenied
	}
	raw, err = io.ReadAll(io.LimitReader(resp.Body, 32769))
	if err != nil || len(raw) > 32768 {
		return AdmissionReceipt{}, ErrDenied
	}
	var receipt AdmissionReceipt
	if strictAdmissionJSON(raw, &receipt) != nil || !validID(receipt.TaskID) || receipt.ContractRevision != 1 || (receipt.State != "waiting" && receipt.State != "queued" && receipt.State != "executing" && receipt.State != "result_ready") || receipt.State != "waiting" && !validID(receipt.InputID) {
		return AdmissionReceipt{}, ErrDenied
	}
	return receipt, nil
}
