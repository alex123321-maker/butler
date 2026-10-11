package generatedtools

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
)

const CandidateReturn = "candidate_return"
const ActivatedCatalog = "activated_catalog"

// ContinuationIntent is admitted by the owning converter delegation transaction.
// It cannot be created by a model final, a child report or an arbitrary input.
type ContinuationIntent struct {
	Version             int                 `json:"version"`
	ID                  string              `json:"id"`
	ChildTaskID         string              `json:"child_task_id"`
	OwnerID             string              `json:"owner_id"`
	ParentTaskID        string              `json:"parent_task_id"`
	ParentRevision      int64               `json:"parent_revision"`
	ConstraintsRevision int64               `json:"constraints_revision"`
	Purpose             string              `json:"purpose"`
	SnapshotHash        string              `json:"snapshot_hash"`
	OriginalRunID       string              `json:"original_run_id"`
	OriginalInputID     string              `json:"original_input_id"`
	OriginalSources     []catalog.SourceRef `json:"original_sources"`
	Deadline            time.Time           `json:"deadline"`
}
type ContinuationReady struct {
	ReturnHash         string              `json:"return_hash"`
	PriorRunID         string              `json:"prior_run_id"`
	PriorInputID       string              `json:"prior_input_id"`
	ActivationActionID string              `json:"activation_action_id,omitempty"`
	ActivationEffectID string              `json:"activation_effect_id,omitempty"`
	Generation         int64               `json:"generation,omitempty"`
	Sources            []catalog.SourceRef `json:"sources"`
}
type Continuation struct {
	Intent  ContinuationIntent
	Ready   ContinuationReady
	InputID string
	State   string
}

func RunSourcePinsTx(ctx context.Context, tx pgx.Tx, runID string) ([]catalog.SourceRef, error) {
	rows, err := tx.Query(ctx, `SELECT source_id,source_revision,access_revision,content_hash FROM memory_run_sources WHERE run_id=$1 ORDER BY source_id`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []catalog.SourceRef{}
	for rows.Next() {
		var r catalog.SourceRef
		if err = rows.Scan(&r.ID, &r.Revision, &r.AccessRevision, &r.Hash); err != nil {
			return nil, err
		}
		out = append(out, r)
		if len(out) > 256 {
			return nil, ErrLimit
		}
	}
	return out, rows.Err()
}

func WriteContinuationIntentsTx(ctx context.Context, tx pgx.Tx, snapshot DeveloperSnapshot) error {
	b := snapshot.Binding
	var inputID string
	if tx.QueryRow(ctx, `SELECT input_id FROM accepted_inputs WHERE run_id=$1 AND task_id=$2 AND owner_id=$3 AND status='running' FOR SHARE`, b.RunID, b.TaskID, b.OwnerID).Scan(&inputID) != nil {
		return ErrAccess
	}
	pins, err := RunSourcePinsTx(ctx, tx, b.RunID)
	if err != nil || len(pins) == 0 {
		return ErrAccess
	}
	for _, purpose := range []string{CandidateReturn, ActivatedCatalog} {
		id := "converter-continuation:" + Hash([]string{snapshot.ChildTaskID, purpose})
		v := ContinuationIntent{Version: 1, ID: id, ChildTaskID: snapshot.ChildTaskID, OwnerID: b.OwnerID, ParentTaskID: b.TaskID, ParentRevision: b.TaskRevision, ConstraintsRevision: b.ConstraintsRevision, Purpose: purpose, SnapshotHash: Hash(snapshot), OriginalRunID: b.RunID, OriginalInputID: inputID, OriginalSources: pins, Deadline: snapshot.Args.Deadline}
		raw, err := json.Marshal(v)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO converter_parent_continuations(continuation_id,child_task_id,owner_id,parent_task_id,parent_revision,constraints_revision,purpose,intent_hash,intent_json,deadline) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10)`, id, v.ChildTaskID, v.OwnerID, v.ParentTaskID, v.ParentRevision, v.ConstraintsRevision, purpose, Hash(v), raw, v.Deadline); err != nil {
			return err
		}
	}
	return nil
}

func ReadContinuationTx(ctx context.Context, tx pgx.Tx, id string) (Continuation, error) {
	var c Continuation
	var intent, ready []byte
	var hash, readyHash string
	var child, owner, parent, purpose string
	var rev, cr int64
	var deadline time.Time
	err := tx.QueryRow(ctx, `SELECT intent_json,intent_hash,ready_json,ready_hash,COALESCE(input_id,''),state,child_task_id,owner_id,parent_task_id,parent_revision,constraints_revision,purpose,deadline FROM converter_parent_continuations WHERE continuation_id=$1 FOR SHARE`, id).Scan(&intent, &hash, &ready, &readyHash, &c.InputID, &c.State, &child, &owner, &parent, &rev, &cr, &purpose, &deadline)
	if err != nil {
		return c, err
	}
	v := &c.Intent
	if json.Unmarshal(intent, v) != nil || Hash(*v) != hash || v.ID != id || v.Version != 1 || v.ChildTaskID != child || v.OwnerID != owner || v.ParentTaskID != parent || v.ParentRevision != rev || v.ConstraintsRevision != cr || v.Purpose != purpose || !v.Deadline.Equal(deadline) || len(v.OriginalSources) == 0 || len(v.OriginalSources) > 256 {
		return c, ErrAccess
	}
	if c.State == "queued" && (json.Unmarshal(ready, &c.Ready) != nil || Hash(c.Ready) != readyHash || c.InputID == "") {
		return c, ErrAccess
	}
	return c, nil
}

// CheckContinuationTx verifies retained facts and current owner authority. Caller
// holds ownership/session/run/task locks before entering sources/effect records.
func CheckContinuationTx(ctx context.Context, tx pgx.Tx, sources DeveloperSources, c Continuation) (DeveloperSnapshot, DeveloperReturn, error) {
	v := c.Intent
	var role bool
	if tx.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM delegation_admission_snapshots WHERE task_id=$1)`, v.ChildTaskID).Scan(&role) != nil || !role {
		return DeveloperSnapshot{}, DeveloperReturn{}, ErrAccess
	}
	s, err := ReadDeveloperSnapshotTx(ctx, tx, sources, v.OwnerID, v.ChildTaskID)
	if err != nil || s == nil || Hash(*s) != v.SnapshotHash || s.Binding.TaskID != v.ParentTaskID || s.Binding.TaskRevision != v.ParentRevision || s.Binding.ConstraintsRevision != v.ConstraintsRevision || s.Binding.RunID != v.OriginalRunID || !s.Args.Deadline.Equal(v.Deadline) {
		return DeveloperSnapshot{}, DeveloperReturn{}, ErrAccess
	}
	if err = checkContinuationPolicyTx(ctx, tx, *s, v.Purpose); err != nil {
		return *s, DeveloperReturn{}, err
	}
	var original bool
	if tx.QueryRow(ctx, `SELECT TRUE FROM accepted_inputs i JOIN runs r ON r.run_id=i.run_id JOIN run_final_results f ON f.run_id=r.run_id WHERE i.input_id=$1 AND i.run_id=$2 AND i.task_id=$3 AND i.owner_id=$4 AND i.status='completed' AND r.current_state='completed' AND r.task_revision=$5 AND f.task_id=i.task_id AND f.task_revision=$5 AND f.constraints_revision=$6 AND NOT f.privacy_redacted FOR SHARE OF i,r,f`, v.OriginalInputID, v.OriginalRunID, v.ParentTaskID, v.OwnerID, v.ParentRevision, v.ConstraintsRevision).Scan(&original) != nil {
		return *s, DeveloperReturn{}, ErrAccess
	}
	pins, err := RunSourcePinsTx(ctx, tx, v.OriginalRunID)
	// Later preparations may add pins to that run; losing an original dependency
	// is forbidden, and the entire retained graph is checked by the source guard.
	if err != nil {
		return *s, DeveloperReturn{}, err
	}
	for _, r := range v.OriginalSources {
		found := false
		for _, p := range pins {
			if p == r {
				found = true
				break
			}
		}
		if !found {
			return *s, DeveloperReturn{}, ErrAccess
		}
	}
	if err = confirmedContinuationActionTx(ctx, tx, s.Binding, s.EffectID, s.AuthorizationID); err != nil {
		return *s, DeveloperReturn{}, err
	}
	p, err := ReadDeveloperReturnTx(ctx, tx, *s)
	if err != nil {
		return *s, p, err
	}
	if c.State == "queued" {
		if Hash(p) != c.Ready.ReturnHash {
			return *s, p, ErrAccess
		}
		if v.Purpose == ActivatedCatalog {
			if err = checkContinuationActivationTx(ctx, tx, *s, p, c.Ready); err != nil {
				return *s, p, err
			}
		} else if v.Purpose != CandidateReturn || c.Ready.ActivationActionID != "" || c.Ready.PriorRunID != v.OriginalRunID || c.Ready.PriorInputID != v.OriginalInputID {
			return *s, p, ErrAccess
		}
	}
	return *s, p, nil
}

func checkContinuationPolicyTx(ctx context.Context, tx pgx.Tx, s DeveloperSnapshot, purpose string) error {
	var now time.Time
	if err := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return err
	}
	var policy string
	if tx.QueryRow(ctx, `SELECT execution_policy FROM tasks WHERE task_id=$1`, s.Binding.TaskID).Scan(&policy) != nil {
		return ErrAccess
	}
	if purpose == CandidateReturn && actionauth.CheckExecutionPolicy(actionauth.ContextSnapshot{ExecutionPolicy: policy}, actionauth.EffectUnknown) != nil {
		return ErrAccess
	}
	if purpose != CandidateReturn && purpose != ActivatedCatalog {
		return ErrAccess
	}
	if purpose == CandidateReturn {
		for _, c := range Contracts() {
			if c.ToolName != "converters.collect" && c.ToolName != "converters.test" && c.ToolName != "converters.trial" && c.ToolName != "converters.activate" {
				continue
			}
			b := s.Binding
			b.ToolName, b.Resource = c.ToolName, "tool:"+c.ToolName
			b.ContractHash, _ = actionauth.ContractHash(c)
			b.Effect = actionauth.EffectPrepare
			if c.ToolName == "converters.collect" {
				b.Effect = actionauth.EffectRead
			}
			if err := actionauth.CheckTaskActionTx(ctx, tx, b, now); err != nil {
				return err
			}
		}
	}
	b := s.Binding
	b.ToolName = "converter." + s.Permission.ConverterID + ".convert"
	b.Resource = "tool:" + b.ToolName
	b.Effect = actionauth.EffectPrepare
	return actionauth.CheckTaskActionTx(ctx, tx, b, now)
}

func confirmedContinuationActionTx(ctx context.Context, tx pgx.Tx, b actionauth.ActionBinding, effect, authorization string) error {
	var raw []byte
	if tx.QueryRow(ctx, `SELECT i.binding FROM action_intents i JOIN action_effects e ON e.action_id=i.effect_id JOIN action_authorizations a ON a.action_id=i.action_id JOIN action_effect_dispatches d ON d.authorization_id=a.authorization_id WHERE i.action_id=$1 AND i.effect_id=$2 AND a.authorization_id=$3 AND e.state='effect_confirmed' AND a.state='dispatch_completed' AND d.outcome='dispatch_completed' AND d.execution_action_id=i.action_id FOR SHARE OF i,e,a,d`, b.ActionID, effect, authorization).Scan(&raw) != nil {
		return ErrAccess
	}
	var actual actionauth.ActionBinding
	if json.Unmarshal(raw, &actual) != nil || !reflect.DeepEqual(actual, b) {
		return ErrAccess
	}
	return nil
}

func checkContinuationActivationTx(ctx context.Context, tx pgx.Tx, s DeveloperSnapshot, p DeveloperReturn, r ContinuationReady) error {
	var final bool
	if err := tx.QueryRow(ctx, `SELECT TRUE FROM converter_parent_continuations c JOIN accepted_inputs i ON i.input_id=c.input_id JOIN runs run ON run.run_id=i.run_id JOIN run_final_results f ON f.run_id=run.run_id WHERE c.child_task_id=$1 AND c.purpose='candidate_return' AND c.state='queued' AND i.input_id=$2 AND i.run_id=$3 AND i.owner_id=$4 AND i.task_id=$5 AND i.status='completed' AND run.current_state='completed' AND f.task_id=i.task_id AND f.task_revision=$6 AND f.constraints_revision=$7 AND NOT f.privacy_redacted FOR SHARE OF c,i,run,f`, s.ChildTaskID, r.PriorInputID, r.PriorRunID, s.OwnerID, s.Binding.TaskID, s.Binding.TaskRevision, s.Binding.ConstraintsRevision).Scan(&final); err != nil {
		return ErrAccess
	}
	var raw []byte
	var auth string
	if tx.QueryRow(ctx, `SELECT i.binding,a.authorization_id FROM converter_events ce JOIN action_intents i ON i.effect_id=ce.action_id JOIN action_authorizations a ON a.action_id=i.action_id WHERE ce.owner_id=$1 AND ce.digest=$2 AND ce.kind='converters.activate' AND ce.action_id=$3 AND i.action_id=$4 AND ce.run_id=$5 AND ce.generation=$6 AND a.state='dispatch_completed'`, s.OwnerID, p.Candidate.Digest, r.ActivationEffectID, r.ActivationActionID, r.PriorRunID, r.Generation).Scan(&raw, &auth) != nil {
		return ErrAccess
	}
	var b actionauth.ActionBinding
	if json.Unmarshal(raw, &b) != nil || b.TaskID != s.Binding.TaskID || b.TaskRevision != s.Binding.TaskRevision || b.ConstraintsRevision != s.Binding.ConstraintsRevision || b.GoalID != s.Binding.GoalID || b.GoalRevision != s.Binding.GoalRevision || b.MandateID != s.Binding.MandateID || b.MandateRevision != s.Binding.MandateRevision || b.OwnerPolicyRevision != s.Binding.OwnerPolicyRevision || b.Purpose != s.Binding.Purpose || b.ConverterPackageHash != p.Candidate.Digest || b.ToolName != "converters.activate" || b.Resource != "tool:converters.activate" || b.Effect != actionauth.EffectPrepare || len(b.CredentialRefs) != 0 {
		return ErrAccess
	}
	var current bool
	if tx.QueryRow(ctx, `SELECT TRUE FROM converter_catalog c JOIN converter_states s ON s.owner_id=c.owner_id AND s.digest=c.active_digest WHERE c.owner_id=$1 AND c.converter_id=$2 AND c.active_digest=$3 AND c.generation=$4 AND s.status='active' FOR SHARE OF c,s`, s.OwnerID, p.Candidate.ID, p.Candidate.Digest, r.Generation).Scan(&current) != nil {
		return ErrAccess
	}
	if err := confirmedContinuationActionTx(ctx, tx, b, r.ActivationEffectID, auth); err != nil {
		return err
	}
	var now time.Time
	if err := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return err
	}
	return actionauth.CheckTaskActionTx(ctx, tx, b, now)
}

// CheckContinuationSourcesTx checks the full immutable original/past-parent
// graph and technical return privacy pins without granting any body to a model.
func CheckContinuationSourcesTx(ctx context.Context, tx pgx.Tx, sources *catalog.Store, c Continuation, p DeveloperReturn) error {
	pins := append(append([]catalog.SourceRef(nil), c.Intent.OriginalSources...), c.Ready.Sources...)
	pins = append(pins, p.Candidate.BodySource, p.Candidate.NeedSource, p.ContractSource, p.ReportSource, p.FinalSource)
	checker := continuationPrivacyCheck{sources, c, pins}
	if _, err := sources.AccessTx(catalog.WithAccessChecker(ctx, checker), tx, nil, "converter_continuation"); err != nil {
		return err
	}
	if err := sources.CheckRunSourcesTx(ctx, tx, c.Intent.OriginalRunID); err != nil {
		return err
	}
	if c.Ready.PriorRunID != "" && c.Ready.PriorRunID != c.Intent.OriginalRunID {
		if err := sources.CheckRunSourcesTx(ctx, tx, c.Ready.PriorRunID); err != nil {
			return err
		}
	}
	for _, r := range append(append([]catalog.SourceRef(nil), c.Intent.OriginalSources...), c.Ready.Sources...) {
		var current bool
		err := tx.QueryRow(ctx, `SELECT TRUE FROM memory_sources s JOIN tasks t ON t.task_id=$2 WHERE s.source_id=$1 AND s.owner_id=$3 AND t.owner_id=$3 AND t.revision=$4 AND t.constraints_revision=$5 AND t.status='active' AND ((s.scope_type='task' AND s.scope_id=t.task_id) OR (s.scope_type='session' AND s.scope_id=t.session_key) OR (s.scope_type='user' AND s.scope_id=t.owner_id) OR EXISTS(SELECT 1 FROM memory_scope_grants g WHERE g.task_id=t.task_id AND g.source_id=s.source_id AND g.owner_id=t.owner_id AND g.task_revision=t.revision AND g.access_revision=s.access_revision AND g.purpose='memory_read'))`, r.ID, c.Intent.ParentTaskID, c.Intent.OwnerID, c.Intent.ParentRevision, c.Intent.ConstraintsRevision).Scan(&current)
		if err != nil || !current {
			return ErrAccess
		}
	}
	return nil
}

type continuationPrivacyCheck struct {
	sources *catalog.Store
	c       Continuation
	pins    []catalog.SourceRef
}

func (c continuationPrivacyCheck) CheckMemoryAccessTx(ctx context.Context, tx pgx.Tx, refs []catalog.SourceRef, purpose string) (string, error) {
	if len(refs) != 0 || purpose != "converter_continuation" {
		return "", ErrAccess
	}
	if _, _, err := CheckContinuationTx(ctx, tx, c.sources, c.c); err != nil {
		return "", err
	}
	return c.c.Intent.OwnerID, nil
}
func (c continuationPrivacyCheck) FinalizeMemoryAccessTx(ctx context.Context, tx pgx.Tx, refs []catalog.SourceRef, purpose string) error {
	_, err := c.CheckMemoryAccessTx(ctx, tx, refs, purpose)
	return err
}
func (c continuationPrivacyCheck) PinnedMemorySources() []catalog.SourceRef {
	return append([]catalog.SourceRef(nil), c.pins...)
}

// Compose after current ownership/task/goal and Catalog source guards in Broker,
// provider/recovery and atomic final. Ordinary inputs need no055 capability.
func CheckContinuationRunTx(ctx context.Context, tx pgx.Tx, sources *catalog.Store, runID string) error {
	var found bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM accepted_inputs WHERE run_id=$1 AND source='converter-return')`, runID).Scan(&found)
	if !found && err == nil {
		return nil
	}
	if err != nil {
		return err
	}
	// InputEvent's durable JSON uses exported field names; payload is checked
	// against the exact immutable055/input identity rather than trusting its ID.
	var raw []byte
	if tx.QueryRow(ctx, `SELECT event_json FROM accepted_inputs WHERE run_id=$1 AND source='converter-return' AND status='running' AND claim_expires_at>clock_timestamp() AND expires_at>clock_timestamp() FOR SHARE`, runID).Scan(&raw) != nil {
		return ErrAccess
	}
	var event ingress.InputEvent
	var binding struct {
		TaskID string `json:"task_id"`
		ID     string `json:"continuation_id"`
	}
	if json.Unmarshal(raw, &event) != nil || Decode([]byte(event.PayloadJSON), &binding) != nil || event.Source != "converter-return" || event.EventID != "converter-return:"+binding.ID || event.IdempotencyKey != event.EventID {
		return ErrAccess
	}
	c, err := ReadContinuationTx(ctx, tx, binding.ID)
	if err != nil || c.State != "queued" || c.Intent.ParentTaskID != binding.TaskID {
		return ErrAccess
	}
	tok, ok := ownership.TokenFromContext(ctx)
	if !ok || tok.RunID != runID {
		return ErrAccess
	}
	var valid bool
	if tx.QueryRow(ctx, `SELECT TRUE FROM accepted_inputs i JOIN runs r ON r.run_id=i.run_id WHERE i.input_id=$1 AND i.run_id=$2 AND i.task_id=$3 AND i.owner_id=$4 AND r.task_revision=$5 AND r.session_key=$6 FOR SHARE OF i,r`, c.InputID, runID, c.Intent.ParentTaskID, c.Intent.OwnerID, c.Intent.ParentRevision, tok.SessionKey).Scan(&valid) != nil {
		return ErrAccess
	}
	_, p, err := CheckContinuationTx(ctx, tx, sources, c)
	if err != nil {
		return fmt.Errorf("retained continuation: %w", err)
	}
	if err = CheckContinuationSourcesTx(ctx, tx, sources, c, p); err != nil {
		return fmt.Errorf("continuation sources: %w", err)
	}
	// All source/authority lock waits precede this fresh DB-clock fence check.
	if err = tx.QueryRow(ctx, `SELECT TRUE FROM session_ownership o JOIN runs r ON r.run_id=o.run_id JOIN tasks t ON t.task_id=r.task_id JOIN accepted_inputs i ON i.run_id=r.run_id WHERE o.session_key=$1 AND o.run_id=$2 AND o.lease_id=$3 AND o.fence_epoch=$4 AND o.expires_at>clock_timestamp() AND i.input_id=$5 AND i.status='running' AND i.claim_expires_at>clock_timestamp() AND i.expires_at>clock_timestamp() AND t.task_id=$6 AND t.owner_id=$7 AND t.status='active' AND t.revision=$8 AND t.constraints_revision=$9 AND r.task_revision=t.revision AND clock_timestamp()<$10 FOR SHARE OF o,r,t,i`, tok.SessionKey, tok.RunID, tok.LeaseID, tok.FenceEpoch, c.InputID, c.Intent.ParentTaskID, c.Intent.OwnerID, c.Intent.ParentRevision, c.Intent.ConstraintsRevision, c.Intent.Deadline).Scan(&valid); err != nil {
		return fmt.Errorf("continuation current fence: %w", ErrAccess)
	}
	return nil
}
