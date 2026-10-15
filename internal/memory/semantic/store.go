package semantic

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/sanitize"
	"github.com/butler/butler/internal/security/content"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SourceGuard derives the authoritative chunk->catalog link from PostgreSQL;
// optional provenance catalog metadata is a citation, never an access grant.
// CheckSourcesTx locks sorted catalog/job authority before caller chunk locks.
type SourceGuard interface {
	CheckSources(context.Context, []Source) error
	CheckSourcesTx(context.Context, pgx.Tx, []Source) error
}
type Store struct {
	pool  *pgxpool.Pool
	Guard SourceGuard
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }
func accessObservation(err error) error {
	// Preserve only typed, content-free reasons across the guard boundary.
	for _, reason := range []error{catalog.ErrStale, catalog.ErrDeleted, catalog.ErrRestore} {
		if errors.Is(err, reason) {
			return errors.Join(ErrAccess, reason)
		}
	}
	return ErrAccess
}
func (s *Store) check(ctx context.Context, sources []Source) error {
	if s.Guard == nil {
		return ErrAccess
	}
	if err := s.Guard.CheckSources(ctx, sources); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return accessObservation(err)
	}
	return nil
}
func (s *Store) checkTx(ctx context.Context, tx pgx.Tx, sources []Source) error {
	if s.Guard == nil {
		return ErrAccess
	}
	if err := s.Guard.CheckSourcesTx(ctx, tx, sources); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return accessObservation(err)
	}
	return nil
}
func (s *Store) Create(ctx context.Context, space Space, scope Scope) (Generation, error) {
	if s == nil || s.pool == nil {
		return Generation{}, ErrUnavailable
	}
	if space.Validate() != nil || !scope.valid() {
		return Generation{}, ErrInvalid
	}
	g := Generation{ID: freshID(), Space: space, Scope: scope, State: "building"}
	raw, _ := json.Marshal(space)
	err := s.pool.QueryRow(ctx, `INSERT INTO semantic_index_generations(id,space_id,space_data,dimensions,scope_type,scope_id) VALUES($1,$2,$3::jsonb,$4,$5,$6) RETURNING created_at`, g.ID, space.ID(), string(raw), space.Dimensions, scope.Type, scope.ID).Scan(&g.CreatedAt)
	return g, err
}

type rowReader interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func readGeneration(ctx context.Context, db rowReader, id string, lock bool) (Generation, error) {
	var g Generation
	var raw string
	var spaceID string
	var dims int
	sql := `SELECT id,space_id,space_data::text,dimensions,scope_type,scope_id,state,created_at FROM semantic_index_generations WHERE id=$1`
	if lock {
		sql += ` FOR UPDATE`
	}
	err := db.QueryRow(ctx, sql, id).Scan(&g.ID, &spaceID, &raw, &dims, &g.Scope.Type, &g.Scope.ID, &g.State, &g.CreatedAt)
	if err != nil {
		return g, err
	}
	if json.Unmarshal([]byte(raw), &g.Space) != nil || g.Space.Validate() != nil || g.Space.ID() != spaceID || dims != g.Space.Dimensions {
		return g, ErrInvalid
	}
	return g, nil
}
func (s *Store) Generation(ctx context.Context, id string) (Generation, error) {
	if s == nil || s.pool == nil {
		return Generation{}, ErrUnavailable
	}
	return readGeneration(ctx, s.pool, id, false)
}

const liveSource = `status='active' AND effective_status='active' AND NOT suppressed AND confirmation_state IN ('confirmed','auto_confirmed') AND (expires_at IS NULL OR expires_at>clock_timestamp())`
const sourceColumns = `id,scope_type,scope_id,title,content,summary,source_type,source_id,provenance::text,updated_at`

func scanSource(row pgx.Row) (Source, error) {
	var src Source
	err := row.Scan(&src.ID, &src.Scope.Type, &src.Scope.ID, &src.Title, &src.Content, &src.Summary, &src.SourceType, &src.SourceID, &src.ProvenanceJSON, &src.Revision)
	src.catalogMetadata()
	return src, err
}
func readSources(ctx context.Context, db rowReader, scope Scope, limit int, lock bool) ([]Source, error) {
	if !scope.valid() || limit < 1 || limit > 100000 {
		return nil, ErrInvalid
	}
	sql := `SELECT ` + sourceColumns + ` FROM memory_chunks WHERE scope_type=$1 AND scope_id=$2 AND ` + liveSource + ` ORDER BY id LIMIT $3`
	if lock {
		sql += ` FOR SHARE`
	}
	rows, err := db.Query(ctx, sql, scope.Type, scope.ID, limit+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var sources []Source
	count := 0
	for rows.Next() {
		count++
		src, err := scanSource(rows)
		if err != nil {
			return nil, err
		}
		if src.MemoryOrigin().Sensitivity != content.Secret {
			sources = append(sources, src)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if count > limit {
		return nil, ErrSourceLimit
	}
	return sources, nil
}
func (s *Store) Sources(ctx context.Context, scope Scope, limit int) ([]Source, error) {
	if s == nil || s.pool == nil {
		return nil, ErrUnavailable
	}
	sources, err := readSources(ctx, s.pool, scope, limit, false)
	if err != nil {
		return nil, err
	}
	if err := s.check(ctx, sources); err != nil {
		return nil, err
	}
	return sources, nil
}
func (s *Store) LiveSource(ctx context.Context, scope Scope, id int64) (Source, error) {
	if s == nil || s.pool == nil {
		return Source{}, ErrUnavailable
	}
	if !scope.valid() || id <= 0 {
		return Source{}, ErrInvalid
	}
	src, err := scanSource(s.pool.QueryRow(ctx, `SELECT `+sourceColumns+` FROM memory_chunks WHERE id=$1 AND scope_type=$2 AND scope_id=$3 AND `+liveSource, id, scope.Type, scope.ID))
	if err != nil {
		return Source{}, err
	}
	if err := s.check(ctx, []Source{src}); err != nil {
		return Source{}, err
	}
	return src, nil
}
func (s *Store) HasEntry(ctx context.Context, generation string, source Source) (bool, error) {
	if s == nil || s.pool == nil {
		return false, ErrUnavailable
	}
	var found bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM semantic_index_entries WHERE generation_id=$1 AND chunk_id=$2 AND source_revision=$3 AND source_hash=$4)`, generation, source.ID, source.Revision, source.Hash()).Scan(&found)
	return found, err
}
func (s *Store) Put(ctx context.Context, id string, source Source, vector []float32) error {
	if s == nil || s.pool == nil {
		return ErrUnavailable
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	g, err := readGeneration(ctx, tx, id, true)
	if err != nil {
		return err
	}
	if g.State != "building" {
		return ErrStale
	}
	if g.Scope != source.Scope || ValidateVector(vector, g.Space.Dimensions) != nil {
		return ErrInvalid
	}
	if err := s.checkTx(ctx, tx, []Source{source}); err != nil {
		return err
	}
	current, err := scanSource(tx.QueryRow(ctx, `SELECT `+sourceColumns+` FROM memory_chunks WHERE id=$1 AND `+liveSource+` FOR SHARE`, source.ID))
	if err != nil {
		return ErrStale
	}
	if current.Scope != source.Scope || !current.Revision.Equal(source.Revision) || current.Hash() != source.Hash() {
		return ErrStale
	}
	if current.MemoryOrigin().Sensitivity == content.Secret {
		return content.ErrSecret
	}
	_, err = tx.Exec(ctx, `INSERT INTO semantic_index_entries(generation_id,chunk_id,dimensions,source_revision,source_hash,embedding_hash,embedding) VALUES($1,$2,$3,$4,$5,$6,$7::vector) ON CONFLICT(generation_id,chunk_id) DO UPDATE SET source_revision=EXCLUDED.source_revision,source_hash=EXCLUDED.source_hash,embedding_hash=EXCLUDED.embedding_hash,embedding=EXCLUDED.embedding,indexed_at=clock_timestamp()`, id, source.ID, g.Space.Dimensions, source.Revision, source.Hash(), source.EmbeddingHash(), vectorLiteral(vector))
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// checkCoverage compares current authoritative rows with the generation. It
// never treats stale/deleted data or equal vector dimensions as verification.
func (s *Store) checkCoverage(ctx context.Context, tx pgx.Tx, g Generation) error {
	snapshots, err := readSources(ctx, tx, g.Scope, 100000, false)
	if err != nil {
		return err
	}
	if err := s.checkTx(ctx, tx, snapshots); err != nil {
		return err
	}
	sources, err := readSources(ctx, tx, g.Scope, 100000, true)
	if err != nil {
		return err
	}
	if len(sources) == 0 {
		return ErrUnverified
	}
	if len(sources) != len(snapshots) {
		return ErrStale
	}
	for i, src := range sources {
		if src.ID != snapshots[i].ID || src.Hash() != snapshots[i].Hash() || !src.Revision.Equal(snapshots[i].Revision) {
			return ErrStale
		}
	}
	rows, err := tx.Query(ctx, `SELECT chunk_id,source_revision,source_hash,embedding::text FROM semantic_index_entries WHERE generation_id=$1`, g.ID)
	if err != nil {
		return err
	}
	type entry struct {
		rev    time.Time
		hash   string
		vector []float32
	}
	entries := map[int64]entry{}
	for rows.Next() {
		var id int64
		var e entry
		var raw string
		if err := rows.Scan(&id, &e.rev, &e.hash, &raw); err != nil {
			rows.Close()
			return err
		}
		if json.Unmarshal([]byte(raw), &e.vector) != nil || ValidateVector(e.vector, g.Space.Dimensions) != nil {
			rows.Close()
			return ErrInvalid
		}
		entries[id] = e
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, src := range sources {
		e, ok := entries[src.ID]
		if !ok || !e.rev.Equal(src.Revision) || e.hash != src.Hash() {
			return ErrStale
		}
	}
	return nil
}

// Verify requires an operator-reviewed artifact demonstrating an improvement.
// The reader cannot invent this proof or switch a live index automatically.
func (s *Store) Verify(ctx context.Context, id string, proof Proof) error {
	if s == nil || s.pool == nil {
		return ErrUnavailable
	}
	if !proof.valid() {
		return ErrUnverified
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	g, err := readGeneration(ctx, tx, id, true)
	if err != nil {
		return err
	}
	if g.State != "building" {
		return ErrStale
	}
	if err := s.checkCoverage(ctx, tx, g); err != nil {
		return err
	}
	raw, _ := json.Marshal(proof)
	_, err = tx.Exec(ctx, `UPDATE semantic_index_generations SET state='ready',quality_proof=$2::jsonb,verified_at=clock_timestamp() WHERE id=$1`, id, string(raw))
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *Store) Active(ctx context.Context, scope Scope) (Generation, error) {
	if s == nil || s.pool == nil {
		return Generation{}, ErrUnavailable
	}
	if !scope.valid() {
		return Generation{}, ErrInvalid
	}
	var id string
	err := s.pool.QueryRow(ctx, `SELECT generation_id FROM semantic_index_active WHERE scope_type=$1 AND scope_id=$2`, scope.Type, scope.ID).Scan(&id)
	if err != nil {
		return Generation{}, err
	}
	return s.Generation(ctx, id)
}

// Activate is also the rollback primitive. The expected previous ID prevents
// concurrent operators from overwriting a newer choice. Old generations remain.
func (s *Store) Activate(ctx context.Context, id, expected string) error {
	if s == nil || s.pool == nil {
		return ErrUnavailable
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	g, err := readGeneration(ctx, tx, id, true)
	if err != nil {
		return err
	}
	if g.State != "ready" {
		return ErrUnverified
	}
	if err := s.checkCoverage(ctx, tx, g); err != nil {
		return err
	}
	if expected == "" {
		tag, err := tx.Exec(ctx, `INSERT INTO semantic_index_active(scope_type,scope_id,generation_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, g.Scope.Type, g.Scope.ID, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return ErrStale
		}
	} else {
		tag, err := tx.Exec(ctx, `UPDATE semantic_index_active SET generation_id=$3,changed_at=clock_timestamp() WHERE scope_type=$1 AND scope_id=$2 AND generation_id=$4`, g.Scope.Type, g.Scope.ID, id, expected)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return ErrStale
		}
	}
	return tx.Commit(ctx)
}
func (s *Store) Search(ctx context.Context, g Generation, query []float32, limit int) ([]Hit, error) {
	return s.search(ctx, g, query, limit, false)
}

// Preview evaluates a staged generation without publishing it to readers.
func (s *Store) Preview(ctx context.Context, g Generation, query []float32, limit int) ([]Hit, error) {
	return s.search(ctx, g, query, limit, true)
}
func (s *Store) search(ctx context.Context, g Generation, query []float32, limit int, preview bool) ([]Hit, error) {
	if s == nil || s.pool == nil {
		return nil, ErrUnavailable
	}
	if g.Space.Validate() != nil || !g.Scope.valid() || ValidateVector(query, g.Space.Dimensions) != nil || limit < 1 || limit > 100 {
		return nil, ErrInvalid
	}
	current, err := s.Generation(ctx, g.ID)
	if err != nil {
		return nil, err
	}
	if current.Space.ID() != g.Space.ID() || current.Scope != g.Scope || current.State != "ready" && !preview {
		return nil, ErrStale
	}
	rows, err := s.pool.Query(ctx, `SELECT c.id,c.scope_type,c.scope_id,c.title,c.content,c.summary,c.source_type,c.source_id,c.provenance::text,c.updated_at,e.source_hash,e.embedding <=> $2::vector FROM semantic_index_entries e JOIN memory_chunks c ON c.id=e.chunk_id WHERE e.generation_id=$1 AND e.dimensions=$3 AND c.scope_type=$4 AND c.scope_id=$5 AND c.updated_at=e.source_revision AND c.`+liveSource+` ORDER BY e.embedding <=> $2::vector,c.id LIMIT $6`, g.ID, vectorLiteral(query), g.Space.Dimensions, g.Scope.Type, g.Scope.ID, limit*4)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hits []Hit
	for rows.Next() {
		var h Hit
		var hash string
		src := &h.Source
		err := rows.Scan(&src.ID, &src.Scope.Type, &src.Scope.ID, &src.Title, &src.Content, &src.Summary, &src.SourceType, &src.SourceID, &src.ProvenanceJSON, &src.Revision, &hash, &h.Distance)
		if err != nil {
			return nil, err
		}
		if src.Hash() != hash || src.MemoryOrigin().Sensitivity == content.Secret || sanitize.Text(src.Content) != src.Content || sanitize.Text(src.Summary) != src.Summary || sanitize.Text(src.Title) != src.Title {
			continue
		}
		h.GenerationID = g.ID
		src.catalogMetadata()
		h.SpaceID = g.Space.ID()
		hits = append(hits, h)
		if len(hits) == limit {
			break
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	sources := make([]Source, len(hits))
	for i, h := range hits {
		sources[i] = h.Source
	}
	if err := s.check(ctx, sources); err != nil {
		return nil, err
	}
	return hits, nil
}

// Reuse copies an exact-space vector into a building generation after metadata
// or locator changes. Source access and current publication are still rechecked.
func (s *Store) Reuse(ctx context.Context, g Generation, source Source) (bool, error) {
	if s == nil || s.pool == nil {
		return false, ErrUnavailable
	}
	if err := s.check(ctx, []Source{source}); err != nil {
		return false, err
	}
	var raw string
	err := s.pool.QueryRow(ctx, `SELECT e.embedding::text FROM semantic_index_entries e JOIN semantic_index_generations old ON old.id=e.generation_id WHERE e.chunk_id=$1 AND e.embedding_hash=$2 AND old.space_id=$3 AND old.scope_type=$4 AND old.scope_id=$5 ORDER BY e.indexed_at DESC LIMIT 1`, source.ID, source.EmbeddingHash(), g.Space.ID(), source.Scope.Type, source.Scope.ID).Scan(&raw)
	if IsMissing(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var vector []float32
	if json.Unmarshal([]byte(raw), &vector) != nil || ValidateVector(vector, g.Space.Dimensions) != nil {
		return false, ErrInvalid
	}
	if err := s.Put(ctx, g.ID, source, vector); err != nil {
		return false, err
	}
	return true, nil
}
func IsMissing(err error) bool { return errors.Is(err, pgx.ErrNoRows) }
