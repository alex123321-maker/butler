package actionauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/butler/butler/internal/communicationcontrol"
	"github.com/jackc/pgx/v5"
)

// These hashes were reproduced from commit 4dff78b5. This is a recovery
// compatibility boundary, NOT authorization to execute an obsolete contract.
const stoppedMessageContract = "f86dbe12464322551ff3d386412b54644fd32227fe0011f1bbac13b7db27b6fc"
const stoppedFetchContract = "18a7d901908ac523b3ab1539d3368a9ea9a4b33ce8190d4d37aa58a4a11ff3d2"

// Reproduced from the revision1 public GET compiler in 1e24a910. This
// compatibility proof qualifies only absence of write uncertainty; it never
// upgrades the failed read, imports revision2 absence semantics or replays it.
const stoppedGETFormContract = "504555e5109e2f7478d87fc21a5598c28f1f27b3aaa58830851675cab9bff90f"

// ReviewStoppedLocalTx uses only kernel-owned durable evidence. It can resolve
// the reviewed atomic publication protocol after its executor is fenced out.
// A pinned read may prove absence of WRITE uncertainty without proving that the
// read succeeded; its UNKNOWN journal state and spending remain unchanged.
func ReviewStoppedLocalTx(ctx context.Context, tx pgx.Tx, input string) (map[string]bool, error) {
	reads := map[string]bool{}
	var owner, run, session string
	err := tx.QueryRow(ctx, `SELECT i.owner_id,i.run_id,i.session_key
 FROM accepted_inputs i JOIN input_stopped_attempts f USING(input_id)
 JOIN sessions s ON s.session_key=i.session_key AND s.user_id=i.owner_id
 JOIN runs r ON r.run_id=i.run_id AND r.session_key=i.session_key
 JOIN session_ownership l ON l.session_key=i.session_key
 WHERE i.input_id=$1 AND f.run_id=i.run_id AND f.owner_id=i.owner_id AND f.session_key=i.session_key
 AND f.classification='uncertain' AND i.status='failed' AND r.current_state='failed' AND r.input_event_id=i.event_id
 AND i.task_id IS NULL AND r.task_id IS NULL AND i.claim_id='' AND i.claim_expires_at IS NULL
 AND (l.lease_id='' OR l.expires_at<=clock_timestamp())
 FOR UPDATE OF l,s,r,i NOWAIT`, input).Scan(&owner, &run, &session)
	if errors.Is(err, pgx.ErrNoRows) {
		return reads, nil
	}
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT e.action_id FROM action_effects e JOIN action_intents a ON a.effect_id=e.action_id
 WHERE a.run_id=$1 AND e.initial_binding->>'run_id'=$1 AND e.state='effect_unknown'
 ORDER BY e.action_id LIMIT 21`, run)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			break
		}
		ids = append(ids, id)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return nil, err
	}
	if len(ids) > 20 {
		return nil, fmt.Errorf("stopped local review exceeds bounded batch")
	}
	for _, id := range ids {
		e, err := effectIn(ctx, tx, id, true)
		if err != nil {
			return nil, err
		}
		if e.State != StateUnknown {
			continue
		}
		b := e.Binding
		if b.OwnerID != owner || b.RunID != run || b.TaskID != "" || b.ActionID != id || b.ToolCallID == "" || len(b.CredentialRefs) != 0 || b.ExtensionID != "" || b.ConverterID != "" {
			continue
		}
		var intent, fp, hash, binding string
		if err = tx.QueryRow(ctx, `SELECT action_id,effect_fingerprint,action_hash,binding::text FROM action_intents WHERE effect_id=$1 AND run_id=$2 AND tool_call_id=$3`, id, run, b.ToolCallID).Scan(&intent, &fp, &hash, &binding); err != nil {
			return nil, err
		}
		var ib ActionBinding
		if json.Unmarshal([]byte(binding), &ib) != nil {
			continue
		}
		// Compare canonical serialized bindings (slices make direct equality invalid).
		x, _ := json.Marshal(ib)
		y, _ := json.Marshal(b)
		if string(x) != string(y) || fp != e.Fingerprint || hash != b.ActionHash || intent != id {
			continue
		}
		check := b
		check.ActionHash = ""
		encoded, _ := json.Marshal(check)
		_, fingerprint, _ := EffectIdentity(b, "")
		if digest(string(encoded)) != b.ActionHash || fingerprint != e.Fingerprint {
			continue
		}
		method, source := "", ""
		verdict := StateUnknown
		proof := ""
		switch {
		case ((b.ToolName == "web.fetch" && b.ContractHash == stoppedFetchContract) || (b.ToolName == "browser.submit_get_form" && b.ContractHash == stoppedGETFormContract)) && b.ContractRevision == "1" && b.Effect == EffectRead && e.Policy.RetrySafety == "read_only" && e.Policy.VerificationMethod == "read_result" && e.Policy.IdempotencyHeader == "" && e.Policy.ScopeOrigin == "" && e.Policy.VerificationURL == "" && e.Policy.LocalPreparation == nil:
			method, source = "pinned_read_no_write_v1", "owning_read_contract"
			proof = digest(id + "\x00" + b.ContractHash + "\x00no_write_result_unknown")
			reads[id] = true
		case b.ToolName == "communication.message" && b.ContractRevision == "1" && b.ContractHash == stoppedMessageContract && b.Effect == EffectPrepare && b.Resource == "tool:communication.message" && len(b.CommunicationDestinationsHash) == 64 && e.Policy.RetrySafety == "unsafe" && e.Policy.VerificationMethod == "local_runtime_ack" && e.Policy.IdempotencyHeader == "" && e.Policy.ScopeOrigin == "" && e.Policy.VerificationURL == "" && e.Policy.LocalPreparation == nil:
			// Require a permanently consumed dispatch, not an unclaimed proposal.
			var dispatched bool
			if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM action_effect_dispatches d JOIN action_authorizations g ON g.authorization_id=d.authorization_id
 WHERE d.action_id=$1 AND d.execution_action_id=$2 AND d.authorization_id=$3 AND d.outcome='unknown' AND d.finished_at IS NOT NULL AND g.state='unknown' AND g.action_id=$2 AND g.action_hash=$4
 AND NOT EXISTS(SELECT 1 FROM action_effect_dispatches other WHERE other.action_id=$1 AND other.outcome='executing'))`, id, intent, e.AuthorizationID, b.ActionHash).Scan(&dispatched); err != nil {
				return nil, err
			}
			if !dispatched {
				continue
			}
			var pOwner, pRun, pSession, pInput, pCall, pKind, pTask, payloadHash string
			var taskVersion, constraintVersion int64
			var rawReceipt []byte
			err = tx.QueryRow(ctx, `SELECT owner_id,run_id,session_key,input_id,tool_call_id,kind,COALESCE(task_id,''),task_revision,constraints_revision,payload_hash,receipt_json
 FROM communication_publications WHERE effect_id=$1 FOR SHARE`, id).Scan(&pOwner, &pRun, &pSession, &pInput, &pCall, &pKind, &pTask, &taskVersion, &constraintVersion, &payloadHash, &rawReceipt)
			if errors.Is(err, pgx.ErrNoRows) {
				// Publisher takes the same session/run/Input locks and checks a live
				// claim at commit. No late publication can cross this terminal fence.
				verdict = StateAbsent
				proof = digest(id + "\x00" + input + "\x00" + run + "\x00terminal_publication_absence")
			} else if err != nil {
				return nil, err
			} else {
				var receipt communicationcontrol.Receipt
				if pOwner != owner || pRun != run || pSession != session || pInput != input || pCall != b.ToolCallID || pKind != "message" || pTask != "" || taskVersion != b.TaskRevision || constraintVersion != b.ConstraintsRevision || len(payloadHash) != 64 || json.Unmarshal(rawReceipt, &receipt) != nil || receipt.Status != "accepted" || receipt.PublicationRef != "publication:"+digest(id) || len(receipt.Delivery) == 0 {
					continue
				}
				verdict = StateConfirmed
				proof = digest(string(rawReceipt) + "\x00" + payloadHash)
			}
			method, source = "stopped_publication_v1", "owning_publication_store"
		default:
			continue
		}
		var at time.Time
		if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&at); err != nil {
			return nil, err
		}
		ev := EffectEvidence{EvidenceID: "evidence_" + digest(id+"\x00"+method), ActionID: id, SourceActionID: intent, SourceToolCallID: b.ToolCallID, SourceRunID: run, Source: source, Method: method, Verdict: verdict, ResultDigest: proof, ObservedAt: at, RecordedAt: at}
		var exists bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM action_effect_evidence WHERE evidence_id=$1)`, ev.EvidenceID).Scan(&exists); err != nil {
			return nil, err
		}
		if !exists {
			if err = insertEvidence(ctx, tx, ev); err != nil {
				return nil, err
			}
		}
		if exists {
			var raw []byte
			if err = tx.QueryRow(ctx, `SELECT evidence FROM action_effect_evidence WHERE evidence_id=$1`, ev.EvidenceID).Scan(&raw); err != nil {
				return nil, err
			}
			var old EffectEvidence
			if json.Unmarshal(raw, &old) != nil || old.ActionID != id || old.SourceActionID != intent || old.SourceRunID != run || old.SourceToolCallID != b.ToolCallID || old.Source != source || old.Method != method || old.ResultDigest != proof || old.Verdict != verdict {
				return nil, fmt.Errorf("stopped local evidence mismatch")
			}
		}
		if verdict != StateUnknown {
			if _, err = tx.Exec(ctx, `UPDATE action_effects SET state=$2,updated_at=clock_timestamp() WHERE action_id=$1 AND state='effect_unknown'`, id, string(verdict)); err != nil {
				return nil, err
			}
		}
	}
	return reads, nil
}
