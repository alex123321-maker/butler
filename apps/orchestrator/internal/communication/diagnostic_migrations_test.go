package communication_test

import (
	"context"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/internal/memory/memorytest"
)

func TestCommunicationDiagnosticMigrationPreservesLegacyDeliveryIntegration(t *testing.T) {
	pool := memorytest.DatabaseAtVersion(t, 88)
	ctx := context.Background()
	const key = "web:legacy-diagnostic"
	if _, _, err := session.NewPostgresRepository(pool).CreateSession(ctx, session.CreateSessionParams{SessionKey: key, UserID: "legacy-owner", Channel: "web", MetadataJSON: `{}`}); err != nil {
		t.Fatal(err)
	}
	if _, err := run.NewPostgresRepository(pool).CreateRun(ctx, run.Record{RunID: "legacy-run", SessionKey: key, InputEventID: "legacy-input", Status: "completed", CurrentState: "completed", AutonomyMode: "mode_1", ModelProvider: "fixture", StartedAt: time.Now(), UpdatedAt: time.Now(), MetadataJSON: `{}`}); err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{
		`INSERT INTO artifacts(artifact_id,run_id,session_key,artifact_type,content_text) VALUES('legacy-artifact','legacy-run',$1,'assistant_final','saved legacy answer')`,
		`INSERT INTO messages(message_id,session_key,run_id,role,content,metadata) VALUES('legacy-message',$1,'legacy-run','assistant','saved legacy answer','{}')`,
		`INSERT INTO run_final_results(result_id,run_id,session_key,artifact_id,message_id,content,content_hash,answer_state) VALUES('legacy-result','legacy-run',$1,'legacy-artifact','legacy-message','saved legacy answer','unchanged-hash','final')`,
		`INSERT INTO delivery_outbox(notification_id,result_id,run_id,session_key,channel,chunk_index,content,status,deadline_at) VALUES('legacy-notification','legacy-result','legacy-run',$1,'telegram',0,'saved legacy answer','pending',clock_timestamp()+INTERVAL '24 hours')`,
	} {
		if _, err := pool.Exec(ctx, sql, key); err != nil {
			t.Fatal(err)
		}
	}
	names := []string{"089_communication_core", "090_communication_privacy", "091_communication_media", "092_telegram_communication_cursor", "093_telegram_ingress_admission", "094_telegram_transport_notices", "095_communication_widgets", "100_communication_owner_limits"}
	for _, suffix := range []string{"up", "down", "up"} {
		for step := range names {
			i := step
			if suffix == "down" {
				i = len(names) - 1 - step
			}
			if _, err := pool.Exec(ctx, migration(t, names[i]+"."+suffix+".sql")); err != nil {
				t.Fatal(names[i], suffix, err)
			}
		}
		var id, result, content, status string
		var attempts, publications int
		if err := pool.QueryRow(ctx, `SELECT notification_id,result_id,content,status,attempt_count FROM delivery_outbox WHERE notification_id='legacy-notification'`).Scan(&id, &result, &content, &status, &attempts); err != nil || id != "legacy-notification" || result != "legacy-result" || content != "saved legacy answer" || status != "pending" || attempts != 0 {
			t.Fatal("migration changed legacy delivery", id, result, content, status, attempts, err)
		}
		if suffix == "up" {
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM communication_publications`).Scan(&publications); err != nil || publications != 0 {
				t.Fatal("legacy backfill created new publications", publications, err)
			}
		}
	}
}
