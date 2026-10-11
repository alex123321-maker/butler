package generatedtools

import (
	"context"
	"encoding/json"

	"github.com/butler/butler/internal/actionauth"
	converterv1 "github.com/butler/butler/internal/gen/converter/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

func (s *Store) Test(ctx context.Context, call *toolbrokerv1.ToolCall, contract *toolbrokerv1.ToolContract, hash string) (Report, error) {
	c, e := s.run(ctx, call.GetRunId())
	if e != nil {
		return Report{}, e
	}
	ctx = runContext(ctx, c)
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return Report{}, e
	}
	defer tx.Rollback(ctx)
	m, now, e := s.guardRunTx(ctx, tx, c)
	if e != nil {
		return Report{}, e
	}
	r, e := s.metadataTx(ctx, tx, hash)
	if e != nil {
		return Report{}, e
	}
	p, n, data, e := s.packageTx(ctx, tx, r)
	if e != nil {
		return Report{}, e
	}
	if _, e = s.permission(m, n, now); e != nil {
		return Report{}, e
	}
	var state string
	if tx.QueryRow(ctx, `SELECT status FROM converter_states WHERE owner_id=$1 AND digest=$2 FOR SHARE`, s.owner, hash).Scan(&state) != nil || state == "revoked" {
		return Report{}, ErrAccess
	}
	if e = s.dispatchTx(ctx, tx, c, call, contract); e != nil {
		return Report{}, e
	}
	if e = tx.Commit(ctx); e != nil {
		return Report{}, e
	}
	response, e := s.runtime.TestPackage(ctx, &converterv1.TestPackageRequest{Proof: proof(ctx, call), PackageSource: dataEnvelope(data[r.BodySource.ID]), NeedSource: dataEnvelope(data[r.NeedSource.ID]), PackageHash: r.Digest, NeedHash: r.NeedHash, ProfilePin: Hash(s.profile)})
	if e != nil {
		return Report{}, ErrChecks
	}
	var report Report
	if Decode([]byte(response.GetReportJson()), &report) != nil {
		return Report{}, ErrChecks
	}
	// The author/runtime cannot fabricate a pass or replace the fixed checks.
	// Core recomputes only synthetic checks, never the working document action.
	expected, checkError := Check(ctx, p, n, s.profile)
	if Hash(report) != Hash(expected) || checkError != nil && checkError != ErrChecks {
		return report, ErrChecks
	}
	tx, e = s.pool.Begin(ctx)
	if e != nil {
		return report, e
	}
	defer tx.Rollback(ctx)
	m, now, e = s.guardRunTx(ctx, tx, c)
	if e != nil {
		return report, e
	}
	current, e := s.metadataTx(ctx, tx, hash)
	if e != nil || current.Generation != r.Generation {
		return report, ErrAccess
	}
	fresh, freshNeed, _, e := s.packageTx(ctx, tx, current)
	if e != nil || fresh.Digest != p.Digest || Hash(freshNeed) != Hash(n) {
		return report, ErrAccess
	}
	if _, e = s.permission(m, n, now); e != nil {
		return report, e
	}
	var generation int64
	if tx.QueryRow(ctx, `SELECT generation FROM converter_catalog WHERE owner_id=$1 AND converter_id=$2 FOR SHARE`, s.owner, current.ID).Scan(&generation) != nil || generation != current.Generation {
		return report, ErrAccess
	}
	if tx.QueryRow(ctx, `SELECT status FROM converter_states WHERE owner_id=$1 AND digest=$2 FOR UPDATE`, s.owner, hash).Scan(&state) != nil || state == "revoked" {
		return report, ErrAccess
	}
	if e = s.dispatchTx(ctx, tx, c, call, contract); e != nil {
		return report, e
	}
	d := actionauth.DispatchIdentityFromContext(ctx)
	if _, e = tx.Exec(ctx, `INSERT INTO converter_validations(owner_id,digest,run_id,action_id,report_json) VALUES($1,$2,$3,$4,$5::jsonb)`, s.owner, hash, c.RunID, d.ActionID, encoded(report)); e != nil {
		return report, e
	}
	if report.Passed && state == "candidate" {
		if _, e = tx.Exec(ctx, `UPDATE converter_states SET status='tested',updated_at=clock_timestamp() WHERE owner_id=$1 AND digest=$2`, s.owner, hash); e != nil {
			return report, e
		}
	}
	if _, e = tx.Exec(ctx, `INSERT INTO converter_events(owner_id,converter_id,digest,generation,kind,run_id,action_id) VALUES($1,$2,$3,$4,$5,$6,$7)`, s.owner, r.ID, hash, r.Generation, map[bool]string{true: "checks_passed", false: "checks_failed"}[report.Passed], c.RunID, d.ActionID); e != nil {
		return report, e
	}
	return report, tx.Commit(ctx)
}

func CheckOutput(ctx context.Context, n Need, p Profile, input, output []byte) error {
	// Independently derive the expected mapping from the trusted Need rather than
	// the author program or a returned runtime verdict. No working input is used
	// in package development or synthetic testing.
	built, e := Build(n, "1.0.0", p)
	if e != nil {
		return ErrChecks
	}
	if _, e = actionauth.ArgumentsHash(string(output)); e != nil {
		return ErrChecks
	}
	want, e := Convert(ctx, built.Manifest.Code, n.Limits, input)
	if e != nil || int64(len(output)) > n.Limits.OutputBytes || !json.Valid(output) || !jsonEqual(output, want) {
		return ErrChecks
	}
	return nil
}
