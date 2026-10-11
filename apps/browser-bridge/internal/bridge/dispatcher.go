package bridge

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

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
	var result protocol.ActionDispatchResult
	e := d.request(ctx, "action.dispatch", params, &result)
	return result, e
}
func (d *Dispatcher) request(ctx context.Context, method string, params any, dest any) error {
	if d == nil || d.write == nil {
		return ErrNoNativeClient
	}

	var nonce [24]byte
	if _, e := rand.Read(nonce[:]); e != nil {
		return e
	}
	requestID := "action-" + hex.EncodeToString(nonce[:])
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
		Method: method,
		Params: params,
	}); err != nil {
		return err
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case response := <-responseCh:
		if !response.OK {
			if response.Error != nil {
				return &DispatchError{
					Code:    response.Error.Code,
					Message: response.Error.Message,
				}
			}
			return &DispatchError{Code: "dispatch_failed", Message: "dispatch failed"}
		}
		payload, err := json.Marshal(response.Result)
		if err != nil {
			return fmt.Errorf("marshal action dispatch result: %w", err)
		}
		if err := json.Unmarshal(payload, dest); err != nil {
			return fmt.Errorf("decode action dispatch result: %w", err)
		}
		return nil
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
