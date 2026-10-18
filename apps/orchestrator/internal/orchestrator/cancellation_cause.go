package orchestrator

import (
	"context"
	"errors"
)

// ownedCancellation contains only a fixed Core reason, never storage/provider
// details. It retains the cancelled classification and the existing ownership
// sentinel independently. Source text and transport events cannot construct it.
type ownedCancellation struct {
	reason  string
	failure error
}

func (e *ownedCancellation) Error() string { return e.reason }
func (e *ownedCancellation) Unwrap() []error {
	return []error{context.Canceled, e.failure}
}

func preserveOwnedCancellation(ctx context.Context, err error) error {
	if !errors.Is(err, context.Canceled) {
		return err
	}
	var cause *ownedCancellation
	if errors.As(context.Cause(ctx), &cause) {
		return cause
	}
	return err
}
