package domain

import "errors"

// Shared sentinel errors for cross-boundary error matching.
// These live in domain so that neither the session package nor the run
// package needs to import the other.

var (
	ErrRunNotFound     = errors.New("run not found")
	ErrRunDuplicate    = errors.New("run already exists for idempotency key")
	ErrSessionNotFound = errors.New("session not found")
)
