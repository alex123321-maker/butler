package ingestion

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5"
)

type Upload struct {
	ExternalKey        string     `json:"external_key"`
	ExpectedRevision   int64      `json:"expected_revision"`
	ScopeType          string     `json:"scope_type"`
	ScopeID            string     `json:"scope_id"`
	Format             string     `json:"format"`
	Path               string     `json:"path"`
	RepositoryRevision string     `json:"repository_revision"`
	Author             string     `json:"author"`
	EventTime          *time.Time `json:"event_time"`
	Raw                []byte     `json:"original_bytes"`
}
type Receipt struct {
	Source        catalog.Source         `json:"source"`
	Status        string                 `json:"status"`
	ErrorCode     string                 `json:"error_code,omitempty"`
	ParserVersion string                 `json:"parser_version"`
	Uncertain     bool                   `json:"uncertain"`
	Fragments     catalog.FragmentResult `json:"fragments"`
}
type Store struct {
	Catalog *catalog.Store
	Parser  Parser
}

// Ingest is deterministic, bounded Go parsing, not auxiliary inference. Source
// selection is owner-proved; no downloaded URLs or directory scans are accepted.
func (s *Store) Ingest(ctx context.Context, u Upload) (Receipt, error) {
	if s == nil || s.Catalog == nil {
		return Receipt{}, catalog.ErrInvalid
	}
	if _, ok := owneridentity.FromContext(ctx); !ok {
		return Receipt{}, catalog.ErrDenied
	}
	if len(u.Raw) == 0 || len(u.Raw) > MaxBytes || u.Path == "" || len(u.Path) > 1024 || u.ExternalKey == "" {
		return Receipt{}, ErrLimit
	}
	if e := s.Catalog.CheckOwnerScope(ctx, u.ScopeType, u.ScopeID); e != nil {
		return Receipt{}, e
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	r, parseErr := s.Parser.Parse(ctx, Input{Format: u.Format, Path: u.Path, RepositoryRevision: u.RepositoryRevision, Raw: u.Raw})
	if ctx.Err() != nil {
		return Receipt{}, ctx.Err()
	}
	status, code := "ready", ""
	if parseErr != nil {
		status = "failed"
		code = "corrupt_source"
		switch {
		case errors.Is(parseErr, ErrLimit):
			code = "parse_limit"
		case errors.Is(parseErr, ErrFormat):
			code = "unsupported_format"
		case errors.Is(parseErr, ErrOCR):
			code = "ocr_required"
		}
		r = Result{}
	}
	h := sha256.Sum256(u.Raw)
	originalHash := hex.EncodeToString(h[:])
	loc, _ := json.Marshal(map[string]any{"path": u.Path, "repository_revision": u.RepositoryRevision, "format": u.Format, "original_hash": originalHash, "parser_version": Version, "status": status, "method": r.Method, "uncertain": r.Uncertain})
	kind := "document"
	if u.Format == "go" {
		kind = "code"
	}
	if u.Format == "messages" {
		kind = "message"
	}
	src, e := s.Catalog.Put(ctx, catalog.PutSource{ExternalKey: u.ExternalKey, Kind: kind, ScopeType: u.ScopeType, ScopeID: u.ScopeID, ExpectedRevision: u.ExpectedRevision, Title: u.Path, Body: r.Text, Locator: loc, Author: u.Author, EventTime: u.EventTime})
	if e != nil {
		return Receipt{}, e
	}
	ref := catalog.SourceRef{ID: src.ID, Revision: src.Revision, AccessRevision: src.AccessRevision, Hash: src.Hash}
	assetStatus := status
	if parseErr == nil {
		assetStatus = "parsed"
	}
	// Persist the original before publishing derived chunks. If interrupted, an
	// exact replay repairs the missing stage without duplicate source/chunk IDs.
	tx, e := s.Catalog.Pool().Begin(ctx)
	if e != nil {
		return Receipt{}, e
	}
	defer tx.Rollback(ctx)
	if _, e = s.Catalog.AccessTx(ctx, tx, []catalog.SourceRef{ref}, "memory_ingest"); e != nil {
		return Receipt{}, e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO memory_source_assets(source_id,source_revision,format,path,repository_revision,original_hash,original_bytes,parser_version,method,uncertain,status,error_code) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) ON CONFLICT(source_id,source_revision) DO NOTHING`, src.ID, src.Revision, u.Format, u.Path, u.RepositoryRevision, originalHash, u.Raw, Version, r.Method, r.Uncertain, assetStatus, code); e != nil {
		return Receipt{}, e
	}
	var exact bool
	if e = tx.QueryRow(ctx, `SELECT original_hash=$3 AND parser_version=$4 AND (status=$5 OR ($5='parsed' AND status='ready')) FROM memory_source_assets WHERE source_id=$1 AND source_revision=$2`, src.ID, src.Revision, originalHash, Version, assetStatus).Scan(&exact); e != nil || !exact {
		return Receipt{}, catalog.ErrStale
	}
	if e = tx.Commit(ctx); e != nil {
		return Receipt{}, e
	}
	receipt := Receipt{Source: src, Status: status, ErrorCode: code, ParserVersion: Version, Uncertain: r.Uncertain}
	if parseErr != nil {
		return receipt, nil
	}
	receipt.Fragments, e = s.Catalog.PublishFragments(ctx, ref, r.Fragments)
	if e != nil {
		receipt.Status = "parsed"
		return receipt, e
	}
	tx, e = s.Catalog.Pool().Begin(ctx)
	if e != nil {
		return receipt, e
	}
	defer tx.Rollback(ctx)
	if _, e = s.Catalog.AccessTx(ctx, tx, []catalog.SourceRef{ref}, "memory_ingest"); e != nil {
		return receipt, e
	}
	if _, e = tx.Exec(ctx, `UPDATE memory_source_assets SET status='ready' WHERE source_id=$1 AND source_revision=$2 AND status IN ('parsed','ready')`, src.ID, src.Revision); e != nil {
		return receipt, e
	}
	return receipt, tx.Commit(ctx)
}

// Original keeps real file bytes and revision available through the same current
// access proof as its fragments. A tombstone erases every asset revision.
func (s *Store) Original(ctx context.Context, id string, revision int64) ([]byte, string, error) {
	if s == nil || s.Catalog == nil || revision < 1 {
		return nil, "", catalog.ErrInvalid
	}
	if _, ok := owneridentity.FromContext(ctx); !ok {
		return nil, "", catalog.ErrDenied
	}
	tx, e := s.Catalog.Pool().Begin(ctx)
	if e != nil {
		return nil, "", e
	}
	defer tx.Rollback(ctx)
	if _, e = s.Catalog.AccessTx(ctx, tx, []catalog.SourceRef{{ID: id}}, "memory_read"); e != nil {
		return nil, "", e
	}
	var raw []byte
	var path string
	e = tx.QueryRow(ctx, `SELECT original_bytes,path FROM memory_source_assets WHERE source_id=$1 AND source_revision=$2 AND status<>'deleted'`, id, revision).Scan(&raw, &path)
	if e == pgx.ErrNoRows {
		return nil, "", catalog.ErrDenied
	}
	if e != nil {
		return nil, "", e
	}
	return raw, path, tx.Commit(ctx)
}
