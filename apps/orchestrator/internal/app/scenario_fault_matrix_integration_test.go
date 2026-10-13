package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/completion"
	"github.com/butler/butler/apps/orchestrator/internal/deliveryoutbox"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/generatedtools"
	"github.com/butler/butler/internal/owneridentity"
)

type converterWorkerFaultHooks struct {
	factory       func(*testing.T, generatedtools.Profile, string) *developerFixture
	before        func(*testing.T, *developerFixture)
	after         func(*testing.T, *developerFixture, *converterWorkerProvider)
	providerSetup func(*testing.T, *developerFixture, *converterWorkerProvider)
	terminal      func(*testing.T, *developerFixture, *converterWorkerProvider, func())
}

// This is the actual converter workflow's result/delivery tail, not the complete
// mandate/initiative/self-repair acceptance chain. Three ordinary parent attempts
// produce three immutable notices. The local HTTP channel below is an external
// effect simulator, not Telegram, an owner account or a recipient idempotency API.
func TestScenarioFaultConverterResultDeliveryContainerE2E(t *testing.T) {
	for _, fault := range []string{"restart_before_send", "explicit_absence", "ack_commit_failure"} {
		t.Run(fault, func(t *testing.T) {
			hooks := &converterWorkerFaultHooks{
				before: func(t *testing.T, f *developerFixture) {
					// Only the synthetic session's delivery class is configured. The
					// simulator validates its actual stored session, not a copied key.
					if _, err := f.f.pool.Exec(f.f.ctx, `UPDATE sessions SET channel='telegram' WHERE session_key=$1`, f.f.parent.SessionKey); err != nil {
						t.Fatal("fixture channel configuration failed")
					}
				},
				after: func(t *testing.T, f *developerFixture, provider *converterWorkerProvider) {
					checkConverterResultDeliveryFault(t, f, provider, fault)
				},
			}
			testConverterDeveloperWorkersObserved(t, true, hooks)
		})
	}
}

func checkConverterResultDeliveryFault(t *testing.T, f *developerFixture, provider *converterWorkerProvider, fault string) {
	t.Helper()
	// The execution worker is already drained. This independent deadline also
	// permits natural expiry of the production worker's actual 30-second claim.
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	provider.mu.Lock()
	output := *provider.output
	childStarts := provider.childStarts
	provider.mu.Unlock()
	actor := owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: f.f.parent.OwnerID, Channel: "web", DeliveryID: "independent-converter-verification"})
	original, err := f.f.sources.Get(actor, f.document.ID)
	if err != nil || original.Body != "name,count\nprivate-original-document,2\n" || original.Hash != f.document.Hash {
		t.Fatal("original immutable input witness changed")
	}
	converted, err := f.f.sources.Get(actor, output.Source.ID)
	if err != nil || converted.Body != `[{"label":"private-original-document","quantity":2}]` || generatedtools.HashBytes([]byte(converted.Body)) != output.SHA256 {
		t.Fatal("independent exact original conversion witness failed")
	}
	var runID string
	if err = f.f.pool.QueryRow(ctx, `SELECT run_id FROM tool_calls WHERE tool_name='converter.developer-inventory.convert'`).Scan(&runID); err != nil {
		t.Fatal("actual original conversion attempt missing")
	}
	results := completion.New(f.f.pool)
	final, err := results.GetByRun(ctx, runID)
	if err != nil || final.Verdict != "unverified" || final.PrivacyRedacted || len(final.Effects) != 1 || final.Effects[0].EffectState != "effect_confirmed" {
		t.Fatal("actual immutable final/effect witness missing")
	}
	current, err := f.f.store.Get(ctx, f.f.parent.TaskID)
	if err != nil || current.Status != "active" {
		t.Fatal("technical converter work completed semantic task")
	}
	if _, err = f.f.store.Transition(actor, current.TaskID, current.Revision, "completed", final.ResultID); err == nil {
		t.Fatal("model final substituted for owner verification")
	}
	proof := task.Result{TaskID: current.TaskID, TaskRevision: current.Revision, RunID: runID,
		Summary:          "Independently compared exact original input and actual converted output",
		EvidenceRefs:     []string{"source:" + f.document.ID, "source:" + output.Source.ID, "sha256:" + output.SHA256},
		VerifiedCriteria: append([]string(nil), current.CompletionCriteria...), Verifier: "owner"}
	foreign := owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: "foreign-fixture-owner", Channel: "web", DeliveryID: "foreign-verification"})
	if _, err = f.f.store.SaveVerifiedResult(foreign, proof); err == nil {
		t.Fatal("foreign owner verified original task")
	}
	stale := proof
	stale.TaskRevision--
	if _, err = f.f.store.SaveVerifiedResult(actor, stale); err == nil {
		t.Fatal("stale original criteria verified task")
	}
	unchecked := proof
	unchecked.VerifiedCriteria = nil
	if _, err = f.f.store.SaveVerifiedResult(actor, unchecked); err == nil {
		t.Fatal("unchecked original criteria verified task")
	}
	verified, err := f.f.store.SaveVerifiedResult(actor, proof)
	if err != nil || verified.Verdict != "verified" {
		t.Fatal("current explicit owner proof was not saved")
	}
	completed, err := f.f.store.Transition(actor, current.TaskID, current.Revision, "completed", verified.ResultID)
	if err != nil || completed.CompletionResultID != verified.ResultID {
		t.Fatal("owner result did not complete exact task")
	}
	// A fresh repository sees the same durable owner decision; neither a new
	// worker nor delivery can alter its criteria or convert child finals to proof.
	completed, err = task.NewStore(f.f.pool).Get(ctx, current.TaskID)
	if err != nil || completed.Status != "completed" || completed.Revision != current.Revision+1 {
		t.Fatal("owner task completion did not survive reconstruction")
	}
	store := deliveryoutbox.NewStore(f.f.pool)
	rows, err := store.ListByRun(ctx, runID)
	if err != nil || len(rows) != 1 || rows[0].Status != "pending" || rows[0].AttemptCount != 0 {
		t.Fatal("original conversion final's frozen notice missing")
	}
	latestID, latestContent := rows[0].NotificationID, rows[0].Content
	var parentNoticeCount, modelAttempts int
	if err = f.f.pool.QueryRow(ctx, `SELECT count(*) FROM delivery_outbox WHERE channel='telegram'`).Scan(&parentNoticeCount); err != nil || parentNoticeCount != 3 {
		t.Fatal("three actual parent attempts lost their separate notices")
	}
	if err = f.f.pool.QueryRow(ctx, `SELECT count(*) FROM execution_model_attempts`).Scan(&modelAttempts); err != nil || modelAttempts != 14 {
		t.Fatal("actual finite workflow inference admissions changed")
	}
	before := converterTailHistory(t, ctx, f)
	expectedNotices := map[string]string{}
	noticeRows, err := f.f.pool.Query(ctx, `SELECT notification_id,content FROM delivery_outbox WHERE channel='telegram'`)
	if err != nil {
		t.Fatal("stored channel witness unavailable")
	}
	for noticeRows.Next() {
		var id, content string
		if noticeRows.Scan(&id, &content) != nil {
			noticeRows.Close()
			t.Fatal("stored channel witness malformed")
		}
		expectedNotices[id] = content
	}
	err = noticeRows.Err()
	noticeRows.Close()
	if err != nil || len(expectedNotices) != 3 {
		t.Fatal("exact parent notice witnesses unavailable")
	}
	var requests, accepted atomic.Int32
	bot := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/stored-notice" {
			http.Error(w, "fixture route denied", http.StatusBadRequest)
			return
		}
		var request converterNoticeRequest
		decoder := json.NewDecoder(io.LimitReader(r.Body, 8192))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF || request.Session != current.SessionKey || request.ID == "" || request.Content == "" || expectedNotices[request.ID] != request.Content {
			http.Error(w, "fixture stored notice denied", http.StatusBadRequest)
			return
		}
		n := requests.Add(1)
		if fault == "explicit_absence" && n == 1 {
			// Explicit absent response permits only a retry of this frozen row.
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		accepted.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(converterNoticeReceipt{ID: request.ID, ACK: fmt.Sprintf("fixture-%d", n)})
	}))
	defer bot.Close()
	sender := converterNoticeSender{url: bot.URL + "/stored-notice", client: bot.Client()}
	worker := deliveryoutbox.Worker{Store: store, Sender: sender}
	if fault == "ack_commit_failure" {
		// Deliver earlier attempt notices normally, then fail persistence AFTER
		// the recipient accepted the exact final conversion notice.
		for range 2 {
			if processed, err := worker.ProcessOne(ctx); err != nil || !processed {
				t.Fatal("earlier immutable attempt notice did not deliver")
			}
		}
		if _, err = f.f.pool.Exec(ctx, `CREATE FUNCTION fixture_notice_ack_fault() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.status='sent' THEN RAISE EXCEPTION 'controlled acknowledgement commit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER fixture_notice_ack_fault BEFORE UPDATE ON delivery_outbox FOR EACH ROW EXECUTE FUNCTION fixture_notice_ack_fault()`); err != nil {
			t.Fatal("ACK commit fault not installed")
		}
		if processed, err := worker.ProcessOne(ctx); !processed || err == nil || requests.Load() != 3 || accepted.Load() != 3 {
			t.Fatal("effect-before-ACK fault did not occur at actual boundary")
		}
		if _, err = f.f.pool.Exec(ctx, `DROP TRIGGER fixture_notice_ack_fault ON delivery_outbox; DROP FUNCTION fixture_notice_ack_fault()`); err != nil {
			t.Fatal("ACK fault cleanup failed")
		}
		worker = deliveryoutbox.Worker{Store: deliveryoutbox.NewStore(f.f.pool), Sender: sender}
		if processed, err := worker.ProcessOne(ctx); err != nil || processed || requests.Load() != 3 {
			t.Fatal("restart replayed live sending claim")
		}
		// Wait for the actual production 30-second claim, never rewrite clock
		// functions or claim_until. Recovery must quarantine, not resend.
		waitConverterWorkerFor(t, 35*time.Second, func() bool {
			var expired bool
			if f.f.pool.QueryRow(ctx, `SELECT claim_until<=clock_timestamp() FROM delivery_outbox WHERE notification_id=$1`, latestID).Scan(&expired) != nil {
				t.Fatal("natural ACK claim expiry observation failed")
			}
			return expired
		}, "natural original delivery claim expiry")
	} else {
		if fault == "explicit_absence" {
			if processed, err := worker.ProcessOne(ctx); err != nil || !processed || accepted.Load() != 0 {
				t.Fatal("explicit absence was not stored")
			}
			// Natural retry delay, with no forced PG time/next_attempt rewrite.
			waitConverterWorker(t, func() bool {
				var due bool
				if f.f.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM delivery_outbox WHERE status='pending' AND attempt_count=1 AND next_attempt_at<=clock_timestamp())`).Scan(&due) != nil {
					t.Fatal("absent retry delay observation failed")
				}
				return due
			}, "bounded explicit absence retry")
		}
		// Reconstruction has only Store and channel capability, not executor.
		worker = deliveryoutbox.Worker{Store: deliveryoutbox.NewStore(f.f.pool), Sender: sender}
		for range 3 {
			if processed, err := worker.ProcessOne(ctx); err != nil || !processed {
				t.Fatal("reconstructed stored notice delivery failed")
			}
		}
	}
	for range 2 {
		if processed, err := worker.ProcessOne(ctx); err != nil || processed {
			t.Fatal("delivery repeated a terminal/uncertain notice")
		}
	}
	rows, err = deliveryoutbox.NewStore(f.f.pool).ListByRun(ctx, runID)
	expected := "sent"
	if fault == "ack_commit_failure" {
		expected = "delivery_unknown"
	}
	if err != nil || len(rows) != 1 || rows[0].Status != expected || rows[0].NotificationID != latestID || rows[0].Content != latestContent || rows[0].AttemptCount != 1 {
		t.Fatal("latest exact immutable notice identity/outcome changed")
	}
	if fault == "ack_commit_failure" && (rows[0].ErrorCode != "sending_claim_expired" || rows[0].ChannelACK != "") {
		t.Fatal("uncertain recipient acceptance invented an ACK")
	}
	expectedRequests := int32(3)
	if fault == "explicit_absence" {
		expectedRequests = 4
	}
	if requests.Load() != expectedRequests || accepted.Load() != 3 {
		t.Fatal("recipient requests/acceptances were repeated or lost")
	}
	var earlierACKs int
	if err = f.f.pool.QueryRow(ctx, `SELECT count(*) FROM delivery_outbox WHERE channel='telegram' AND notification_id<>$1 AND status='sent' AND channel_ack<>''`, latestID).Scan(&earlierACKs); err != nil || earlierACKs != 2 {
		t.Fatal("later uncertain delivery erased earlier channel ACKs")
	}
	if after := converterTailHistory(t, ctx, f); before != after {
		t.Fatal("channel fault changed work/effect/result/source/budget history")
	}
	kept, err := completion.New(f.f.pool).GetByRun(ctx, runID)
	if err != nil || kept.ContentHash != final.ContentHash || kept.Verdict != "unverified" {
		t.Fatal("owner verification/delivery rewrote model final")
	}
	provider.mu.Lock()
	startsAfter := provider.childStarts
	provider.mu.Unlock()
	if startsAfter != childStarts || childStarts != 1 {
		t.Fatal("delivery restarted the developer child")
	}
	t.Logf("fault=%s owner_verified_task=true model_final_unverified=true stored_attempt_notices=3 recipient_requests=%d recipient_acceptances=3 latest_delivery=%s work_history_unchanged=true actual_native_calls=0", fault, requests.Load(), expected)
}

// A content-free frozen projection covers actual work, not just call counts.
func converterTailHistory(t *testing.T, ctx context.Context, f *developerFixture) string {
	t.Helper()
	var raw string
	err := f.f.pool.QueryRow(ctx, `SELECT jsonb_build_object(
 'effects',(SELECT jsonb_agg(jsonb_build_object('id',action_id,'state',state,'fingerprint',fingerprint) ORDER BY action_id) FROM action_effects),
 'model_attempts',(SELECT count(*) FROM execution_model_attempts),
 'budgets',(SELECT jsonb_agg(jsonb_build_object('id',scope_id,'limits',limits_json,'used',used_json,'deadline',deadline) ORDER BY scope_id) FROM execution_budget_scopes),
 'reservations',(SELECT count(*) FROM execution_budget_reservations),
 'finals',(SELECT jsonb_agg(jsonb_build_object('id',result_id,'hash',content_hash,'verdict',verdict) ORDER BY result_id) FROM run_final_results),
 'sources',(SELECT jsonb_agg(jsonb_build_object('id',source_id,'revision',revision,'hash',content_hash) ORDER BY source_id,revision) FROM memory_source_revisions),
 'tasks',(SELECT jsonb_agg(jsonb_build_object('id',task_id,'state',status,'revision',revision,'constraints',constraints_revision,'result',completion_result_id) ORDER BY task_id) FROM tasks),
 'returns',(SELECT count(*) FROM converter_developer_returns),
 'continuations',(SELECT jsonb_agg(jsonb_build_object('id',continuation_id,'state',state,'hash',ready_hash,'input',input_id) ORDER BY continuation_id) FROM converter_parent_continuations)
 )::text`).Scan(&raw)
	if err != nil {
		t.Fatal("content-free actual workflow history unavailable")
	}
	hash := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(hash[:])
}

type converterNoticeRequest struct {
	ID      string `json:"notification_id"`
	Session string `json:"session_key"`
	Content string `json:"content"`
}
type converterNoticeReceipt struct {
	ID  string `json:"notification_id"`
	ACK string `json:"ack"`
}
type converterNoticeSender struct {
	url    string
	client *http.Client
}

func (s converterNoticeSender) SendStored(ctx context.Context, m deliveryoutbox.Message) deliveryoutbox.Outcome {
	unknown := deliveryoutbox.Outcome{Status: "delivery_unknown", ErrorCode: "transport_outcome_unknown"}
	raw, err := json.Marshal(converterNoticeRequest{m.NotificationID, m.SessionKey, m.Content})
	if err != nil || m.Channel != "telegram" {
		return unknown
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, strings.NewReader(string(raw)))
	if err != nil {
		return unknown
	}
	req.Header.Set("Content-Type", "application/json")
	client := *s.client
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		return unknown
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		return deliveryoutbox.Outcome{Status: "absent", ErrorCode: "telegram_rejected", Retryable: true}
	}
	var receipt converterNoticeReceipt
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 4096))
	decoder.DisallowUnknownFields()
	if resp.StatusCode != http.StatusOK || decoder.Decode(&receipt) != nil || decoder.Decode(new(any)) != io.EOF || receipt.ID != m.NotificationID || receipt.ACK == "" {
		return unknown
	}
	return deliveryoutbox.Outcome{Status: "sent", ACK: receipt.ACK}
}
