package budget

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/jackc/pgx/v5"
)

// Recovery compatibility pin, mirrored from the owning journal's reproduced
// revision2 contract. No caller flag can qualify an unknown local preparation.
const reviewedLocalFillContract = "4b4d58b1ddfadd6a2c56e5b2596fe85d2f4edad96105f52304e9c6fa3546aff3"

func preparationProofHash(raw string) string {
	v := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(v[:])
}

// verifiedPreparationNoWriteTx does not make a local result known. It checks
// every scoped non-read uncertainty and permits ordinary policy evaluation only
// when each is an independently reviewed, terminal, credential-free frozen-local
// preparation. It never authorizes/replays an action or changes spending.
func verifiedPreparationNoWriteTx(ctx context.Context, tx pgx.Tx, owner, root, currentRun string) (bool, error) {
	var candidate bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM action_effects WHERE state='effect_unknown' AND initial_binding->>'owner_id'=$1 AND initial_binding->>'tool_name'='browser.prepare_fill' AND initial_binding->>'contract_hash'=$2)`, owner, reviewedLocalFillContract).Scan(&candidate); err != nil || !candidate {
		return false, err
	}
	rows, err := tx.Query(ctx, `WITH RECURSIVE tree AS(SELECT task_id FROM tasks WHERE task_id=$2 UNION ALL SELECT t.task_id FROM tasks t JOIN tree p ON t.parent_task_id=p.task_id)
 SELECT e.action_id,COALESCE(e.initial_binding->>'run_id',''),COALESCE(e.initial_binding->>'tool_call_id',''),
 e.state='effect_unknown' AND e.dispatch_outcome='unknown' AND e.initial_binding->>'tool_name'='browser.prepare_fill' AND e.initial_binding->>'contract_revision'='2' AND e.initial_binding->>'contract_hash'=$4 AND e.initial_binding->>'effect'='prepare' AND e.initial_binding->>'task_id'='' AND e.initial_binding->>'action_id'=e.action_id
 AND e.initial_binding->'credential_refs'='[]'::jsonb AND COALESCE(e.initial_binding->>'extension_id','')='' AND COALESCE(e.initial_binding->>'converter_id','')=''
 AND e.policy->>'retry_safety'='unsafe' AND e.policy->>'verification_method'='local_browser_preparation_v1' AND COALESCE(e.policy->>'idempotency_header','')='' AND COALESCE(e.policy->>'scope_origin','')='' AND COALESCE(e.policy->>'verification_url','')=''
 AND length(e.policy->'local_preparation'->>'browser_session_id')>0 AND CASE WHEN jsonb_typeof(e.policy->'local_preparation'->'observation_version')='number' THEN (e.policy->'local_preparation'->>'observation_version')::numeric>0 ELSE false END
 AND e.initial_binding->>'resource'='browser:'||(e.policy->'local_preparation'->>'browser_session_id')||':'||(e.policy->'local_preparation'->>'expected_url')
 AND EXISTS(SELECT 1 FROM action_intents a JOIN runs old ON old.run_id=a.run_id JOIN accepted_inputs i ON i.run_id=old.run_id JOIN sessions ss ON ss.session_key=i.session_key
 JOIN input_stopped_attempts f ON f.input_id=i.input_id
 JOIN action_authorizations g ON g.authorization_id=e.authorization_id AND g.action_id=a.action_id
 JOIN action_effect_dispatches d ON d.authorization_id=g.authorization_id AND d.action_id=e.action_id AND d.execution_action_id=a.action_id
 WHERE a.effect_id=e.action_id AND a.action_id=e.action_id AND a.binding=e.initial_binding AND a.action_hash=e.initial_binding->>'action_hash' AND a.effect_fingerprint=e.fingerprint
 AND old.run_id<>$3 AND old.run_id=e.initial_binding->>'run_id' AND a.tool_call_id=e.initial_binding->>'tool_call_id' AND old.session_key=i.session_key AND ss.user_id=$1 AND i.owner_id=$1
 AND old.task_id IS NULL AND i.task_id IS NULL AND old.current_state='failed' AND old.input_event_id=i.event_id AND i.status='failed' AND i.claim_id='' AND i.claim_expires_at IS NULL
 AND f.owner_id=$1 AND f.run_id=old.run_id AND f.session_key=i.session_key AND f.classification='known'
 AND g.state='unknown' AND g.action_hash=a.action_hash AND g.grant_data->'binding'=a.binding AND d.outcome='unknown' AND d.finished_at IS NOT NULL
 AND NOT EXISTS(SELECT 1 FROM session_ownership l WHERE l.run_id=old.run_id AND l.lease_id<>'' AND l.expires_at>clock_timestamp())
 AND NOT EXISTS(SELECT 1 FROM action_effect_dispatches other WHERE other.action_id=e.action_id AND other.outcome='executing'))
 FROM action_effects e WHERE e.initial_binding->>'owner_id'=$1 AND (e.initial_binding->>'task_id' IN(SELECT task_id FROM tree) OR (e.initial_binding->>'task_id'='' AND e.initial_binding->>'run_id' IN(SELECT run_id FROM runs WHERE session_key=$2))) AND e.state IN('effect_unknown','executing') AND e.initial_binding->>'effect'<>'read' ORDER BY e.action_id LIMIT 21`, owner, root, currentRun, reviewedLocalFillContract)
	if err != nil {
		return false, err
	}
	type pending struct {
		id, run, call string
		valid         bool
	}
	pendingRows := []pending{}
	for rows.Next() {
		var p pending
		var valid *bool
		if err = rows.Scan(&p.id, &p.run, &p.call, &valid); err != nil {
			break
		}
		p.valid = valid != nil && *valid
		pendingRows = append(pendingRows, p)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return false, err
	}
	if len(pendingRows) == 0 || len(pendingRows) > 20 {
		return false, nil
	}
	const method = "pinned_preparation_no_write_v1"
	for _, p := range pendingRows {
		if !p.valid {
			return false, nil
		}
		var raw string
		err = tx.QueryRow(ctx, `SELECT evidence::text FROM action_effect_evidence WHERE evidence_id=$1 AND action_id=$2 AND source_action_id=$2`, "evidence_"+preparationProofHash(p.id+"\x00"+method), p.id).Scan(&raw)
		if err == pgx.ErrNoRows {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		var proof struct {
			Action       string `json:"action_id"`
			SourceAction string `json:"source_action_id"`
			Run          string `json:"source_run_id"`
			Call         string `json:"source_tool_call_id"`
			Source       string `json:"source"`
			Method       string `json:"method"`
			Verdict      string `json:"verdict"`
			Digest       string `json:"result_digest"`
			Artifact     string `json:"artifact_digest"`
		}
		if json.Unmarshal([]byte(raw), &proof) != nil || proof.Action != p.id || proof.SourceAction != p.id || proof.Run != p.run || proof.Call != p.call || proof.Source != "owning_local_preparation_contract" || proof.Method != method || proof.Verdict != "effect_unknown" || proof.Artifact != "" || proof.Digest != preparationProofHash(p.id+"\x00"+reviewedLocalFillContract+"\x00no_write_local_result_unknown") {
			return false, nil
		}
	}
	return true, nil
}
