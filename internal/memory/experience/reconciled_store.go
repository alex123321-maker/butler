package experience

import (
	"context"
	"encoding/json"
	"sort"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/jackc/pgx/v5"
)

// The original may itself have consumed another lesson. Acquire the complete
// ancestor and new-observation union before AccessTx can take an ancestor's
// input/metadata fences. Otherwise a later own-tool lock would invert Catalog's
// source -> primary tools -> input -> metadata order for nested experience.
func lockReconciledSealingRowsTx(ctx context.Context, tx pgx.Tx, o Observation, parents []catalog.SourceRef) error {
	r := o.ReconciledAttempt
	ids := []string{r.OriginalSnapshotID}
	for _, parent := range parents {
		ids = append(ids, parent.ID)
	}
	if _, err := tx.Exec(ctx, `WITH RECURSIVE roots AS(SELECT unnest($1::text[]) AS id UNION SELECT d.parent_source_id FROM memory_source_dependencies d JOIN roots r ON r.id=d.child_source_id),all_ids AS(SELECT id FROM roots UNION SELECT d.original_source_id FROM memory_summary_dependencies d JOIN roots r ON r.id=d.snapshot_source_id UNION SELECT a.original_source_id FROM memory_experience_reconciled_attempts a JOIN roots r ON r.id=a.source_id)
SELECT s.source_id FROM memory_sources s JOIN all_ids a ON a.id=s.source_id ORDER BY s.source_id FOR SHARE OF s`, ids); err != nil {
		return err
	}
	tools := []string{r.LookupPin.ID}
	for _, pin := range o.ToolPins {
		tools = append(tools, pin.ID)
	}
	if _, err := tx.Exec(ctx, `WITH RECURSIVE roots AS(SELECT unnest($1::text[]) AS id UNION SELECT d.parent_source_id FROM memory_source_dependencies d JOIN roots r ON r.id=d.child_source_id),pins AS(SELECT unnest($2::text[]) AS tool_call_id UNION SELECT p.tool_call_id FROM memory_experience_tool_pins p JOIN roots r ON r.id=p.observation_source_id UNION SELECT a.lookup_tool_call_id FROM memory_experience_reconciled_attempts a JOIN roots r ON r.id=a.source_id)
SELECT t.tool_call_id FROM tool_calls t JOIN pins p USING(tool_call_id) ORDER BY t.tool_call_id FOR SHARE OF t`, ids, tools); err != nil {
		return err
	}
	inputs := []string{o.TerminalAttempt.InputID, r.ContinuationInputID}
	if _, err := tx.Exec(ctx, `WITH RECURSIVE roots AS(SELECT unnest($1::text[]) AS id UNION SELECT d.parent_source_id FROM memory_source_dependencies d JOIN roots r ON r.id=d.child_source_id),pins AS(SELECT unnest($2::text[]) AS id UNION SELECT a.input_id FROM memory_experience_terminal_attempts a JOIN roots r ON r.id=a.source_id UNION SELECT a.original_input_id FROM memory_experience_reconciled_attempts a JOIN roots r ON r.id=a.source_id UNION SELECT a.continuation_input_id FROM memory_experience_reconciled_attempts a JOIN roots r ON r.id=a.source_id)
SELECT pg_advisory_xact_lock_shared(hashtextextended('butler-terminal-input:'||id,0)) FROM pins ORDER BY id`, ids, inputs); err != nil {
		return err
	}
	heads, err := json.Marshal(r.Heads)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `WITH RECURSIVE roots AS(SELECT unnest($1::text[]) AS id UNION SELECT d.parent_source_id FROM memory_source_dependencies d JOIN roots r ON r.id=d.child_source_id),pins AS(SELECT p.kind,p.row_id FROM memory_experience_reconciled_pins p JOIN roots r ON r.id=p.source_id UNION SELECT p->>'kind',p->>'id' FROM jsonb_array_elements($2::jsonb)p)
SELECT pg_advisory_xact_lock_shared(hashtextextended('butler-reconciled-row:'||kind||':'||row_id,0)) FROM pins ORDER BY kind,row_id`, ids, string(heads))
	return err
}

func checkReconciledSealingTx(ctx context.Context, tx pgx.Tx, o Observation) error {
	r := o.ReconciledAttempt
	if r == nil || r.validate(o) != nil {
		return ErrEvidence
	}
	// Sources are already held. All exact primary tool rows precede input and
	// small metadata fences, matching Catalog release and privacy deletion.
	pins := append(append([]transcript.ToolPin{}, o.ToolPins...), r.LookupPin)
	sort.Slice(pins, func(i, j int) bool { return pins[i].ID < pins[j].ID })
	for _, pin := range pins {
		var current bool
		var id string
		if err := tx.QueryRow(ctx, `SELECT tool_call_id FROM tool_calls WHERE tool_call_id=$1 AND run_id=$2 FOR SHARE`, pin.ID, pin.RunID).Scan(&id); err != nil {
			return catalog.ErrStale
		}
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM memory_experience_tool_heads WHERE tool_call_id=$1 AND run_id=$2 AND row_hash=$3 AND revision=$4 AND alive)`, pin.ID, pin.RunID, pin.Hash, pin.Revision).Scan(&current); err != nil || !current {
			return catalog.ErrStale
		}
	}
	inputs := []string{o.TerminalAttempt.InputID, r.ContinuationInputID}
	sort.Strings(inputs)
	for _, id := range inputs {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock_shared(hashtextextended('butler-terminal-input:'||$1::text,0))`, id); err != nil {
			return err
		}
	}
	for _, pin := range r.Heads {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock_shared(hashtextextended('butler-reconciled-row:'||$1::text||':'||$2::text,0))`, pin.Kind, pin.ID); err != nil {
			return err
		}
	}
	expected, err := json.Marshal(r.proofContext(o))
	if err != nil {
		return err
	}
	var current bool
	err = tx.QueryRow(ctx, `WITH proof AS(SELECT butler_memory_reconciled_context($1,$2,$3,$4) AS p) SELECT COALESCE(p=$5::jsonb AND encode(sha256(convert_to(p::text,'UTF8')),'hex')=$6,FALSE) FROM proof`, o.RunID, r.OriginalSnapshotID, r.ControlID, o.OwnerID, string(expected), r.ProofHash).Scan(&current)
	if err != nil || !current {
		return ErrEvidence
	}
	return nil
}

func insertReconciledTx(ctx context.Context, tx pgx.Tx, source string, o Observation) error {
	r := o.ReconciledAttempt
	_, err := tx.Exec(ctx, `INSERT INTO memory_experience_reconciled_attempts(source_id,original_source_id,original_access_revision,head_count,control_id,current_task_revision,current_constraints_revision,original_input_id,original_input_hash,original_input_revision,continuation_input_id,continuation_input_hash,continuation_input_revision,continuation_run_id,continuation_result_id,target_action_id,reconciliation_evidence_id,lookup_action_id,lookup_tool_call_id,lookup_tool_hash,lookup_tool_revision)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21)`, source, r.OriginalSnapshotID, r.OriginalAccessRevision, len(r.Heads), r.ControlID, r.CurrentTaskRevision, r.CurrentConstraintsRevision, o.TerminalAttempt.InputID, r.OriginalCurrentInputHash, r.OriginalCurrentInputRevision, r.ContinuationInputID, r.ContinuationInputHash, r.ContinuationInputRevision, r.ContinuationRunID, r.ContinuationResultID, r.TargetActionID, r.ReconciliationEvidenceID, r.Lookup.ID, r.LookupPin.ID, r.LookupPin.Hash, r.LookupPin.Revision)
	if err != nil {
		return err
	}
	for _, pin := range r.Heads {
		if _, err = tx.Exec(ctx, `INSERT INTO memory_experience_reconciled_pins(source_id,kind,row_id,row_hash,revision)VALUES($1,$2,$3,$4,$5)`, source, pin.Kind, pin.ID, pin.Hash, pin.Revision); err != nil {
			return err
		}
	}
	return nil
}

func putTerminalActionsTx(ctx context.Context, tx pgx.Tx, source string, o Observation) error {
	if o.TerminalAttempt == nil || o.ReconciledAttempt != nil {
		return nil
	}
	for _, action := range o.Actions {
		digest := ActionHash(action)
		if _, err := tx.Exec(ctx, `INSERT INTO memory_experience_terminal_actions(source_id,action_id,observed_effect,evidence_refs,action_hash)VALUES($1,$2,$3,$4,$5)ON CONFLICT DO NOTHING`, source, action.ID, action.Effect, action.Evidence, digest); err != nil {
			return err
		}
		var recorded string
		if err := tx.QueryRow(ctx, `SELECT action_hash FROM memory_experience_terminal_actions WHERE source_id=$1 AND action_id=$2`, source, action.ID).Scan(&recorded); err != nil || recorded != digest {
			return ErrEvidence
		}
	}
	return nil
}
