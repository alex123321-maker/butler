package actionauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/butler/butler/internal/credentialcontrol"
	"github.com/butler/butler/internal/externalauth"
	"github.com/jackc/pgx/v5"
)

// Reproduced from 6bee49c3. This pin permits only terminal creation-absence
// review, not execution of an old contract or any provider/POST inference.
const stoppedExternalLoginContract = "34c3f3c573afdd8d3ecc99480972c87afa0df01b9d79be9269865cadc6ca47ab"
const stoppedTaskMessageContract = "2bcb3ec0772e14cc5600d63ac1bd06d5786c4dffe5319c74d5b373c2b48a9539"

// ReviewStoppedTaskMetadataTx proves only absence of pinned atomic request /
// publication creation with distinct Task effect/intent IDs. It cannot infer a
// provider outcome, channel delivery or absence of an arbitrary write.
func ReviewStoppedTaskMetadataTx(ctx context.Context, tx pgx.Tx, input string) error {
	var owner, run string
	err := tx.QueryRow(ctx, `SELECT i.owner_id,i.run_id FROM accepted_inputs i JOIN input_stopped_attempts f USING(input_id)
 JOIN sessions s ON s.session_key=i.session_key AND s.user_id=i.owner_id
 JOIN runs r ON r.run_id=i.run_id AND r.session_key=i.session_key AND r.task_id=i.task_id
 JOIN session_ownership l ON l.session_key=i.session_key
 WHERE i.input_id=$1 AND f.run_id=i.run_id AND f.owner_id=i.owner_id AND f.session_key=i.session_key
 AND f.classification='uncertain' AND i.status='failed' AND r.current_state='failed' AND r.input_event_id=i.event_id
 AND i.task_id IS NOT NULL AND i.claim_id='' AND i.claim_expires_at IS NULL AND (l.lease_id='' OR l.expires_at<=clock_timestamp())
 FOR UPDATE OF l,s,r,i NOWAIT`, input).Scan(&owner, &run)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT DISTINCT e.action_id FROM action_effects e JOIN action_intents a ON a.effect_id=e.action_id WHERE a.run_id=$1 AND e.initial_binding->>'run_id'=$1 AND e.state='effect_unknown' AND e.initial_binding->>'tool_name' IN ($2,'communication.message') ORDER BY e.action_id LIMIT 9`, run, externalauth.RequestTool)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) != nil {
			rows.Close()
			return ErrOwnerAuthority
		}
		ids = append(ids, id)
	}
	rows.Close()
	if rows.Err() != nil {
		return rows.Err()
	}
	if len(ids) > 8 {
		return fmt.Errorf("task metadata review exceeds bounded batch")
	}
	for _, id := range ids {
		e, err := effectIn(ctx, tx, id, true)
		if err != nil {
			return err
		}
		b := e.Binding
		if e.State != StateUnknown || b.OwnerID != owner || b.RunID != run || b.TaskID == "" || b.ToolCallID == "" || b.ActionID != StableActionID(run, b.ToolCallID) || b.Effect != EffectPrepare || b.Resource != "tool:"+b.ToolName || len(b.CredentialRefs) != 0 || b.ExtensionID != "" || b.ConverterID != "" || len(b.DataSources) != 0 || b.BrowserSSO != nil || e.Policy.RetrySafety != "unsafe" || e.Policy.VerificationMethod != "local_runtime_ack" || e.Policy.IdempotencyHeader != "" || e.Policy.ScopeOrigin != "" || e.Policy.VerificationURL != "" || e.Policy.LocalPreparation != nil || e.Policy.Login != nil || e.Policy.SSO != nil {
			continue
		}
		external := b.ToolName == externalauth.RequestTool && b.ContractRevision == "1" && b.ContractHash == stoppedExternalLoginContract && validExternalScope(b) && b.BrowserPage != nil && b.BrowserPage.Valid(true) && b.BrowserPage.Ref == b.ExternalAuth.RPPage && b.BrowserPage.URL == b.ExternalAuth.RPURL
		message := b.ToolName == "communication.message" && b.ContractRevision == "2" && b.ContractHash == stoppedTaskMessageContract && len(b.CommunicationDestinationsHash) == 64 && b.ExternalAuth == nil && b.BrowserPage == nil && b.BrowserScopeURL == ""
		if !external && !message {
			continue
		}
		var raw, fp, hash string
		if tx.QueryRow(ctx, `SELECT binding::text,effect_fingerprint,action_hash FROM action_intents WHERE action_id=$1 AND effect_id=$2 AND run_id=$3 AND tool_call_id=$4`, b.ActionID, id, run, b.ToolCallID).Scan(&raw, &fp, &hash) != nil {
			continue
		}
		var original ActionBinding
		if json.Unmarshal([]byte(raw), &original) != nil {
			continue
		}
		x, _ := json.Marshal(original)
		y, _ := json.Marshal(b)
		check := b
		check.ActionHash = ""
		canonical, _ := json.Marshal(check)
		_, expectedFP, _ := EffectIdentity(b, "")
		if string(x) != string(y) || hash != b.ActionHash || digest(string(canonical)) != b.ActionHash || fp != e.Fingerprint || expectedFP != e.Fingerprint {
			continue
		}
		var settled bool
		if tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM action_effect_dispatches d JOIN action_authorizations g ON g.authorization_id=d.authorization_id
 JOIN runs r ON r.run_id=$4 JOIN accepted_inputs i ON i.input_id=$5
 WHERE d.action_id=$1 AND d.execution_action_id=$2 AND d.authorization_id=$3 AND d.outcome='unknown' AND d.finished_at IS NOT NULL
 AND g.state='unknown' AND g.action_id=$2 AND g.action_hash=$6 AND g.grant_data->'binding'=$7::jsonb AND g.expires_at<=clock_timestamp()
 AND r.task_id=$8 AND r.task_revision=$9 AND i.task_id=$8
 AND NOT EXISTS(SELECT 1 FROM action_effect_dispatches other WHERE other.action_id=$1 AND other.outcome='executing'))`, id, b.ActionID, e.AuthorizationID, run, input, b.ActionHash, string(y), b.TaskID, b.TaskRevision).Scan(&settled) != nil || !settled {
			continue
		}
		var absent bool
		method, source := "stopped_external_login_request_absence_v1", "owning_external_request_store"
		if external {
			absent, err = credentialcontrol.StoppedExternalLoginAbsentTx(ctx, tx, b.ToolCallID)
		} else {
			// The pinned publisher holds the same original session/run/Input
			// locks and checks a live claim before its atomic outbox commit.
			err = tx.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM communication_publications WHERE effect_id=$1)`, id).Scan(&absent)
			method, source = "stopped_task_publication_absence_v1", "owning_publication_store"
		}
		if err != nil {
			return err
		}
		if !absent {
			continue
		}
		var now time.Time
		if tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now) != nil {
			return ErrOwnerAuthority
		}
		proofKind := method
		if external {
			proofKind = "terminal_external_request_absence"
		}
		proof := digest(id + "\x00" + b.ActionID + "\x00" + b.ContractHash + "\x00" + proofKind)
		observed := EffectEvidence{EvidenceID: "evidence_" + digest(id+"\x00"+method), ActionID: id, SourceActionID: b.ActionID, SourceRunID: run, SourceToolCallID: b.ToolCallID, Source: source, Method: method, Verdict: StateAbsent, ResultDigest: proof, ObservedAt: now, RecordedAt: now}
		if insertEvidence(ctx, tx, observed) != nil {
			return ErrOwnerAuthority
		}
		if _, err = tx.Exec(ctx, `UPDATE action_effects SET state='effect_absent',updated_at=clock_timestamp() WHERE action_id=$1 AND state='effect_unknown'`, id); err != nil {
			return err
		}
	}
	return nil
}
