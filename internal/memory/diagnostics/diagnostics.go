// Package diagnostics defines content-free memory observations. Availability,
// operation outcomes and quality evidence are deliberately independent.
package diagnostics

import (
	"context"
	"errors"
	"time"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/security/content"
)

const Version = "memory-observations-v1"

var ErrContextBudget = errors.New("mandatory memory context exceeds allowance")

type Component struct {
	Configured bool      `json:"configured"`
	Effective  bool      `json:"effective"`
	Required   bool      `json:"required"`
	Status     string    `json:"status"`
	Reason     string    `json:"reason,omitempty"`
	CheckedAt  time.Time `json:"checked_at"`
}

// Aggregate cannot turn missing or expired checks into healthy. Disabled optional
// features remain visible without making required stores unavailable.
func Aggregate(checks []Component, now time.Time, maxAge time.Duration) string {
	status := "healthy"
	if len(checks) == 0 {
		return "unknown"
	}
	for _, c := range checks {
		if c.Required && (c.Status == "failed" || c.Status == "unhealthy") {
			return "unhealthy"
		}
		if c.CheckedAt.IsZero() || now.Sub(c.CheckedAt) > maxAge || c.CheckedAt.After(now) || c.Status == "unknown" || c.Required && (!c.Effective || c.Status == "disabled") {
			if status != "degraded" {
				status = "unknown"
			}
		} else if c.Status == "degraded" || c.Status == "failed" || c.Status == "unhealthy" {
			status = "degraded"
		}
	}
	return status
}

// ErrorOutcome uses identities, never error text, SQL details or provider bodies.
func ErrorOutcome(err error) (outcome, reason string) {
	var exhausted *budget.ExhaustedError
	switch {
	case err == nil:
		return "success", "none"
	case errors.Is(err, context.Canceled):
		return "cancelled", "cancelled"
	case errors.Is(err, context.DeadlineExceeded):
		return "failed", "timeout"
	case errors.Is(err, ErrContextBudget):
		return "failed", "context_budget"
	case errors.Is(err, catalog.ErrStale):
		return "denied", "stale"
	case errors.Is(err, catalog.ErrDeleted):
		return "denied", "revoked"
	case errors.Is(err, catalog.ErrRestore):
		return "failed", "restore_unverified"
	case errors.Is(err, catalog.ErrDenied), errors.Is(err, content.ErrSecret):
		return "denied", "access"
	case errors.As(err, &exhausted):
		return "denied", "execution_budget"
	default:
		return "failed", "backend"
	}
}

// Reason bounds labels even if a backend supplies an unexpected diagnostic.
func Reason(value string) string {
	switch value {
	case "none", "access", "stale", "revoked", "context_budget", "execution_budget", "cancelled", "timeout", "backend", "restore_unverified",
		"query_search_disabled", "local_query_backend_unavailable", "hybrid_base", "reranked", "semantic_index_unavailable", "embedding_unavailable", "reranker_unavailable", "reranker_input_limit", "exact_identifier_only", "retrieval_context_limited", "local_query_backend_unverified", "query_input_limit", "duplicate", "item_limit", "class_quota", "context_limit", "profile_relevance_or_limit":
		return value
	default:
		return "unknown"
	}
}

// SearchOutcome requires positive knowledge of the permitted corpus to call a
// zero result empty/no_match. A zero result alone proves neither.
func SearchOutcome(found int, corpus string, reasons []string) string {
	if found > 0 {
		return "success"
	}
	for _, r := range reasons {
		if r == "query_search_disabled" {
			return "disabled"
		}
	}
	switch corpus {
	case "empty":
		return "empty"
	case "present":
		return "no_match"
	}
	return "unknown"
}
