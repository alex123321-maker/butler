package catalog

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/butler/butler/internal/budget"
	"github.com/jackc/pgx/v5"
)

type temporalReadObservation struct {
	at     time.Time
	action string
}

func (s *Store) SetTemporalFromBrokerRead(ctx context.Context, p BrokerTemporalUpdate) (TemporalMetadata, error) {
	if _, err := owner(ctx); err != nil {
		return TemporalMetadata{}, err
	}
	if !bounded(p.ClaimID, 1024) || !bounded(p.EvidenceID, 1024) || !p.ConfirmedSameFact || p.ExpectedRevision < 1 || p.ExpectedHeadRevision < 1 || len(p.ExpectedResourceHash) != 64 {
		return TemporalMetadata{}, ErrInvalid
	}
	if _, err := hex.DecodeString(p.ExpectedResourceHash); err != nil {
		return TemporalMetadata{}, ErrInvalid
	}
	page, err := s.ReadAcceptedFacts(ctx, []string{p.ClaimID}, TemporalQuery{})
	if err != nil {
		return TemporalMetadata{}, err
	}
	if len(page.Items) != 1 {
		return TemporalMetadata{}, ErrStale
	}
	f := page.Items[0]
	if f.Pin.HeadRevision != p.ExpectedHeadRevision || f.Temporal.Revision != p.ExpectedRevision || f.Temporal.Class != FreshnessWorkStatus && f.Temporal.Class != FreshnessTechnicalFact {
		return TemporalMetadata{}, ErrStale
	}
	return s.setTemporal(ctx, TemporalUpdate{ClaimID: p.ClaimID, ExpectedRevision: p.ExpectedRevision, ExpectedHeadRevision: p.ExpectedHeadRevision, Validity: f.Temporal.Validity, ValidFrom: f.Temporal.ValidFrom, ValidTo: f.Temporal.ValidTo, Class: f.Temporal.Class}, &p)
}

// Only the Broker's immutable, successful read_result receipt, exact dispatch,
// original prepared fact pin and already charged finite budget can qualify.
// The private resource is compared by hash and never copied into memory/API.
func checkTemporalBrokerReadTx(ctx context.Context, tx pgx.Tx, c Claim, p BrokerTemporalUpdate) (temporalReadObservation, error) {
	var o temporalReadObservation
	var result, normalized string
	err := tx.QueryRow(ctx, `SELECT e.observed_at,e.action_id,tc.result::text,e.evidence->>'normalized_result_digest' FROM action_effect_evidence e
 JOIN action_effects f ON f.action_id=e.action_id
 JOIN action_intents i ON i.action_id=e.source_action_id AND i.effect_id=f.action_id
 JOIN action_authorizations a ON a.authorization_id=f.authorization_id AND a.action_id=i.action_id
 JOIN action_effect_dispatches d ON d.authorization_id=a.authorization_id AND d.execution_action_id=i.action_id AND d.action_id=f.action_id
 JOIN runs r ON r.run_id=i.run_id JOIN sessions ss ON ss.session_key=r.session_key
 JOIN tool_calls tc ON tc.run_id=i.run_id AND tc.tool_call_id=i.tool_call_id AND tc.tool_name='http.request' AND tc.status='completed' AND tc.error IS NULL
 JOIN memory_temporal_run_pins p ON p.run_id=i.run_id AND p.claim_id=$2
 JOIN execution_budget_reservations b ON b.operation_id='effect:'||f.action_id AND b.owner_id=$3
 JOIN execution_loop_observations l ON l.effect_id=f.action_id AND l.owner_id=$3 AND l.category='effect_confirmed'
 WHERE e.evidence_id=$1 AND ss.user_id=$3 AND i.binding->>'owner_id'=$3
 AND i.binding->>'effect'='read' AND i.binding->>'tool_name'='http.request'
 AND f.policy->>'verification_method'='read_result' AND f.state='effect_confirmed' AND f.dispatch_outcome='dispatch_completed'
 AND a.state='dispatch_completed' AND d.outcome='dispatch_completed'
 AND e.evidence->>'source'='runtime_response' AND e.evidence->>'method'='read_result' AND e.evidence->>'verdict'='effect_confirmed'
 AND e.evidence->>'source_run_id'=i.run_id AND e.evidence->>'source_tool_call_id'=i.tool_call_id
 AND e.evidence->>'source_action_id'=i.action_id AND e.evidence->>'action_id'=f.action_id
 AND length(e.evidence->>'result_digest')=64 AND e.evidence->>'error_category' IS NULL
 AND encode(sha256(convert_to(i.binding->>'resource','UTF8')),'hex')=$4
 AND f.initial_binding=i.binding AND a.action_hash=i.action_hash AND i.action_hash=i.binding->>'action_hash'
 AND p.head_revision=$5 AND p.active_id=$2 AND p.temporal_revision=$6
 AND p.original_source_id=$7 AND p.source_revision=$8 AND p.valid_at IS NULL AND p.known_at IS NULL
 AND butler_memory_temporal_run_current(i.run_id)
 AND i.created_at>=p.observed_at AND e.observed_at>=d.dispatched_at AND e.observed_at<=e.recorded_at AND e.recorded_at<=clock_timestamp()
 AND b.created_at<=e.observed_at AND b.request_json->>'cost_known'='true'
 AND (b.request_json->'charge'->>'external_calls')::bigint=1 AND (b.request_json->'charge'->>'active_millis')::bigint>0
 AND jsonb_array_length(b.request_json->'scope_ids')>0`, p.EvidenceID, c.ID, c.OwnerID, p.ExpectedResourceHash, p.ExpectedHeadRevision, p.ExpectedRevision, c.SourceID, c.SourceRevision).Scan(&o.at, &o.action, &result, &normalized)
	if err != nil {
		return o, ErrDenied
	}
	var response struct {
		Status    int  `json:"status_code"`
		Truncated bool `json:"truncated"`
	}
	actual, err := budget.NormalizeResult(result)
	if err != nil || actual != normalized || json.Unmarshal([]byte(result), &response) != nil || response.Status < 200 || response.Status >= 300 || response.Truncated {
		return o, ErrDenied
	}
	return o, nil
}
