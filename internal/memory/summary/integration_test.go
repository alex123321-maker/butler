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

func TestCoveragePublicationIntegration(t *testing.T) {
	dsn := os.Getenv("BUTLER_TEST_POSTGRES_URL")
	if dsn == "" {
		t.Skip("BUTLER_TEST_POSTGRES_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := postgresstore.Open(ctx, postgresstore.Config{URL: dsn, MaxConns: 4, MinConns: 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.RunMigrations(ctx, filepath.Join("..", "..", "..", "migrations")); err != nil {
		t.Fatal(err)
	}
	key := fmt.Sprintf("summary:%d", time.Now().UnixNano())
	if _, err := db.Pool().Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES($1,'owner','test')`, key); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.Pool().Exec(context.Background(), `DELETE FROM sessions WHERE session_key=$1`, key) })
	messages := transcript.NewStore(db.Pool())
	store := NewStore(db.Pool())
	a, err := messages.AppendMessage(ctx, transcript.Message{SessionKey: key, Role: "user", Content: "можно отправлять", CreatedAt: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	b, err := messages.AppendMessage(ctx, transcript.Message{SessionKey: key, Role: "user", Content: "только черновики", CreatedAt: time.Now().Add(-time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	source, err := messages.GetTranscript(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if a.Sequence != 1 || b.Sequence != 2 || source.Messages[0].MessageID != a.MessageID || source.SourceRevision != 1 {
		t.Fatal("ordering depended on timestamps")
	}
	old := Snapshot{SessionKey: key, Summary: "Можно отправлять", ThroughMessageID: a.MessageID, ThroughSequence: a.Sequence, SourceRevision: source.SourceRevision, FormatVersion: 1}
	newer := old
	newer.Summary = "Только черновики"
	newer.ThroughMessageID = b.MessageID
	newer.ThroughSequence = b.Sequence
	if saved, err := store.SaveCoveredSummary(ctx, old); err != nil || !saved {
		t.Fatalf("first publication: %v %v", saved, err)
	}
	// Simulate a slow older inference completing after a newer inference.
	start := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		<-start
		saved, err := store.SaveCoveredSummary(ctx, old)
		if err == nil && saved {
			err = fmt.Errorf("stale worker overwrote newer coverage")
		}
		done <- err
	}()
	if saved, err := store.SaveCoveredSummary(ctx, newer); err != nil || !saved {
		t.Fatalf("new publication: %v %v", saved, err)
	}
	close(start)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	current, err := store.GetSummarySnapshot(ctx, key)
	if err != nil || current.ThroughSequence != 2 {
		t.Fatalf("current: %+v %v", current, err)
	}
	wrong := newer
	wrong.ThroughSequence = 3
	if saved, err := store.SaveCoveredSummary(ctx, wrong); err != nil || saved {
		t.Fatalf("mismatched boundary accepted: %v %v", saved, err)
	}
	if _, err := db.Pool().Exec(ctx, `UPDATE messages SET content='не отправляй' WHERE message_id=$1`, a.MessageID); err != nil {
		t.Fatal(err)
	}
	if saved, err := store.SaveCoveredSummary(ctx, newer); err != nil || saved {
		t.Fatalf("edited source accepted: %v %v", saved, err)
	}
	current, err = store.GetSummarySnapshot(ctx, key)
	if err != nil || current.Summary != "" {
		t.Fatalf("edited source summary survived: %+v %v", current, err)
	}
	source, err = messages.GetTranscript(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	newer.SourceRevision = source.SourceRevision
	if saved, err := store.SaveCoveredSummary(ctx, newer); err != nil || !saved {
		t.Fatalf("new source publication: %v %v", saved, err)
	}
	if _, err := db.Pool().Exec(ctx, `DELETE FROM messages WHERE message_id=$1`, a.MessageID); err != nil {
		t.Fatal(err)
	}
	if saved, err := store.SaveCoveredSummary(ctx, newer); err != nil || saved {
		t.Fatalf("deleted source accepted: %v %v", saved, err)
	}
	c, err := messages.AppendMessage(ctx, transcript.Message{SessionKey: key, Role: "assistant", Content: "Черновик"})
	if err != nil {
		t.Fatal(err)
	}
	if c.Sequence != 3 {
		t.Fatalf("deletion reused a sequence: %d", c.Sequence)
	}
	if _, err := db.Pool().Exec(ctx, `UPDATE messages SET message_sequence=20 WHERE message_id=$1`, c.MessageID); err == nil {
		t.Fatal("message sequence changed")
	}
}
