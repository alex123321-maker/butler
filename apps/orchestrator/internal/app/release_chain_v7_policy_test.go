package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/jackc/pgx/v5"
)

// The zero value preserves the old V5/V6 profile and metadata shape. Only the
// independently pinned new window declares the longer bounded request profile
// and both trusted nominal-timeout phases used by ordinary Core.
type releaseChainHighWindowPolicy struct {
	ProposalRetention           bool   `json:"proposal_retention_required,omitempty"`
	MaximumPrivateProposalBytes int    `json:"maximum_private_proposal_bytes,omitempty"`
	Generation                  int    `json:"maximum_generation_tokens,omitempty"`
	TimeoutStages               string `json:"safe_timeout_stages,omitempty"`
	AttemptMillis               int    `json:"maximum_attempt_ms,omitempty"`
	CaseMillis                  int    `json:"maximum_case_ms,omitempty"`
	GlobalMillis                int    `json:"maximum_global_ms,omitempty"`
}

func (p releaseChainHighWindowPolicy) valid() bool {
	return p == (releaseChainHighWindowPolicy{}) || p == releaseChainV7Policy() || p == releaseChainV8Policy()
}
func (p releaseChainHighWindowPolicy) generation() int {
	if p.Generation == 0 {
		return 32768
	}
	return p.Generation
}
func (p releaseChainHighWindowPolicy) attemptMillis() int {
	if p.AttemptMillis == 0 {
		return 120000
	}
	return p.AttemptMillis
}
func (p releaseChainHighWindowPolicy) caseMillis() int {
	if p.CaseMillis == 0 {
		return 1200000
	}
	return p.CaseMillis
}
func (p releaseChainHighWindowPolicy) globalMillis() int {
	if p.GlobalMillis == 0 {
		return 3600000
	}
	return p.GlobalMillis
}
func (p releaseChainHighWindowPolicy) json() []byte { raw, _ := json.Marshal(p); return raw }
func releaseChainV7Policy() releaseChainHighWindowPolicy {
	return releaseChainHighWindowPolicy{Generation: 32768, TimeoutStages: "http_or_stream", AttemptMillis: 300000, CaseMillis: 2400000, GlobalMillis: 7200000}
}

func (w *releaseChainV5Window) metadataSQL() string {
	const old = "json_build_object('native_baseline',native_baseline,'attempts',attempts,'prior_history_hash',prior_history_hash,'prerequisite_hash',prerequisite_hash,'expires_at',expires_at,'maximum_new_attempts',maximum_new_attempts,'provider_stage_observation',provider_stage_observation,'kernel_operation_binding',kernel_operation_binding)"
	if w.policy == (releaseChainHighWindowPolicy{}) {
		return old
	}
	return "(" + old + "::jsonb || jsonb_build_object('protocol_policy',protocol_policy))"
}

func releaseChainHighAcceptedMeasurements(m releaseChainV6RetryMeasurements, err error) releaseChainV6RetryMeasurements {
	if err != nil {
		return releaseChainV6RetryMeasurements{}
	}
	return m
}

func TestReleaseChainV7ProfileAndPredicates(t *testing.T) {
	p := releaseChainV7Policy()
	w := &releaseChainV5Window{policy: p}
	if !p.valid() || p.generation() != 32768 || p.attemptMillis() != 300000 || p.caseMillis() != 2400000 || p.globalMillis() != 7200000 || (releaseChainHighWindowPolicy{Generation: 8192, TimeoutStages: "authentication"}).valid() {
		t.Fatal("unfixed profile admitted")
	}
	if releaseChainV5Configuration().OutputTokens != 32768 {
		t.Fatal("conservative Core quote changed")
	}
	cfg := releaseChainHighConfigurationFor(p)
	if cfg.Policy.ModelMillis != 300000 || cfg.Policy.TaskLifetimeMillis != 2400000 || cfg.FixtureTimeout != 40*time.Minute || cfg.ChildBudget.ActiveMillis != 1320000 || cfg.ChildBudget.ModelTurns != 4 || cfg.Policy.Day.ModelTurns != 76 {
		t.Fatal("request/case/child/shared quotes inconsistent")
	}
	sql := releaseChainHighSafeTimeoutSQL(w, "r")
	if !strings.Contains(sql, "IN('http_round_trip','response_stream')") || !strings.Contains(sql, "r.total_tokens BETWEEN r.input_tokens+r.output_tokens AND r.input_tokens+32768") || !strings.Contains(sql, "r.elapsed_ms BETWEEN 0 AND 300000") {
		t.Fatal("new timeout profile not exact")
	}
	if strings.Contains(releaseChainHighSafeTimeoutSQL(&releaseChainV5Window{}, "r"), "http_round_trip") {
		t.Fatal("old window phase silently expanded")
	}
	if !strings.Contains(releaseChainHighCompleteNativeSQL(w, "r"), "r.provider_operation_stage='response_stream'") {
		t.Fatal("successful HTTP-only observation accepted")
	}
	if strings.Contains((&releaseChainV5Window{}).metadataSQL(), "protocol_policy") || !strings.Contains(w.metadataSQL(), "protocol_policy") {
		t.Fatal("legacy/new metadata not explicit")
	}
}

func TestReleaseChainV7UnvalidatedRetryCountersStayZero(t *testing.T) {
	candidate := releaseChainV6RetryMeasurements{Failures: 1, UnknownCost: 1, UnknownVersion: 1, UnknownMeasurement: 1}
	if releaseChainHighAcceptedMeasurements(candidate, errNativeFixtureDenied) != (releaseChainV6RetryMeasurements{}) {
		t.Fatal("denied candidate reported accepted")
	}
	if releaseChainHighAcceptedMeasurements(candidate, nil) != candidate {
		t.Fatal("independently checked retry measurement lost")
	}
}

func TestReleaseChainV7PolicyCannotReconstructOrRefillIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx := context.Background()
	var schema string
	if pool.QueryRow(ctx, "SELECT current_schema()").Scan(&schema) != nil {
		t.Fatal("schema unavailable")
	}
	pin, history := strings.Repeat("a", 64), strings.Repeat("b", 64)
	deadline := time.Now().UTC().Add(time.Minute)
	w, err := createReleaseChainHighWindowWithPolicy(ctx, pool, pin, history, 539, schema, deadline, 76, true, releaseChainV7Policy(), true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = createReleaseChainHighWindowInSchemaAt(ctx, pool, pin, history, 539, schema, deadline, 76, true, true); err == nil {
		t.Fatal("default old policy reconstructed V7")
	}
	if _, err = createReleaseChainHighWindowWithPolicy(ctx, pool, pin, history, 539, schema, deadline, 78, true, releaseChainV7Policy(), true); err == nil {
		t.Fatal("spent admissions refunded")
	}
	var at int
	var policy json.RawMessage
	if pool.QueryRow(ctx, "SELECT attempts,protocol_policy FROM fixture_chain_v5_high_window").Scan(&at, &policy) != nil || at != 539 || string(policy) == "{}" {
		t.Fatal("policy not durable")
	}
	if _, err = pool.Exec(ctx, "UPDATE fixture_chain_v5_high_window SET attempts=attempts+1,protocol_policy='{}'"); err == nil {
		t.Fatal("SQL policy changed")
	}
	copy := *w
	copy.policy = releaseChainHighWindowPolicy{}
	copy.kernelOperations = false
	guard := func(context.Context, pgx.Tx, string) (bool, error) { return true, nil }
	if _, _, err = copy.reserve(ctx, "normal_sent", "start", "root", "run", nil, guard); err == nil {
		t.Fatal("reconstructed wrapper admitted")
	}
	if pool.QueryRow(ctx, "SELECT attempts FROM fixture_chain_v5_high_window").Scan(&at) != nil || at != 539 {
		t.Fatal("denial spent inference")
	}
}

func TestReleaseChainV7SafeRetryPredecessorAdmissionIntegration(t *testing.T) {
	testReleaseChainSafeRetryPredecessorAdmission(t, releaseChainV7Policy(), map[string]bool{
		"http_phase": true, "unknown_timeout_exact_started_retry": true, "entry_response_stream_timeout": true,
		"unknown_phase": true, "wrong_retry_operation": true, "second_retry": true, "partial_failed_usage": true,
		"failed_final": true, "cancelled_terminal": true, "child_after_completed_retry_start": true, "next_input_after_completed_retry": true,
	})
}

func TestReleaseChainV7MeasuredRetryCountersIntegration(t *testing.T) {
	testReleaseChainMeasuredRetryFailureGate(t, releaseChainV7Policy(), map[string]bool{"http_stage": true, "unknown_all_cost_and_version": true, "no_fresh_start": true, "fresh_unknown_cost": true, "more_than_one_retry": true})
}

func releaseChainV8Policy() releaseChainHighWindowPolicy {
	p := releaseChainV7Policy()
	p.ProposalRetention = true
	p.MaximumPrivateProposalBytes = releaseChainProposalMaxBytes
	return p
}
