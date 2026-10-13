package app

import (
	"context"
	"github.com/butler/butler/internal/transport"
	"github.com/butler/butler/internal/transport/antigravity"
)

func releaseChainV20Policy() releaseChainHighWindowPolicy {
	p := releaseChainV9Policy()
	p.AllowIndependentClosedCases, p.ObserveProviderUnavailableClass = true, true
	return p
}

// Only fixed transport-authored strings enter this optional diagnostic enum.
// The class is not provider status proof, authority, cost, retry eligibility or
// a reconstruction of historical706. Unknown text is never retained.
func releaseChainV20UnavailableClass(err *transport.Error) *string {
	if err == nil || err.Type != transport.ErrorTypeProviderUnavailable {
		return nil
	}
	class := "unknown"
	if err.ProviderName == antigravity.Name {
		switch err.Message {
		case "Antigravity backend is unavailable":
			class = "backend_unavailable"
		case "Antigravity account access was rejected; reconnect through the private control plane":
			class = "account_access_rejected"
		}
	}
	return &class
}

// This is only an independent-case scheduling proof. The closed earlier case
// remains failed and its expense stays nullable/spent. It grants no current
// action or retry: reserve still checks the actual new owner's source, lease,
// kernel reservation and immutable window/deadline before its counter CAS.
func releaseChainV20IndependentClosedCaseSQL(w *releaseChainV5Window) string {
	return `(r.scenario<>$1 AND r.root_task_id<>$5 AND r.finished_at IS NOT NULL AND r.outcome<>'unavailable'
 AND EXISTS(SELECT 1 FROM ` + w.table("fixture_chain_v5_high_case_evidence") + ` closed
 WHERE closed.scenario=r.scenario AND closed.payload_json->>'version'='butler-release-chain-v5-product-witness-v1'
 AND closed.payload_json->>'scenario'=r.scenario AND closed.payload_json->>'test_failed'='true'
 AND closed.payload_json->'postgres'->>'root_task_id'=r.root_task_id
 AND closed.payload_json->'postgres'->>'live_tree_leases'='0'
 AND jsonb_typeof(closed.payload_json->'postgres'->'actions')='array'
 AND NOT EXISTS(SELECT 1 FROM jsonb_array_elements(closed.payload_json->'postgres'->'actions') a
 WHERE (a->>'state'='effect_confirmed' AND a->>'dispatch_outcome'='dispatch_completed') IS NOT TRUE)
 AND NOT EXISTS(SELECT 1 FROM ` + w.table("fixture_chain_v5_high_requests") + ` pending
 WHERE pending.scenario=r.scenario AND (pending.root_task_id<>r.root_task_id OR pending.finished_at IS NULL OR pending.outcome='unavailable' OR pending.finished_at>closed.created_at))))`
}

// Aggregate counters still include every earlier failed/UNKNOWN row and keep
// the whole window NO_GO. This verdict answers only whether this own case has
// complete known results or the unchanged fully proved single timeout retry.
func releaseChainV20CaseMeasurementsDenied(ctx context.Context, w *releaseChainV5Window, scenario string) bool {
	if w == nil || w.policy != releaseChainV20Policy() || !releaseChainV5ScenarioKnown(scenario) {
		return true
	}
	query := `SELECT NOT EXISTS(SELECT 1 FROM ` + w.table("fixture_chain_v5_high_requests") + ` r WHERE r.scenario=$1 AND
 (` + releaseChainHighCompleteNativeSQL(w, "r") + ` OR (` + releaseChainHighSafeTimeoutSQL(w, "r") + ` AND ` + releaseChainV6SealedRetryProofSQL(w) + `)) IS NOT TRUE)
 AND EXISTS(SELECT 1 FROM ` + w.table("fixture_chain_v5_high_requests") + ` WHERE scenario=$1)`
	var allowed bool
	if w.pool.QueryRow(ctx, query, scenario).Scan(&allowed) != nil {
		return true
	}
	return !allowed
}
