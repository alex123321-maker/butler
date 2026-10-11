package skills

import (
	"context"
	"encoding/json"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/extensions"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/generatedtools"
	"github.com/jackc/pgx/v5"
	"sort"
	"strings"
)

type TransactionContractReader interface {
	GetSkillContractTx(context.Context, pgx.Tx, string) (*toolbrokerv1.ToolContract, error)
}

func sortedDependencies(deps map[string]Dependency) []string {
	names := make([]string, 0, len(deps))
	for name := range deps {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (s *Store) checkDependenciesTx(ctx context.Context, tx pgx.Tx, deps map[string]Dependency) error {
	reader, ok := s.contracts.(TransactionContractReader)
	if !ok {
		return ErrUnavailable
	}
	for _, name := range sortedDependencies(deps) {
		d := deps[name]
		c, e := reader.GetSkillContractTx(ctx, tx, name)
		hash, he := actionauth.ContractHash(c)
		if e != nil || he != nil || hash != d.ContractHash || c.GetContractRevision() != d.ContractRevision {
			return ErrConflict
		}
	}
	return nil
}

// CheckPinnedLifecycleTx composes after the owned source guard in the final
// result transaction. It never performs a broker RPC while locks are held.
func CheckPinnedLifecycleTx(ctx context.Context, tx pgx.Tx, runID string) error {
	rows, err := tx.Query(ctx, `SELECT st.status,v.owner_id,v.dependencies_json FROM skill_usages u JOIN skill_versions v USING(digest) JOIN skill_states st USING(digest) WHERE u.run_id=$1 ORDER BY u.digest FOR SHARE OF st`, runID)
	if err != nil {
		return err
	}
	deps := map[string]Dependency{}
	owner := ""
	for rows.Next() {
		var state, currentOwner string
		var raw []byte
		if err = rows.Scan(&state, &currentOwner, &raw); err != nil {
			rows.Close()
			return err
		}
		if state == "revoked" {
			rows.Close()
			return ErrUnavailable
		}
		if owner != "" && currentOwner != owner {
			rows.Close()
			return ErrAccess
		}
		owner = currentOwner
		var dependencies []Dependency
		if json.Unmarshal(raw, &dependencies) != nil || len(dependencies) == 0 {
			rows.Close()
			return ErrUnavailable
		}
		for _, d := range dependencies {
			if old, ok := deps[d.Tool]; ok && old != d {
				rows.Close()
				return ErrConflict
			}
			deps[d.Tool] = d
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, name := range sortedDependencies(deps) {
		if strings.HasPrefix(name, "converter.") {
			if err = generatedtools.CheckInstalledRevisionTx(ctx, tx, owner, name, deps[name].ContractRevision); err != nil {
				return err
			}
		}
		if strings.HasPrefix(name, "extension.") {
			if err = extensions.CheckInstalledRevisionTx(ctx, tx, owner, name, deps[name].ContractRevision); err != nil {
				return err
			}
		}
	}
	return nil
}

// CheckRunTx runs after ownership/run/task/goal/source locks and before effect
// admission. No HTTP, provider call or second pool query occurs under these locks.
func (s *Store) CheckRunTx(ctx context.Context, tx pgx.Tx, run, owner string) error {
	rows, err := tx.Query(ctx, `SELECT u.digest,st.status,v.dependencies_json FROM skill_usages u JOIN skill_versions v USING(digest) JOIN skill_states st USING(digest) WHERE u.run_id=$1 ORDER BY u.digest FOR SHARE OF st`, run)
	if err != nil {
		return err
	}
	packages := []Package{}
	for rows.Next() {
		var hash, status string
		var raw []byte
		if err = rows.Scan(&hash, &status, &raw); err != nil {
			rows.Close()
			return err
		}
		if status == "revoked" {
			rows.Close()
			return ErrUnavailable
		}
		var dependencies []Dependency
		if json.Unmarshal(raw, &dependencies) != nil || len(dependencies) == 0 {
			rows.Close()
			return ErrUnavailable
		}
		packages = append(packages, Package{Manifest: Manifest{Dependencies: dependencies}})
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(packages) == 0 {
		return nil
	}
	if owner != s.owner {
		return ErrAccess
	}
	// Stable dependency lock order across multiple loaded skills.
	deps := map[string]Dependency{}
	for _, p := range packages {
		for _, d := range p.Manifest.Dependencies {
			if old, ok := deps[d.Tool]; ok && old != d {
				return ErrConflict
			}
			deps[d.Tool] = d
		}
	}
	return s.checkDependenciesTx(ctx, tx, deps)
}
