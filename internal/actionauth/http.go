package actionauth

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type contextRequest struct {
	RunID    string           `json:"run_id"`
	Approval *ApprovalRequest `json:"approval,omitempty"`
}
type contextResponse struct {
	Context  *ContextSnapshot  `json:"context,omitempty"`
	Approval *ApprovalSnapshot `json:"approval,omitempty"`
}
type HTTPContextReader struct {
	endpoint, token string
	client          *http.Client
}

func ReadServiceToken(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("private action authority token file is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("private action authority token file is unavailable")
	}
	token := strings.TrimSpace(string(data))
	if len(token) < 32 || len(token) > 4096 || strings.ContainsAny(token, "\r\n ") {
		return "", fmt.Errorf("private action authority token is invalid")
	}
	return token, nil
}
func NewHTTPContextReader(endpoint, token string) (*HTTPContextReader, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Hostname() == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") || token == "" {
		return nil, fmt.Errorf("private action authority endpoint and token are required")
	}
	return &HTTPContextReader{endpoint: endpoint, token: token, client: &http.Client{Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (c *HTTPContextReader) read(ctx context.Context, request contextRequest) (contextResponse, error) {
	var response contextResponse
	encoded, err := json.Marshal(request)
	if err != nil {
		return response, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(encoded))
	if err != nil {
		return response, fmt.Errorf("action authority request unavailable")
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return response, fmt.Errorf("action authority is unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return response, fmt.Errorf("action authority denied the request")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 128*1024+1))
	if err != nil || len(data) > 128*1024 {
		return response, fmt.Errorf("action authority response is invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&response) != nil {
		return response, fmt.Errorf("action authority response is incompatible")
	}
	return response, nil
}
func (c *HTTPContextReader) GetActionContext(ctx context.Context, id string) (ContextSnapshot, error) {
	r, err := c.read(ctx, contextRequest{RunID: id})
	if err != nil {
		return ContextSnapshot{}, err
	}
	if r.Context == nil {
		return ContextSnapshot{}, fmt.Errorf("trusted run context is missing")
	}
	return *r.Context, nil
}
func (c *HTTPContextReader) VerifyActionApproval(ctx context.Context, request ApprovalRequest) (ApprovalSnapshot, error) {
	r, err := c.read(ctx, contextRequest{RunID: request.Binding.RunID, Approval: &request})
	if err != nil {
		return ApprovalSnapshot{}, err
	}
	if r.Approval == nil {
		return ApprovalSnapshot{}, fmt.Errorf("verified action approval is missing")
	}
	return *r.Approval, nil
}

// NewContextHandler belongs to orchestrator's private service boundary. The
// public API and model tools do not expose its authentication material.
func NewContextHandler(token string, reader ContextReader) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if len(token) < 32 || subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
			http.Error(w, "private authority authentication required", http.StatusUnauthorized)
			return
		}
		var request contextRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128*1024))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&request) != nil || request.RunID == "" {
			http.Error(w, "invalid authority request", http.StatusBadRequest)
			return
		}
		if reader == nil {
			http.Error(w, "authority unavailable", http.StatusServiceUnavailable)
			return
		}
		result := contextResponse{}
		if request.Approval != nil {
			if request.Approval.Binding.RunID != request.RunID {
				http.Error(w, "incompatible approval request", http.StatusForbidden)
				return
			}
			a, err := reader.VerifyActionApproval(r.Context(), *request.Approval)
			if err != nil {
				http.Error(w, "approval denied", http.StatusForbidden)
				return
			}
			result.Approval = &a
		} else {
			c, err := reader.GetActionContext(r.Context(), request.RunID)
			if err != nil {
				http.Error(w, "context denied", http.StatusForbidden)
				return
			}
			result.Context = &c
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	})
}
