package generatedtools

import (
	"context"
	"encoding/json"

	"github.com/butler/butler/internal/actionauth"
	converterv1 "github.com/butler/butler/internal/gen/converter/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/jackc/pgx/v5"
)

type TrialResult struct {
	ActionID string  `json:"action_id"`
	Status   string  `json:"status"`
	Output   *Output `json:"output,omitempty"`
}

func (s *Store) lockVersionTx(ctx context.Context, tx pgx.Tx, r Record, active bool, exclusive bool) error {
	var generation int64
	var current string
	query := `SELECT generation,COALESCE(active_digest,'') FROM converter_catalog WHERE owner_id=$1 AND converter_id=$2 FOR SHARE`
	if exclusive {
		query = `SELECT generation,COALESCE(active_digest,'') FROM converter_catalog WHERE owner_id=$1 AND converter_id=$2 FOR UPDATE`
	}
	if tx.QueryRow(ctx, query, s.owner, r.ID).Scan(&generation, &current) != nil || generation != r.Generation || active && current != r.Digest {
		return ErrAccess
	}
	var state string
	if tx.QueryRow(ctx, `SELECT status FROM converter_states WHERE owner_id=$1 AND digest=$2 FOR SHARE`, s.owner, r.Digest).Scan(&state) != nil || state == "candidate" || state == "revoked" || active && state != "active" {
		return ErrAccess
	}
	var raw []byte
	if tx.QueryRow(ctx, `SELECT report_json FROM converter_validations WHERE owner_id=$1 AND digest=$2 ORDER BY validation_id DESC LIMIT 1`, s.owner, r.Digest).Scan(&raw) != nil {
		return ErrChecks
	}
	var report Report
	if json.Unmarshal(raw, &report) != nil || !report.Passed || report.PackageHash != r.Digest || report.NeedHash != r.NeedHash || report.ProfileHash != Hash(s.profile) || report.Checker != Checker {
		return ErrChecks
	}
	return nil
}

func (s *Store) Apply(ctx context.Context, call *toolbrokerv1.ToolCall, contract *toolbrokerv1.ToolContract, hash string, input catalog.SourceRef, trial bool) (TrialResult, error) {
	d := actionauth.DispatchIdentityFromContext(ctx)
	result := TrialResult{ActionID: d.ActionID, Status: "unknown"}
	c, e := s.run(ctx, call.GetRunId())
	if e != nil {
		return result, e
	}
	ctx = runContext(ctx, c)
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return result, e
	}
	defer tx.Rollback(ctx)
	m, now, e := s.guardRunTx(ctx, tx, c)
	if e != nil {
		return result, e
	}
	r, e := s.metadataTx(ctx, tx, hash)
	if e != nil {
		return result, e
	}
	_, n, data, e := s.packageTx(ctx, tx, r, input)
	if e != nil {
		return result, e
	}
	permission, e := s.permission(m, n, now)
	if e != nil {
		return result, e
	}
	if e = s.lockVersionTx(ctx, tx, r, !trial, trial); e != nil {
		return result, e
	}
	if e = s.dispatchTx(ctx, tx, c, call, contract); e != nil {
		return result, e
	}
	if trial {
		var count int64
		if tx.QueryRow(ctx, `SELECT count(*) FROM converter_trials WHERE owner_id=$1 AND digest=$2`, s.owner, hash).Scan(&count) != nil || count >= permission.Trials {
			return result, ErrLimit
		}
		// Serialize allocation under the catalog row; a crashed slot is spent.
		if _, e = tx.Exec(ctx, `INSERT INTO converter_trials(owner_id,action_id,digest,run_id,task_id,task_revision,input_ref_json,status) VALUES($1,$2,$3,$4,$5,$6,$7::jsonb,'running')`, s.owner, d.ActionID, hash, c.RunID, c.TaskID, c.TaskRevision, encoded(input)); e != nil {
			return result, e
		}
	}
	if e = tx.Commit(ctx); e != nil {
		return result, e
	}
	response, runtimeErr := s.runtime.Convert(ctx, &converterv1.ConvertRequest{Proof: proof(ctx, call), PackageSource: dataEnvelope(data[r.BodySource.ID]), NeedSource: dataEnvelope(data[r.NeedSource.ID]), InputSource: dataEnvelope(data[input.ID]), PackageHash: r.Digest, NeedHash: r.NeedHash, ProfilePin: Hash(s.profile)})
	valid := runtimeErr == nil && response.GetMediaType() == "application/json" && response.GetOutputHash() == HashBytes(response.GetOutput()) && CheckOutput(ctx, n, s.profile, []byte(data[input.ID].Body), response.GetOutput()) == nil
	if !valid && !trial {
		return result, ErrChecks
	}
	tx, e = s.pool.Begin(ctx)
	if e != nil {
		return result, e
	}
	defer tx.Rollback(ctx)
	m, now, e = s.guardRunTx(ctx, tx, c)
	if e != nil {
		return result, e
	}
	current, e := s.metadataTx(ctx, tx, hash)
	if e != nil || current.Generation != r.Generation {
		return result, ErrAccess
	}
	_, freshNeed, _, e := s.packageTx(ctx, tx, current, input)
	if e != nil || Hash(freshNeed) != Hash(n) {
		return result, ErrAccess
	}
	if _, e = s.permission(m, n, now); e != nil {
		return result, e
	}
	if e = s.lockVersionTx(ctx, tx, current, !trial, trial); e != nil {
		return result, e
	}
	if trial {
		var status string
		if tx.QueryRow(ctx, `SELECT status FROM converter_trials WHERE owner_id=$1 AND action_id=$2 AND digest=$3 FOR UPDATE`, s.owner, d.ActionID, hash).Scan(&status) != nil || status != "running" {
			return result, ErrAccess
		}
	}
	if e = s.dispatchTx(ctx, tx, c, call, contract); e != nil {
		return result, e
	}
	if !valid {
		result.Status = "failed"
		if runtimeErr != nil {
			result.Status = "unknown"
		}
		if _, e = tx.Exec(ctx, `UPDATE converter_trials SET status=$3,finished_at=clock_timestamp() WHERE owner_id=$1 AND action_id=$2 AND status='running'`, s.owner, d.ActionID, result.Status); e != nil {
			return result, e
		}
		return result, tx.Commit(ctx)
	}
	// All original document/package/need edges remain deletion dependencies.
	body, e := s.sources.CreateDerivedTx(ctx, tx, []catalog.SourceRef{input, r.BodySource, r.NeedSource}, "memory_read", r.ID+" conversion", string(response.GetOutput()), json.RawMessage(encoded(map[string]string{"kind": "converter_output_v1", "package_hash": r.Digest, "need_hash": r.NeedHash, "sha256": response.GetOutputHash()})))
	if e != nil {
		return result, e
	}
	out := Output{ConverterID: r.ID, PackageHash: r.Digest, NeedHash: r.NeedHash, Source: sourceRef(body), SHA256: response.GetOutputHash(), MediaType: "application/json", InstructionAuthority: false}
	result.Status = "passed"
	result.Output = &out
	if trial {
		if _, e = tx.Exec(ctx, `UPDATE converter_trials SET status='passed',output_ref_json=$3::jsonb,output_hash=$4,finished_at=clock_timestamp() WHERE owner_id=$1 AND action_id=$2 AND status='running'`, s.owner, d.ActionID, encoded(out.Source), out.SHA256); e != nil {
			return result, e
		}
	} else {
		if _, e = tx.Exec(ctx, `INSERT INTO converter_usages(owner_id,action_id,digest,run_id,task_id,task_revision,constraints_revision,generation,output_ref_json,output_hash) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10)`, s.owner, d.ActionID, hash, c.RunID, c.TaskID, c.TaskRevision, c.ConstraintsRevision, r.Generation, encoded(out.Source), out.SHA256); e != nil {
			return result, e
		}
	}
	return result, tx.Commit(ctx)
}
