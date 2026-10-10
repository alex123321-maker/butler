package skills

import (
	"context"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/extensions"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
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

// CheckPinnedLifecycleTx composes after the owned source guard in the final
// result transaction. It never performs a broker RPC while locks are held.
func CheckPinnedLifecycleTx(ctx context.Context, tx pgx.Tx, runID string) error {
	rows, err := tx.Query(ctx, `SELECT st.status,v.owner_id,v.manifest_text FROM skill_usages u JOIN skill_versions v USING(digest) JOIN skill_states st USING(digest) WHERE u.run_id=$1 ORDER BY u.digest FOR SHARE OF st`, runID)
	if err != nil {
		return err
	}
	deps := map[string]Dependency{}
	owner := ""
	for rows.Next() {
		var state, raw, currentOwner string
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
		pack, e := Parse([]byte(raw))
		if e != nil {
			rows.Close()
			return e
		}
		for _, d := range pack.Manifest.Dependencies {
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
	rows, err := tx.Query(ctx, `SELECT u.digest,st.status,v.manifest_text FROM skill_usages u JOIN skill_versions v USING(digest) JOIN skill_states st USING(digest) WHERE u.run_id=$1 ORDER BY u.digest FOR SHARE OF st`, run)
	if err != nil {
		return err
	}
	packages := []Package{}
	for rows.Next() {
		var hash, status, raw string
		if err = rows.Scan(&hash, &status, &raw); err != nil {
			rows.Close()
			return err
		}
		if status == "revoked" {
			rows.Close()
			return ErrUnavailable
		}
		p, e := Parse([]byte(raw))
		if e != nil || p.Digest != hash {
			rows.Close()
			return ErrUnavailable
		}
		packages = append(packages, p)
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
	reader, ok := s.contracts.(TransactionContractReader)
	if !ok {
		return ErrUnavailable
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
	names := sortedDependencies(deps)
	for _, name := range names {
		d := deps[name]
		c, e := reader.GetSkillContractTx(ctx, tx, name)
		hash, he := actionauth.ContractHash(c)
		if e != nil || he != nil || hash != d.ContractHash || c.GetContractRevision() != d.ContractRevision {
			return ErrConflict
		}
	}
	return nil
}
