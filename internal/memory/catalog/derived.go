package catalog

import (
	"context"
	"encoding/json"
	"github.com/butler/butler/internal/memory/sanitize"
	"github.com/butler/butler/internal/security/content"
	"github.com/jackc/pgx/v5"
)

// CreateDerivedTx creates immutable DATA under current parent authority. It
// cannot accept facts, change owner policies or turn a job into an owner actor.
// A fresh invisible row avoids acquiring an existing child after parent locks.
// The caller must commit this in the same transaction as its result CAS.
func (s *Store) CreateDerivedTx(ctx context.Context, tx pgx.Tx, parents []SourceRef, purpose, title, body string, locator json.RawMessage) (Source, error) {
	if tx == nil || len(parents) < 1 || len(parents) > 64 || len(title) > 2048 || len(body) > 1024*1024 || !json.Valid(locator) || sanitize.Text(body) != body || sanitize.Text(title) != title {
		return Source{}, ErrInvalid
	}
	for _, p := range parents {
		if p.ID == "" || p.Revision < 1 || p.AccessRevision < 1 || len(p.Hash) != 64 {
			return Source{}, ErrInvalid
		}
	}
	sources, e := s.AccessTx(ctx, tx, parents, purpose)
	if e != nil {
		return Source{}, e
	}
	// AccessTx returns rows in lock order, not in the trusted parent's order.
	// Anchor scope to the first requested parent; an output or API body cannot
	// choose a wider scope. Every parent still needs independent current access.
	var first Source
	for _, src := range sources {
		if src.ID == parents[0].ID {
			first = src
			break
		}
	}
	if first.ID == "" {
		return Source{}, ErrDenied
	}
	origins := make([]content.Origin, 0, len(sources))
	for _, src := range sources {
		if src.OwnerID != first.OwnerID || src.Origin.Sensitivity == content.Secret {
			return Source{}, ErrDenied
		}
		origins = append(origins, src.Origin)
	}
	id, e := randomID("source_")
	if e != nil {
		return Source{}, e
	}
	origin := content.Derive(content.MemoryDerived, origins...)
	origin.ParentRefs = append(origin.ParentRefs, origin.SourceRefs...)
	for _, p := range parents {
		origin.ParentRefs = append(origin.ParentRefs, p.ID)
	}
	origin.SourceRefs = []string{id}
	origin = content.StoredDataOrigin(content.MemoryDerived, id, mustJSON(origin))
	sum := hash([]any{title, body, locator, "derived", nil, origin})
	if _, e = tx.Exec(ctx, `INSERT INTO memory_sources(source_id,owner_id,source_kind,scope_type,scope_id,external_key,head_revision) VALUES($1,$2,'model_candidate',$3,$4,$5,1)`, id, first.OwnerID, first.ScopeType, first.ScopeID, "derived:"+id); e != nil {
		return Source{}, e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO memory_source_revisions(source_id,revision,content_hash,title,body,locator,origin,author) VALUES($1,1,$2,$3,$4,$5,$6,'derived')`, id, sum, title, body, locator, mustJSON(origin)); e != nil {
		return Source{}, e
	}
	for _, p := range parents {
		if _, e = tx.Exec(ctx, `INSERT INTO memory_source_dependencies(child_source_id,parent_source_id,parent_revision) VALUES($1,$2,$3)`, id, p.ID, p.Revision); e != nil {
			return Source{}, e
		}
	}
	// Time/claim expiry can cross while inserts wait; recheck before publication.
	if _, e = s.AccessTx(ctx, tx, parents, purpose); e != nil {
		return Source{}, e
	}
	src, e := getSourceTx(ctx, tx, id, false)
	if e != nil {
		return Source{}, e
	}
	var xid string
	if e = tx.QueryRow(ctx, `SELECT pg_current_xact_id()::text`).Scan(&xid); e != nil {
		return Source{}, e
	}
	src.creation = &derivedCreation{xid: xid, source: SourceRef{src.ID, src.Revision, src.AccessRevision, src.Hash}, owner: src.OwnerID, scopeType: src.ScopeType, scopeID: src.ScopeID}
	return src, nil
}
