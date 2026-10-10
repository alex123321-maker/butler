package summary

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/butler/butler/internal/memory/transcript"
	postgresstore "github.com/butler/butler/internal/storage/postgres"
)

func TestLegacySummaryCoverageMigrationIntegration(t *testing.T) {
	dsn := os.Getenv("BUTLER_SUMMARY_MIGRATION_POSTGRES_URL")
	if dsn == "" {
		t.Skip("requires an exclusive disposable summary migration database")
	}
	if dsn == os.Getenv("BUTLER_TEST_POSTGRES_URL") {
		t.Fatal("migration database must be exclusive")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := postgresstore.Open(ctx, postgresstore.Config{URL: dsn, MaxConns: 4, MinConns: 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	dir := filepath.Join("..", "..", "..", "migrations")
	if err := db.RunMigrations(ctx, dir); err != nil {
		t.Fatal(err)
	}
	downDir := t.TempDir()
	down, err := os.ReadFile(filepath.Join(dir, "030_transcript_summary_coverage.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(downDir, "030_transcript_summary_coverage.down.sql"), down, 0600); err != nil {
		t.Fatal(err)
	}
	if err := db.RunDownMigrations(ctx, downDir); err != nil {
		t.Fatal(err)
	}
	defer db.RunMigrations(context.Background(), dir)
	key := fmt.Sprintf("summary:legacy:%d", time.Now().UnixNano())
	defer db.Pool().Exec(context.Background(), `DELETE FROM sessions WHERE session_key=$1`, key)
	if _, err := db.Pool().Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel,summary) VALUES($1,'owner','test','Old unversioned summary')`, key); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"b", "a"} {
		if _, err := db.Pool().Exec(ctx, `INSERT INTO messages(message_id,session_key,role,content,created_at) VALUES($1,$2,'user','Старое сообщение','2026-01-01T00:00:00Z')`, key+id, key); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.RunMigrations(ctx, dir); err != nil {
		t.Fatal(err)
	}
	source, err := transcript.NewStore(db.Pool()).GetTranscript(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if len(source.Messages) != 2 || source.Messages[0].MessageID != key+"a" || source.Messages[0].Sequence != 1 || source.Messages[1].Sequence != 2 {
		t.Fatalf("unstable backfill: %+v", source)
	}
	if snapshot, err := NewStore(db.Pool()).GetSummarySnapshot(ctx, key); err != nil || snapshot.Summary != "" {
		t.Fatalf("legacy coverage invented: %+v %v", snapshot, err)
	}
	if err := db.RunDownMigrations(ctx, downDir); err != nil {
		t.Fatal(err)
	}
	var count int
	var legacy string
	if err := db.Pool().QueryRow(ctx, `SELECT COUNT(*) FROM messages WHERE session_key=$1`, key).Scan(&count); err != nil || count != 2 {
		t.Fatalf("rollback deleted source: %d %v", count, err)
	}
	if err := db.Pool().QueryRow(ctx, `SELECT summary FROM sessions WHERE session_key=$1`, key).Scan(&legacy); err != nil || legacy != "Old unversioned summary" {
		t.Fatalf("rollback deleted legacy summary: %q %v", legacy, err)
	}
}
