package service

import (
	"context"
	"time"

	"github.com/butler/butler/internal/memory/diagnostics"
)

type OperationObservation struct {
	Stage   string   `json:"stage"`
	Outcome string   `json:"outcome"`
	Reasons []string `json:"reasons,omitempty"`
}
type observationKey struct{}

func (s *Service) observeOperation(ctx context.Context, stage string, started time.Time, err error, outcome string, reasons []string) {
	reason := "none"
	if err != nil {
		outcome, reason = diagnostics.ErrorOutcome(err)
	}
	seen := map[string]bool{}
	values := append([]string(nil), reasons...)
	if reason != "none" {
		values = append(values, reason)
	}
	bounded := []string{}
	for _, value := range values {
		r := diagnostics.Reason(value)
		if seen[r] || r == "none" || r == "hybrid_base" || r == "reranked" || r == "exact_identifier_only" {
			continue
		}
		seen[r] = true
		bounded = append(bounded, r)
	}
	if observations, ok := ctx.Value(observationKey{}).(*[]OperationObservation); ok {
		*observations = append(*observations, OperationObservation{Stage: stage, Outcome: outcome, Reasons: bounded})
	}
	m := s.config.Metrics
	if m == nil {
		return
	}
	_ = m.IncrCounter("butler_memory_operations_total", map[string]string{"stage": stage, "outcome": outcome})
	_ = m.ObserveHistogram("butler_memory_operation_duration_seconds", time.Since(started).Seconds(), map[string]string{"stage": stage})
	for _, r := range bounded {
		_ = m.IncrCounter("butler_memory_limitations_total", map[string]string{"stage": stage, "reason": r})
	}
}

func (s *Service) observeSelection(found int, d SelectionDiagnostics) {
	m := s.config.Metrics
	if m == nil {
		return
	}
	// Counts use observations, rather than one counter increment per item.
	_ = m.ObserveHistogram("butler_memory_items", float64(found), map[string]string{"disposition": "found"})
	selected := 0
	for _, n := range d.Selected {
		selected += n
	}
	_ = m.ObserveHistogram("butler_memory_items", float64(selected), map[string]string{"disposition": "selected"})
	omitted := 0
	for r, n := range d.Omitted {
		omitted += n
		if n > 0 {
			_ = m.IncrCounter("butler_memory_limitations_total", map[string]string{"stage": "context", "reason": diagnostics.Reason(r)})
		}
	}
	if d.Omitted["item_limit"]+d.Omitted["class_quota"]+d.Omitted["context_limit"] > 0 {
		_ = m.IncrCounter("butler_memory_limitations_total", map[string]string{"stage": "context", "reason": "context_budget"})
	}
	_ = m.ObserveHistogram("butler_memory_items", float64(omitted), map[string]string{"disposition": "excluded"})
}
