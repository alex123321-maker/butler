package generatedtools

import (
	"context"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/jackc/pgx/v5"
	"sort"
)

func (s *Store) developerTx(ctx context.Context, tx pgx.Tx, c actionauth.ContextSnapshot, n Need, ref catalog.SourceRef) (*DeveloperSnapshot, error) {
	snapshot, err := ReadDeveloperSnapshotTx(ctx, tx, s.sources, s.owner, c.TaskID)
	if err != nil || snapshot == nil {
		return snapshot, err
	}
	if snapshot.Args.NeedSource != ref || snapshot.NeedID != n.ID || snapshot.NeedHash != Hash(n) || snapshot.Permission.RuntimePin != Hash(s.profile) {
		return nil, ErrAccess
	}
	if err = CheckCompatibleInstalledTx(ctx, tx, s.owner, n); err != nil {
		return nil, err
	}
	return snapshot, nil
}

func (s *Store) developerPackageTx(ctx context.Context, tx pgx.Tx, c actionauth.ContextSnapshot, r Record, n Need) error {
	snapshot, err := s.developerTx(ctx, tx, c, n, r.NeedSource)
	if err != nil || snapshot == nil {
		return err
	}
	var createdTask string
	if r.Version != snapshot.Spec.Version || r.ProfilePin != snapshot.Permission.RuntimePin || r.ID != snapshot.Permission.ConverterID || r.NeedHash != snapshot.NeedHash || tx.QueryRow(ctx, `SELECT COALESCE(r.task_id,'') FROM converter_versions v JOIN runs r ON r.run_id=v.created_by_run WHERE v.owner_id=$1 AND v.digest=$2`, s.owner, r.Digest).Scan(&createdTask) != nil || createdTask != c.TaskID {
		return ErrAccess
	}
	return nil
}

// Compose after the owning current-run/source guard at release/recovery/final.
// No old parent lease or accepted snapshot alone authorizes a fresh model turn.
func CheckDeveloperLifecycleTx(ctx context.Context, tx pgx.Tx, sources DeveloperSources, runID string) error {
	var owner, taskID string
	if tx.QueryRow(ctx, `SELECT COALESCE(t.owner_id,''),COALESCE(t.task_id,'') FROM runs r LEFT JOIN tasks t ON t.task_id=r.task_id WHERE r.run_id=$1`, runID).Scan(&owner, &taskID) != nil {
		return ErrAccess
	}
	if taskID == "" {
		return nil
	}
	_, err := ReadDeveloperSnapshotTx(ctx, tx, sources, owner, taskID)
	return err
}

// No parent installed/global or skill catalog is exported to the developer.
// Its accepted receipt is represented only by the three compiled capabilities.
func developerDiscovery() (Discovery, error) {
	permissions, err := DeveloperPermissions()
	if err != nil {
		return Discovery{}, err
	}
	result := Discovery{Tools: []Capability{}, Skills: []SkillIdentity{}, Compatible: []string{}}
	for _, p := range permissions {
		result.Tools = append(result.Tools, Capability{Name: p.Tool, ContractHash: p.ContractHash, Revision: "1"})
	}
	sort.Slice(result.Tools, func(i, j int) bool { return result.Tools[i].Name < result.Tools[j].Name })
	return result, nil
}

func (s *Store) discoverForRun(ctx context.Context, c actionauth.ContextSnapshot, n Need, ref catalog.SourceRef) (Discovery, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Discovery{}, err
	}
	defer tx.Rollback(ctx)
	if _, _, err = s.guardRunTx(ctx, tx, c); err != nil {
		return Discovery{}, err
	}
	snapshot, err := s.developerTx(ctx, tx, c, n, ref)
	if err != nil {
		return Discovery{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Discovery{}, err
	}
	if snapshot != nil {
		return developerDiscovery()
	}
	return s.environment.Discover(ctx, c.RunID, n)
}

// The independent selected synthetic contract remains a deletion dependency of
// every child-generated package; it is never copied into an intent/checkpoint.
func (s *Store) candidateSourceTx(ctx context.Context, tx pgx.Tx, c actionauth.ContextSnapshot, n Need, ref catalog.SourceRef, p Package) (catalog.Source, error) {
	snapshot, err := s.developerTx(ctx, tx, c, n, ref)
	if err != nil {
		return catalog.Source{}, err
	}
	locator := []byte(encoded(map[string]string{"kind": "generated_converter_v1", "digest": p.Digest}))
	if snapshot == nil {
		return s.sources.CreateDerivedTx(ctx, tx, []catalog.SourceRef{ref}, "memory_read", n.ConverterID, string(p.Raw), locator)
	}
	if p.Manifest.Version != snapshot.Spec.Version {
		return catalog.Source{}, ErrAccess
	}
	return s.sources.CreateTaskDerivedTx(ctx, tx, []catalog.SourceRef{ref, snapshot.Args.ContractSource}, "memory_read", n.ConverterID, string(p.Raw), locator)
}
