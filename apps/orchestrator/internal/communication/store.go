// Package communication owns canonical publications and durable owner questions.
// It has no provider, runtime or channel client and is not enabled at startup.
package communication

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/communicationcontrol"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrDenied = communicationcontrol.ErrDenied
var ErrConflict = communicationcontrol.ErrConflict
var ErrResolved = errors.New("interaction already resolved")

type Store struct {
	Pool   *pgxpool.Pool
	Inputs *durableinput.Store
	// Required current Catalog checks. A missing guard always denies publication,
	// read, submit, send and continuation, rather than trusting copied source refs.
	SourceGuard func(context.Context, pgx.Tx, string) error
	// Required export policy, evaluated separately for every pinned destination
	// at acceptance and immediately before send. Bootstrap supplies neither guard.
	ChannelGuard func(context.Context, pgx.Tx, string, Binding, communicationcontrol.Args) error
}
type Principal struct {
	DestinationsHash                            string
	OwnerID, SessionKey, RunID, InputID, TaskID string
	TaskRevision, ConstraintsRevision           int64
}
type Binding struct {
	ID           string   `json:"binding_id"`
	Channel      string   `json:"channel"`
	Recipient    string   `json:"-"`
	Version      int64    `json:"version"`
	Enabled      bool     `json:"enabled"`
	Capabilities []string `json:"capabilities"`
}

func hash(s string) string { v := sha256.Sum256([]byte(s)); return hex.EncodeToString(v[:]) }
func checkToken(ctx context.Context, p Principal) bool {
	t, ok := ownership.TokenFromContext(ctx)
	return ok && t.RunID == p.RunID && t.SessionKey == p.SessionKey
}

// Bind is only called behind verified owner identity. Rebinding versions cannot
// redirect a publication's immutable destination snapshot or replay old backlog.
func (s *Store) Bind(ctx context.Context, b Binding, expected int64) error {
	a, ok := owneridentity.FromContext(ctx)
	if !ok || s == nil || s.Pool == nil || b.ID == "" || len(b.ID) > 128 || b.Channel != a.Channel || b.Recipient == "" || b.Recipient != a.SessionKey || len(b.Recipient) > 512 || expected < 0 {
		return ErrDenied
	}
	if len(b.Capabilities) > 4 {
		return ErrDenied
	}
	seen := map[string]bool{}
	for _, c := range b.Capabilities {
		if c != "message" && c != "photo" && c != "report" && c != "widget" || seen[c] {
			return ErrDenied
		}
		seen[c] = true
	}
	caps, _ := json.Marshal(b.Capabilities)
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(context.Background())
	tag, e := tx.Exec(ctx, `INSERT INTO communication_bindings(binding_id,owner_id,channel,recipient,version,enabled,verified,capabilities) SELECT $1,$2,$3,$4,1,$5,TRUE,$6::jsonb WHERE $7=0 OR EXISTS(SELECT 1 FROM communication_bindings WHERE binding_id=$1 AND owner_id=$2 AND channel=$3 AND version=$7) ON CONFLICT(binding_id) DO UPDATE SET recipient=EXCLUDED.recipient,enabled=EXCLUDED.enabled,capabilities=EXCLUDED.capabilities,version=communication_bindings.version+1,updated_at=clock_timestamp() WHERE communication_bindings.owner_id=$2 AND communication_bindings.channel=$3 AND communication_bindings.version=$7`, b.ID, a.OwnerID, b.Channel, b.Recipient, b.Enabled, caps, expected)
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		return ErrConflict
	}
	return tx.Commit(ctx)
}

// AdmitTx is a kernel composition hook, never a model or public HTTP operation.
// It pins only supported backend methods, under the existing owned Input. This
// revision deliberately provides no startup flag or production admission path.
func (s *Store) AdmitTx(ctx context.Context, tx pgx.Tx, p Principal, quiet bool, methods []string) error {
	if !checkToken(ctx, p) || s.Inputs == nil || s.SourceGuard == nil || len(methods) == 0 {
		return ErrDenied
	}
	var valid bool
	if tx.QueryRow(ctx, `SELECT TRUE FROM sessions WHERE session_key=$1 AND user_id=$2 FOR UPDATE`, p.SessionKey, p.OwnerID).Scan(&valid) != nil {
		return ErrDenied
	}
	tok, _ := ownership.TokenFromContext(ctx)
	if tx.QueryRow(ctx, `SELECT TRUE FROM runs r JOIN session_ownership o USING(session_key) WHERE r.run_id=$1 AND r.session_key=$2 AND COALESCE(r.task_id,'')=$3 AND COALESCE(r.task_revision,0)=$4 AND r.lease_id=$5 AND o.run_id=r.run_id AND o.lease_id=$5 AND o.fence_epoch=$6 AND o.expires_at>clock_timestamp() AND r.current_state IN ('preparing','model_running','tool_pending','tool_running','awaiting_model_resume') FOR SHARE OF r,o`, p.RunID, p.SessionKey, p.TaskID, p.TaskRevision, tok.LeaseID, tok.FenceEpoch).Scan(&valid) != nil {
		return ErrDenied
	}
	seen := map[string]bool{}
	for _, m := range methods {
		if m != "communication.message" && m != "communication.widget" || seen[m] {
			return ErrDenied
		}
		seen[m] = true
	}
	raw, _ := json.Marshal(methods)
	if p.TaskID != "" {
		var mode string
		var parent *string
		if tx.QueryRow(ctx, `SELECT result_delivery,parent_task_id FROM tasks WHERE task_id=$1 AND owner_id=$2 AND session_key=$3 AND revision=$4 AND constraints_revision=$5 AND status='active' FOR SHARE`, p.TaskID, p.OwnerID, p.SessionKey, p.TaskRevision, p.ConstraintsRevision).Scan(&mode, &parent) != nil || parent != nil {
			return ErrDenied
		}
		if _, e := actionauth.CheckTaskAuthorityTx(ctx, tx, p.TaskID, p.OwnerID, nil, time.Now().UTC()); e != nil {
			return ErrDenied
		}
		// Verification owns result delivery; asking the model to publish an
		// unverified result would contradict the owner's stricter policy.
		quiet = quiet || mode == "verified_task"
	}
	if s.Inputs.CheckCommunicationClaimTx(ctx, tx, p.RunID, p.InputID) != nil {
		return ErrDenied
	}
	if e := s.SourceGuard(ctx, tx, p.RunID); e != nil {
		return ErrDenied
	}
	state := "open"
	if quiet {
		state = "response_committed"
	}
	tag, e := tx.Exec(ctx, `INSERT INTO communication_admissions(input_id,owner_id,session_key,schema_epoch,methods,quiet,obligation_state) SELECT input_id,owner_id,session_key,1,$4::jsonb,$5,$6 FROM accepted_inputs WHERE input_id=$1 AND owner_id=$2 AND session_key=$3 AND run_id=$7 AND status='running' AND claim_expires_at>clock_timestamp() AND expires_at>clock_timestamp() ON CONFLICT DO NOTHING`, p.InputID, p.OwnerID, p.SessionKey, raw, quiet, state, p.RunID)
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		return ErrDenied
	}
	return nil
}

func (s *Store) guard(ctx context.Context, tx pgx.Tx, p Principal) error {
	if s.SourceGuard == nil || s.Inputs == nil || !checkToken(ctx, p) {
		return ErrDenied
	}
	var yes bool
	tok, _ := ownership.TokenFromContext(ctx)
	e := tx.QueryRow(ctx, `SELECT TRUE FROM runs r JOIN sessions ss USING(session_key) JOIN session_ownership o USING(session_key) JOIN accepted_inputs i ON i.run_id=r.run_id JOIN communication_admissions a ON a.input_id=i.input_id WHERE r.run_id=$1 AND r.session_key=$2 AND ss.user_id=$3 AND i.input_id=$4 AND i.owner_id=$3 AND COALESCE(r.task_id,'')=$5 AND COALESCE(r.task_revision,0)=$6 AND a.schema_epoch=1 AND a.owner_id=$3 AND a.session_key=$2 AND i.status='running' AND i.claim_expires_at>clock_timestamp() AND i.expires_at>clock_timestamp() AND o.run_id=r.run_id AND o.lease_id=r.lease_id AND o.lease_id=$7 AND o.fence_epoch=$8 AND o.expires_at>clock_timestamp()`, p.RunID, p.SessionKey, p.OwnerID, p.InputID, p.TaskID, p.TaskRevision, tok.LeaseID, tok.FenceEpoch).Scan(&yes)
	if e != nil {
		return ErrDenied
	}
	if p.TaskID != "" {
		e = tx.QueryRow(ctx, `SELECT TRUE FROM tasks WHERE task_id=$1 AND owner_id=$2 AND session_key=$3 AND revision=$4 AND constraints_revision=$5 AND status='active' FOR SHARE`, p.TaskID, p.OwnerID, p.SessionKey, p.TaskRevision, p.ConstraintsRevision).Scan(&yes)
		if e != nil {
			return ErrDenied
		}
		if _, e = actionauth.CheckTaskAuthorityTx(ctx, tx, p.TaskID, p.OwnerID, nil, time.Now().UTC()); e != nil {
			return ErrDenied
		}
	}
	if s.Inputs.CheckCommunicationClaimTx(ctx, tx, p.RunID, p.InputID) != nil {
		return ErrDenied
	}
	return s.SourceGuard(ctx, tx, p.RunID)
}

// ApplyTx is called only by the owning dispatched-action verifier. Its commit is
// shared with transcript, obligation, interaction suspension and existing outbox.
func (s *Store) ApplyTx(ctx context.Context, tx pgx.Tx, p Principal, e communicationcontrol.Envelope) (communicationcontrol.Receipt, error) {
	return s.applyTx(ctx, tx, p, e, false)
}

func (s *Store) applyTx(ctx context.Context, tx pgx.Tx, p Principal, e communicationcontrol.Envelope, systemFailure bool) (communicationcontrol.Receipt, error) {
	var out communicationcontrol.Receipt
	a, err := communicationcontrol.DecodeArgs(e.Tool, e.ArgsJSON)
	if err != nil {
		return out, err
	}
	if e.EffectID == "" || e.ToolCallID == "" || e.RunID != p.RunID || s.Inputs == nil {
		return out, ErrDenied
	}
	// Session precedes run/task/input and transcript-head locks on all writers.
	var owner string
	if tx.QueryRow(ctx, `SELECT user_id FROM sessions WHERE session_key=$1 FOR UPDATE`, p.SessionKey).Scan(&owner) != nil || owner != p.OwnerID {
		return out, ErrDenied
	}
	if err = s.guard(ctx, tx, p); err != nil {
		return out, err
	}
	canon, _ := json.Marshal(a)
	digest := hash(string(canon))
	var oldHash, oldRun, oldCall string
	var receipt []byte
	err = tx.QueryRow(ctx, `SELECT payload_hash,run_id,tool_call_id,receipt_json FROM communication_publications WHERE NOT privacy_redacted AND effect_id=$1`, e.EffectID).Scan(&oldHash, &oldRun, &oldCall, &receipt)
	if err == nil {
		if oldHash != digest || oldRun != p.RunID || oldCall != e.ToolCallID {
			return out, ErrConflict
		}
		err = json.Unmarshal(receipt, &out)
		return out, err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return out, err
	}
	if e.Lookup {
		return out, ErrDenied
	}
	var methods []byte
	var count int
	if tx.QueryRow(ctx, `SELECT methods,publication_count FROM communication_admissions WHERE input_id=$1 AND schema_epoch=1 FOR UPDATE`, p.InputID).Scan(&methods, &count) != nil {
		return out, ErrDenied
	}
	var ready []string
	_ = json.Unmarshal(methods, &ready)
	allowed := false
	for _, m := range ready {
		allowed = allowed || m == e.Tool
	}
	if !allowed || count >= 20 && !systemFailure {
		return out, ErrDenied
	}
	kind := strings.TrimPrefix(e.Tool, "communication.")
	if p.DestinationsHash != "" {
		current, e := DestinationHash(ctx, tx, p.OwnerID, kind)
		if e != nil || current != p.DestinationsHash {
			return out, ErrDenied
		}
	}
	if kind != "message" && kind != "widget" {
		return out, ErrDenied
	}
	if a.ExpiresAt != nil {
		var now time.Time
		_ = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now)
		if !a.ExpiresAt.After(now) {
			return out, ErrDenied
		}
	}
	// verified_task permits questions/blocked intervention, never progress or an
	// unverified result; direct child publication is denied until separately scoped.
	if p.TaskID != "" {
		var mode string
		var parent *string
		if tx.QueryRow(ctx, `SELECT result_delivery,parent_task_id FROM tasks WHERE task_id=$1`, p.TaskID).Scan(&mode, &parent) != nil {
			return out, ErrDenied
		}
		if parent != nil || mode == "verified_task" && kind != "widget" && a.Purpose != "blocked" {
			return out, ErrDenied
		}
	}
	pub := "publication:" + hash(e.EffectID)
	interaction := ""
	purpose := a.Purpose
	text := a.Text
	if kind == "widget" {
		interaction = "interaction:" + hash(e.EffectID)
		purpose = "question"
		text = a.Question
	}
	out = communicationcontrol.Receipt{Status: "accepted", PublicationRef: pub, InteractionRef: interaction, Delivery: []communicationcontrol.Delivery{}}
	rows, err := tx.Query(ctx, `SELECT binding_id,channel,recipient,version FROM communication_bindings WHERE owner_id=$1 AND enabled AND verified AND capabilities ? $2 ORDER BY binding_id FOR SHARE`, p.OwnerID, kind)
	if err != nil {
		return out, err
	}
	var bindings []Binding
	for rows.Next() {
		var b Binding
		if err = rows.Scan(&b.ID, &b.Channel, &b.Recipient, &b.Version); err != nil {
			rows.Close()
			return out, err
		}
		bindings = append(bindings, b)
		out.Delivery = append(out.Delivery, communicationcontrol.Delivery{Channel: b.Channel, State: "pending"})
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if p.DestinationsHash != "" {
		current, e := DestinationHash(ctx, tx, p.OwnerID, kind)
		if e != nil || current != p.DestinationsHash {
			return out, ErrDenied
		}
	}
	// No configured recipient is an explicit failure, never a fulfilled obligation.
	if len(bindings) == 0 {
		return out, ErrDenied
	}
	if s.ChannelGuard == nil {
		return out, ErrDenied
	}
	for _, b := range bindings {
		if err = s.ChannelGuard(ctx, tx, p.RunID, b, a); err != nil {
			return out, ErrDenied
		}
	}
	// Binding/export checks may wait for locks or policy. Recheck live deadlines
	// and the exact private claim after those waits, before any publication write.
	if err = s.guard(ctx, tx, p); err != nil {
		return out, err
	}
	receipt, _ = json.Marshal(out)
	sources, _ := json.Marshal(a.SourceRefs)
	_, err = tx.Exec(ctx, `INSERT INTO communication_publications(publication_id,effect_id,tool_call_id,input_id,owner_id,session_key,run_id,task_id,task_revision,constraints_revision,kind,purpose,payload,payload_hash,source_refs,receipt_json) VALUES($1,$2,$3,$4,$5,$6,$7,NULLIF($8,''),$9,$10,$11,$12,$13::jsonb,$14,$15::jsonb,$16::jsonb)`, pub, e.EffectID, e.ToolCallID, p.InputID, p.OwnerID, p.SessionKey, p.RunID, p.TaskID, p.TaskRevision, p.ConstraintsRevision, kind, purpose, canon, digest, sources, receipt)
	if err != nil {
		return out, err
	}
	metadata, _ := json.Marshal(map[string]any{"kind": "publication", "publication_ref": pub, "purpose": purpose, "visibility": "owner"})
	_, err = tx.Exec(ctx, `INSERT INTO messages(message_id,session_key,run_id,role,content,metadata) VALUES($1,$2,$3,'assistant',$4,$5::jsonb)`, pub, p.SessionKey, p.RunID, text, metadata)
	if err != nil {
		return out, err
	}
	for _, b := range bindings {
		_, err = tx.Exec(ctx, `INSERT INTO delivery_outbox(notification_id,result_id,run_id,session_key,channel,chunk_index,content,status,deadline_at,publication_id,binding_id,binding_version,recipient_snapshot,representation_json) VALUES($1,NULL,$2,$3,$4,0,$5,'pending',clock_timestamp()+INTERVAL '24 hours',$6,$7,$8,$9,$10::jsonb)`, pub+":"+b.ID+":"+stringID(b.Version), p.RunID, p.SessionKey, b.Channel, text, pub, b.ID, b.Version, b.Recipient, canon)
		if err != nil {
			return out, err
		}
	}
	state := ""
	if purpose == "result" || purpose == "blocked" {
		state = "response_committed"
	}
	if kind == "widget" {
		state = "waiting_for_user"
	}
	increment := 1
	if systemFailure {
		increment = 0 // One fixed kernel notice is separate from the agent quota.
	}
	_, err = tx.Exec(ctx, `UPDATE communication_admissions SET publication_count=publication_count+$3,obligation_state=CASE WHEN $2='' THEN obligation_state ELSE $2 END WHERE input_id=$1`, p.InputID, state, increment)
	if err != nil {
		return out, err
	}
	if kind == "widget" {
		_, err = tx.Exec(ctx, `INSERT INTO communication_interactions(interaction_id,publication_id,input_id,owner_id,session_key,run_id,task_id,task_revision,constraints_revision,question_json,state,expires_at) VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,''),$8,$9,$10::jsonb,'open',$11)`, interaction, pub, p.InputID, p.OwnerID, p.SessionKey, p.RunID, p.TaskID, p.TaskRevision, p.ConstraintsRevision, canon, a.ExpiresAt)
		if err != nil {
			return out, err
		}
		if err = s.Inputs.SuspendInteractionTx(ctx, tx, p.RunID, interaction); err != nil {
			return out, err
		}
		if err = suspendRunTaskTx(ctx, tx, p); err != nil {
			return out, err
		}
	}
	if err = s.eventTx(ctx, tx, p.OwnerID, p.SessionKey, pub, "publication.created"); err != nil {
		return out, err
	}
	return out, nil
}

func stringID(v int64) string { raw, _ := json.Marshal(v); return string(raw) }
func (s *Store) eventTx(ctx context.Context, tx pgx.Tx, owner, key, id, kind string) error {
	_, e := tx.Exec(ctx, `INSERT INTO communication_events(owner_id,session_key,aggregate_id,event_type) VALUES($1,$2,$3,$4)`, owner, key, id, kind)
	return e
}
