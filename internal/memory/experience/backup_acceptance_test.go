package experience

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/readcontrol"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5/pgxpool"
)

type backupProof struct {
	Original           catalog.SourceRef `json:"original"`
	Snapshot           catalog.SourceRef `json:"snapshot"`
	Lesson             catalog.SourceRef `json:"lesson"`
	Unrelated          catalog.SourceRef `json:"unrelated"`
	ClaimID            string            `json:"claim_id"`
	EpisodeID          int64             `json:"episode_id"`
	UnrelatedEpisodeID int64             `json:"unrelated_episode_id"`
	UnrelatedBodyHash  string            `json:"unrelated_body_hash"`
	AccountingHash     string            `json:"accounting_hash"`
	Terminal           bool              `json:"terminal"`
	TerminalHash       string            `json:"terminal_hash,omitempty"`
}

func accountingHash(t *testing.T, pool *pgxpool.Pool, ctx context.Context) string {
	t.Helper()
	var raw string
	if err := pool.QueryRow(ctx, `SELECT jsonb_build_object('attempts',(SELECT jsonb_agg(to_jsonb(a)ORDER BY operation_id)FROM memory_model_attempts a),'reservations',(SELECT jsonb_agg(to_jsonb(r)ORDER BY reservation_id)FROM execution_budget_reservations r))::text`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// Three externally driven phases surround real pg_dump/pg_restore. Fresh test
// databases and a current independent private deletion anchor are mandatory.
// Controlled inference proves lifecycle mechanics; native A/B/C remains separate.
func TestActualPostgresExperienceDumpRestoreDeletionIntegration(t *testing.T) {
	phase := os.Getenv("BUTLER_TEST_EXPERIENCE_BACKUP_PHASE")
	if phase == "" {
		t.Skip("actual external pg_dump/restore procedure required")
	}
	cfg, err := pgxpool.ParseConfig(os.Getenv("BUTLER_TEST_EXPERIENCE_BACKUP_POSTGRES_URL"))
	if err != nil || (cfg.ConnConfig.Database != "butler_experience_backup_test" && cfg.ConnConfig.Database != "butler_experience_restore_test") {
		t.Fatal("dedicated experience backup/restore DB required")
	}
	cfg.MaxConns = 2
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	ctx = owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "experience-actual-backup"})
	anchorPath, receiptPath := os.Getenv("BUTLER_TEST_EXPERIENCE_BACKUP_MANIFEST"), os.Getenv("BUTLER_TEST_EXPERIENCE_BACKUP_RECEIPT")
	if anchorPath == "" || receiptPath == "" {
		t.Fatal("private anchor/receipt paths required")
	}
	manifest := catalog.NewFileManifest(anchorPath)
	sources := catalog.New(pool, manifest)
	store, err := New(sources, strings.Repeat("b", 64), toolsFixture{readcontrol.Contract(readcontrol.Open)})
	if err != nil {
		t.Fatal(err)
	}
	if phase == "seed" {
		if _, err = pool.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public`); err != nil {
			t.Fatal(err)
		}
		_, file, _, _ := runtime.Caller(0)
		paths, err := filepath.Glob(filepath.Join(filepath.Dir(file), "..", "..", "..", "migrations", "*.up.sql"))
		if err != nil {
			t.Fatal(err)
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
		f := newLessonFixtureWithPool(t, pool, anchorPath)
		terminal := os.Getenv("BUTLER_TEST_EXPERIENCE_BACKUP_TERMINAL") == "1"
		var unrelated catalog.SourceRef
		if terminal {
			unrelated, _ = f.verifiedCase(t, 2)
			stopFixture(t, f)
		}
		lesson := f.publish(t)
		f.accept(t, lesson)
		if !terminal {
			unrelated, _ = f.verifiedCase(t, 2)
		}
		p := backupProof{Original: ref(f.parent), Snapshot: f.snapshot, Lesson: lesson, Unrelated: unrelated, AccountingHash: accountingHash(t, pool, ctx), Terminal: terminal}
		if terminal {
			if err = pool.QueryRow(ctx, `SELECT encode(sha256(convert_to(to_jsonb(a)::text,'UTF8')),'hex') FROM memory_experience_terminal_attempts a WHERE source_id=$1`, p.Snapshot.ID).Scan(&p.TerminalHash); err != nil {
				t.Fatal(err)
			}
		}
		if err = pool.QueryRow(ctx, `SELECT l.claim_id,c.episode_id FROM memory_experience_lessons l JOIN memory_claims c ON c.claim_id=l.claim_id WHERE l.source_id=$1`, lesson.ID).Scan(&p.ClaimID, &p.EpisodeID); err != nil {
			t.Fatal(err)
		}
		var unrelatedBody string
		if err = pool.QueryRow(ctx, `SELECT c.episode_id,e.content FROM memory_experience_lessons l JOIN memory_claims c ON c.claim_id=l.claim_id JOIN memory_episodes e ON e.id=c.episode_id WHERE l.source_id=$1`, unrelated.ID).Scan(&p.UnrelatedEpisodeID, &unrelatedBody); err != nil {
			t.Fatal(err)
		}
		p.UnrelatedBodyHash = hash(unrelatedBody)
		data, _ := json.Marshal(p)
		if err = os.WriteFile(receiptPath, data, 0600); err != nil {
			t.Fatal(err)
		}
		return
	}
	data, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	var p backupProof
	if json.Unmarshal(data, &p) != nil || p.EpisodeID < 1 || p.ClaimID == "" {
		t.Fatal("content-free backup receipt invalid")
	}
	if phase == "delete" {
		if _, err = sources.Delete(ctx, p.Original.ID, "delete", p.Original.Revision); err != nil {
			t.Fatal(err)
		}
	} else if phase == "restore" {
		if err = sources.CheckRestore(ctx); !errors.Is(err, catalog.ErrRestore) {
			t.Fatal("old backup read before current anchor replay", err)
		}
		if _, err = sources.Get(ctx, p.Lesson.ID); err == nil {
			t.Fatal("old accepted recipe resurrected before replay")
		}
		if err = sources.Reconcile(ctx); err != nil {
			t.Fatal(err)
		}
		if err = sources.Reconcile(ctx); err != nil {
			t.Fatal("second anchor replay", err)
		}
	} else {
		t.Fatal("unknown backup phase")
	}
	for _, r := range []catalog.SourceRef{p.Original, p.Snapshot, p.Lesson} {
		if _, err = sources.Get(ctx, r.ID); err == nil {
			t.Fatal("deleted experience source readable")
		}
		var bytes int
		if err = pool.QueryRow(ctx, `SELECT COALESCE(sum(octet_length(body)+octet_length(title)+octet_length(author)),0) FROM memory_source_revisions WHERE source_id=$1`, r.ID).Scan(&bytes); err != nil || bytes != 0 {
			t.Fatal("copied observation/lesson bytes retained", r.ID, bytes, err)
		}
	}
	var retained int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM memory_episodes WHERE id=$1`, p.EpisodeID).Scan(&retained); err != nil || retained != 0 {
		t.Fatal("episode projection survived privacy erasure", err)
	}
	var status, value, summary string
	if err = pool.QueryRow(ctx, `SELECT status,value_json::text,summary FROM memory_claims WHERE claim_id=$1`, p.ClaimID).Scan(&status, &value, &summary); err != nil || status != "deleted" || value != "{}" || summary != "" {
		t.Fatal("accepted lesson claim retained text", err)
	}
	if _, err = sources.Decide(ctx, catalog.Decision{ClaimID: p.ClaimID}); err == nil {
		t.Fatal("deleted experience reaccepted")
	}
	if _, err = store.VerifyPromotion(ctx, "owner", []catalog.SourceRef{p.Lesson, p.Unrelated}, nil, nil); err == nil {
		t.Fatal("deleted recipe qualified for promotion")
	}
	unrelated, err := sources.Get(ctx, p.Unrelated.ID)
	if err != nil || unrelated.Hash != p.Unrelated.Hash {
		t.Fatal("unrelated lesson lost", err)
	}
	var body string
	if err = pool.QueryRow(ctx, `SELECT content FROM memory_episodes WHERE id=$1`, p.UnrelatedEpisodeID).Scan(&body); err != nil || hash(body) != p.UnrelatedBodyHash {
		t.Fatal("unrelated accepted episode changed", err)
	}
	if accountingHash(t, pool, ctx) != p.AccountingHash {
		t.Fatal("paid attempts/reservations/usage hashes changed across delete/restore")
	}
	if p.Terminal {
		var actual string
		if err = pool.QueryRow(ctx, `SELECT encode(sha256(convert_to(to_jsonb(a)::text,'UTF8')),'hex') FROM memory_experience_terminal_attempts a WHERE source_id=$1`, p.Snapshot.ID).Scan(&actual); err != nil || actual != p.TerminalHash {
			t.Fatal("stopped-attempt identity/outcome metadata changed across delete/restore", err)
		}
		var unknown bool
		if err = pool.QueryRow(ctx, `SELECT i.status='blocked' AND i.error_code='effect_unknown' AND r.current_state='tool_running' AND t.status='blocked' FROM accepted_inputs i JOIN runs r ON r.run_id=i.run_id JOIN tasks t ON t.task_id=i.task_id WHERE i.input_id='stopped-input'`).Scan(&unknown); err != nil || !unknown {
			t.Fatal("delete/restore invented a terminal final or resolved UNKNOWN", err)
		}
	}
	var metadata string
	if err = pool.QueryRow(ctx, `SELECT jsonb_agg(to_jsonb(l))::text FROM memory_experience_lessons l`).Scan(&metadata); err != nil || strings.Contains(metadata, "Observed attempt") || strings.Contains(metadata, "I report success") {
		t.Fatal("unparented body in immutable metadata", err)
	}
}
