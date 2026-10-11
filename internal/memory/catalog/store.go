package catalog

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/delegation"
	"github.com/butler/butler/internal/memory/sanitize"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/security/content"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool     *pgxpool.Pool
	manifest Manifest
}

func New(pool *pgxpool.Pool, manifest Manifest) *Store { return &Store{pool, manifest} }
func (s *Store) Pool() *pgxpool.Pool                   { return s.pool }
func randomID(prefix string) (string, error) {
	var b [20]byte
	_, e := rand.Read(b[:])
	return prefix + hex.EncodeToString(b[:]), e
}
func owner(ctx context.Context) (string, error) {
	a, ok := owneridentity.FromContext(ctx)
	if !ok {
		return "", ErrDenied
	}
	return a.OwnerID, nil
}
func (s *Store) CheckRestore(ctx context.Context) error {
	if s == nil || s.pool == nil || s.manifest == nil {
		return ErrRestore
	}
	rows, e := s.manifest.Load(ctx)
	if e != nil {
		return ErrRestore
	}
	var mark int64
	if e = s.pool.QueryRow(ctx, `SELECT watermark FROM memory_restore_state WHERE singleton`).Scan(&mark); e != nil {
		return e
	}
	if mark != int64(len(rows)) {
		return ErrRestore
	}
	return checkManifestRows(ctx, s.pool, rows)
}
func (s *Store) CheckRestoreTx(ctx context.Context, tx pgx.Tx) error {
	if s == nil || s.manifest == nil {
		return ErrRestore
	}
	records, e := s.manifest.Load(ctx)
	if e != nil {
		return ErrRestore
	}
	var mark int64
	if e = tx.QueryRow(ctx, `SELECT watermark FROM memory_restore_state WHERE singleton`).Scan(&mark); e != nil {
		return e
	}
	if mark != int64(len(records)) {
		return ErrRestore
	}
	return checkManifestRows(ctx, tx, records)
}

func checkManifestRows(ctx context.Context, q interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, records []ManifestRecord) error {
	rows, e := q.Query(ctx, `SELECT deletion_id,source_id,owner_id,action,watermark FROM memory_deletions ORDER BY watermark`)
	if e != nil {
		return e
	}
	defer rows.Close()
	index := 0
	for rows.Next() {
		var id, sourceID, own, action string
		var mark int64
		if e = rows.Scan(&id, &sourceID, &own, &action, &mark); e != nil {
			return e
		}
		if index >= len(records) {
			return ErrRestore
		}
		r := records[index]
		if r.ID != id || r.SourceID != sourceID || r.Action != action || r.Sequence != mark || own != "" && r.OwnerHash != hash(own) {
			return ErrRestore
		}
		index++
	}
	if e = rows.Err(); e != nil {
		return e
	}
	if index != len(records) {
		return ErrRestore
	}
	return nil
}
func validateSource(p PutSource) error {
	if p.ExternalKey == "" || len(p.ExternalKey) > 512 || len(p.Title) > 2048 || len(p.Body) > 8*1024*1024 || !json.Valid(p.Locator) {
		return ErrInvalid
	}
	switch p.Kind {
	case "message", "document", "code", "owner_statement", "model_candidate":
	default:
		return ErrInvalid
	}
	switch p.ScopeType {
	case "user", "session", "task", "project":
	default:
		return ErrInvalid
	}
	if p.ScopeID == "" || len(p.ScopeID) > 512 || len(p.Author) > 512 {
		return ErrInvalid
	}
	return nil
}

// Put accepts source data from an authenticated owner, not instruction authority.
// External timestamps/order never supersede an accepted preference by themselves.
func (s *Store) Put(ctx context.Context, p PutSource) (Source, error) {
	own, e := owner(ctx)
	if e != nil {
		return Source{}, e
	}
	if e = s.CheckRestore(ctx); e != nil {
		return Source{}, e
	}
	if len(p.Locator) == 0 {
		p.Locator = json.RawMessage(`{}`)
	}
	if e = validateSource(p); e != nil {
		return Source{}, e
	}
	if p.ScopeType == "user" && p.ScopeID != own {
		return Source{}, ErrDenied
	}
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return Source{}, e
	}
	defer tx.Rollback(ctx)
	if e = checkOwnerScopeTx(ctx, tx, own, p.ScopeType, p.ScopeID); e != nil {
		return Source{}, e
	}
	sid := sourceID(own, p.ExternalKey)
	var tombstoned bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM memory_deletions WHERE source_id=$1)`, sid).Scan(&tombstoned); e != nil {
		return Source{}, e
	}
	if tombstoned {
		return Source{}, ErrDeleted
	}
	// Serialize creation and revision CAS, including initial duplicate uploads.
	if _, e = tx.Exec(ctx, `INSERT INTO memory_sources(source_id,owner_id,source_kind,scope_type,scope_id,external_key) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(owner_id,external_key) DO NOTHING`, sid, own, p.Kind, p.ScopeType, p.ScopeID, p.ExternalKey); e != nil {
		return Source{}, e
	}
	lockIDs := []string{sid}
	for _, parent := range p.Parents {
		lockIDs = append(lockIDs, parent.ID)
	}
	sort.Strings(lockIDs)
	for _, id := range lockIDs {
		var locked string
		if e = tx.QueryRow(ctx, `SELECT source_id FROM memory_sources WHERE source_id=$1 FOR UPDATE`, id).Scan(&locked); e != nil {
			return Source{}, ErrStale
		}
	}
	src, e := getSourceTx(ctx, tx, sid, true)
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		return Source{}, e
	}
	var revision, access int64
	var status, scopeType, scopeID, kind string
	if e = tx.QueryRow(ctx, `SELECT head_revision,access_revision,status,scope_type,scope_id,source_kind FROM memory_sources WHERE source_id=$1 FOR UPDATE`, sid).Scan(&revision, &access, &status, &scopeType, &scopeID, &kind); e != nil {
		return Source{}, e
	}
	if status != "active" {
		return Source{}, ErrDeleted
	}
	if scopeType != p.ScopeType || scopeID != p.ScopeID || kind != p.Kind {
		return Source{}, ErrStale
	}
	if p.Origin.Kind == "" {
		p.Origin = content.Origin{Kind: content.Document, SourceRefs: []string{sid}, Sensitivity: content.Private}
	}
	// Only trusted adapters can provide parent references; preserve their maximum
	// sensitivity, and never promote the parsed text into owner instructions.
	parents := append([]SourceRef(nil), p.Parents...)
	sort.Slice(parents, func(i, j int) bool { return parents[i].ID < parents[j].ID })
	for _, ref := range parents {
		var cyclic bool
		if e = tx.QueryRow(ctx, `WITH RECURSIVE parents AS(SELECT parent_source_id FROM memory_source_dependencies WHERE child_source_id=$1 UNION SELECT d.parent_source_id FROM memory_source_dependencies d JOIN parents p ON d.child_source_id=p.parent_source_id) SELECT $1=$2 OR EXISTS(SELECT 1 FROM parents WHERE parent_source_id=$2)`, ref.ID, sid).Scan(&cyclic); e != nil || cyclic {
			return Source{}, ErrInvalid
		}
		par, e := getSourceTx(ctx, tx, ref.ID, false)
		if e != nil || par.OwnerID != own || par.Status != "active" || par.Revision != ref.Revision || par.AccessRevision != ref.AccessRevision || par.Hash != ref.Hash {
			return Source{}, ErrStale
		}
		p.Origin = content.Derive(content.MemoryDerived, p.Origin, par.Origin)
	}
	p.Origin = content.StoredDataOrigin(content.Document, sid, mustJSON(p.Origin))
	if sanitize.Text(p.Body) != p.Body || sanitize.Text(p.Title) != p.Title {
		p.Origin.Sensitivity = content.Secret
	}
	sum := hash([]any{p.Title, p.Body, p.Locator, p.Author, p.EventTime, p.Origin})
	if src.Hash == sum {
		if e = tx.Commit(ctx); e != nil {
			return Source{}, e
		}
		return src, nil
	}
	if src.ID != "" {
		if p.ExpectedRevision != revision {
			return Source{}, ErrStale
		}
		revision++
	} else if p.ExpectedRevision != 0 {
		return Source{}, ErrStale
	}
	if _, e = tx.Exec(ctx, `INSERT INTO memory_source_revisions(source_id,revision,content_hash,title,body,locator,origin,author,event_time) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, sid, revision, sum, p.Title, p.Body, p.Locator, mustJSON(p.Origin), p.Author, p.EventTime); e != nil {
		return Source{}, e
	}
	if _, e = tx.Exec(ctx, `UPDATE memory_sources SET head_revision=$2,updated_at=clock_timestamp() WHERE source_id=$1`, sid, revision); e != nil {
		return Source{}, e
	}
	for _, ref := range parents {
		if _, e = tx.Exec(ctx, `INSERT INTO memory_source_dependencies(child_source_id,parent_source_id,parent_revision) VALUES($1,$2,$3) ON CONFLICT(child_source_id,parent_source_id) DO UPDATE SET parent_revision=EXCLUDED.parent_revision`, sid, ref.ID, ref.Revision); e != nil {
			return Source{}, e
		}
	}
	src, e = getSourceTx(ctx, tx, sid, false)
	if e != nil {
		return Source{}, e
	}
	return src, tx.Commit(ctx)
}
func mustJSON(v any) string { b, _ := json.Marshal(v); return string(b) }
func checkOwnerScopeTx(ctx context.Context, tx pgx.Tx, own, typ, id string) error {
	if typ == "user" {
		if id != own {
			return ErrDenied
		}
		return nil
	}
	if typ == "project" {
		return nil
	}
	var actual string
	q := `SELECT user_id FROM sessions WHERE session_key=$1 FOR KEY SHARE`
	if typ == "task" {
		q = `SELECT owner_id FROM tasks WHERE task_id=$1 FOR SHARE`
	}
	if tx.QueryRow(ctx, q, id).Scan(&actual) != nil || actual != own {
		return ErrDenied
	}
	return nil
}
func getSourceTx(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, id string, update bool) (Source, error) {
	var s Source
	var origin []byte
	lock := ""
	if update {
		lock = " FOR UPDATE OF s"
	} else {
		lock = " FOR SHARE OF s"
	}
	e := q.QueryRow(ctx, `SELECT s.source_id,s.owner_id,s.source_kind,s.scope_type,s.scope_id,s.external_key,s.head_revision,s.access_revision,s.status,r.content_hash,r.title,r.body,r.locator,r.origin,r.author,r.event_time,r.extracted_at FROM memory_sources s JOIN memory_source_revisions r ON r.source_id=s.source_id AND r.revision=s.head_revision WHERE s.source_id=$1`+lock, id).Scan(&s.ID, &s.OwnerID, &s.Kind, &s.ScopeType, &s.ScopeID, &s.ExternalKey, &s.Revision, &s.AccessRevision, &s.Status, &s.Hash, &s.Title, &s.Body, &s.Locator, &origin, &s.Author, &s.EventTime, &s.ExtractedAt)
	if e == nil {
		e = json.Unmarshal(origin, &s.Origin)
	}
	return s, e
}

// AccessTx acquires live task authority before sorted source rows. An auxiliary
// worker supplies its independent job proof through the trusted checker seam.
// Neither a run ID string nor caller-selected scope is a permission.
type AccessChecker interface {
	CheckMemoryAccessTx(context.Context, pgx.Tx, []SourceRef, string) (string, error)
	FinalizeMemoryAccessTx(context.Context, pgx.Tx, []SourceRef, string) error
	PinnedMemorySources() []SourceRef
}
type checkerKey struct{}
type writeSourcesKey struct{}

func WithAccessChecker(ctx context.Context, c AccessChecker) context.Context {
	return context.WithValue(ctx, checkerKey{}, c)
}
func (s *Store) AccessTx(ctx context.Context, tx pgx.Tx, refs []SourceRef, purpose string) ([]Source, error) {
	if e := s.CheckRestoreTx(ctx, tx); e != nil {
		return nil, e
	}
	var own, taskID, session, liveRun string
	var bounded *delegation.Contract
	if a, ok := owneridentity.FromContext(ctx); ok {
		own = a.OwnerID
	} else if c, ok := ctx.Value(checkerKey{}).(AccessChecker); ok {
		var e error
		own, e = c.CheckMemoryAccessTx(ctx, tx, refs, purpose)
		if e != nil {
			return nil, e
		}
	} else if tok, ok := ownership.TokenFromContext(ctx); ok {
		liveRun = tok.RunID
		var valid bool
		if tx.QueryRow(ctx, `SELECT TRUE FROM session_ownership WHERE session_key=$1 AND run_id=$2 AND lease_id=$3 AND fence_epoch=$4 AND expires_at>clock_timestamp() FOR SHARE`, tok.SessionKey, tok.RunID, tok.LeaseID, tok.FenceEpoch).Scan(&valid) != nil {
			return nil, ErrDenied
		}
		var state string
		var revision int64
		if tx.QueryRow(ctx, `SELECT COALESCE(task_id,''),COALESCE(task_revision,0),session_key,current_state FROM runs WHERE run_id=$1 FOR SHARE`, tok.RunID).Scan(&taskID, &revision, &session, &state) != nil || state == "completed" || state == "failed" || state == "cancelled" || state == "timed_out" {
			return nil, ErrDenied
		}
		if tx.QueryRow(ctx, `SELECT user_id FROM sessions WHERE session_key=$1`, session).Scan(&own) != nil {
			return nil, ErrDenied
		}
		if taskID != "" {
			var current int64
			var status string
			if tx.QueryRow(ctx, `SELECT revision,status FROM tasks WHERE task_id=$1 AND owner_id=$2 FOR SHARE`, taskID, own).Scan(&current, &status) != nil || current != revision || status != "active" {
				return nil, ErrDenied
			}
			var now time.Time
			_ = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now)
			if _, e := actionauth.CheckTaskAuthorityTx(ctx, tx, taskID, own, nil, now); e != nil {
				return nil, e
			}
			var e error
			bounded, e = delegation.LoadTx(ctx, tx, taskID)
			if e != nil {
				return nil, e
			}
			if bounded != nil {
				if e = s.CheckDelegationSourcesTx(ctx, tx, taskID, delegationRoots(bounded), "memory_read"); e != nil {
					return nil, e
				}
			}
		}
	} else {
		return nil, ErrDenied
	}
	ordered := append([]SourceRef(nil), refs...)
	if c, ok := ctx.Value(checkerKey{}).(AccessChecker); ok {
		seen := map[string]bool{}
		for _, r := range ordered {
			seen[r.ID] = true
		}
		for _, r := range c.PinnedMemorySources() {
			if !seen[r.ID] {
				ordered = append(ordered, r)
				seen[r.ID] = true
			}
		}
	}
	var graphErr error
	graphRoots := append([]SourceRef(nil), ordered...)
	ordered, graphErr = s.CanonicalSourcesTx(ctx, tx, ordered)
	if graphErr != nil {
		return nil, graphErr
	}
	result := make([]Source, 0, len(ordered))
	for _, ref := range ordered {
		write, _ := ctx.Value(writeSourcesKey{}).(bool)
		src, e := getSourceTx(ctx, tx, ref.ID, write)
		if e != nil || src.OwnerID != own || src.Status != "active" {
			return nil, ErrDenied
		}
		if ref.Revision != 0 && ref.Revision != src.Revision || ref.AccessRevision != 0 && ref.AccessRevision != src.AccessRevision || ref.Hash != "" && ref.Hash != src.Hash {
			return nil, ErrStale
		}
		if bounded != nil {
			for _, requested := range refs {
				if requested.ID == src.ID && !delegatedBodyAllowed(bounded, src, session) {
					return nil, ErrDenied
				}
			}
			// The complete original graph is a privacy dependency, not additional
			// model-selectable context. Every inherited node needs a current grant.
			if !(src.ScopeType == "session" && src.ScopeID == session || src.ScopeType == "task" && src.ScopeID == taskID) {
				if e = checkInheritedPermissionTx(ctx, tx, taskID, own, src, purpose, 0); e != nil {
					return nil, e
				}
			}
		}
		_, jobProof := ctx.Value(checkerKey{}).(AccessChecker)
		if _, isOwner := owneridentity.FromContext(ctx); !isOwner && !jobProof && !(src.ScopeType == "user" && src.ScopeID == own || src.ScopeType == "session" && src.ScopeID == session || taskID != "" && src.ScopeType == "task" && src.ScopeID == taskID) {
			if taskID == "" {
				return nil, ErrDenied
			}
			var yes bool
			if tx.QueryRow(ctx, `SELECT TRUE FROM memory_scope_grants g JOIN tasks t USING(task_id) WHERE g.task_id=$1 AND g.source_id=$2 AND g.task_revision=t.revision AND g.access_revision=$3 AND g.owner_id=$4 AND g.purpose=$5`, taskID, src.ID, src.AccessRevision, own, purpose).Scan(&yes) != nil {
				return nil, ErrDenied
			}
		}
		// A source derived from a stale/deleted parent is not current evidence.
		var stale bool
		if e = tx.QueryRow(ctx, `WITH RECURSIVE parents AS(SELECT parent_source_id,parent_revision FROM memory_source_dependencies WHERE child_source_id=$1 UNION SELECT d.parent_source_id,d.parent_revision FROM memory_source_dependencies d JOIN parents p ON d.child_source_id=p.parent_source_id) SELECT EXISTS(SELECT 1 FROM parents p JOIN memory_sources s ON s.source_id=p.parent_source_id WHERE s.status<>'active' OR s.head_revision<>p.parent_revision)`, src.ID).Scan(&stale); e != nil || stale {
			return nil, ErrStale
		}
		if src.Origin.Sensitivity == content.Secret {
			return nil, ErrDenied
		}
		if liveRun != "" {
			if _, e = tx.Exec(ctx, `INSERT INTO memory_run_sources(run_id,source_id,source_revision,access_revision,content_hash) VALUES($1,$2,$3,$4,$5) ON CONFLICT(run_id,source_id) DO NOTHING`, liveRun, src.ID, src.Revision, src.AccessRevision, src.Hash); e != nil {
				return nil, e
			}
			var current bool
			if e = tx.QueryRow(ctx, `SELECT source_revision=$3 AND access_revision=$4 AND content_hash=$5 FROM memory_run_sources WHERE run_id=$1 AND source_id=$2`, liveRun, src.ID, src.Revision, src.AccessRevision, src.Hash).Scan(&current); e != nil || !current {
				return nil, ErrStale
			}
		}
		for _, requested := range refs {
			if requested.ID == src.ID {
				result = append(result, src)
				break
			}
		}
	}
	currentGraph, e := s.CanonicalSourcesTx(ctx, tx, graphRoots)
	if e != nil || len(currentGraph) != len(ordered) {
		return nil, ErrStale
	}
	for i, r := range currentGraph {
		if r != ordered[i] {
			return nil, ErrStale
		}
	}
	if c, ok := ctx.Value(checkerKey{}).(AccessChecker); ok {
		if e := c.FinalizeMemoryAccessTx(ctx, tx, refs, purpose); e != nil {
			return nil, e
		}
	}
	return result, nil
}
func (s *Store) Get(ctx context.Context, id string) (Source, error) {
	if e := s.CheckRestore(ctx); e != nil {
		return Source{}, e
	}
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return Source{}, e
	}
	defer tx.Rollback(ctx)
	sources, e := s.AccessTx(ctx, tx, []SourceRef{{ID: id}}, "memory_read")
	if e != nil {
		return Source{}, e
	}
	if e = tx.Commit(ctx); e != nil {
		return Source{}, e
	}
	return sources[0], nil
}
func (s *Store) View(ctx context.Context) ([]Source, error) {
	own, e := owner(ctx)
	if e != nil {
		return nil, e
	}
	if e = s.CheckRestore(ctx); e != nil {
		return nil, e
	}
	rows, e := s.pool.Query(ctx, `SELECT source_id FROM memory_sources WHERE owner_id=$1 ORDER BY source_id LIMIT 1000`, own)
	if e != nil {
		return nil, e
	}
	var ids []string
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return nil, e
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	result := []Source{}
	for _, id := range ids {
		v, e := getSourceTx(ctx, s.pool, id, false)
		if e != nil {
			return nil, e
		}
		result = append(result, v)
	}
	return result, nil
}

type ObjectRef struct {
	Type string
	ID   int64
}

func (s *Store) CheckObjectsTx(ctx context.Context, tx pgx.Tx, objects []ObjectRef, purpose string) error {
	var refs []SourceRef
	for _, o := range objects {
		var r SourceRef
		if tx.QueryRow(ctx, `SELECT source_id,source_revision FROM memory_object_sources WHERE memory_type=$1 AND object_id=$2`, o.Type, o.ID).Scan(&r.ID, &r.Revision) != nil {
			return ErrDenied
		}
		refs = append(refs, r)
	}
	_, e := s.AccessTx(ctx, tx, refs, purpose)
	return e
}
func (s *Store) CheckObjects(ctx context.Context, objects []ObjectRef, purpose string) error {
	if e := s.CheckRestore(ctx); e != nil {
		return e
	}
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if e = s.CheckObjectsTx(ctx, tx, objects, purpose); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func bounded(text string, n int) bool { return strings.TrimSpace(text) != "" && len(text) <= n }
func wrap(e error) error              { return fmt.Errorf("memory catalog: %w", e) }
