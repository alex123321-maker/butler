package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
	"time"

	"github.com/butler/butler/internal/memory/candidates"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The external driver carries only IDs, revisions and evidence hashes across
// real pg_dump/pg_restore. No source, candidate, episode or provider text.
type candidateEpisodeBackupReceipt struct {
	Version          int               `json:"version"`
	Source           catalog.SourceRef `json:"source"`
	Result           catalog.SourceRef `json:"result"`
	ClaimID          string            `json:"claim_id"`
	EpisodeID        int64             `json:"episode_id"`
	Unrelated        catalog.SourceRef `json:"unrelated"`
	UnrelatedResult  catalog.SourceRef `json:"unrelated_result"`
	UnrelatedClaim   string            `json:"unrelated_claim_id"`
	UnrelatedEpisode int64             `json:"unrelated_episode_id"`
	UnrelatedHash    string            `json:"unrelated_episode_sha256"`
	SpendHash        string            `json:"attempts_and_reservations_sha256"`
}

// Ordinary package execution explicitly skips the external procedure; only the
// three-process driver with a genuine old dump/fresh database qualifies it.
func TestActualPostgresCandidateEpisodeDumpRestoreDeletionIntegration(t *testing.T) {
	phase := os.Getenv("BUTLER_TEST_CANDIDATE_BACKUP_PHASE")
	if phase == "" {
		t.Skip("actual external accepted-episode pg_dump/restore procedure required")
	}
	if phase != "seed" && phase != "delete" && phase != "restore" {
		t.Fatal("explicit seed, delete or restore phase required")
	}
	cfg, err := pgxpool.ParseConfig(os.Getenv("BUTLER_TEST_CANDIDATE_BACKUP_POSTGRES_URL"))
	if err != nil || cfg == nil || (cfg.ConnConfig.Database != "butler_candidate_episode_backup_test" && cfg.ConnConfig.Database != "butler_candidate_episode_restore_test") || (phase == "restore") != (cfg.ConnConfig.Database == "butler_candidate_episode_restore_test") {
		t.Fatal("dedicated candidate-episode backup/restore database required")
	}
	cfg.MaxConns = 2
	ctx, stop := context.WithTimeout(context.Background(), time.Minute)
	defer stop()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	proof, anchor := os.Getenv("BUTLER_TEST_CANDIDATE_BACKUP_RECEIPT"), os.Getenv("BUTLER_TEST_CANDIDATE_BACKUP_MANIFEST")
	if !filepath.IsAbs(proof) || !filepath.IsAbs(anchor) {
		t.Fatal("private receipt and independent anchor paths required")
	}
	manifest := catalog.NewFileManifest(anchor)
	if phase == "seed" {
		seedCandidateEpisodeBackup(t, ctx, pool, manifest, proof)
		return
	}
	raw, err := os.ReadFile(proof)
	if err != nil || len(raw) > 8192 {
		t.Fatal("private content-free episode receipt unavailable")
	}
	var r candidateEpisodeBackupReceipt
	if json.Unmarshal(raw, &r) != nil || r.Version != 1 || r.EpisodeID < 1 || r.UnrelatedEpisode < 1 || r.EpisodeID == r.UnrelatedEpisode {
		t.Fatal("invalid episode backup receipt")
	}
	f := newCandidateFixtureWithPool(t, pool, manifest)
	switch phase {
	case "delete":
		f.request("/api/v4/memory/delete", http.MethodPost, map[string]any{"source_id": r.Source.ID, "action": "delete", "expected_revision": r.Source.Revision}, http.StatusOK, nil)
	case "restore":
		var episodes, claims, copies int
		if err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM memory_episodes WHERE id=$1),(SELECT count(*) FROM memory_claims WHERE claim_id=$2 AND status='confirmed'),(SELECT count(*) FROM memory_source_revisions WHERE source_id=ANY($3) AND body<>'')`, r.EpisodeID, r.ClaimID, []string{r.Source.ID, r.Result.ID}).Scan(&episodes, &claims, &copies); err != nil || episodes != 1 || claims != 1 || copies != 2 {
			t.Fatal("restored fixture is not the old content-bearing accepted projection", episodes, claims, copies, err)
		}
		if err = f.catalog.CheckRestore(f.owner); !errors.Is(err, catalog.ErrRestore) {
			t.Fatal("old restored database accepted the newer independent deletion anchor", err)
		}
		for _, ref := range []catalog.SourceRef{r.Source, r.Result, r.Unrelated, r.UnrelatedResult} {
			if _, err = f.catalog.Get(f.owner, ref.ID); !errors.Is(err, catalog.ErrRestore) {
				t.Fatal("old restored source escaped the fail-closed boundary", err)
			}
		}
		for _, id := range []int64{r.EpisodeID, r.UnrelatedEpisode} {
			if err = f.catalog.CheckObjects(f.owner, []catalog.ObjectRef{{Type: "episodic", ID: id}}, "memory_read"); !errors.Is(err, catalog.ErrRestore) {
				t.Fatal("restored accepted episode readable before anchor replay", err)
			}
		}
		f.request("/api/v4/memory/decisions", http.MethodPost, catalog.Decision{ClaimID: r.ClaimID}, http.StatusServiceUnavailable, nil)
		if err = f.catalog.Reconcile(ctx); err != nil {
			t.Fatal(err)
		}
		afterFirstReplay := candidateEpisodeBusinessHash(t, ctx, pool)
		if err = f.catalog.Reconcile(ctx); err != nil {
			t.Fatal("second anchor replay was not idempotent", err)
		}
		if candidateEpisodeBusinessHash(t, ctx, pool) != afterFirstReplay {
			t.Fatal("second anchor replay changed retained business state")
		}
	}
	assertCandidateEpisodeBackupDeleted(t, ctx, f, r)
}

func seedCandidateEpisodeBackup(t *testing.T, ctx context.Context, pool *pgxpool.Pool, manifest *catalog.FileManifest, proof string) {
	t.Helper()
	if _, err := pool.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS vector`); err != nil {
		t.Fatal(err)
	}
	_, file, _, _ := runtime.Caller(0)
	paths, err := filepath.Glob(filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "migrations", "*.up.sql"))
	if err != nil || len(paths) == 0 {
		t.Fatal("migrations unavailable", err)
	}
	sort.Strings(paths)
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, string(raw)); err != nil {
			t.Fatalf("%s: %v", filepath.Base(path), err)
		}
	}
	if err = manifest.Initialize(); err != nil {
		t.Fatal(err)
	}
	f := newCandidateFixtureWithPool(t, pool, manifest)
	// This tests durable publication and privacy, not a one-second latency SLO.
	// Each finite owner policy permits exactly this ten-second call quote.
	f.budgets.Policy.ModelMillis = 10000
	f.enable()
	seed := func(key, body string) (catalog.SourceRef, catalog.SourceRef, string, int64) {
		t.Helper()
		src := f.source(key, body)
		p := f.worker.Provider.(*candidateProvider)
		p.source = src
		out, err := json.Marshal(candidates.Output{Units: []candidates.Unit{{SourceID: src.ID, Type: "outcome_episode", SpeechAct: "assertion", Key: "episode." + key, Quote: src.Body}}, Skips: []candidates.Skip{}})
		if err != nil {
			t.Fatal(err)
		}
		p.output = string(out)
		f.policy("backup-"+key, src, "cloud_allowed")
		if err = f.process(); err != nil {
			t.Fatal("actual controlled candidate worker publication failed", err)
		}
		var claim catalog.Claim
		for _, got := range f.claims() {
			if got.SourceID == src.ID {
				claim = got
			}
		}
		if claim.ID == "" || claim.Status != "candidate" || claim.MemoryType != "episodic" || claim.Confidence != 0 {
			t.Fatal("worker did not create a neutral typed episodic candidate")
		}
		f.request("/api/v4/memory/decisions", http.MethodPost, catalog.Decision{ClaimID: claim.ID}, http.StatusOK, &claim)
		var episodeID int64
		var noVector bool
		if err = pool.QueryRow(ctx, `SELECT e.id,e.embedding IS NULL FROM memory_claims c JOIN memory_episodes e ON e.id=c.episode_id WHERE c.claim_id=$1 AND c.status='confirmed' AND c.profile_id IS NULL AND e.confirmation_state='confirmed' AND e.scope_type='user' AND e.scope_id='owner'`, claim.ID).Scan(&episodeID, &noVector); err != nil || !noVector || claim.Status != "confirmed" {
			t.Fatal("owner HTTP decision did not project accepted episode", err)
		}
		if err = f.catalog.CheckObjects(f.owner, []catalog.ObjectRef{{Type: "episodic", ID: episodeID}}, "memory_read"); err != nil {
			t.Fatal("accepted episode not readable before backup", err)
		}
		var resultID string
		if err = pool.QueryRow(ctx, `SELECT a.result_source_id FROM memory_model_attempts a JOIN memory_aux_jobs j USING(job_id) WHERE j.policy_id=$1 AND a.state='published'`, "backup-"+key).Scan(&resultID); err != nil {
			t.Fatal(err)
		}
		result, err := f.catalog.Get(f.owner, resultID)
		if err != nil || result.Body == "" {
			t.Fatal("published candidate result missing", err)
		}
		ref := func(s catalog.Source) catalog.SourceRef {
			return catalog.SourceRef{ID: s.ID, Revision: s.Revision, AccessRevision: s.AccessRevision, Hash: s.Hash}
		}
		return ref(src), ref(result), claim.ID, episodeID
	}
	r := candidateEpisodeBackupReceipt{Version: 1}
	r.Source, r.Result, r.ClaimID, r.EpisodeID = seed("primary", "После отката тестовый сервис снова заработал.")
	r.Unrelated, r.UnrelatedResult, r.UnrelatedClaim, r.UnrelatedEpisode = seed("unrelated", "После перезапуска независимый тестовый экспорт завершился успешно.")
	if f.worker.Provider.(*candidateProvider).calls.Load() != 2 {
		t.Fatal("expected exactly two controlled worker attempts")
	}
	r.UnrelatedHash = candidateEpisodeRowHash(t, ctx, pool, r.UnrelatedEpisode)
	r.SpendHash = candidateEpisodeSpendHash(t, ctx, pool)
	out, err := os.OpenFile(proof, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	if err = json.NewEncoder(out).Encode(r); err != nil {
		t.Fatal(err)
	}
	if err = out.Sync(); err != nil {
		t.Fatal(err)
	}
	t.Log("actual worker and owner HTTP acceptance published two typed episodes; private receipt contains identities/hashes only")
}

func assertCandidateEpisodeBackupDeleted(t *testing.T, ctx context.Context, f *candidateFixture, r candidateEpisodeBackupReceipt) {
	t.Helper()
	if err := f.catalog.CheckRestore(f.owner); err != nil {
		t.Fatal(err)
	}
	for _, ref := range []catalog.SourceRef{r.Source, r.Result} {
		if _, err := f.catalog.Get(f.owner, ref.ID); !errors.Is(err, catalog.ErrDenied) {
			t.Fatal("deleted source/candidate result readable", err)
		}
		var retained int
		var hash, status string
		if err := f.pool.QueryRow(ctx, `SELECT s.status,r.content_hash,(SELECT COALESCE(sum(length(body)+length(title)+length(author)),0)::int FROM memory_source_revisions WHERE source_id=s.source_id) FROM memory_sources s JOIN memory_source_revisions r ON r.source_id=s.source_id AND r.revision=$2 WHERE s.source_id=$1`, ref.ID, ref.Revision).Scan(&status, &hash, &retained); err != nil || status != "deleted" || hash != ref.Hash || retained != 0 {
			t.Fatal("source/result text survived or original evidence hash changed", status, retained, err)
		}
	}
	var retained, heads int
	var status, summary, value, source string
	var revision int64
	var noEpisode bool
	if err := f.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM memory_episodes WHERE id=$1),(SELECT count(*) FROM memory_fact_heads WHERE active_claim_id=$2),c.status,c.summary,c.value_json::text,c.source_id,c.source_revision,c.episode_id IS NULL FROM memory_claims c WHERE c.claim_id=$2`, r.EpisodeID, r.ClaimID).Scan(&retained, &heads, &status, &summary, &value, &source, &revision, &noEpisode); err != nil || retained != 0 || heads != 0 || status != "deleted" || summary != "" || value != "{}" || source != r.Source.ID || revision != r.Source.Revision || !noEpisode {
		t.Fatal("accepted episodic projection/claim/head survived or evidence identity changed", retained, heads, status, err)
	}
	if err := f.catalog.CheckObjects(f.owner, []catalog.ObjectRef{{Type: "episodic", ID: r.EpisodeID}}, "memory_read"); !errors.Is(err, catalog.ErrDenied) {
		t.Fatal("deleted accepted episode remained context eligible", err)
	}
	// Neither a repeat owner decision nor retrying the source can resurrect it.
	f.request("/api/v4/memory/decisions", http.MethodPost, catalog.Decision{ClaimID: r.ClaimID}, http.StatusForbidden, nil)
	if _, err := f.catalog.Put(f.owner, catalog.PutSource{ExternalKey: "primary", Kind: "owner_statement", ScopeType: "user", ScopeID: "owner", Body: "resurrection attempt"}); !errors.Is(err, catalog.ErrDeleted) {
		t.Fatal("restored tombstone permitted source recreation", err)
	}
	for _, ref := range []catalog.SourceRef{r.Unrelated, r.UnrelatedResult} {
		got, err := f.catalog.Get(f.owner, ref.ID)
		if err != nil || got.Body == "" || got.Hash != ref.Hash || got.Revision != ref.Revision || got.AccessRevision != ref.AccessRevision {
			t.Fatal("selective replay changed unrelated source/result", err)
		}
	}
	if err := f.catalog.CheckObjects(f.owner, []catalog.ObjectRef{{Type: "episodic", ID: r.UnrelatedEpisode}}, "memory_read"); err != nil {
		t.Fatal("unrelated accepted episode lost access", err)
	}
	if got := candidateEpisodeRowHash(t, ctx, f.pool, r.UnrelatedEpisode); got != r.UnrelatedHash {
		t.Fatal("unrelated episode text/provenance/decision changed")
	}
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM memory_claims c JOIN memory_fact_heads h ON h.active_claim_id=c.claim_id WHERE c.claim_id=$1 AND c.status='confirmed' AND c.episode_id=$2`, r.UnrelatedClaim, r.UnrelatedEpisode).Scan(&retained); err != nil || retained != 1 {
		t.Fatal("selective deletion removed unrelated owner decision", retained, err)
	}
	if got := candidateEpisodeSpendHash(t, ctx, f.pool); got != r.SpendHash {
		t.Fatal("deletion/replay changed original paid attempts, usage, reservations or cumulative spend")
	}
	t.Log("accepted episode physically erased; claim/read/reaccept/reindex deny; unrelated accepted episode and every paid observation/reservation/hash survive")
}

func candidateEpisodeRowHash(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id int64) string {
	t.Helper()
	var raw []byte
	if err := pool.QueryRow(ctx, `SELECT to_jsonb(e) FROM memory_episodes e WHERE id=$1`, id).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func candidateEpisodeSpendHash(t *testing.T, ctx context.Context, pool *pgxpool.Pool) string {
	t.Helper()
	var raw []byte
	var attempts, reservations int
	if err := pool.QueryRow(ctx, `SELECT
 (SELECT jsonb_build_object('attempts',(SELECT jsonb_agg(to_jsonb(a) ORDER BY operation_id) FROM memory_model_attempts a),'reservations',(SELECT jsonb_agg(to_jsonb(r) ORDER BY reservation_id) FROM execution_budget_reservations r),'scopes',(SELECT jsonb_agg(to_jsonb(s) ORDER BY scope_id) FROM execution_budget_scopes s))),
 (SELECT count(*) FROM memory_model_attempts WHERE state='published' AND usage->>'InputTokens'='20' AND usage->>'OutputTokens'='12' AND usage->>'TotalTokens'='32' AND output_hash<>'' AND result_source_id<>''),
 (SELECT count(*) FROM execution_budget_reservations)`).Scan(&raw, &attempts, &reservations); err != nil || attempts != 2 || reservations != 2 {
		t.Fatal("original two observed and reserved attempts missing", attempts, reservations, err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func candidateEpisodeBusinessHash(t *testing.T, ctx context.Context, pool *pgxpool.Pool) string {
	t.Helper()
	var raw []byte
	if err := pool.QueryRow(ctx, `SELECT jsonb_build_object(
 'sources',(SELECT jsonb_agg(to_jsonb(s) ORDER BY source_id) FROM memory_sources s),
 'revisions',(SELECT jsonb_agg(to_jsonb(r) ORDER BY source_id,revision) FROM memory_source_revisions r),
 'claims',(SELECT jsonb_agg(to_jsonb(c) ORDER BY claim_id) FROM memory_claims c),
 'heads',(SELECT jsonb_agg(to_jsonb(h) ORDER BY owner_id,scope_type,scope_id,fact_key) FROM memory_fact_heads h),
 'episodes',(SELECT jsonb_agg(to_jsonb(e) ORDER BY id) FROM memory_episodes e),
 'objects',(SELECT jsonb_agg(to_jsonb(o) ORDER BY memory_type,object_id,source_id) FROM memory_object_sources o),
 'deletions',(SELECT jsonb_agg(to_jsonb(d) ORDER BY watermark) FROM memory_deletions d),
 'restore',(SELECT to_jsonb(s) FROM memory_restore_state s WHERE singleton))`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
