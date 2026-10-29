// Package reader serves bounded catalog DATA after fresh source authorization.
package reader

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/readcontrol"
	"github.com/butler/butler/internal/memory/sanitize"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
)

// SemanticCandidate is server-derived from the existing guarded hybrid reader,
// never from model arguments. Revision is the exact current chunk timestamp.
type SemanticCandidate struct {
	ID       int64
	Revision time.Time
}
type Reader struct {
	Sources            *catalog.Store
	SemanticCandidates []SemanticCandidate
	// Trusted cohort exposure subset, not authority. nil preserves the ordinary
	// catalog; non-nil filters exact sources before ranking or loading a body.
	SourceSubset []catalog.SourceRef
	// Trusted owning composition only. When set, discard incompatible lesson
	// metadata before ranking, pagination or Catalog's durable run pinning.
	// Empty means no configured experience consumer; ordinary sources still pass.
	// This is only a prefilter: SourceEligibility and full Catalog proof remain.
	ExperienceEnvironment *string
	// Exact current permitted Broker dependencies, encoded by the owning typed
	// experience adapter. Non-nil applies before any source pins. An empty array
	// permits ordinary DATA only. nil retains the lower-level environment filter.
	ExperienceDependencies json.RawMessage
	// Concrete typed-lesson applicability proof, provided by the owning app.
	// It runs after Catalog access in this same transaction, without IPC.
	SourceEligibility func(context.Context, pgx.Tx, catalog.Source) (bool, error)
	// Optional content-free pointers into a fully authorized typed source.
	SourceLocator func(context.Context, pgx.Tx, catalog.Source) (json.RawMessage, error)
}

func (r Reader) discoveryItem(ctx context.Context, tx pgx.Tx, src catalog.Source, key, title, text string, loc json.RawMessage) (readcontrol.Item, error) {
	if r.SourceLocator != nil {
		selected, err := r.SourceLocator(ctx, tx, src)
		if err != nil {
			return readcontrol.Item{}, err
		}
		if selected != nil {
			loc = selected
		}
	}
	return item(src, key, title, text, loc, 240), nil
}

func (r Reader) eligible(ctx context.Context, tx pgx.Tx, src catalog.Source) (bool, error) {
	if r.SourceSubset != nil {
		match := false
		for _, ref := range r.SourceSubset {
			if ref.ID == src.ID && ref.Revision == src.Revision && ref.AccessRevision == src.AccessRevision && ref.Hash == src.Hash {
				match = true
				break
			}
		}
		if !match {
			return false, nil
		}
	}
	if r.SourceEligibility == nil {
		return true, nil
	}
	return r.SourceEligibility(ctx, tx, src)
}

const permittedSources = `WITH actor AS (SELECT s.user_id AS owner,r.session_key,r.task_id FROM runs r JOIN sessions s USING(session_key) WHERE r.run_id=$1), permitted AS (
 SELECT s.source_id,s.head_revision,s.access_revision,r.content_hash,r.title,r.body,r.event_time FROM memory_sources s JOIN memory_source_revisions r ON r.source_id=s.source_id AND r.revision=s.head_revision CROSS JOIN actor a
 WHERE s.owner_id=a.owner AND s.status='active' /* trusted_subset */ AND (s.scope_type='user' AND s.scope_id=a.owner OR s.scope_type='session' AND s.scope_id=a.session_key OR s.scope_type='task' AND s.scope_id=a.task_id OR EXISTS(SELECT 1 FROM memory_scope_grants g JOIN tasks t USING(task_id) WHERE g.source_id=s.source_id AND g.task_id=a.task_id AND g.owner_id=a.owner AND g.task_revision=t.revision AND g.access_revision=s.access_revision AND g.purpose='memory_read')))`

func (r Reader) subsetIDs() []string {
	if r.SourceSubset == nil {
		return nil
	}
	ids := []string{}
	for _, s := range r.SourceSubset {
		ids = append(ids, s.ID)
	}
	return ids
}
func (r Reader) permittedSources(parameter int) string {
	filter := fmt.Sprintf("AND ($%d::text[] IS NULL OR s.source_id=ANY($%d))", parameter, parameter)
	if r.ExperienceEnvironment != nil {
		// The empty-table test is statement-local, not a cached eligibility result.
		filter += " AND (NOT EXISTS(SELECT 1 FROM memory_experience_lessons) OR (" + r.experiencePredicate("ARRAY[s.source_id]", parameter+1) + "))"
	}
	return strings.Replace(permittedSources, "/* trusted_subset */", filter, 1)
}

// ids is a static SQL expression supplied only by this package. Both owning
// values remain SQL parameters; no model text/identifier becomes SQL syntax.
func (r Reader) experiencePredicate(ids string, parameter int) string {
	predicate := fmt.Sprintf("butler_memory_experience_lessons_eligible(%s,$%d)", ids, parameter)
	if r.ExperienceDependencies != nil {
		predicate += fmt.Sprintf(` AND NOT EXISTS(WITH RECURSIVE roots AS(SELECT unnest(%s::text[]) AS id UNION SELECT d.parent_source_id FROM memory_source_dependencies d JOIN roots r ON r.id=d.child_source_id)
 SELECT 1 FROM roots r JOIN memory_experience_lessons l ON l.source_id=r.id WHERE jsonb_array_length(l.dependencies_json)=0 OR NOT l.dependencies_json <@ $%d::jsonb)`, ids, parameter+1)
	}
	return predicate
}

func (r Reader) sourceQueryArgs(args ...any) []any {
	if r.ExperienceEnvironment != nil {
		args = append(args, *r.ExperienceEnvironment)
		if r.ExperienceDependencies != nil {
			args = append(args, string(r.ExperienceDependencies))
		}
	}
	return args
}

func status(e error) string {
	switch {
	case errors.Is(e, catalog.ErrStale):
		return "stale"
	case errors.Is(e, catalog.ErrDenied), errors.Is(e, catalog.ErrDeleted), errors.Is(e, catalog.ErrRestore):
		return "denied"
	default:
		return "backend_error"
	}
}
func clip(s string, offset, limit int) (string, bool, int) {
	// Keep continuation positions in Unicode characters, including masked spans.
	r := []rune(s)
	if offset > len(r) {
		offset = len(r)
	}
	end := offset + limit
	if end > len(r) {
		end = len(r)
	}
	return string(r[offset:end]), end < len(r), end
}
func item(src catalog.Source, key, title, text string, loc json.RawMessage, max int) readcontrol.Item {
	if len(loc) > 2048 || !json.Valid(loc) || sanitize.JSON(string(loc)) != string(loc) {
		loc = json.RawMessage(`{}`)
	}
	safeTitle := sanitize.MaskCredentials(title)
	safeText := sanitize.MaskCredentials(text)
	redacted := safeTitle != title || safeText != text
	title, _, _ = clip(safeTitle, 0, 160)
	safeText, truncated, next := clip(safeText, 0, max)
	out := readcontrol.Item{Source: readcontrol.SourceRef{ID: src.ID, Revision: src.Revision, AccessRevision: src.AccessRevision, Hash: src.Hash}, Kind: src.Kind, ScopeType: src.ScopeType, Title: title, Text: safeText, FragmentKey: key, Locator: loc, Freshness: "current_revision_unverified_fact", Truncated: truncated, Redacted: redacted, NextOffset: next}
	if src.EventTime != nil {
		out.EventTime = src.EventTime.UTC().Format(time.RFC3339Nano)
	}
	return out
}

// ReadTx never accepts owner, scope, paths or SQL from the model. The caller
// owns the actual dispatch transaction and rechecks it before commit/release.
func (r Reader) ReadTx(ctx context.Context, tx pgx.Tx, tool string, a readcontrol.Args) (readcontrol.Result, error) {
	if r.Sources == nil || tx == nil {
		return readcontrol.Empty("denied"), readcontrol.ErrDenied
	}
	if len(r.SourceSubset) > 8 {
		return readcontrol.Empty("denied"), readcontrol.ErrDenied
	}
	if tool == readcontrol.Search {
		if a.Strategy == "semantic_hybrid" {
			return r.semanticSearch(ctx, tx, a)
		}
		return r.search(ctx, tx, a)
	}
	// Fragment keys are exact catalog handles. A masked replacement would no
	// longer address the stored fragment; never release or accept a dirty key.
	if sanitize.Text(a.FragmentKey) != a.FragmentKey {
		return readcontrol.Empty("denied"), nil
	}
	if r.SourceSubset != nil {
		allowed := false
		for _, ref := range r.SourceSubset {
			if ref.ID == a.SourceID {
				allowed = true
				break
			}
		}
		if !allowed {
			return readcontrol.Empty("denied"), nil
		}
	}
	if r.ExperienceEnvironment != nil {
		var allowed bool
		err := tx.QueryRow(ctx, "SELECT "+r.experiencePredicate("$1", 2), r.sourceQueryArgs([]string{a.SourceID})...).Scan(&allowed)
		if err != nil {
			return readcontrol.Empty("backend_error"), err
		}
		if !allowed {
			return readcontrol.Empty("denied"), nil
		}
	}
	// This read is safe even when the ID was acquired from a previous run: a
	// source reference is evidence, never a capability. AccessTx pins ancestors.
	sources, e := r.Sources.AccessTx(ctx, tx, []catalog.SourceRef{{ID: a.SourceID, Revision: a.Revision, AccessRevision: a.AccessRevision, Hash: a.Hash}}, "memory_read")
	if e != nil {
		return readcontrol.Empty(status(e)), nil
	}
	if len(sources) != 1 {
		return readcontrol.Empty("denied"), nil
	}
	src := sources[0]
	allowed, eligibilityErr := r.eligible(ctx, tx, src)
	if eligibilityErr != nil {
		return readcontrol.Empty("backend_error"), eligibilityErr
	}
	if !allowed {
		return readcontrol.Empty("denied"), nil
	}
	if tool == readcontrol.Neighbors {
		return r.neighbors(ctx, tx, src, a)
	}
	var title, text string
	var loc json.RawMessage
	if a.FragmentKey == "" {
		// AccessTx already loaded and authorized this complete source. Detect
		// credential spans before slicing, including across page boundaries.
		title, text, loc = src.Title, src.Body, src.Locator
	} else {
		e = tx.QueryRow(ctx, `SELECT c.title,c.content,o.locator FROM memory_object_sources o JOIN memory_chunks c ON c.id=o.object_id WHERE o.memory_type='chunk' AND o.source_id=$1 AND o.source_revision=$2 AND o.fragment_key=$3 ORDER BY o.object_id LIMIT 1`, src.ID, src.Revision, a.FragmentKey).Scan(&title, &text, &loc)
	}
	if errors.Is(e, pgx.ErrNoRows) {
		return readcontrol.Empty("not_found"), nil
	}
	if e != nil {
		return readcontrol.Empty("backend_error"), e
	}
	out := readcontrol.Empty("ok")
	masked := sanitize.MaskCredentials(text)
	page, _, _ := clip(masked, a.Offset, a.MaxChars+1)
	v := item(src, a.FragmentKey, title, page, loc, a.MaxChars)
	v.Redacted = v.Redacted || masked != text
	v.NextOffset += a.Offset
	// AccessTx already checked and locked the exact body/evidence privacy graph.
	v.Evidence = src.RetainedEvidence
	out.Items = append(out.Items, v)
	out.Truncated = v.Truncated
	if e = r.conflicts(ctx, tx, &out); e != nil {
		return readcontrol.Empty("backend_error"), e
	}
	return out, nil
}
func (r Reader) search(ctx context.Context, tx pgx.Tx, a readcontrol.Args) (readcontrol.Result, error) {
	token, ok := ownership.TokenFromContext(ctx)
	if !ok {
		return readcontrol.Empty("denied"), nil
	}
	if _, e := r.Sources.AccessTx(ctx, tx, nil, "memory_read"); e != nil {
		return readcontrol.Empty(status(e)), nil
	}
	// Search only same-owner current scopes or individually granted sources.
	// The catalog repeats complete current ancestor/privacy proof afterwards.
	var sourceTruncated bool
	if e := tx.QueryRow(ctx, r.permittedSources(2)+` SELECT EXISTS(SELECT 1 FROM permitted WHERE char_length(body)>65536)`, r.sourceQueryArgs(token.RunID, r.subsetIDs())...).Scan(&sourceTruncated); e != nil {
		return readcontrol.Empty("backend_error"), e
	}
	rows, e := tx.Query(ctx, r.permittedSources(7)+`, matches AS (
	 SELECT p.source_id,p.head_revision,p.access_revision,p.content_hash,''::text AS fragment_key,p.title,p.body AS snippet,'{}'::jsonb AS locator,0 AS preference
 FROM permitted p WHERE ($5='' OR p.event_time>=NULLIF($5,'')::timestamptz) AND ($6='' OR p.event_time<NULLIF($6,'')::timestamptz) AND (p.source_id=$2 OR to_tsvector('simple',p.title||' '||substring(p.body FROM 1 FOR 65536)) @@ plainto_tsquery('simple',$2))
	 UNION ALL SELECT p.source_id,p.head_revision,p.access_revision,p.content_hash,o.fragment_key,c.title,c.content,o.locator,1
 FROM permitted p JOIN memory_object_sources o ON o.source_id=p.source_id AND o.source_revision=p.head_revision AND o.memory_type='chunk' JOIN memory_chunks c ON c.id=o.object_id
 WHERE ($5='' OR p.event_time>=NULLIF($5,'')::timestamptz) AND ($6='' OR p.event_time<NULLIF($6,'')::timestamptz) AND to_tsvector('simple',c.title||' '||c.content) @@ plainto_tsquery('simple',$2))
 SELECT source_id,head_revision,access_revision,content_hash,fragment_key,title,snippet,locator FROM matches ORDER BY preference DESC,source_id,fragment_key LIMIT $3 OFFSET $4`, r.sourceQueryArgs(token.RunID, a.Query, a.Limit+1, a.Page*a.Limit, a.EventFrom, a.EventUntil, r.subsetIDs())...)
	if e != nil {
		return readcontrol.Empty("backend_error"), e
	}
	type hit struct {
		ref              catalog.SourceRef
		key, title, text string
		loc              json.RawMessage
	}
	var hits []hit
	var unsafeFragment bool
	for rows.Next() {
		var h hit
		if e = rows.Scan(&h.ref.ID, &h.ref.Revision, &h.ref.AccessRevision, &h.ref.Hash, &h.key, &h.title, &h.text, &h.loc); e != nil {
			rows.Close()
			return readcontrol.Empty("backend_error"), e
		}
		if sanitize.Text(h.key) != h.key {
			unsafeFragment = true
			continue
		}
		hits = append(hits, h)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return readcontrol.Empty("backend_error"), e
	}
	out := readcontrol.Empty("empty")
	if a.EventFrom != "" || a.EventUntil != "" {
		out.Diagnostics = []string{"reported_event_time_only", "unknown_event_times_excluded"}
	}
	out.Truncated = sourceTruncated || unsafeFragment
	if len(hits) == 0 {
		return out, nil
	}
	out.Truncated = out.Truncated || len(hits) > a.Limit
	if len(hits) > a.Limit {
		hits = hits[:a.Limit]
	}
	refs := []catalog.SourceRef{}
	seen := map[string]bool{}
	for _, h := range hits {
		if !seen[h.ref.ID] {
			refs = append(refs, h.ref)
			seen[h.ref.ID] = true
		}
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].ID < refs[j].ID })
	sources, e := r.Sources.AccessTx(ctx, tx, refs, "memory_read")
	if e != nil {
		return readcontrol.Empty(status(e)), nil
	}
	byID := map[string]catalog.Source{}
	for _, s := range sources {
		byID[s.ID] = s
	}
	out.Status = "ok"
	for _, h := range hits {
		allowed, err := r.eligible(ctx, tx, byID[h.ref.ID])
		if err != nil {
			return readcontrol.Empty("backend_error"), err
		}
		if !allowed {
			out.Truncated = true
			continue
		}
		v, err := r.discoveryItem(ctx, tx, byID[h.ref.ID], h.key, h.title, h.text, h.loc)
		if err != nil {
			return readcontrol.Empty("backend_error"), err
		}
		out.Items = append(out.Items, v)
	}
	if len(out.Items) == 0 {
		out.Status = "empty"
	}
	if e = r.conflicts(ctx, tx, &out); e != nil {
		return readcontrol.Empty("backend_error"), e
	}
	return out, nil
}

func (r Reader) semanticSearch(ctx context.Context, tx pgx.Tx, a readcontrol.Args) (readcontrol.Result, error) {
	out := readcontrol.Empty("empty")
	out.Strategy = "semantic_hybrid"
	token, ok := ownership.TokenFromContext(ctx)
	if !ok {
		return readcontrol.Empty("denied"), nil
	}
	if len(r.SemanticCandidates) > 4 {
		return readcontrol.Empty("denied"), readcontrol.ErrDenied
	}
	if _, err := r.Sources.AccessTx(ctx, tx, nil, "memory_read"); err != nil {
		return readcontrol.Empty(status(err)), nil
	}
	type hit struct {
		ref              catalog.SourceRef
		key, title, text string
		loc              json.RawMessage
	}
	hits := []hit{}
	refs := []catalog.SourceRef{}
	seen := map[string]bool{}
	for _, candidate := range r.SemanticCandidates {
		if candidate.ID < 1 || candidate.Revision.IsZero() {
			return readcontrol.Empty("denied"), readcontrol.ErrDenied
		}
		var h hit
		err := tx.QueryRow(ctx, r.permittedSources(6)+` SELECT p.source_id,p.head_revision,p.access_revision,p.content_hash,o.fragment_key,c.title,c.content,o.locator FROM permitted p JOIN memory_object_sources o ON o.source_id=p.source_id AND o.source_revision=p.head_revision AND o.memory_type='chunk' JOIN memory_chunks c ON c.id=o.object_id WHERE c.id=$2 AND c.updated_at=$3 AND ($4='' OR p.event_time>=NULLIF($4,'')::timestamptz) AND ($5='' OR p.event_time<NULLIF($5,'')::timestamptz)`, r.sourceQueryArgs(token.RunID, candidate.ID, candidate.Revision, a.EventFrom, a.EventUntil, r.subsetIDs())...).Scan(&h.ref.ID, &h.ref.Revision, &h.ref.AccessRevision, &h.ref.Hash, &h.key, &h.title, &h.text, &h.loc)
		if errors.Is(err, pgx.ErrNoRows) {
			out.Truncated = true
			continue
		}
		if err != nil {
			return readcontrol.Empty("backend_error"), err
		}
		if sanitize.Text(h.key) != h.key {
			out.Truncated = true
			continue
		}
		hits = append(hits, h)
		if !seen[h.ref.ID] {
			refs = append(refs, h.ref)
			seen[h.ref.ID] = true
		}
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].ID < refs[j].ID })
	sources, err := r.Sources.AccessTx(ctx, tx, refs, "memory_read")
	if err != nil {
		return readcontrol.Empty(status(err)), nil
	}
	byID := map[string]catalog.Source{}
	for _, src := range sources {
		byID[src.ID] = src
	}
	for _, h := range hits {
		allowed, err := r.eligible(ctx, tx, byID[h.ref.ID])
		if err != nil {
			return readcontrol.Empty("backend_error"), err
		}
		if !allowed {
			out.Truncated = true
			continue
		}
		v, err := r.discoveryItem(ctx, tx, byID[h.ref.ID], h.key, h.title, h.text, h.loc)
		if err != nil {
			return readcontrol.Empty("backend_error"), err
		}
		out.Items = append(out.Items, v)
	}
	if len(out.Items) > 0 {
		out.Status = "ok"
	}
	if a.EventFrom != "" || a.EventUntil != "" {
		out.Diagnostics = []string{"reported_event_time_only", "unknown_event_times_excluded"}
	}
	if err = r.conflicts(ctx, tx, &out); err != nil {
		return readcontrol.Empty("backend_error"), err
	}
	return out, nil
}
func (r Reader) neighbors(ctx context.Context, tx pgx.Tx, src catalog.Source, a readcontrol.Args) (readcontrol.Result, error) {
	var ordinal int
	e := tx.QueryRow(ctx, `SELECT fragment_ordinal FROM memory_object_sources WHERE memory_type='chunk' AND source_id=$1 AND source_revision=$2 AND fragment_key=$3 ORDER BY object_id LIMIT 1`, src.ID, src.Revision, a.FragmentKey).Scan(&ordinal)
	if errors.Is(e, pgx.ErrNoRows) {
		return readcontrol.Empty("not_found"), nil
	}
	if e != nil {
		return readcontrol.Empty("backend_error"), e
	}
	max := a.MaxChars / 2
	if max < 1 {
		max = 1
	}
	rows, e := tx.Query(ctx, `SELECT o.fragment_key,c.title,c.content,o.locator FROM memory_object_sources o JOIN memory_chunks c ON c.id=o.object_id WHERE o.memory_type='chunk' AND o.source_id=$1 AND o.source_revision=$2 AND o.fragment_ordinal IN ($3-1,$3+1) ORDER BY o.fragment_ordinal,o.object_id LIMIT 2`, src.ID, src.Revision, ordinal)
	if e != nil {
		return readcontrol.Empty("backend_error"), e
	}
	out := readcontrol.Empty("empty")
	remaining := a.MaxChars
	for rows.Next() {
		var key, title, text string
		var loc json.RawMessage
		if e = rows.Scan(&key, &title, &text, &loc); e != nil {
			rows.Close()
			return readcontrol.Empty("backend_error"), e
		}
		if sanitize.Text(key) != key {
			out.Truncated = true
			continue
		}
		if remaining == 0 {
			out.Truncated = true
			continue
		}
		if max > remaining {
			max = remaining
		}
		v := item(src, key, title, text, loc, max)
		remaining -= utf8.RuneCountInString(v.Text)
		out.Items = append(out.Items, v)
		out.Truncated = out.Truncated || v.Truncated
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return readcontrol.Empty("backend_error"), e
	}
	if len(out.Items) > 0 {
		out.Status = "ok"
	}
	if e = r.conflicts(ctx, tx, &out); e != nil {
		return readcontrol.Empty("backend_error"), e
	}
	return out, nil
}
func (r Reader) conflicts(ctx context.Context, tx pgx.Tx, out *readcontrol.Result) error {
	for i := range out.Items {
		if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM memory_claims c JOIN memory_claims other ON other.owner_id=c.owner_id AND other.scope_type=c.scope_type AND other.scope_id=c.scope_id AND other.fact_key=c.fact_key WHERE c.source_id=$1 AND c.status='confirmed' AND other.status='candidate' AND other.claim_id<>c.claim_id)`, out.Items[i].Source.ID).Scan(&out.Items[i].Conflict); e != nil {
			return e
		}
		if !utf8.ValidString(out.Items[i].Text) {
			return readcontrol.ErrDenied
		}
	}
	return nil
}
