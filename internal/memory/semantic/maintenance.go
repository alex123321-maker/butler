package semantic

import (
	"context"
	"encoding/json"
	"github.com/butler/butler/internal/memory/jobs"
)

// ValidProof validates the initial owner review of a model space.
func ValidProof(p Proof) bool { return p.valid() }

// VerifyMaintenance records mechanical coverage under an explicit current
// maintenance policy. It makes no claim of a new quality experiment or fact
// confirmation. Guard must carry the actual durable job claim and manifest.
func (s *Store) VerifyMaintenance(ctx context.Context, id, jobID, spaceID string) error {
	if s.Guard == nil || jobs.IndexClaimID(ctx) != jobID {
		return ErrAccess
	}
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	var policyID string
	var ownerID string
	var revision int64
	var enabled bool
	e = tx.QueryRow(ctx, `SELECT p.policy_id,p.owner_id,p.revision,p.enabled AND p.expires_at>clock_timestamp() AND p.revision=l.policy_revision AND p.space_id=$3 AND j.owner_id=p.owner_id AND j.kind='index' AND j.purpose='memory_index_maintenance' AND j.status='running' FROM memory_index_policy_jobs l JOIN memory_index_policies p USING(policy_id) JOIN memory_aux_jobs j USING(job_id) JOIN memory_index_jobs i USING(job_id) WHERE j.job_id=$1 AND i.generation_id=$2 FOR SHARE OF p`, jobID, id, spaceID).Scan(&policyID, &ownerID, &revision, &enabled)
	if e != nil {
		return e
	}
	if !enabled {
		return ErrAccess
	}
	g, e := readGeneration(ctx, tx, id, true)
	if e != nil {
		return e
	}
	if g.State != "building" || g.Space.ID() != spaceID || g.Scope != (Scope{Type: "user", ID: ownerID}) {
		return ErrStale
	}
	if e = s.checkCoverage(ctx, tx, g); e != nil {
		return e
	}
	raw, _ := json.Marshal(map[string]any{"basis": "owner_authorized_maintenance", "policy_id": policyID, "policy_revision": revision, "job_id": jobID, "space_id": spaceID})
	_, e = tx.Exec(ctx, `UPDATE semantic_index_generations SET state='ready',quality_proof=$2,verified_at=clock_timestamp() WHERE id=$1`, id, raw)
	if e != nil {
		return e
	}
	return tx.Commit(ctx)
}
