package catalog

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/butler/butler/internal/memory/sanitize"
	"github.com/butler/butler/internal/security/content"
	"github.com/jackc/pgx/v5"
)

type Correction struct {
	SourceID         string     `json:"source_id"`
	ExpectedRevision int64      `json:"expected_revision"`
	Title            string     `json:"title"`
	Body             string     `json:"body"`
	Author           string     `json:"author"`
	EventTime        *time.Time `json:"event_time"`
	ReviewLegacy     bool       `json:"review_legacy"`
}

// Correct appends a revision. It never changes scope/owner or treats document
// text as current permission. Legacy review makes data available for explicit
// candidate decisions; it does not confirm a legacy automatic profile value.
func (s *Store) Correct(ctx context.Context, p Correction) (Source, error) {
	own, e := owner(ctx)
	if e != nil {
		return Source{}, e
	}
	if len(p.Body) > 8*1024*1024 || len(p.Title) > 2048 || len(p.Author) > 512 || p.ExpectedRevision < 1 {
		return Source{}, ErrInvalid
	}
	if e = s.CheckRestore(ctx); e != nil {
		return Source{}, e
	}
	// Unlocked routing data only chooses the common session-before-source order.
	var scopeType, scopeID string
	if e = s.pool.QueryRow(ctx, `SELECT scope_type,scope_id FROM memory_sources WHERE source_id=$1 AND owner_id=$2`, p.SourceID, own).Scan(&scopeType, &scopeID); e != nil {
		return Source{}, ErrDenied
	}
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return Source{}, e
	}
	defer tx.Rollback(ctx)
	if e = checkOwnerScopeTx(ctx, tx, own, scopeType, scopeID); e != nil {
		return Source{}, e
	}
	if strings.HasPrefix(p.SourceID, "message:") {
		if _, e = tx.Exec(ctx, `SELECT session_key FROM sessions WHERE session_key=$1 FOR UPDATE`, scopeID); e != nil {
			return Source{}, e
		}
		if _, e = tx.Exec(ctx, `SELECT session_key FROM transcript_heads WHERE session_key=$1 FOR UPDATE`, scopeID); e != nil {
			return Source{}, e
		}
	}
	src, e := getSourceTx(ctx, tx, p.SourceID, true)
	if e != nil || src.OwnerID != own {
		return Source{}, ErrDenied
	}
	if src.Revision != p.ExpectedRevision {
		return Source{}, ErrStale
	}
	if src.Status == "deleted" || src.Status == "revoked" {
		return Source{}, ErrDeleted
	}
	if src.Status == "quarantined" && !p.ReviewLegacy {
		return Source{}, ErrDenied
	}
	if strings.HasPrefix(p.SourceID, "message:") {
		if src.Title != "user" || p.ReviewLegacy {
			return Source{}, ErrDenied
		}
		if _, e = tx.Exec(ctx, `UPDATE messages SET content=$2 WHERE message_id=$1 AND role='user' AND NOT privacy_redacted`, strings.TrimPrefix(p.SourceID, "message:"), p.Body); e != nil {
			return Source{}, e
		}
	} else {
		origin := src.Origin
		if sanitize.Text(p.Body) != p.Body || sanitize.Text(p.Title) != p.Title {
			origin.Sensitivity = content.Secret
		}
		pOrigin := mustJSON(origin)
		revision := src.Revision + 1
		if _, e = tx.Exec(ctx, `INSERT INTO memory_source_revisions(source_id,revision,content_hash,title,body,locator,origin,author,event_time) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, src.ID, revision, hash([]any{p.Title, p.Body, json.RawMessage(src.Locator), p.Author, p.EventTime, origin}), p.Title, p.Body, src.Locator, pOrigin, p.Author, p.EventTime); e != nil {
			return Source{}, e
		}
		if _, e = tx.Exec(ctx, `UPDATE memory_sources SET head_revision=$2,status='active',access_revision=access_revision+CASE WHEN status='quarantined' THEN 1 ELSE 0 END,updated_at=clock_timestamp() WHERE source_id=$1`, src.ID, revision); e != nil {
			return Source{}, e
		}
		if p.ReviewLegacy {
			for _, table := range []string{"memory_profile", "memory_episodes"} {
				// Noncatalog legacy rows remain candidates and historical data.
				if _, e = tx.Exec(ctx, `UPDATE `+table+` SET confirmation_state='pending' WHERE id IN(SELECT object_id FROM memory_object_sources WHERE source_id=$1 AND memory_type=$2)`, src.ID, map[string]string{"memory_profile": "profile", "memory_episodes": "episodic"}[table]); e != nil {
					return Source{}, e
				}
			}
		}
	}
	src, e = getSourceTx(ctx, tx, src.ID, false)
	if e != nil {
		return Source{}, e
	}
	return src, tx.Commit(ctx)
}

func (s *Store) CheckOwnerScope(ctx context.Context, typ, id string) error {
	own, e := owner(ctx)
	if e != nil {
		return e
	}
	if e = s.CheckRestore(ctx); e != nil {
		return e
	}
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if e = checkOwnerScopeTx(ctx, tx, own, typ, id); e != nil {
		return e
	}
	return tx.Commit(ctx)
}

// Export emits one bounded JSON row at a time, including every revision and
// claim. It has no silent list limit and no temporary text cache. A content-free
// tombstone remains exported after deletion; removed text cannot be recovered.
func (s *Store) Export(ctx context.Context, emit func(any) error) error {
	own, e := owner(ctx)
	if e != nil {
		return e
	}
	if e = s.CheckRestore(ctx); e != nil {
		return e
	}
	tx, e := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	queries := []struct{ kind, sql string }{
		{"source_revision", `SELECT to_jsonb(s)||to_jsonb(r) FROM memory_sources s JOIN memory_source_revisions r USING(source_id) WHERE s.owner_id=$1 ORDER BY s.source_id,r.revision`},
		{"claim", `SELECT to_jsonb(c) FROM memory_claims c WHERE owner_id=$1 ORDER BY claim_id`},
		{"dependency", `SELECT to_jsonb(d) FROM memory_source_dependencies d JOIN memory_sources s ON s.source_id=d.child_source_id WHERE s.owner_id=$1 ORDER BY d.child_source_id,d.parent_source_id`},
		{"deletion", `SELECT to_jsonb(d) FROM memory_deletions d WHERE owner_id=$1 ORDER BY watermark`},
	}
	for _, q := range queries {
		rows, e := tx.Query(ctx, q.sql, own)
		if e != nil {
			return e
		}
		for rows.Next() {
			var b json.RawMessage
			if e = rows.Scan(&b); e != nil {
				rows.Close()
				return e
			}
			if e = emit(map[string]any{"record_type": q.kind, "data": b}); e != nil {
				rows.Close()
				return e
			}
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
	}
	return tx.Commit(ctx)
}
