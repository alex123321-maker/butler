package actionauth

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/butler/butler/internal/budget"

	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrOwnerOutcome = errors.New("owner outcome review unavailable")
var ErrOwnerOutcomePending = errors.New("owner outcome wait commit may have completed")

type OwnerOutcomeReview struct {
	OwnerID             string    `json:"-"`
	SessionKey          string    `json:"-"`
	TaskID              string    `json:"-"`
	TaskRevision        int64     `json:"-"`
	ConstraintsRevision int64     `json:"-"`
	ID                  string    `json:"review_id"`
	EffectID            string    `json:"effect_id"`
	Fingerprint         string    `json:"fingerprint"`
	RunID               string    `json:"run_id"`
	ToolCallID          string    `json:"tool_call_id"`
	SourceKind          string    `json:"source_kind"`
	State               string    `json:"state"`
	Revision            int64     `json:"revision"`
	ResourceHint        string    `json:"resource_hint"`
	ExpiresAt           time.Time `json:"expires_at"`
}

type OwnerOutcomeResolution struct {
	ReviewID     string      `json:"review_id"`
	EffectID     string      `json:"effect_id"`
	ToolCallID   string      `json:"tool_call_id"`
	State        EffectState `json:"effect_state"`
	EvidenceRefs []string    `json:"evidence_refs"`
	Source       string      `json:"source"`
}

type OwnerOutcomeDecision struct {
	ID                string `json:"review_id"`
	Fingerprint       string `json:"fingerprint"`
	Revision          int64  `json:"revision"`
	Outcome           string `json:"outcome"`
	RecipientChecked  bool   `json:"recipient_checked"`
	ProcessingSettled bool   `json:"processing_settled"`
	ReceiptReference  string `json:"receipt_reference"`
}

type OwnerOutcomes struct {
	Pool            *pgxpool.Pool
	AfterDecisionTx func(context.Context, pgx.Tx, OwnerOutcomeReview) error
}

// RequestTx is kernel-only. A live request requires the existing worker token;
// a historical request requires an authenticated owner and an expired context.
// Neither source is accepted from tool args or an arbitrary caller-selected URL.
func (s *OwnerOutcomes) RequestTx(ctx context.Context, tx pgx.Tx, effectID, observationCall string, historical bool) (OwnerOutcomeReview, error) {
	e, err := effectIn(ctx, tx, effectID, false)
	if err != nil || e.State != StateUnknown || e.DispatchOutcome != "dispatch_completed" {
		return OwnerOutcomeReview{}, ErrOwnerOutcome
	}
	var b ActionBinding
	var raw string
	err = tx.QueryRow(ctx, `SELECT a.binding::text FROM action_effect_dispatches d JOIN action_intents a ON a.action_id=d.execution_action_id WHERE d.action_id=$1 AND d.authorization_id=$2 AND d.outcome='dispatch_completed' AND d.finished_at IS NOT NULL`, effectID, e.AuthorizationID).Scan(&raw)
	if err != nil || json.Unmarshal([]byte(raw), &b) != nil || b.OwnerID == "" {
		return OwnerOutcomeReview{}, ErrOwnerOutcome
	}
	switch b.ToolName {
	case "browser.click", "browser.fill", "browser.type":
	default:
		return OwnerOutcomeReview{}, ErrOwnerOutcome
	}
	var argsRaw, resultRaw, sessionKey, runState string
	var finished time.Time
	err = tx.QueryRow(ctx, `SELECT c.args::text,c.result::text,c.finished_at,r.session_key,r.current_state FROM tool_calls c JOIN runs r USING(run_id) JOIN sessions ss ON ss.session_key=r.session_key AND ss.user_id=$3 WHERE c.run_id=$1 AND c.tool_call_id=$2 AND c.tool_name=$4 AND c.status='effect_unknown' AND c.finished_at IS NOT NULL`, b.RunID, b.ToolCallID, b.OwnerID, b.ToolName).Scan(&argsRaw, &resultRaw, &finished, &sessionKey, &runState)
	var args struct {
		ID string `json:"browser_session_id"`
	}
	var observed struct {
		OK      bool   `json:"ok"`
		ID      string `json:"browser_session_id"`
		Version uint64 `json:"observation_version"`
	}
	if err != nil || json.Unmarshal([]byte(argsRaw), &args) != nil || json.Unmarshal([]byte(resultRaw), &observed) != nil || !observed.OK || observed.ID != args.ID || observed.Version == 0 {
		return OwnerOutcomeReview{}, ErrOwnerOutcome
	}
	id, err := hex.DecodeString(args.ID)
	if err != nil || len(id) != 16 || hex.EncodeToString(id) != args.ID {
		return OwnerOutcomeReview{}, ErrOwnerOutcome
	}
	argsHash, err := ArgumentsHash(argsRaw)
	if err != nil || argsHash != b.ArgsHash {
		return OwnerOutcomeReview{}, ErrOwnerOutcome
	}
	var lease, leaseRun string
	var epoch int64
	var leaseExpires, now time.Time
	if tx.QueryRow(ctx, `SELECT lease_id,run_id,fence_epoch,expires_at,clock_timestamp() FROM session_ownership WHERE session_key=$1 FOR UPDATE`, sessionKey).Scan(&lease, &leaseRun, &epoch, &leaseExpires, &now) != nil {
		return OwnerOutcomeReview{}, ErrOwnerOutcome
	}
	kind := "live_frozen"
	var expires time.Time
	if historical {
		actor, ok := owneridentity.FromContext(ctx)
		if !ok || actor.OwnerID != b.OwnerID || (lease != "" && leaseExpires.After(now)) || (runState != "failed" && runState != "cancelled" && runState != "timed_out") {
			return OwnerOutcomeReview{}, ErrOwnerOutcome
		}
		rows, err := tx.Query(ctx, `SELECT result::text FROM tool_calls WHERE run_id=$1 AND tool_name='browser.open' AND status='completed' AND finished_at IS NOT NULL`, b.RunID)
		if err != nil {
			return OwnerOutcomeReview{}, ErrOwnerOutcome
		}
		found := false
		for rows.Next() {
			var body string
			if rows.Scan(&body) != nil {
				rows.Close()
				return OwnerOutcomeReview{}, ErrOwnerOutcome
			}
			var birth struct {
				ID      string    `json:"browser_session_id"`
				Expires time.Time `json:"expires_at"`
			}
			if json.Unmarshal([]byte(body), &birth) == nil && birth.ID == args.ID && !birth.Expires.IsZero() && !birth.Expires.After(now) {
				found = true
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil || !found {
			return OwnerOutcomeReview{}, ErrOwnerOutcome
		}
		kind = "historical_expired"
		expires = now.Add(20 * time.Minute)
	} else {
		token, ok := ownership.TokenFromContext(ctx)
		if !ok || token.RunID != b.RunID || token.SessionKey != sessionKey || token.LeaseID != lease || token.FenceEpoch != epoch || leaseRun != b.RunID || !leaseExpires.After(now) || runState != "tool_running" || observationCall == "" {
			return OwnerOutcomeReview{}, ErrOwnerOutcome
		}
		var proof, readEffect, readAction string
		err = tx.QueryRow(ctx, `SELECT c.result::text,e.action_id,a.action_id FROM tool_calls c JOIN action_intents a ON a.run_id=c.run_id AND a.tool_call_id=c.tool_call_id JOIN action_effects e ON e.action_id=a.effect_id WHERE c.run_id=$1 AND c.tool_call_id=$2 AND c.tool_name='browser.freeze_snapshot' AND c.status='completed' AND c.finished_at>=$3 AND e.state='effect_confirmed' AND a.binding->>'effect'='read' AND a.binding->>'contract_revision'='1' AND a.binding->>'owner_id'=$4 AND a.binding->>'task_id'=$5 AND a.binding->>'resource'=$6 AND a.binding->'credential_refs'='[]'::jsonb AND e.policy->>'retry_safety'='read_only' AND e.policy->>'verification_method'='read_result'`, b.RunID, observationCall, finished, b.OwnerID, b.TaskID, "browser:"+args.ID+":").Scan(&proof, &readEffect, &readAction)
		if err != nil {
			return OwnerOutcomeReview{}, ErrOwnerOutcome
		}
		var captured bool
		normalized, normalizeErr := budget.NormalizeResult(proof)
		if normalizeErr != nil || normalized == "" {
			return OwnerOutcomeReview{}, ErrOwnerOutcome
		}
		if tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM action_effect_evidence WHERE action_id=$1 AND source_action_id=$2 AND evidence->>'source'='runtime_response' AND evidence->>'verdict'='effect_confirmed' AND evidence->>'source_run_id'=$3 AND evidence->>'source_tool_call_id'=$4 AND evidence->>'normalized_result_digest'=$5)`, readEffect, readAction, b.RunID, observationCall, normalized).Scan(&captured) != nil || !captured {
			return OwnerOutcomeReview{}, ErrOwnerOutcome
		}
		var view struct {
			OK      bool      `json:"ok"`
			ID      string    `json:"browser_session_id"`
			Network string    `json:"network_state"`
			Expires time.Time `json:"expires_at"`
		}
		if json.Unmarshal([]byte(proof), &view) != nil || !view.OK || view.ID != args.ID || view.Network != "frozen" || !view.Expires.After(now) {
			return OwnerOutcomeReview{}, ErrOwnerOutcome
		}
		expires = view.Expires
	}
	u, _ := url.Parse(strings.TrimPrefix(b.Resource, "browser:"+args.ID+":"))
	hint := b.ToolName
	if u != nil && u.Host != "" {
		hint = u.Scheme + "://" + u.Host + u.EscapedPath()
	}
	r := OwnerOutcomeReview{OwnerID: b.OwnerID, SessionKey: sessionKey, TaskID: b.TaskID, TaskRevision: b.TaskRevision, ConstraintsRevision: b.ConstraintsRevision, ID: "review_" + randomID(), EffectID: e.ActionID, Fingerprint: e.Fingerprint, RunID: b.RunID, ToolCallID: b.ToolCallID, SourceKind: kind, State: "pending", Revision: 1, ResourceHint: hint, ExpiresAt: expires}
	locked, err := effectIn(ctx, tx, effectID, true)
	if err != nil || locked.State != StateUnknown || locked.Fingerprint != e.Fingerprint || locked.AuthorizationID != e.AuthorizationID {
		return OwnerOutcomeReview{}, ErrOwnerOutcome
	}
	if historical {
		if _, err = tx.Exec(ctx, `UPDATE browser_owner_outcome_reviews SET state='unresolved',decision_digest='expired',decided_at=clock_timestamp() WHERE effect_id=$1 AND state='pending' AND expires_at<=clock_timestamp()`, effectID); err != nil {
			return OwnerOutcomeReview{}, ErrOwnerOutcome
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO browser_owner_outcome_reviews(review_id,effect_id,fingerprint,execution_action_id,owner_id,run_id,session_key,tool_call_id,browser_session_id,task_id,task_revision,constraints_revision,observation_call_id,source_kind,resource_hint,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`, r.ID, r.EffectID, r.Fingerprint, b.ActionID, b.OwnerID, b.RunID, sessionKey, b.ToolCallID, args.ID, b.TaskID, b.TaskRevision, b.ConstraintsRevision, observationCall, kind, hint, expires)
	if err != nil {
		return OwnerOutcomeReview{}, ErrOwnerOutcome
	}
	return r, nil
}

const ownerReviewColumns = `review_id,effect_id,fingerprint,run_id,tool_call_id,source_kind,state,revision,resource_hint,expires_at,owner_id,session_key,task_id,task_revision,constraints_revision`

func scanOwnerReview(row pgx.Row) (OwnerOutcomeReview, error) {
	var r OwnerOutcomeReview
	err := row.Scan(&r.ID, &r.EffectID, &r.Fingerprint, &r.RunID, &r.ToolCallID, &r.SourceKind, &r.State, &r.Revision, &r.ResourceHint, &r.ExpiresAt, &r.OwnerID, &r.SessionKey, &r.TaskID, &r.TaskRevision, &r.ConstraintsRevision)
	return r, err
}

func (s *OwnerOutcomes) Historical(ctx context.Context, id, fingerprint string) (OwnerOutcomeReview, error) {
	a, ok := owneridentity.FromContext(ctx)
	if !ok || id == "" || fingerprint == "" {
		return OwnerOutcomeReview{}, ErrOwnerOutcome
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return OwnerOutcomeReview{}, ErrOwnerOutcome
	}
	defer tx.Rollback(ctx)
	var matches bool
	if tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM action_effects WHERE action_id=$1 AND fingerprint=$2 AND initial_binding->>'owner_id'=$3)`, id, fingerprint, a.OwnerID).Scan(&matches) != nil || !matches {
		return OwnerOutcomeReview{}, ErrOwnerOutcome
	}
	r, err := s.RequestTx(ctx, tx, id, "", true)
	if err != nil {
		return r, err
	}
	if tx.Commit(ctx) != nil {
		return OwnerOutcomeReview{}, ErrOwnerOutcome
	}
	return r, nil
}

func (s *OwnerOutcomes) Pending(ctx context.Context) ([]OwnerOutcomeReview, error) {
	a, ok := owneridentity.FromContext(ctx)
	if !ok {
		return nil, ErrOwnerOutcome
	}
	rows, err := s.Pool.Query(ctx, `SELECT `+ownerReviewColumns+` FROM browser_owner_outcome_reviews WHERE owner_id=$1 AND state='pending' ORDER BY created_at LIMIT 50`, a.OwnerID)
	if err != nil {
		return nil, ErrOwnerOutcome
	}
	defer rows.Close()
	out := []OwnerOutcomeReview{}
	for rows.Next() {
		r, err := scanOwnerReview(rows)
		if err != nil {
			return nil, ErrOwnerOutcome
		}
		out = append(out, r)
	}
	if rows.Err() != nil {
		return nil, ErrOwnerOutcome
	}
	return out, nil
}

// Decide accepts a new factual owner report. The authenticated channel supplies
// Actor; no public body field, model verdict or old approval can supply it.
func (s *OwnerOutcomes) Decide(ctx context.Context, q OwnerOutcomeDecision) (OwnerOutcomeReview, error) {
	a, ok := owneridentity.FromContext(ctx)
	known := q.Outcome == string(StateConfirmed) || q.Outcome == string(StateAbsent)
	if !ok || q.ID == "" || q.Fingerprint == "" || q.Revision != 1 || (!known && q.Outcome != "unknown") || (known && (!q.RecipientChecked || !q.ProcessingSettled || strings.TrimSpace(q.ReceiptReference) == "")) || len(q.ReceiptReference) > 2048 {
		return OwnerOutcomeReview{}, ErrOwnerOutcome
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return OwnerOutcomeReview{}, ErrOwnerOutcome
	}
	defer tx.Rollback(ctx)
	// Lock ownership before run/Input/effect/review, matching dispatch ordering.
	var key string
	if tx.QueryRow(ctx, `SELECT session_key FROM browser_owner_outcome_reviews WHERE review_id=$1 AND owner_id=$2`, q.ID, a.OwnerID).Scan(&key) != nil {
		return OwnerOutcomeReview{}, ErrOwnerOutcome
	}
	var live bool
	if tx.QueryRow(ctx, `SELECT lease_id<>'' AND expires_at>clock_timestamp() FROM session_ownership WHERE session_key=$1 FOR UPDATE`, key).Scan(&live) != nil || live {
		return OwnerOutcomeReview{}, ErrOwnerOutcome
	}
	r, err := scanOwnerReview(tx.QueryRow(ctx, `SELECT `+ownerReviewColumns+` FROM browser_owner_outcome_reviews WHERE review_id=$1 AND owner_id=$2 FOR UPDATE`, q.ID, a.OwnerID))
	if err != nil || r.Fingerprint != q.Fingerprint || r.Revision != q.Revision {
		return OwnerOutcomeReview{}, ErrOwnerOutcome
	}
	decisionHash := digest(mustOwnerJSON(q))
	deliveryHash := digest(a.Channel + "\x00" + a.DeliveryID)
	if r.State != "pending" {
		var replay bool
		if tx.QueryRow(ctx, `SELECT decision_digest=$2 AND delivery_digest=$3 FROM browser_owner_outcome_reviews WHERE review_id=$1`, q.ID, decisionHash, deliveryHash).Scan(&replay) != nil || !replay {
			return OwnerOutcomeReview{}, ErrOwnerOutcome
		}
		return r, nil
	}
	var fresh bool
	if tx.QueryRow(ctx, `SELECT expires_at>clock_timestamp() FROM browser_owner_outcome_reviews WHERE review_id=$1`, r.ID).Scan(&fresh) != nil || !fresh {
		return OwnerOutcomeReview{}, ErrOwnerOutcome
	}
	e, err := effectIn(ctx, tx, r.EffectID, true)
	if err != nil || e.State != StateUnknown || e.Fingerprint != r.Fingerprint || e.Binding.OwnerID != a.OwnerID {
		return OwnerOutcomeReview{}, ErrOwnerOutcome
	}
	var ownerStillCurrent bool
	if tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM runs rr JOIN sessions ss ON ss.session_key=rr.session_key WHERE rr.run_id=$1 AND rr.session_key=$2 AND ss.user_id=$3 AND COALESCE(rr.task_id,'')=$4)`, r.RunID, r.SessionKey, a.OwnerID, r.TaskID).Scan(&ownerStillCurrent) != nil || !ownerStillCurrent {
		return OwnerOutcomeReview{}, ErrOwnerOutcome
	}
	var execution, taskID string
	var rev, cr int64
	if tx.QueryRow(ctx, `SELECT execution_action_id,task_id,task_revision,constraints_revision FROM browser_owner_outcome_reviews WHERE review_id=$1`, r.ID).Scan(&execution, &taskID, &rev, &cr) != nil {
		return OwnerOutcomeReview{}, ErrOwnerOutcome
	}
	if r.SourceKind == "live_frozen" && taskID != "" {
		var valid bool
		if tx.QueryRow(ctx, `SELECT owner_id=$2 AND revision=$3 AND constraints_revision=$4 AND status='waiting_for_user' FROM tasks WHERE task_id=$1 FOR SHARE`, taskID, a.OwnerID, rev, cr).Scan(&valid) != nil || !valid {
			return OwnerOutcomeReview{}, ErrOwnerOutcome
		}
	}
	evidenceID := ""
	state := "unresolved"
	if known {
		state = q.Outcome
		evidenceID = "evidence_" + randomID()
		now := time.Now().UTC()
		evidence := EffectEvidence{EvidenceID: evidenceID, ActionID: r.EffectID, SourceActionID: execution, SourceRunID: r.RunID, SourceToolCallID: r.ToolCallID, Source: "authenticated_owner", Method: "owner_outcome_attestation_v1", Verdict: EffectState(state), ResultDigest: decisionHash, ArtifactDigest: digest(q.ReceiptReference), ObservedAt: now, RecordedAt: now}
		if _, err = tx.Exec(ctx, `INSERT INTO action_effect_evidence(evidence_id,action_id,source_action_id,evidence,observed_at,recorded_at) VALUES($1,$2,$3,$4::jsonb,$5,$6)`, evidenceID, r.EffectID, execution, mustOwnerJSON(evidence), now, now); err != nil {
			return OwnerOutcomeReview{}, ErrOwnerOutcome
		}
		if _, err = tx.Exec(ctx, `UPDATE action_effects SET state=$2,updated_at=clock_timestamp() WHERE action_id=$1 AND state='effect_unknown'`, r.EffectID, state); err != nil {
			return OwnerOutcomeReview{}, ErrOwnerOutcome
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE browser_owner_outcome_reviews SET state=$2,decision_digest=$3,delivery_digest=$4,receipt_digest=$5,evidence_id=$6,decided_at=clock_timestamp() WHERE review_id=$1`, r.ID, state, decisionHash, deliveryHash, digest(q.ReceiptReference), evidenceID); err != nil {
		return OwnerOutcomeReview{}, ErrOwnerOutcome
	}
	r.State = state
	if s.AfterDecisionTx != nil {
		if err = s.AfterDecisionTx(ctx, tx, r); err != nil {
			return OwnerOutcomeReview{}, ErrOwnerOutcome
		}
	}
	if tx.Commit(ctx) != nil {
		return OwnerOutcomeReview{}, ErrOwnerOutcome
	}
	return r, nil
}

func mustOwnerJSON(value any) string { raw, _ := json.Marshal(value); return string(raw) }

// Expire closes only the control wait. No outcome, permission or receipt is
// inferred from elapsed time. Its callback can queue a no-inference cleanup.
func (s *OwnerOutcomes) Expire(ctx context.Context) error {
	rows, err := s.Pool.Query(ctx, `SELECT review_id,session_key FROM browser_owner_outcome_reviews o WHERE state='pending' AND (expires_at<=clock_timestamp() OR EXISTS(SELECT 1 FROM tasks t WHERE t.task_id=o.task_id AND (t.revision<>o.task_revision OR t.constraints_revision<>o.constraints_revision OR t.status IN ('cancelled','completed','blocked')))) ORDER BY created_at LIMIT 8`)
	if err != nil {
		return ErrOwnerOutcome
	}
	type key struct{ id, session string }
	keys := []key{}
	for rows.Next() {
		var k key
		if rows.Scan(&k.id, &k.session) != nil {
			rows.Close()
			return ErrOwnerOutcome
		}
		keys = append(keys, k)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return ErrOwnerOutcome
	}
	for _, k := range keys {
		tx, err := s.Pool.Begin(ctx)
		if err != nil {
			return ErrOwnerOutcome
		}
		var live bool
		if tx.QueryRow(ctx, `SELECT lease_id<>'' AND expires_at>clock_timestamp() FROM session_ownership WHERE session_key=$1 FOR UPDATE`, k.session).Scan(&live) != nil || live {
			tx.Rollback(ctx)
			continue
		}
		r, err := scanOwnerReview(tx.QueryRow(ctx, `UPDATE browser_owner_outcome_reviews o SET state='unresolved',decision_digest='expired',decided_at=clock_timestamp() WHERE review_id=$1 AND state='pending' AND (expires_at<=clock_timestamp() OR EXISTS(SELECT 1 FROM tasks t WHERE t.task_id=o.task_id AND (t.revision<>o.task_revision OR t.constraints_revision<>o.constraints_revision OR t.status IN ('cancelled','completed','blocked')))) RETURNING `+ownerReviewColumns, k.id))
		if errors.Is(err, pgx.ErrNoRows) {
			tx.Rollback(ctx)
			continue
		}
		if err != nil {
			tx.Rollback(ctx)
			return ErrOwnerOutcome
		}
		if s.AfterDecisionTx != nil {
			if err = s.AfterDecisionTx(ctx, tx, r); err != nil {
				tx.Rollback(ctx)
				return ErrOwnerOutcome
			}
		}
		if tx.Commit(ctx) != nil {
			return ErrOwnerOutcome
		}
	}
	return nil
}
