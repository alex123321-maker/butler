package diagnostics

import (
	"context"
	"errors"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
	"testing"
	"time"
)

func TestOperationSemantics(t *testing.T) {
	for _, tc := range []struct {
		name    string
		found   int
		corpus  string
		reasons []string
		want    string
	}{
		{"empty", 0, "empty", nil, "empty"}, {"no_match", 0, "present", nil, "no_match"}, {"unknown", 0, "unknown", nil, "unknown"}, {"disabled", 0, "unknown", []string{"query_search_disabled"}, "disabled"}, {"sql_fallback", 1, "present", []string{"semantic_index_unavailable"}, "success"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := SearchOutcome(tc.found, tc.corpus, tc.reasons); got != tc.want {
				t.Fatal(got)
			}
		})
	}
	for _, tc := range []struct {
		err             error
		outcome, reason string
	}{{catalog.ErrDenied, "denied", "access"}, {catalog.ErrStale, "denied", "stale"}, {catalog.ErrDeleted, "denied", "revoked"}, {&budget.ExhaustedError{}, "denied", "execution_budget"}, {errors.New("PRIVATE_SECRET"), "failed", "backend"}, {context.Canceled, "cancelled", "cancelled"}, {context.DeadlineExceeded, "failed", "timeout"}} {
		o, r := ErrorOutcome(tc.err)
		if o != tc.outcome || r != tc.reason {
			t.Fatalf("%s/%s", o, r)
		}
	}
	if Reason("context_budget") != "context_budget" || Reason("query: PRIVATE_SECRET") != "unknown" {
		t.Fatal("unbounded labels")
	}
}
func TestAggregation(t *testing.T) {
	now := time.Now()
	healthy := Component{Required: true, Effective: true, Status: "healthy", CheckedAt: now}
	for _, tc := range []struct {
		name   string
		checks []Component
		want   string
	}{
		{"healthy_empty_store", []Component{healthy}, "healthy"},
		{"optional_disabled", []Component{healthy, {Status: "disabled", CheckedAt: now}}, "healthy"},
		{"required_failure", []Component{healthy, {Required: true, Status: "failed", CheckedAt: now}}, "unhealthy"},
		{"incomplete", []Component{healthy, {Status: "unknown", CheckedAt: now}}, "unknown"},
		{"expired", []Component{{Required: true, Effective: true, Status: "healthy", CheckedAt: now.Add(-time.Hour)}}, "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Aggregate(tc.checks, now, time.Minute); got != tc.want {
				t.Fatal(got)
			}
		})
	}
}

func TestRestoreReasonSurvivesAccessGuard(t *testing.T) {
	o, r := ErrorOutcome(errors.Join(catalog.ErrDenied, catalog.ErrRestore))
	if o != "failed" || r != "restore_unverified" {
		t.Fatal(o, r)
	}
}
