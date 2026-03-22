package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/butler/butler/apps/browser-bridge/internal/protocol"
)

var ErrNoNativeClient = errors.New("no native browser client is connected")

type writeFunc func(protocol.Request) error

type Dispatcher struct {
	write writeFunc

	mu      sync.Mutex
	pending map[string]chan protocol.Response
}

type DispatchError struct {
	Code    string
	Message string
}

func (e *DispatchError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return e.Code
}

func NewDispatcher(write writeFunc) *Dispatcher {
	return &Dispatcher{
		write:   write,
		pending: make(map[string]chan protocol.Response),
	}
}

func (d *Dispatcher) DispatchAction(ctx context.Context, params protocol.ActionDispatchParams) (protocol.ActionDispatchResult, error) {
	if d == nil || d.write == nil {
		return protocol.ActionDispatchResult{}, ErrNoNativeClient
	}

	requestID := fmt.Sprintf("action-%d", time.Now().UTC().UnixNano())
	responseCh := make(chan protocol.Response, 1)

	d.mu.Lock()
	d.pending[requestID] = responseCh
	d.mu.Unlock()

	defer func() {
		d.mu.Lock()
		delete(d.pending, requestID)
		d.mu.Unlock()
	}()

	if err := d.write(protocol.Request{
		ID:     requestID,
		Method: "action.dispatch",
		Params: params,
	}); err != nil {
		return protocol.ActionDispatchResult{}, err
	}

	select {
	case <-ctx.Done():
		return protocol.ActionDispatchResult{}, ctx.Err()
	case response := <-responseCh:
		if !response.OK {
			if response.Error != nil {
				return protocol.ActionDispatchResult{}, &DispatchError{
					Code:    response.Error.Code,
					Message: response.Error.Message,
				}
			}
			return protocol.ActionDispatchResult{}, &DispatchError{Code: "dispatch_failed", Message: "dispatch failed"}
		}
		var result protocol.ActionDispatchResult
		payload, err := json.Marshal(response.Result)
		if err != nil {
			return protocol.ActionDispatchResult{}, fmt.Errorf("marshal action dispatch result: %w", err)
		}
		if err := json.Unmarshal(payload, &result); err != nil {
			return protocol.ActionDispatchResult{}, fmt.Errorf("decode action dispatch result: %w", err)
		}
		return result, nil
	}
}

func (d *Dispatcher) Resolve(response protocol.Response) bool {
	if d == nil {
		return false
	}

	d.mu.Lock()
	responseCh, ok := d.pending[response.ID]
	if ok {
		delete(d.pending, response.ID)
	}
	d.mu.Unlock()
	if !ok {
		return false
	}

	responseCh <- response
	return true
}
