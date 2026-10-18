package generatedtools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/butler/butler/internal/actionauth"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/jackc/pgx/v5"
)

func (s *Store) Catalog(ctx context.Context, c actionauth.ContextSnapshot, query string, limit int) ([]*toolbrokerv1.ToolContract, error) {
	if len(query) > 256 {
		return nil, ErrLimit
	}
	actual, e := s.run(ctx, c.RunID)
	if e != nil || actual.OwnerID != c.OwnerID || actual.TaskID != c.TaskID || actual.TaskRevision != c.TaskRevision || actual.ConstraintsRevision != c.ConstraintsRevision || actual.LeaseID != c.LeaseID || actual.LeaseEpoch != c.LeaseEpoch {
		return nil, ErrAccess
	}
	ctx = runContext(ctx, actual)
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	m, now, e := s.guardRunTx(ctx, tx, actual)
	if e != nil {
		return nil, e
	}
	rows, e := tx.Query(ctx, `SELECT active_digest FROM converter_catalog WHERE owner_id=$1 AND active_digest IS NOT NULL ORDER BY converter_id`, s.owner)
	if e != nil {
		return nil, e
	}
	hashes := []string{}
	for rows.Next() {
		var h string
		if e = rows.Scan(&h); e != nil {
			rows.Close()
			return nil, e
		}
		hashes = append(hashes, h)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	result := []*toolbrokerv1.ToolContract{}
	terms := strings.Fields(strings.ToLower(query))
	for _, hash := range hashes {
		r, e := s.metadataTx(ctx, tx, hash)
		if e != nil || !r.Active || r.Status != "active" {
			continue
		}
		// Permission shape is content-free. Current source access still checks the
		// complete dependency graph before this capability enters model context.
		if !actionauth.AllowsConverterAttachment(m, r.Shape, now) {
			continue
		}
		_, n, _, e := s.packageTx(ctx, tx, r)
		if e != nil {
			continue
		}
		if _, e = s.permission(m, n, now); e != nil {
			continue
		}
		tool := contract(r)
		if actionauth.CheckExecutionPolicy(actual, actionauth.EffectPrepare) != nil || actual.AutonomyMode == 1 {
			continue
		}
		matched := true
		for _, term := range terms {
			if !strings.Contains(strings.ToLower(tool.ToolName+" "+tool.Description), term) {
				matched = false
			}
		}
		if matched {
			result = append(result, tool)
		}
	}
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, e
	}
	return result, nil
}

// CheckPinnedLifecycleTx composes AFTER source/current task/goal guards. It
// performs no RPC and never grants permissions from a previous use record.
func CheckPinnedLifecycleTx(ctx context.Context, tx pgx.Tx, runID string) error {
	var now time.Time
	if tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now) != nil {
		return ErrAccess
	}
	rows, e := tx.Query(ctx, `SELECT u.digest,u.generation,c.generation,COALESCE(c.active_digest,''),st.status FROM converter_usages u JOIN converter_catalog c ON c.owner_id=u.owner_id AND c.converter_id=(SELECT converter_id FROM converter_versions WHERE owner_id=u.owner_id AND digest=u.digest) JOIN converter_states st ON st.owner_id=u.owner_id AND st.digest=u.digest WHERE u.run_id=$1 ORDER BY u.owner_id,u.digest FOR SHARE OF c,st`, runID)
	if e != nil {
		return e
	}
	defer rows.Close()
	for rows.Next() {
		var hash, currentHash, status string
		var old, current int64
		if e = rows.Scan(&hash, &old, &current, &currentHash, &status); e != nil {
			return e
		}
		if status != "active" || hash != currentHash || old != current {
			return ErrAccess
		}
	}
	return rows.Err()
}

// Reviewed procedural dependencies must also match an active exact generated
// revision on the caller transaction; procedure text cannot revive a converter.
func CheckInstalledRevisionTx(ctx context.Context, tx pgx.Tx, owner, name, revision string) error {
	id := strings.TrimSuffix(strings.TrimPrefix(name, "converter."), ".convert")
	if name != "converter."+id+".convert" || !identifier.MatchString(id) {
		return ErrAccess
	}
	var hash, status string
	var generation int64
	if tx.QueryRow(ctx, `SELECT c.active_digest,c.generation,st.status FROM converter_catalog c JOIN converter_states st ON st.owner_id=c.owner_id AND st.digest=c.active_digest WHERE c.owner_id=$1 AND c.converter_id=$2 FOR SHARE OF c,st`, owner, id).Scan(&hash, &generation, &status) != nil || status != "active" || revision != fmt.Sprintf("%s:%d", hash, generation) {
		return ErrAccess
	}
	return nil
}
