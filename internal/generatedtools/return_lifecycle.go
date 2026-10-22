package generatedtools

import (
	"context"
	"fmt"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
)

func (s *Store) ownedSnapshotTx(ctx context.Context, tx pgx.Tx) (actionauth.ContextSnapshot, error) {
	token, ok := ownership.TokenFromContext(ctx)
	var c actionauth.ContextSnapshot
	if !ok {
		return c, ErrAccess
	}
	c.RunID, c.SessionKey, c.LeaseID, c.LeaseEpoch = token.RunID, token.SessionKey, token.LeaseID, token.FenceEpoch
	if tx.QueryRow(ctx, `SELECT s.user_id,COALESCE(r.task_id,''),COALESCE(r.task_revision,0),COALESCE(t.constraints_revision,0),r.current_state,COALESCE(t.execution_policy,'normal') FROM runs r JOIN sessions s USING(session_key) LEFT JOIN tasks t ON t.task_id=r.task_id WHERE r.run_id=$1 AND r.session_key=$2`, token.RunID, token.SessionKey).Scan(&c.OwnerID, &c.TaskID, &c.TaskRevision, &c.ConstraintsRevision, &c.RunState, &c.ExecutionPolicy) != nil {
		return c, ErrAccess
	}
	return c, nil
}

// A collected child package never becomes an ambient grant to its report or
// session, nor can it outlive current accepted role/final/report privacy proof.
func (s *Store) checkReturnedPackageTx(ctx context.Context, tx pgx.Tx, c actionauth.ContextSnapshot, r Record) error {
	var childID string
	err := tx.QueryRow(ctx, `SELECT a.task_id FROM converter_versions v JOIN runs created ON created.run_id=v.created_by_run JOIN converter_developer_snapshots a ON a.task_id=created.task_id WHERE v.owner_id=$1 AND v.digest=$2`, r.OwnerID, r.Digest).Scan(&childID)
	if err == pgx.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	if c.TaskID == childID {
		return nil
	}
	snapshot, err := ReadDeveloperSnapshotTx(ctx, tx, s.sources, c.OwnerID, childID)
	if err != nil || snapshot == nil || snapshot.Binding.TaskID != c.TaskID || snapshot.Binding.TaskRevision != c.TaskRevision || snapshot.Binding.ConstraintsRevision != c.ConstraintsRevision {
		return fmt.Errorf("returned snapshot: %w", ErrAccess)
	}
	p, err := ReadDeveloperReturnTx(ctx, tx, *snapshot)
	if err != nil || p.Candidate.BodySource != r.BodySource || p.Candidate.Digest != r.Digest || p.Candidate.ProfilePin != r.ProfilePin {
		return fmt.Errorf("returned projection: %w", ErrAccess)
	}
	checker := collectionAccess{s, c, *snapshot, p}
	_, err = s.sources.AccessTx(catalog.WithAccessChecker(ctx, checker), tx, developerReturnSources(p), "memory_read")
	if err != nil {
		return fmt.Errorf("returned access: %w", err)
	}
	return nil
}

// Compose after current run/source authority on provider/recovery/final paths.
// It is read-only and uses neither an old worker lease nor another connection.
func CheckDeveloperReturnLifecycleTx(ctx context.Context, tx pgx.Tx, sources *catalog.Store, runID string) error {
	// This guard is also composed into retained publication reads/delivery.
	// An unrelated run has no returned-package authority to revalidate and must
	// not borrow a completed worker's lease merely to expose its own message.
	// Check actual captured provenance before requiring the execution context;
	// returned packages keep the original owned-snapshot and lifecycle checks.
	var applicable bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM memory_run_sources p JOIN converter_versions v ON v.source_ref_json->>'source_id'=p.source_id JOIN runs created ON created.run_id=v.created_by_run JOIN converter_developer_snapshots a ON a.task_id=created.task_id WHERE p.run_id=r.run_id) FROM runs r WHERE r.run_id=$1`, runID).Scan(&applicable); err != nil {
		return ErrAccess
	}
	if !applicable {
		return nil
	}
	s := Store{sources: sources}
	c, err := s.ownedSnapshotTx(ctx, tx)
	if err != nil || c.RunID != runID {
		return ErrAccess
	}
	if c.TaskID == "" {
		return nil
	}
	s.owner = c.OwnerID
	rows, err := tx.Query(ctx, `SELECT DISTINCT v.digest FROM memory_run_sources p JOIN converter_versions v ON v.source_ref_json->>'source_id'=p.source_id JOIN runs created ON created.run_id=v.created_by_run JOIN converter_developer_snapshots a ON a.task_id=created.task_id WHERE p.run_id=$1 AND v.owner_id=$2 ORDER BY v.digest`, runID, c.OwnerID)
	if err != nil {
		return err
	}
	digests := []string{}
	for rows.Next() {
		var hash string
		if err = rows.Scan(&hash); err != nil {
			break
		}
		digests = append(digests, hash)
		if len(digests) > 16 {
			err = ErrLimit
			break
		}
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return err
	}
	for _, hash := range digests {
		r, e := s.metadataTx(ctx, tx, hash)
		if e != nil {
			return e
		}
		if e = s.checkReturnedPackageTx(ctx, tx, c, r); e != nil {
			return e
		}
	}
	return nil
}
