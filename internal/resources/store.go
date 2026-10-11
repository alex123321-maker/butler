package resources

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
)

const MaxLeaseTTL = 30 * time.Second
const MaxResources = 16

// Store uses only a caller transaction. Authority/domain locks always precede
// lexicographically ordered resource rows; no network call occurs under them.
type Store struct {
	guard  Guard
	owners OwnerGuard
}

func New(guard Guard, owners OwnerGuard) (*Store, error) {
	if guard == nil || owners == nil {
		return nil, ErrDenied
	}
	return &Store{guard: guard, owners: owners}, nil
}
func (s *Store) guardTx(ctx context.Context, tx pgx.Tx, b Binding) error {
	if s == nil || s.guard == nil || !validBinding(b) {
		return ErrDenied
	}
	return s.guard.CheckTx(ctx, tx, b)
}

// RegisterTx is invoked by the owning kernel when allocating a resource. The
// owner is immutable; acquiring a lease cannot create an arbitrary resource.
func (s *Store) RegisterTx(ctx context.Context, tx pgx.Tx, b Binding, id string) error {
	if !validID(id) {
		return ErrDenied
	}
	if err := s.guardTx(ctx, tx, b); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO execution_resources(resource_id,owner_id) VALUES($1,$2) ON CONFLICT(resource_id) DO NOTHING`, id, b.OwnerID); err != nil {
		return err
	}
	var owner string
	if err := tx.QueryRow(ctx, `SELECT owner_id FROM execution_resources WHERE resource_id=$1 FOR SHARE`, id).Scan(&owner); err != nil {
		return err
	}
	if owner != b.OwnerID {
		return ErrDenied
	}
	return nil
}

// AcquireTx sorts the complete requested set and grants it all or none. Its
// savepoint undoes earlier leases/fences even if an outer caller commits after
// denial. No automatic wait/retry loop or lease renewal extends the deadline.
func (s *Store) AcquireTx(ctx context.Context, tx pgx.Tx, b Binding, wanted []Request, ttl time.Duration) (proofs []Proof, err error) {
	if len(wanted) < 1 || len(wanted) > MaxResources || ttl < time.Millisecond || ttl > MaxLeaseTTL {
		return nil, ErrDenied
	}
	wanted = append([]Request(nil), wanted...)
	sort.Slice(wanted, func(i, j int) bool { return wanted[i].ResourceID < wanted[j].ResourceID })
	for i, r := range wanted {
		if !validID(r.ResourceID) || r.ExpectedVersion < 1 || (r.Mode != Read && r.Mode != Write) || i > 0 && wanted[i-1].ResourceID == r.ResourceID {
			return nil, ErrDenied
		}
	}
	if err = s.guardTx(ctx, tx, b); err != nil {
		return nil, err
	}
	sp, err := tx.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = sp.Rollback(cleanup)
	}()
	// Clock/deadline is re-read after every contended row lock, not just once
	// before waiting. Claim checks it again at the irreversible boundary.
	for _, r := range wanted {
		var owner, inflight string
		var version, fence int64
		err = sp.QueryRow(ctx, `SELECT owner_id,version,fence_epoch,inflight_lease_id FROM execution_resources WHERE resource_id=$1 FOR UPDATE`, r.ResourceID).Scan(&owner, &version, &fence, &inflight)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrDenied
		}
		if err != nil {
			return nil, err
		}
		if owner != b.OwnerID {
			return nil, ErrDenied
		}
		if version != r.ExpectedVersion || inflight != "" {
			return nil, ErrConflict
		}
		var conflict bool
		err = sp.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM execution_resource_leases WHERE resource_id=$1 AND state IN ('held','claimed') AND expires_at>clock_timestamp() AND ($2='write' OR mode='write'))`, r.ResourceID, string(r.Mode)).Scan(&conflict)
		if err != nil {
			return nil, err
		}
		if conflict {
			return nil, ErrConflict
		}
		if r.Mode == Write {
			if err = sp.QueryRow(ctx, `UPDATE execution_resources SET fence_epoch=fence_epoch+1 WHERE resource_id=$1 RETURNING fence_epoch`, r.ResourceID).Scan(&fence); err != nil {
				return nil, err
			}
		}
		var raw [24]byte
		if _, err = rand.Read(raw[:]); err != nil {
			return nil, err
		}
		id := "resource_" + hex.EncodeToString(raw[:])
		tag, e := sp.Exec(ctx, `INSERT INTO execution_resource_leases(lease_id,resource_id,owner_id,run_id,session_key,task_id,task_revision,constraints_revision,owner_epoch,mode,resource_version,fence_epoch,expires_at)
 SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,LEAST(clock_timestamp()+$13*INTERVAL '1 millisecond',$14::timestamptz) WHERE $14::timestamptz>clock_timestamp()`, id, r.ResourceID, b.OwnerID, b.RunID, b.SessionKey, b.TaskID, b.TaskRevision, b.ConstraintsRevision, b.LeaseEpoch, string(r.Mode), version, fence, ttl.Milliseconds(), b.Deadline)
		if e != nil {
			return nil, e
		}
		if tag.RowsAffected() != 1 {
			return nil, ErrDenied
		}
		proofs = append(proofs, Proof{LeaseID: id, ResourceID: r.ResourceID, FenceEpoch: fence, ExpectedResourceVersion: version, RunID: b.RunID, SessionKey: b.SessionKey, TaskID: b.TaskID})
	}
	if err = s.guardTx(ctx, sp, b); err != nil {
		return nil, err
	}
	if err = sp.Commit(ctx); err != nil {
		return nil, err
	}
	return proofs, nil
}

// CheckTx holds shared resource/lease rows until commit. Write checks require
// an exact exclusive lease; a write lease may also read its own resource.
func (s *Store) CheckTx(ctx context.Context, tx pgx.Tx, b Binding, p Proof, mode Mode) error {
	if !validProof(b, p) || (mode != Read && mode != Write) {
		return ErrDenied
	}
	if err := s.guardTx(ctx, tx, b); err != nil {
		return err
	}
	var ok bool
	lock := " FOR SHARE OF r,l"
	if mode == Write {
		lock = " FOR UPDATE OF r,l"
	}
	err := tx.QueryRow(ctx, `SELECT r.owner_id=$3 AND l.owner_id=$3 AND l.run_id=$4 AND l.session_key=$5 AND l.task_id=$6 AND l.task_revision=$7 AND l.constraints_revision=$8 AND l.owner_epoch=$9
 AND r.version=$10 AND l.resource_version=$10 AND r.fence_epoch=$11 AND l.fence_epoch=$11 AND l.expires_at>clock_timestamp() AND $12::timestamptz>clock_timestamp()
 AND l.state IN ('held','claimed') AND ($13='read' OR l.mode='write') AND (r.inflight_lease_id='' OR r.inflight_lease_id=l.lease_id)
 FROM execution_resources r JOIN execution_resource_leases l ON l.resource_id=r.resource_id WHERE r.resource_id=$1 AND l.lease_id=$2`+lock, p.ResourceID, p.LeaseID, b.OwnerID, b.RunID, b.SessionKey, b.TaskID, b.TaskRevision, b.ConstraintsRevision, b.LeaseEpoch, p.ExpectedResourceVersion, p.FenceEpoch, b.Deadline, string(mode)).Scan(&ok)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && !ok {
		return ErrDenied
	}
	return err
}

func (s *Store) lockProofTx(ctx context.Context, tx pgx.Tx, b Binding, p Proof) error {
	if !validProof(b, p) {
		return ErrDenied
	}
	if err := s.guardTx(ctx, tx, b); err != nil {
		return err
	}
	var id string
	if err := tx.QueryRow(ctx, `SELECT resource_id FROM execution_resources WHERE resource_id=$1 FOR UPDATE`, p.ResourceID).Scan(&id); err != nil {
		return err
	}
	return s.CheckTx(ctx, tx, b, p, Write)
}

// ClaimTx precedes a concrete write. The durable inflight marker survives the
// executor crash and lease expiry, blocking successors until an owner reconciles
// the actual effect. Expiry by itself cannot prove that a remote write stopped.
func (s *Store) ClaimTx(ctx context.Context, tx pgx.Tx, b Binding, p Proof) error {
	if err := s.lockProofTx(ctx, tx, b, p); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE execution_resource_leases SET state='claimed' WHERE lease_id=$1 AND state='held' AND expires_at>clock_timestamp() AND $2::timestamptz>clock_timestamp()`, p.LeaseID, b.Deadline)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrConflict
	}
	_, err = tx.Exec(ctx, `UPDATE execution_resources SET inflight_lease_id=$2 WHERE resource_id=$1`, p.ResourceID, p.LeaseID)
	return err
}

// CompleteTx is called only for a confirmed local/remote write. It advances the
// observation version and consumes the lease; a later plan must observe afresh.
func (s *Store) CompleteTx(ctx context.Context, tx pgx.Tx, b Binding, p Proof, evidenceID string) (int64, error) {
	if !validEvidenceID(evidenceID) {
		return 0, ErrDenied
	}
	if err := s.lockProofTx(ctx, tx, b, p); err != nil {
		return 0, err
	}
	tag, err := tx.Exec(ctx, `UPDATE execution_resource_leases SET state='completed',evidence_id=$2,finished_at=clock_timestamp() WHERE lease_id=$1 AND state='claimed' AND expires_at>clock_timestamp() AND $3::timestamptz>clock_timestamp()`, p.LeaseID, evidenceID, b.Deadline)
	if err != nil {
		return 0, err
	}
	if tag.RowsAffected() != 1 {
		return 0, ErrConflict
	}
	var version int64
	err = tx.QueryRow(ctx, `UPDATE execution_resources SET version=version+1,inflight_lease_id='' WHERE resource_id=$1 AND inflight_lease_id=$2 RETURNING version`, p.ResourceID, p.LeaseID).Scan(&version)
	return version, err
}

// ReleaseTx releases a live unclaimed read/write. Claimed writes remain unknown
// and blocked; abandoning an executor never establishes effect absence.
func (s *Store) ReleaseTx(ctx context.Context, tx pgx.Tx, b Binding, p Proof) error {
	if !validProof(b, p) {
		return ErrDenied
	}
	if err := s.guardTx(ctx, tx, b); err != nil {
		return err
	}
	var id string
	if err := tx.QueryRow(ctx, `SELECT resource_id FROM execution_resources WHERE resource_id=$1 FOR UPDATE`, p.ResourceID).Scan(&id); err != nil {
		return err
	}
	if err := s.CheckTx(ctx, tx, b, p, Read); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `UPDATE execution_resource_leases SET state=CASE WHEN state='claimed' THEN 'unknown' ELSE 'released' END,finished_at=clock_timestamp() WHERE lease_id=$1`, p.LeaseID)
	return err
}

// ReconcileTx accepts a trusted owner decision supported by an independently
// obtained evidence ID. It always invalidates prior observations. It grants no
// lease and never retries/replays a write. A still-live executor must be stopped
// by owner controls before resolving its inflight marker.
func (s *Store) ReconcileTx(ctx context.Context, tx pgx.Tx, owner, id, lease, evidenceID string, expected int64) (int64, error) {
	if s == nil || s.owners == nil || owner == "" || !validID(id) || lease == "" || expected < 1 || !validEvidenceID(evidenceID) {
		return 0, ErrDenied
	}
	if err := s.owners.CheckReconciliationTx(ctx, tx, Decision{OwnerID: owner, ResourceID: id, LeaseID: lease, EvidenceID: evidenceID, ExpectedVersion: expected}); err != nil {
		return 0, err
	}
	var actualOwner, inflight string
	var version int64
	if err := tx.QueryRow(ctx, `SELECT owner_id,version,inflight_lease_id FROM execution_resources WHERE resource_id=$1 FOR UPDATE`, id).Scan(&actualOwner, &version, &inflight); err != nil {
		return 0, err
	}
	if actualOwner != owner {
		return 0, ErrDenied
	}
	if version != expected || inflight != lease {
		return 0, ErrConflict
	}
	var expired bool
	if err := tx.QueryRow(ctx, `SELECT expires_at<=clock_timestamp() FROM execution_resource_leases WHERE lease_id=$1 AND resource_id=$2 FOR UPDATE`, lease, id).Scan(&expired); err != nil {
		return 0, err
	}
	if !expired {
		return 0, ErrConflict
	}
	if _, err := tx.Exec(ctx, `UPDATE execution_resource_leases SET state='reconciled',evidence_id=$2,finished_at=clock_timestamp() WHERE lease_id=$1`, lease, evidenceID); err != nil {
		return 0, err
	}
	if err := tx.QueryRow(ctx, `UPDATE execution_resources SET version=version+1,fence_epoch=fence_epoch+1,inflight_lease_id='' WHERE resource_id=$1 RETURNING version`, id).Scan(&version); err != nil {
		return 0, err
	}
	return version, nil
}

func validEvidenceID(id string) bool {
	if len(id) < 1 || len(id) > 256 {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.' || c == ':') {
			return false
		}
	}
	return true
}
