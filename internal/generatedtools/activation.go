package generatedtools

import (
	"context"
	"encoding/json"

	"github.com/butler/butler/internal/actionauth"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/memory/catalog"
)

func (s *Store) Activate(ctx context.Context, call *toolbrokerv1.ToolCall, contract *toolbrokerv1.ToolContract, hash string, generation int64) (Record, error) {
	c, e := s.run(ctx, call.GetRunId())
	if e != nil {
		return Record{}, e
	}
	ctx = runContext(ctx, c)
	if e = actionauth.CheckExecutionPolicy(c, actionauth.EffectUnknown); e != nil {
		return Record{}, e
	}
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return Record{}, e
	}
	defer tx.Rollback(ctx)
	m, now, e := s.guardRunTx(ctx, tx, c)
	if e != nil {
		return Record{}, e
	}
	r, e := s.metadataTx(ctx, tx, hash)
	if e != nil {
		return Record{}, e
	}
	// A saved pass is not enough after its source document or output was deleted.
	var raw []byte
	if tx.QueryRow(ctx, `SELECT output_ref_json FROM converter_trials WHERE owner_id=$1 AND digest=$2 AND status='passed' ORDER BY finished_at DESC LIMIT 1`, s.owner, hash).Scan(&raw) != nil {
		return r, ErrChecks
	}
	var output catalog.SourceRef
	if json.Unmarshal(raw, &output) != nil {
		return r, ErrChecks
	}
	_, n, _, e := s.packageTx(ctx, tx, r, output)
	if e != nil {
		return r, e
	}
	if _, e = s.permission(m, n, now); e != nil {
		return r, e
	}
	var current int64
	var previous string
	if tx.QueryRow(ctx, `SELECT generation,COALESCE(active_digest,'') FROM converter_catalog WHERE owner_id=$1 AND converter_id=$2 FOR UPDATE`, s.owner, r.ID).Scan(&current, &previous) != nil || current != generation {
		return r, ErrAccess
	}
	var state string
	if tx.QueryRow(ctx, `SELECT status FROM converter_states WHERE owner_id=$1 AND digest=$2 FOR UPDATE`, s.owner, hash).Scan(&state) != nil || state == "candidate" || state == "revoked" {
		return r, ErrChecks
	}
	var report Report
	if tx.QueryRow(ctx, `SELECT report_json FROM converter_validations WHERE owner_id=$1 AND digest=$2 ORDER BY validation_id DESC LIMIT 1`, s.owner, hash).Scan(&raw) != nil || json.Unmarshal(raw, &report) != nil || !report.Passed || report.PackageHash != r.Digest || report.NeedHash != r.NeedHash || report.ProfileHash != Hash(s.profile) || report.Checker != Checker {
		return r, ErrChecks
	}
	if e = s.dispatchTx(ctx, tx, c, call, contract); e != nil {
		return r, e
	}
	if previous != hash {
		if _, e = tx.Exec(ctx, `UPDATE converter_states SET status='obsolete',updated_at=clock_timestamp() WHERE owner_id=$1 AND digest=$2 AND status='active'`, s.owner, previous); e != nil {
			return r, e
		}
		if _, e = tx.Exec(ctx, `UPDATE converter_catalog SET generation=generation+1,active_digest=$3,updated_at=clock_timestamp() WHERE owner_id=$1 AND converter_id=$2`, s.owner, r.ID, hash); e != nil {
			return r, e
		}
	}
	if _, e = tx.Exec(ctx, `UPDATE converter_states SET status='active',updated_at=clock_timestamp() WHERE owner_id=$1 AND digest=$2`, s.owner, hash); e != nil {
		return r, e
	}
	r, e = s.metadataTx(ctx, tx, hash)
	if e != nil {
		return r, e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO converter_events(owner_id,converter_id,digest,generation,kind,run_id,action_id) VALUES($1,$2,$3,$4,$5,$6,$7)`, s.owner, r.ID, hash, r.Generation, call.GetToolName(), c.RunID, actionauth.DispatchIdentityFromContext(ctx).ActionID); e != nil {
		return r, e
	}
	// Rechecking exact parents also refreshes the live owner clock before commit.
	if _, _, _, e = s.packageTx(ctx, tx, r, output); e != nil {
		return r, e
	}
	return r, tx.Commit(ctx)
}
