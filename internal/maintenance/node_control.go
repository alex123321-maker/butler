package maintenance

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/subtle"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/butler/butler/internal/actionauth"
)

const NodeControlPath = "/private/maintenance/node"

// Node control carries signed exact records, never owner permission, author
// verdicts, target selection, signing requests or Docker configuration.
type NodeControlRequest struct {
	Operation string `json:"operation"`
	Raw       []byte `json:"raw"`
	Signature []byte `json:"signature"`
}

type NodeControlResponse struct {
	Claim   *EffectClaim `json:"claim,omitempty"`
	Outcome *NodeOutcome `json:"outcome,omitempty"`
}

func validNodeControlToken(token string) bool {
	return len(token) >= 32 && len(token) <= 512 && !strings.ContainsAny(token, " \t\r\n\x00")
}

// NewNodeControlHandler exposes live one-use dispatch and signed measured
// outcomes for one operator target. It cannot reserve a claim, sign bytes,
// stop/resume a target or perform its physical operation. Store repeats every
// authority, review, permission, observation, expiry and unresolved-effect check.
func NewNodeControlHandler(store Store, owner, target, token string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if !validNodeControlToken(token) || len(r.Header.Values("Authorization")) != 1 || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 {
			http.Error(w, "private node authentication required", http.StatusUnauthorized)
			return
		}
		if owner == "" || target == "" || store.Pool == nil {
			http.Error(w, "node control unavailable", http.StatusServiceUnavailable)
			return
		}
		if r.Method != http.MethodPost || r.URL.Path != NodeControlPath || r.URL.RawPath != "" || r.URL.RawQuery != "" || r.URL.ForceQuery {
			http.Error(w, "fixed node control POST required", http.StatusMethodNotAllowed)
			return
		}
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 32768))
		if err != nil {
			http.Error(w, "bounded node request required", http.StatusBadRequest)
			return
		}
		if _, err := actionauth.ArgumentsHash(string(raw)); err != nil {
			http.Error(w, "unambiguous node request required", http.StatusBadRequest)
			return
		}
		var request NodeControlRequest
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF {
			http.Error(w, "typed node request required", http.StatusBadRequest)
			return
		}
		canonical, err := json.Marshal(request)
		if err != nil || !bytes.Equal(canonical, raw) {
			http.Error(w, "canonical node request required", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		response := NodeControlResponse{}
		switch request.Operation {
		case "dispatch":
			var claim EffectClaim
			claim, err = store.AdmitDispatch(ctx, owner, target, request.Raw, request.Signature)
			response.Claim = &claim
		case "outcome":
			var outcome NodeOutcome
			outcome, err = store.RecordOutcome(ctx, owner, target, request.Raw, request.Signature)
			response.Outcome = &outcome
		default:
			http.Error(w, "unsupported node operation", http.StatusBadRequest)
			return
		}
		if err != nil {
			http.Error(w, "node operation rejected or unavailable", http.StatusConflict)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	})
}

// NodeControlClient is private startup configuration. Losing a dispatch reply
// never retries it: the controller may already have consumed the one-use gate.
// The executor must also verify its original signed claim before acting.
type NodeControlClient struct {
	endpoint string
	token    string
	http     *http.Client
}

func NewNodeControlClient(endpoint, token string) (*NodeControlClient, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, ErrState
	}
	ip := net.ParseIP(u.Hostname())
	privateHTTP := u.Scheme == "http" && (u.Host == "maintenance-controller:8080" || ip != nil && ip.IsLoopback())
	if !validNodeControlToken(token) || u.Hostname() == "" || u.User != nil || u.Opaque != "" || u.RawQuery != "" || u.ForceQuery || strings.Contains(endpoint, "#") || u.Path != NodeControlPath || u.RawPath != "" || u.Scheme != "https" && !privateHTTP {
		return nil, ErrState
	}
	transport := &http.Transport{Proxy: nil, DisableKeepAlives: true, ResponseHeaderTimeout: 5 * time.Second}
	client := &http.Client{Transport: transport, Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return ErrState }}
	return &NodeControlClient{endpoint: endpoint, token: token, http: client}, nil
}

func (client *NodeControlClient) Close() {
	if client != nil && client.http != nil {
		client.http.CloseIdleConnections()
	}
}

func (client *NodeControlClient) call(ctx context.Context, operation string, raw, signature []byte) (NodeControlResponse, error) {
	if client == nil || client.http == nil || len(raw) == 0 || len(raw) > 16384 || len(signature) != ed25519.SignatureSize {
		return NodeControlResponse{}, ErrState
	}
	body, err := json.Marshal(NodeControlRequest{Operation: operation, Raw: raw, Signature: signature})
	if err != nil {
		return NodeControlResponse{}, ErrState
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.endpoint, bytes.NewReader(body))
	if err != nil {
		return NodeControlResponse{}, ErrState
	}
	request.Header.Set("Authorization", "Bearer "+client.token)
	request.Header.Set("Content-Type", "application/json")
	response, err := client.http.Do(request)
	if err != nil {
		return NodeControlResponse{}, ErrState
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return NodeControlResponse{}, ErrState
	}
	value, err := io.ReadAll(io.LimitReader(response.Body, 32769))
	if err != nil || len(value) > 32768 {
		return NodeControlResponse{}, ErrState
	}
	if _, err := actionauth.ArgumentsHash(string(value)); err != nil {
		return NodeControlResponse{}, ErrState
	}
	var result NodeControlResponse
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&result) != nil || decoder.Decode(new(any)) != io.EOF {
		return NodeControlResponse{}, ErrState
	}
	canonical, err := json.Marshal(result)
	if err != nil || !bytes.Equal(canonical, bytes.TrimSpace(value)) {
		return NodeControlResponse{}, ErrState
	}
	var echoed []byte
	if operation == "dispatch" && result.Claim != nil && result.Outcome == nil {
		echoed, err = json.Marshal(result.Claim)
	} else if operation == "outcome" && result.Outcome != nil && result.Claim == nil {
		echoed, err = json.Marshal(result.Outcome)
	} else {
		return NodeControlResponse{}, ErrState
	}
	if err != nil || !bytes.Equal(echoed, raw) {
		return NodeControlResponse{}, ErrState
	}
	return result, nil
}

func (client *NodeControlClient) AdmitDispatch(ctx context.Context, raw, signature []byte) (EffectClaim, error) {
	result, err := client.call(ctx, "dispatch", raw, signature)
	if err != nil {
		return EffectClaim{}, err
	}
	return *result.Claim, nil
}

func (client *NodeControlClient) RecordOutcome(ctx context.Context, raw, signature []byte) (NodeOutcome, error) {
	result, err := client.call(ctx, "outcome", raw, signature)
	if err != nil {
		return NodeOutcome{}, err
	}
	return *result.Outcome, nil
}
