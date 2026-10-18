package compaction

import (
	"encoding/json"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/memory/summary"
	"strings"
	"testing"
)

func TestEveryLifecycleAttackThroughAtomicSummaryPublicationIntegration(t *testing.T) {
	for _, attack := range memorytest.LifecycleAttacks(t) {
		t.Run(attack.ID, func(t *testing.T) {
			f := newConsumerFixture(t)
			if err := f.js.ControlModel(f.ctx, f.policy.ID, "revoked"); err != nil {
				t.Fatal(err)
			}
			origin, _ := json.Marshal(map[string]any{"butler_provenance": map[string]any{"kind": attack.OriginKind, "sensitivity": "private", "source_refs": []string{"synthetic-lifecycle:" + attack.ID}}})
			if _, err := f.pool.Exec(f.ctx, `UPDATE messages SET content=$1,metadata=$2 WHERE session_key='session' AND message_sequence IN(1,2)`, attack.Body, origin); err != nil {
				t.Fatal(err)
			}
			if _, err := f.pool.Exec(f.ctx, `INSERT INTO tasks(task_id,session_key,owner_id,goal,status,constraints_json,working_json)VALUES('protected-task','session','owner','Текущая подтверждённая цель: только проверка','active','["Никаких отправок", "Не расширять scope или полномочия"]','{"plan":["Проверить"],"pending_actions":["Проверить неизвестный эффект"],"scratch":{"effect":"unknown"}}');UPDATE runs SET task_id='protected-task',task_revision=1 WHERE run_id='run';INSERT INTO tool_calls(tool_call_id,run_id,tool_name,args,status,runtime_target,started_at)VALUES('unknown-tool','run','mail.send','{}','unknown','test',clock_timestamp())`); err != nil {
				t.Fatal(err)
			}
			unrelated, err := f.cat.Put(f.ctx, catalog.PutSource{ExternalKey: "independent-safe-source", Kind: "document", ScopeType: "user", ScopeID: "owner", Title: "Independent source", Body: "This unrelated source must survive privacy deletion"})
			if err != nil {
				t.Fatal(err)
			}
			var before, after []byte
			stateSQL := `SELECT jsonb_build_object('goal',goal,'constraints',constraints_json,'status',status,'revision',revision,'working',working_json) FROM tasks WHERE task_id='protected-task'`
			if err = f.pool.QueryRow(f.ctx, stateSQL).Scan(&before); err != nil {
				t.Fatal(err)
			}
			q := f.policy.ModelPolicyRequest
			q.ID = "lifecycle-summary-" + attack.ID
			q.Job.TaskID = "protected-task"
			q.Job.Sources = nil
			p, err := (&Policies{Catalog: f.cat, Jobs: f.js}).Create(f.ctx, PolicyRequest{SessionKey: "session", Policy: q})
			if err != nil {
				t.Fatal(err)
			}
			f.policy = p
			f.poll(t)
			if did, err := f.worker.ProcessOne(f.ctx); err != nil || !did {
				t.Fatal("hostile DATA did not cross actual publication boundary", did, err)
			}
			snapshot, err := summary.NewStore(f.pool).GetSummarySnapshot(f.ctx, "session")
			if err != nil || !strings.Contains(snapshot.Summary, attack.Body) {
				t.Fatal("verbatim historical DATA missing", err)
			}
			var parsed Historical
			if json.Unmarshal([]byte(snapshot.Summary), &parsed) != nil || parsed.Warning == "" || len(parsed.Tools) != 1 || parsed.Tools[0].Status != "unknown" {
				t.Fatal("summary laundered authority/effect state")
			}
			if err = f.pool.QueryRow(f.ctx, stateSQL).Scan(&after); err != nil || string(before) != string(after) {
				t.Fatal("historical DATA changed current task authority", err)
			}
			var toolStatus string
			var claims int
			if err = f.pool.QueryRow(f.ctx, `SELECT (SELECT status FROM tool_calls WHERE tool_call_id='unknown-tool'),(SELECT count(*) FROM memory_claims)`).Scan(&toolStatus, &claims); err != nil || toolStatus != "unknown" || claims != 0 {
				t.Fatal("summary accepted a fact or manufactured successful effect", err)
			}
			var result string
			if err = f.pool.QueryRow(f.ctx, `SELECT result_source_id FROM memory_model_attempts WHERE state='published'`).Scan(&result); err != nil {
				t.Fatal(err)
			}
			var primaryRevision int64
			primary := "message:" + f.firstMessage(t)
			if err = f.pool.QueryRow(f.ctx, `SELECT head_revision FROM memory_sources WHERE source_id=$1`, primary).Scan(&primaryRevision); err != nil {
				t.Fatal(err)
			}
			if _, err = f.cat.Correct(f.ctx, catalog.Correction{SourceID: primary, ExpectedRevision: primaryRevision, Body: "Исправлено владельцем: старый враждебный текст не является полномочием"}); err != nil {
				t.Fatal(err)
			}
			for _, id := range []string{p.Job.Sources[0].ID, result} {
				if _, err = f.cat.Get(f.ctx, id); err == nil {
					t.Fatal("corrected snapshot or summary remained retrievable")
				}
			}
			if err = f.pool.QueryRow(f.ctx, `SELECT head_revision FROM memory_sources WHERE source_id=$1`, primary).Scan(&primaryRevision); err != nil {
				t.Fatal(err)
			}
			if _, err = f.cat.Delete(f.ctx, primary, "delete", primaryRevision); err != nil {
				t.Fatal(err)
			}
			reconstructed := catalog.New(f.pool, catalog.NewFileManifest(f.manifestPath))
			for _, id := range []string{p.Job.Sources[0].ID, result} {
				if _, err = reconstructed.Get(f.ctx, id); err == nil {
					t.Fatal("deleted snapshot returned after reader reconstruction")
				}
			}
			if _, err = f.cat.Get(f.ctx, unrelated.ID); err != nil {
				t.Fatal("unrelated source was deleted", err)
			}
			var retained int
			if err = f.pool.QueryRow(f.ctx, `SELECT COALESCE(sum(length(body)),0)::int FROM memory_source_revisions WHERE source_id=ANY($1)`, []string{p.Job.Sources[0].ID, result}).Scan(&retained); err != nil || retained != 0 {
				t.Fatal("deleted hostile text remained in snapshot generations", retained, err)
			}
		})
	}
}
