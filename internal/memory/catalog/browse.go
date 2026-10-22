package catalog

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/butler/butler/internal/memory/sanitize"
	"github.com/butler/butler/internal/security/content"
)

// BrowseItem is a bounded owner preview, never a source capability or a fact verdict.
type BrowseItem struct {
	Source
	CreatedAt        time.Time `json:"created_at"`
	PreviewTruncated bool      `json:"preview_truncated"`
	Redacted         bool      `json:"redacted"`
}
type BrowsePage struct {
	Items       []BrowseItem `json:"items"`
	HasMore     bool         `json:"has_more"`
	Mode        string       `json:"mode"`
	Diagnostics []string     `json:"diagnostics"`
}

// Browse does not borrow a run lease or admit inference. The independent owner
// HTTP catalog can browse recent sources and PostgreSQL text matches. A semantic
// query still needs its existing live-run/budget admission; never label text as it.
func (s *Store) Browse(ctx context.Context, query string, limit, offset int) (BrowsePage, error) {
	out := BrowsePage{Items: []BrowseItem{}, Mode: "recent", Diagnostics: []string{}}
	query = strings.TrimSpace(query)
	if !utf8.ValidString(query) || len(query) > 2048 || limit < 1 || limit > 20 || offset < 0 || offset > 1000 {
		return out, ErrInvalid
	}
	own, err := owner(ctx)
	if err != nil {
		return out, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if err = s.CheckRestoreTx(ctx, tx); err != nil {
		return out, err
	}
	// Secret sources cannot pass the existing memory access gate. Exclude them
	// from the browse candidates rather than letting one block an entire page.
	clause := ` AND COALESCE(r.origin->>'sensitivity','')<>'secret'`
	order := "s.created_at DESC,s.source_id DESC"
	if query != "" {
		out.Mode = "text"
		out.Diagnostics = []string{"owner_semantic_inference_not_admitted"}
		clause += ` AND to_tsvector('russian',r.title||' '||r.body) @@ websearch_to_tsquery('russian',$5)`
		order = `ts_rank_cd(to_tsvector('russian',r.title||' '||r.body),websearch_to_tsquery('russian',$5)) DESC,` + order
	}
	args := []any{own, limit + 1, offset, "active"}
	if query != "" {
		args = append(args, query)
	}
	rows, err := tx.Query(ctx, `SELECT s.source_id,s.created_at FROM memory_sources s JOIN memory_source_revisions r ON r.source_id=s.source_id AND r.revision=s.head_revision WHERE s.owner_id=$1 AND s.status=$4`+clause+` ORDER BY `+order+` LIMIT $2 OFFSET $3`, args...)
	if err != nil {
		return out, err
	}
	refs := []SourceRef{}
	created := map[string]time.Time{}
	for rows.Next() {
		var id string
		var at time.Time
		if err = rows.Scan(&id, &at); err != nil {
			rows.Close()
			return out, err
		}
		refs = append(refs, SourceRef{ID: id})
		created[id] = at
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	out.HasMore = len(refs) > limit
	if out.HasMore {
		refs = refs[:limit]
	}
	sources, err := s.AccessTx(ctx, tx, refs, "memory_read")
	if err != nil {
		return out, err
	}
	byID := map[string]Source{}
	for _, src := range sources {
		byID[src.ID] = src
	}
	for _, ref := range refs {
		src, ok := byID[ref.ID]
		if !ok {
			return out, ErrDenied
		}
		title, body := sanitize.MaskCredentials(src.Title), sanitize.MaskCredentials(src.Body)
		redacted := title != src.Title || body != src.Body
		if src.Origin.Sensitivity == content.Secret {
			title = "Закрытая запись"
			body = ""
			redacted = true
		}
		src.Title = string([]rune(title)[:min(len([]rune(title)), 200)])
		truncated := utf8.RuneCountInString(body) > 512
		if query != "" && !redacted {
			// Return the matching passage in a long document, not an unrelated
			// opening paragraph. Plain-text delimiters keep markup out of the DTO.
			var snippet string
			if err = tx.QueryRow(ctx, `SELECT ts_headline('russian',$1,websearch_to_tsquery('russian',$2),$3)`, body, query, `MaxFragments=1,MaxWords=42,MinWords=14,StartSel="",StopSel=""`).Scan(&snippet); err != nil {
				return out, err
			}
			body = snippet
		}
		text := []rune(body)
		truncated = truncated || len(text) > 512
		src.Body = string(text[:min(len(text), 512)])
		// A compact preview is not the full source export. Do not copy arbitrary
		// locators/external keys into the initial page, even when content is closed.
		src.ExternalKey = ""
		src.Locator = nil
		src.Author = sanitize.MaskCredentials(src.Author)
		if redacted {
			src.Author = ""
		}
		out.Items = append(out.Items, BrowseItem{Source: src, CreatedAt: created[src.ID], PreviewTruncated: truncated, Redacted: redacted})
	}
	if err = tx.Commit(ctx); err != nil {
		return BrowsePage{}, err
	}
	return out, nil
}
