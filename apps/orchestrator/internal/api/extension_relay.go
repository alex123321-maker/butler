package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultExtensionRelayQueueSize = 32
	defaultExtensionRelayMaxWait   = 30 * time.Second
)

var ErrExtensionDispatchNotFound = errors.New("extension dispatch not found")

type ExtensionRelayError struct {
	Code    string
	Message string
}

func (e *ExtensionRelayError) Error() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.Message) != "" {
		return e.Message
	}
	return e.Code
}

type ExtensionDispatchParams struct {
	SingleTabSessionID string
	BoundTabRef        string
	ActionType         string
	ArgsJSON           string
	BrowserInstanceID  string
}

type ExtensionPendingDispatch struct {
	DispatchID         string `json:"dispatch_id"`
	SingleTabSessionID string `json:"single_tab_session_id"`
	BoundTabRef        string `json:"bound_tab_ref"`
	ActionType         string `json:"action_type"`
	ArgsJSON           string `json:"args_json,omitempty"`
	BrowserInstanceID  string `json:"browser_instance_id,omitempty"`
}

type extensionDispatchOutcome struct {
	Result map[string]any
	Err    *ExtensionRelayError
}

type extensionDispatchRequest struct {
	DispatchID string
	SessionKey string
	Params     ExtensionDispatchParams
	ResultCh   chan extensionDispatchOutcome
	ExpiresAt  time.Time
	Cancelled  atomic.Bool
}

type ExtensionActionRelay struct {
	mu       sync.Mutex
	queues   map[string]chan *extensionDispatchRequest
	inflight map[string]*extensionDispatchRequest
}

func NewExtensionActionRelay() *ExtensionActionRelay {
	return &ExtensionActionRelay{
		queues:   make(map[string]chan *extensionDispatchRequest),
		inflight: make(map[string]*extensionDispatchRequest),
	}
}

func (r *ExtensionActionRelay) Dispatch(ctx context.Context, sessionKey string, params ExtensionDispatchParams) (map[string]any, error) {
	if r == nil {
		return nil, &ExtensionRelayError{Code: "host_unavailable", Message: "extension relay is not configured"}
	}
	if strings.TrimSpace(sessionKey) == "" {
		return nil, &ExtensionRelayError{Code: "invalid_request", Message: "session_key is required"}
	}
	if strings.TrimSpace(params.SingleTabSessionID) == "" || strings.TrimSpace(params.BoundTabRef) == "" || strings.TrimSpace(params.ActionType) == "" {
		return nil, &ExtensionRelayError{Code: "invalid_request", Message: "single_tab_session_id, bound_tab_ref, and action_type are required"}
	}

	dispatchCtx, cancel := withDispatchTimeout(ctx, defaultExtensionRelayMaxWait)
	defer cancel()

	request := &extensionDispatchRequest{
		DispatchID: newExtensionDispatchID(),
		SessionKey: strings.TrimSpace(sessionKey),
		Params:     params,
		ResultCh:   make(chan extensionDispatchOutcome, 1),
		ExpiresAt:  time.Now().UTC().Add(2 * time.Minute),
	}

	queue := r.queueFor(request.SessionKey)
	select {
	case queue <- request:
	case <-dispatchCtx.Done():
		request.Cancelled.Store(true)
		return nil, &ExtensionRelayError{Code: "host_unavailable", Message: "extension relay dispatch timed out"}
	}

	select {
	case outcome := <-request.ResultCh:
		if outcome.Err != nil {
			return nil, outcome.Err
		}
		if outcome.Result == nil {
			return nil, &ExtensionRelayError{Code: "host_unavailable", Message: "extension relay returned empty result"}
		}
		return outcome.Result, nil
	case <-dispatchCtx.Done():
		request.Cancelled.Store(true)
		r.dropInflight(request.DispatchID)
		return nil, &ExtensionRelayError{Code: "host_unavailable", Message: "extension relay response timed out"}
	}
}

func (r *ExtensionActionRelay) PollNext(ctx context.Context, sessionKey string) (ExtensionPendingDispatch, bool, error) {
	if r == nil {
		return ExtensionPendingDispatch{}, false, &ExtensionRelayError{Code: "host_unavailable", Message: "extension relay is not configured"}
	}
	if strings.TrimSpace(sessionKey) == "" {
		return ExtensionPendingDispatch{}, false, &ExtensionRelayError{Code: "invalid_request", Message: "session_key is required"}
	}

	queue := r.queueFor(strings.TrimSpace(sessionKey))
	for {
		select {
		case request := <-queue:
			if request == nil {
				continue
			}
			if request.Cancelled.Load() || time.Now().UTC().After(request.ExpiresAt) {
				continue
			}
			r.mu.Lock()
			r.inflight[request.DispatchID] = request
			r.mu.Unlock()
			return ExtensionPendingDispatch{
				DispatchID:         request.DispatchID,
				SingleTabSessionID: request.Params.SingleTabSessionID,
				BoundTabRef:        request.Params.BoundTabRef,
				ActionType:         request.Params.ActionType,
				ArgsJSON:           request.Params.ArgsJSON,
				BrowserInstanceID:  request.Params.BrowserInstanceID,
			}, true, nil
		case <-ctx.Done():
			return ExtensionPendingDispatch{}, false, nil
		}
	}
}

func (r *ExtensionActionRelay) ResolveSuccess(dispatchID, browserInstanceID string, result map[string]any) error {
	request, err := r.resolveInflight(dispatchID, browserInstanceID)
	if err != nil {
		return err
	}
	request.ResultCh <- extensionDispatchOutcome{Result: result}
	return nil
}

func (r *ExtensionActionRelay) ResolveError(dispatchID, browserInstanceID, code, message string) error {
	request, err := r.resolveInflight(dispatchID, browserInstanceID)
	if err != nil {
		return err
	}
	errPayload := &ExtensionRelayError{
		Code:    firstNonEmptyString(strings.TrimSpace(code), "runtime_error"),
		Message: firstNonEmptyString(strings.TrimSpace(message), "extension relay action failed"),
	}
	request.ResultCh <- extensionDispatchOutcome{Err: errPayload}
	return nil
}

func (r *ExtensionActionRelay) queueFor(sessionKey string) chan *extensionDispatchRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	queue, ok := r.queues[sessionKey]
	if ok {
		return queue
	}
	queue = make(chan *extensionDispatchRequest, defaultExtensionRelayQueueSize)
	r.queues[sessionKey] = queue
	return queue
}

func (r *ExtensionActionRelay) takeInflight(dispatchID string) (*extensionDispatchRequest, error) {
	if strings.TrimSpace(dispatchID) == "" {
		return nil, ErrExtensionDispatchNotFound
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	request, ok := r.inflight[dispatchID]
	if !ok {
		return nil, ErrExtensionDispatchNotFound
	}
	delete(r.inflight, dispatchID)
	return request, nil
}

func (r *ExtensionActionRelay) resolveInflight(dispatchID, browserInstanceID string) (*extensionDispatchRequest, error) {
	if strings.TrimSpace(dispatchID) == "" {
		return nil, ErrExtensionDispatchNotFound
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	request, ok := r.inflight[dispatchID]
	if !ok {
		return nil, ErrExtensionDispatchNotFound
	}
	if !canResolveDispatchFromBrowser(request.Params.BrowserInstanceID, browserInstanceID) {
		return nil, ErrExtensionDispatchNotFound
	}
	if request.Cancelled.Load() {
		delete(r.inflight, dispatchID)
		return nil, ErrExtensionDispatchNotFound
	}
	delete(r.inflight, dispatchID)
	return request, nil
}

func (r *ExtensionActionRelay) dropInflight(dispatchID string) {
	if strings.TrimSpace(dispatchID) == "" {
		return
	}
	r.mu.Lock()
	delete(r.inflight, dispatchID)
	r.mu.Unlock()
}

func newExtensionDispatchID() string {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return fmt.Sprintf("dispatch-%d", time.Now().UTC().UnixNano())
	}
	return fmt.Sprintf("dispatch-%d-%s", time.Now().UTC().UnixNano(), hex.EncodeToString(raw[:]))
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func withDispatchTimeout(ctx context.Context, maxWait time.Duration) (context.Context, context.CancelFunc) {
	if ctx == nil {
		return context.WithTimeout(context.Background(), maxWait)
	}
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining <= maxWait {
			return context.WithCancel(ctx)
		}
	}
	return context.WithTimeout(ctx, maxWait)
}

func canResolveDispatchFromBrowser(expected, actual string) bool {
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return true
	}
	return expected == strings.TrimSpace(actual)
}
