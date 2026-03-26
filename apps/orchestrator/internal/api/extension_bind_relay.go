package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultExtensionBindRelayQueueSize = 16
	defaultExtensionBindRelayMaxWait   = 45 * time.Second
	extensionBindRelayQueueGlobal      = "__global__"
)

type ExtensionBindDispatchParams struct {
	RunID             string
	SessionKey        string
	ToolCallID        string
	RequestedVia      string
	RequestSource     string
	BrowserHint       string
	BrowserInstanceID string
}

type ExtensionPendingBindDispatch struct {
	DispatchID         string `json:"dispatch_id"`
	RunID              string `json:"run_id"`
	SessionKey         string `json:"session_key"`
	ToolCallID         string `json:"tool_call_id"`
	RequestedVia       string `json:"requested_via,omitempty"`
	RequestSource      string `json:"request_source,omitempty"`
	BrowserHint        string `json:"browser_hint,omitempty"`
	BrowserInstanceID  string `json:"browser_instance_id,omitempty"`
	PreferredBrowserID string `json:"preferred_browser_instance_id,omitempty"`
}

type ExtensionBindResolveResult struct {
	BrowserInstanceID string
	BrowserHint       string
	TabCandidates     []createBindCandidateEntry
}

type extensionBindDispatchOutcome struct {
	Result ExtensionBindResolveResult
	Err    *ExtensionRelayError
}

type extensionBindDispatchRequest struct {
	DispatchID string
	QueueKey   string
	Params     ExtensionBindDispatchParams
	ResultCh   chan extensionBindDispatchOutcome
	ExpiresAt  time.Time
	Cancelled  atomic.Bool
}

type ExtensionBindRelay struct {
	mu       sync.Mutex
	queues   map[string]chan *extensionBindDispatchRequest
	inflight map[string]*extensionBindDispatchRequest
}

func NewExtensionBindRelay() *ExtensionBindRelay {
	return &ExtensionBindRelay{
		queues:   make(map[string]chan *extensionBindDispatchRequest),
		inflight: make(map[string]*extensionBindDispatchRequest),
	}
}

func (r *ExtensionBindRelay) Dispatch(ctx context.Context, params ExtensionBindDispatchParams) (ExtensionBindResolveResult, error) {
	if r == nil {
		return ExtensionBindResolveResult{}, &ExtensionRelayError{Code: "host_unavailable", Message: "extension bind relay is not configured"}
	}
	if strings.TrimSpace(params.RunID) == "" || strings.TrimSpace(params.SessionKey) == "" {
		return ExtensionBindResolveResult{}, &ExtensionRelayError{Code: "invalid_request", Message: "run_id and session_key are required"}
	}

	dispatchCtx, cancel := withDispatchTimeout(ctx, defaultExtensionBindRelayMaxWait)
	defer cancel()

	queueKey := extensionBindRelayQueueGlobal
	if strings.TrimSpace(params.BrowserInstanceID) != "" {
		queueKey = strings.TrimSpace(params.BrowserInstanceID)
	}
	request := &extensionBindDispatchRequest{
		DispatchID: newExtensionBindDispatchID(),
		QueueKey:   queueKey,
		Params:     params,
		ResultCh:   make(chan extensionBindDispatchOutcome, 1),
		ExpiresAt:  time.Now().UTC().Add(2 * time.Minute),
	}

	queue := r.queueFor(queueKey)
	select {
	case queue <- request:
	case <-dispatchCtx.Done():
		request.Cancelled.Store(true)
		return ExtensionBindResolveResult{}, &ExtensionRelayError{Code: "host_unavailable", Message: "extension bind relay dispatch timed out"}
	}

	select {
	case outcome := <-request.ResultCh:
		if outcome.Err != nil {
			return ExtensionBindResolveResult{}, outcome.Err
		}
		if len(outcome.Result.TabCandidates) == 0 {
			return ExtensionBindResolveResult{}, &ExtensionRelayError{Code: "host_unavailable", Message: "extension returned no tab candidates"}
		}
		return outcome.Result, nil
	case <-dispatchCtx.Done():
		request.Cancelled.Store(true)
		r.dropInflight(request.DispatchID)
		return ExtensionBindResolveResult{}, &ExtensionRelayError{Code: "host_unavailable", Message: "extension bind relay response timed out"}
	}
}

func (r *ExtensionBindRelay) PollNext(ctx context.Context, browserInstanceID string) (ExtensionPendingBindDispatch, bool, error) {
	if r == nil {
		return ExtensionPendingBindDispatch{}, false, &ExtensionRelayError{Code: "host_unavailable", Message: "extension bind relay is not configured"}
	}
	browserInstanceID = strings.TrimSpace(browserInstanceID)
	if browserInstanceID == "" {
		return ExtensionPendingBindDispatch{}, false, &ExtensionRelayError{Code: "invalid_request", Message: "browser_instance_id is required"}
	}

	ownedQueue := r.queueFor(browserInstanceID)
	globalQueue := r.queueFor(extensionBindRelayQueueGlobal)
	for {
		select {
		case request := <-ownedQueue:
			if request == nil {
				continue
			}
			if request.Cancelled.Load() || time.Now().UTC().After(request.ExpiresAt) {
				continue
			}
			r.mu.Lock()
			r.inflight[request.DispatchID] = request
			r.mu.Unlock()
			return ExtensionPendingBindDispatch{
				DispatchID:         request.DispatchID,
				RunID:              request.Params.RunID,
				SessionKey:         request.Params.SessionKey,
				ToolCallID:         request.Params.ToolCallID,
				RequestedVia:       request.Params.RequestedVia,
				RequestSource:      request.Params.RequestSource,
				BrowserHint:        request.Params.BrowserHint,
				BrowserInstanceID:  browserInstanceID,
				PreferredBrowserID: request.Params.BrowserInstanceID,
			}, true, nil
		case request := <-globalQueue:
			if request == nil {
				continue
			}
			if request.Cancelled.Load() || time.Now().UTC().After(request.ExpiresAt) {
				continue
			}
			r.mu.Lock()
			r.inflight[request.DispatchID] = request
			r.mu.Unlock()
			return ExtensionPendingBindDispatch{
				DispatchID:         request.DispatchID,
				RunID:              request.Params.RunID,
				SessionKey:         request.Params.SessionKey,
				ToolCallID:         request.Params.ToolCallID,
				RequestedVia:       request.Params.RequestedVia,
				RequestSource:      request.Params.RequestSource,
				BrowserHint:        request.Params.BrowserHint,
				BrowserInstanceID:  browserInstanceID,
				PreferredBrowserID: request.Params.BrowserInstanceID,
			}, true, nil
		case <-ctx.Done():
			return ExtensionPendingBindDispatch{}, false, nil
		}
	}
}

func (r *ExtensionBindRelay) ResolveSuccess(dispatchID, browserInstanceID string, result ExtensionBindResolveResult) error {
	request, err := r.resolveInflight(dispatchID, browserInstanceID)
	if err != nil {
		return err
	}
	request.ResultCh <- extensionBindDispatchOutcome{Result: result}
	return nil
}

func (r *ExtensionBindRelay) ResolveError(dispatchID, browserInstanceID, code, message string) error {
	request, err := r.resolveInflight(dispatchID, browserInstanceID)
	if err != nil {
		return err
	}
	errPayload := &ExtensionRelayError{
		Code:    firstNonEmptyString(strings.TrimSpace(code), "runtime_error"),
		Message: firstNonEmptyString(strings.TrimSpace(message), "extension bind relay failed"),
	}
	request.ResultCh <- extensionBindDispatchOutcome{Err: errPayload}
	return nil
}

func (r *ExtensionBindRelay) resolveInflight(dispatchID, browserInstanceID string) (*extensionBindDispatchRequest, error) {
	if strings.TrimSpace(dispatchID) == "" {
		return nil, ErrExtensionDispatchNotFound
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	request, ok := r.inflight[dispatchID]
	if !ok {
		return nil, ErrExtensionDispatchNotFound
	}
	if !canResolveBindFromBrowser(request.Params.BrowserInstanceID, browserInstanceID) {
		return nil, ErrExtensionDispatchNotFound
	}
	if request.Cancelled.Load() {
		delete(r.inflight, dispatchID)
		return nil, ErrExtensionDispatchNotFound
	}
	delete(r.inflight, dispatchID)
	return request, nil
}

func (r *ExtensionBindRelay) queueFor(queueKey string) chan *extensionBindDispatchRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	normalized := strings.TrimSpace(queueKey)
	if normalized == "" {
		normalized = extensionBindRelayQueueGlobal
	}
	queue, ok := r.queues[normalized]
	if ok {
		return queue
	}
	queue = make(chan *extensionBindDispatchRequest, defaultExtensionBindRelayQueueSize)
	r.queues[normalized] = queue
	return queue
}

func (r *ExtensionBindRelay) dropInflight(dispatchID string) {
	if strings.TrimSpace(dispatchID) == "" {
		return
	}
	r.mu.Lock()
	delete(r.inflight, dispatchID)
	r.mu.Unlock()
}

func canResolveBindFromBrowser(expected, actual string) bool {
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return true
	}
	return expected == strings.TrimSpace(actual)
}

func newExtensionBindDispatchID() string {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return fmt.Sprintf("bind-dispatch-%d", time.Now().UTC().UnixNano())
	}
	return fmt.Sprintf("bind-dispatch-%d-%s", time.Now().UTC().UnixNano(), hex.EncodeToString(raw[:]))
}
