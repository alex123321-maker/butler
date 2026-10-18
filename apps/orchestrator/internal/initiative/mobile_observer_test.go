package initiative

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/mobile"
)

func mobileFixture(t *testing.T) (*fixture, *mobile.Store, string, string) {
	f := setup(t)
	manifest := catalog.NewFileManifest(filepath.Join(t.TempDir(), "anchor"))
	if err := manifest.Initialize(); err != nil {
		t.Fatal(err)
	}
	sources := catalog.New(f.pool, manifest)
	f.store.MobileCatalog = sources
	f.source.ObserverKind = "mobile_inbox"
	f.source.ProjectRevision = strings.Repeat("a", 64)
	f.source.FailureThreshold = 1
	f.source.ActiveStart = 0
	f.source.ActiveEnd = 0
	receipt, err := f.store.SaveSource(f.actor("mobile-source"), f.source, 1)
	if err != nil {
		t.Fatal(err)
	}
	f.source.Revision = receipt.Revision
	store := &mobile.Store{Pool: f.pool, Catalog: sources}
	code, err := store.CreateCode(f.actor("pair-code"))
	if err != nil {
		t.Fatal(err)
	}
	pair, err := store.Exchange(f.ctx, mobile.PairRequest{Code: code["pairing_code"].(string), Device: "mobile_observer_device", Name: "synthetic", SDK: 36, Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return f, store, pair.Device, pair.Token
}
func sendMobile(t *testing.T, f *fixture, s *mobile.Store, device, token string, n int) mobile.Batch {
	text := fmt.Sprintf("Synthetic appointment %d", n)
	now := time.Now().UnixMilli()
	b := mobile.Batch{Schema: 1, ID: fmt.Sprintf("attention_batch_%08d", n), Sent: now, Events: []mobile.Event{{Schema: 1, ID: fmt.Sprintf("attention_event_%08d", n), Key: fmt.Sprintf("card-%d", n), Package: "example.synthetic", Change: "POSTED", Posted: now, Observed: now, ContentHash: strings.Repeat("a", 64), Visibility: "PRIVATE", Payload: &mobile.Payload{Text: &text}}}}
	ack, err := s.Accept(f.ctx, device, token, b)
	if err != nil || len(ack.Accepted) != 1 {
		t.Fatal(err, ack)
	}
	return b
}
func TestMobileTwentyMessagesAdmissionAndReplayIntegration(t *testing.T) {
	f, s, device, token := mobileFixture(t)
	for i := 0; i < 19; i++ {
		sendMobile(t, f, s, device, token, i)
	}
	if e, err := f.store.checkMobile(f.ctx, f.source); err != nil || e != nil {
		t.Fatal("19 messages woke agent", err)
	}
	b := sendMobile(t, f, s, device, token, 19)
	if a, err := s.Accept(f.ctx, device, token, b); err != nil || len(a.Duplicate) != 1 {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	events := make(chan *Event, 2)
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); e, err := f.store.checkMobile(f.ctx, f.source); events <- e; errs <- err }()
	}
	wg.Wait()
	close(events)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var event *Event
	for e := range events {
		if e == nil {
			t.Fatal("20 did not wake")
		}
		if event != nil && event.ConditionDigest != e.ConditionDigest {
			t.Fatal("concurrent duplicate batch")
		}
		event = e
	}
	event.ID = "mobile-test-event"
	event.SourceID = f.source.ID
	c, _, err := f.store.Observe(f.ctx, f.source.OwnerID, *event, f.source.Revision, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	c, err = f.promoter.Promote(f.ctx, c.ID, f.source.OwnerID)
	if err != nil || c.TaskID == "" {
		t.Fatal("normal task admission failed", err, c.Status, c.Reason)
	}
	var tasks, inputs, attention int
	var batch string
	if err = f.pool.QueryRow(f.ctx, `SELECT (SELECT count(*) FROM tasks),(SELECT count(*) FROM accepted_inputs),(SELECT COALESCE(sum(admitted_tasks),0) FROM initiative_attention_days),source_json->>'mobile_batch_id' FROM tasks WHERE task_id=$1`, c.TaskID).Scan(&tasks, &inputs, &attention, &batch); err != nil || tasks != 1 || inputs != 1 || attention != 0 || batch != event.ConditionDigest {
		t.Fatal(tasks, inputs, attention, batch, err)
	}
}
func TestMobileHourlyWakeAndDisabledAccessIntegration(t *testing.T) {
	f, s, device, token := mobileFixture(t)
	sendMobile(t, f, s, device, token, 1)
	if _, err := f.pool.Exec(f.ctx, `UPDATE mobile_messages SET received_at=clock_timestamp()-interval '59 minutes'`); err != nil {
		t.Fatal(err)
	}
	if e, err := f.store.checkMobile(f.ctx, f.source); err != nil || e != nil {
		t.Fatal("early hourly wake", err)
	}
	if _, err := f.pool.Exec(f.ctx, `UPDATE mobile_messages SET received_at=clock_timestamp()-interval '61 minutes'`); err != nil {
		t.Fatal(err)
	}
	if e, err := f.store.checkMobile(f.ctx, f.source); err != nil || e == nil {
		t.Fatal("hourly wake absent", err)
	}
	f.source.Enabled = false
	if _, err := f.store.SaveSource(f.actor("disable-mobile"), f.source, f.source.Revision); err != nil {
		t.Fatal(err)
	}
	var active int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM memory_sources WHERE status='active'`).Scan(&active); err != nil || active != 0 {
		t.Fatal("disabled access remains readable", active, err)
	}
}

func TestMobileProcessingVerifierRequiresFinishedBatchAndCompletedInputIntegration(t *testing.T) {
	f, s, device, token := mobileFixture(t)
	for i := 0; i < 20; i++ {
		sendMobile(t, f, s, device, token, i)
	}
	event, err := f.store.checkMobile(f.ctx, f.source)
	if err != nil || event == nil {
		t.Fatal(err)
	}
	event.ID = "verify-mobile"
	event.SourceID = f.source.ID
	candidate, _, err := f.store.Observe(f.ctx, f.source.OwnerID, *event, f.source.Revision, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	c, err := f.promoter.Promote(f.ctx, candidate.ID, f.source.OwnerID)
	if err != nil {
		t.Fatal(err)
	}
	tasks := task.NewStore(f.pool)
	tasks.SetVerifiedResultSourceGuard(s.Catalog.CheckRunSourcesTx)
	// Controlled durable final fixture: this test independently exercises the
	// verifier's evidence boundaries, not model quality or live execution.
	statements := []string{
		`INSERT INTO runs(run_id,session_key,input_event_id,status,current_state,autonomy_mode,model_provider,started_at,updated_at,task_id,task_revision) VALUES('mobile-final',$1,'fixture','completed','completed','mode_2','fixture',clock_timestamp(),clock_timestamp(),$2,1)`,
		`INSERT INTO messages(message_id,session_key,run_id,role,content) VALUES('mobile-final-message',$1,'mobile-final','assistant','fixture final')`,
		`INSERT INTO artifacts(artifact_id,run_id,session_key,artifact_type) VALUES('mobile-final-artifact','mobile-final',$1,'assistant_final')`,
		`INSERT INTO run_final_results(result_id,run_id,session_key,task_id,task_revision,constraints_revision,artifact_id,message_id,content,content_hash,answer_state) VALUES('mobile-final-result','mobile-final',$1,$2,1,1,'mobile-final-artifact','mobile-final-message','fixture final',repeat('a',64),'final')`,
		`UPDATE tasks SET last_run_id='mobile-final' WHERE task_id=$2`,
	}
	for _, sql := range statements {
		args := []any{f.source.SessionKey}
		if strings.HasPrefix(sql, "UPDATE tasks") {
			sql = strings.ReplaceAll(sql, "$2", "$1")
			args = []any{c.TaskID}
		} else if strings.Contains(sql, "$2") {
			args = append(args, c.TaskID)
		}
		if _, err = f.pool.Exec(f.ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	assertStatus := func(want string) {
		t.Helper()
		record, err := tasks.Get(f.ctx, c.TaskID)
		if err != nil || record.Status != want {
			t.Fatal("wrong completion", record.Status, err)
		}
	}
	if _, err = f.pool.Exec(f.ctx, `UPDATE mobile_attention_batches SET finished_at=clock_timestamp()`); err != nil {
		t.Fatal(err)
	}
	if err = tasks.ReconcileMobileResults(f.ctx); err != nil {
		t.Fatal(err)
	}
	assertStatus("active")
	if _, err = f.pool.Exec(f.ctx, `UPDATE accepted_inputs SET status='completed',run_id='mobile-final',checkpoint='completed' WHERE input_id=$1`, c.InputID); err != nil {
		t.Fatal(err)
	}
	if err = tasks.ReconcileMobileResults(f.ctx); err != nil {
		t.Fatal(err)
	}
	assertStatus("active")
	if _, err = f.pool.Exec(f.ctx, `UPDATE runs SET finished_at=clock_timestamp()-interval '61 seconds' WHERE run_id='mobile-final'; UPDATE mobile_attention_batches SET finished_at=NULL`); err != nil {
		t.Fatal(err)
	}
	if err = f.promoter.RecoverMobileInputs(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err = f.promoter.RecoverMobileInputs(f.ctx); err != nil {
		t.Fatal(err)
	}
	var receipts int
	if err = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM accepted_inputs WHERE task_id=$1`, c.TaskID).Scan(&receipts); err != nil || receipts != 2 {
		t.Fatal("incomplete batch did not resume exactly once", receipts, err)
	}
	if _, err = f.pool.Exec(f.ctx, `UPDATE accepted_inputs SET status='failed' WHERE task_id=$1 AND input_id<>$2`, c.TaskID, c.InputID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(f.ctx, `UPDATE mobile_messages SET state='ignored'; UPDATE mobile_attention_batches SET finished_at=clock_timestamp()`); err != nil {
		t.Fatal(err)
	}
	if err = tasks.ReconcileMobileResults(f.ctx); err != nil {
		t.Fatal(err)
	}
	assertStatus("completed")
	if err = tasks.ReconcileMobileResults(f.ctx); err != nil {
		t.Fatal(err)
	}
	var notices int
	if err = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM task_verified_notifications`).Scan(&notices); err != nil || notices != 0 {
		t.Fatal("noise-only completion interrupted owner", notices, err)
	}
	if err = tasks.ReconcileMobileResults(f.actor("cannot-mint-verification")); err == nil {
		t.Fatal("owner actor accepted as code verifier")
	}
}
