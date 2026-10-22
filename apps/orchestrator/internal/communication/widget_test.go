package communication_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/butler/butler/apps/orchestrator/internal/communication"
	"github.com/butler/butler/internal/communicationcontrol"
	"github.com/jackc/pgx/v5"
)

const multiWidget = `{"schema_version":1,"question":"Какую пиццу?","input":{"type":"multi_choice","options":[{"id":"a","label":"Маргарита"},{"id":"b","label":"Пепперони"}],"min_selections":1,"max_selections":2}}`

func widgetAction(f *fixture, channel, op string, version int64, ids ...string) communication.WidgetAction {
	return communication.WidgetAction{BindingID: f.bindings[channel], BindingVersion: 1, ExpectedVersion: 1, DraftVersion: version, Operation: op, Answer: communicationcontrol.Answer{OptionIDs: ids}}
}

func TestCommunicationWidgetDraftCASLostACKAndOneResumeIntegration(t *testing.T) {
	f := setup(t)
	pub, e := f.publish(t, "widget", "communication.widget", multiWidget)
	if e != nil {
		t.Fatal(e)
	}
	draft := widgetAction(f, "web", "draft", 1, "b", "a")
	view, e := f.s.WidgetAction(f.actor("web", "draft-1"), pub.InteractionRef, draft)
	if e != nil || view.DraftVersion != 2 || fmt.Sprint(view.Draft.OptionIDs) != "[a b]" {
		t.Fatal(view, e)
	}
	replay, e := f.s.WidgetAction(f.actor("web", "draft-1"), pub.InteractionRef, draft)
	if e != nil || replay.DraftVersion != 2 {
		t.Fatal("lost ACK mutated draft", replay, e)
	}
	draft.Answer.OptionIDs = []string{"a"}
	if _, e = f.s.WidgetAction(f.actor("web", "draft-1"), pub.InteractionRef, draft); !errors.Is(e, communication.ErrConflict) {
		t.Fatal("identity conflict", e)
	}
	if _, e = f.s.WidgetAction(f.actor("telegram", "stale"), pub.InteractionRef, widgetAction(f, "telegram", "draft", 1, "a")); !errors.Is(e, communication.ErrWidgetStale) {
		t.Fatal("stale draft", e)
	}
	var checkpoint string
	if e = f.pool.QueryRow(f.ctx, `SELECT checkpoint FROM accepted_inputs WHERE input_id=$1`, f.p.InputID).Scan(&checkpoint); e != nil || checkpoint != "awaiting_interaction" {
		t.Fatal("draft resumed worker", checkpoint, e)
	}
	submit := widgetAction(f, "telegram", "submit", 2)
	view, e = f.s.WidgetAction(f.actor("telegram", "submit"), pub.InteractionRef, submit)
	if e != nil || view.State != "resolved" {
		t.Fatal(view, e)
	}
	if _, e = f.s.WidgetAction(f.actor("telegram", "submit"), pub.InteractionRef, submit); e != nil {
		t.Fatal("submit ACK replay", e)
	}
	if _, e = f.s.WidgetAction(f.actor("web", "late"), pub.InteractionRef, widgetAction(f, "web", "submit", 2)); !errors.Is(e, communication.ErrResolved) {
		t.Fatal("second channel accepted", e)
	}
	if e = f.pool.QueryRow(f.ctx, `SELECT checkpoint FROM accepted_inputs WHERE input_id=$1`, f.p.InputID).Scan(&checkpoint); e != nil || checkpoint != "interaction_ready" {
		t.Fatal(checkpoint, e)
	}
	var responses, grants int
	if e = f.pool.QueryRow(f.ctx, `SELECT (SELECT count(*) FROM messages WHERE metadata->>'kind'='interaction_response'),(SELECT count(*) FROM action_authorizations)`).Scan(&responses, &grants); e != nil || responses != 1 || grants != 0 {
		t.Fatal("preference became grant/duplicate", responses, grants, e)
	}
}

func TestCommunicationWidgetConcurrentCrossChannelSubmitIntegration(t *testing.T) {
	f := setup(t)
	pub, e := f.publish(t, "widget", "communication.widget", widget)
	if e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for index, ch := range []string{"web", "telegram"} {
		wg.Add(1)
		go func(ch string, index int) {
			defer wg.Done()
			id := "a"
			if index == 1 {
				id = "b"
			}
			_, e := f.s.WidgetAction(f.actor(ch, "answer"), pub.InteractionRef, widgetAction(f, ch, "answer", 1, id))
			results <- e
		}(ch, index)
	}
	wg.Wait()
	close(results)
	success, resolved := 0, 0
	for e := range results {
		if e == nil {
			success++
		} else if errors.Is(e, communication.ErrResolved) {
			resolved++
		} else {
			t.Fatal(e)
		}
	}
	if success != 1 || resolved != 1 {
		t.Fatal(success, resolved)
	}
	if widgetCount(t, f, "communication_widget_actions") != 1 {
		t.Fatal("two accepted responses")
	}
}

func TestCommunicationWidgetDraftRollbackAndPolicyMatrixIntegration(t *testing.T) {
	for _, scenario := range []string{"rollback", "revoked", "wrong_binding", "expired", "false_wait", "invalid_draft", "missing_submit_min"} {
		t.Run(scenario, func(t *testing.T) {
			f := setup(t)
			raw := multiWidget
			if scenario == "false_wait" {
				raw = `{"schema_version":1,"question":"No","input":{"type":"text"},"wait_for_response":false}`
			}
			pub, e := f.publish(t, "widget", "communication.widget", raw)
			if scenario == "false_wait" {
				if e == nil || f.count(t, "communication_publications") != 0 || widgetCount(t, f, "communication_widget_views") != 0 {
					t.Fatal("nonblocking committed", e)
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			q := widgetAction(f, "web", "draft", 1, "a")
			switch scenario {
			case "rollback":
				tx, e := f.pool.Begin(f.ctx)
				if e != nil {
					t.Fatal(e)
				}
				if _, e = f.s.WidgetActionTx(f.actor("web", "rollback"), tx, pub.InteractionRef, q); e != nil {
					t.Fatal(e)
				}
				_ = tx.Rollback(context.Background())
				var version int64
				if e = f.pool.QueryRow(f.ctx, `SELECT draft_version FROM communication_widget_views WHERE interaction_id=$1`, pub.InteractionRef).Scan(&version); e != nil || version != 1 || widgetCount(t, f, "communication_widget_actions") != 0 {
					t.Fatal("partial commit", version, e)
				}
				return
			case "revoked":
				f.revoked.Store(true)
			case "wrong_binding":
				q.BindingVersion = 2
			case "expired":
				if _, e = f.pool.Exec(f.ctx, `UPDATE communication_interactions SET expires_at=clock_timestamp()-INTERVAL '1 second' WHERE interaction_id=$1`, pub.InteractionRef); e != nil {
					t.Fatal(e)
				}
			case "invalid_draft":
				q.Answer.OptionIDs = []string{"missing"}
			case "missing_submit_min":
				q.Operation = "submit"
				q.Answer = communicationcontrol.Answer{}
			}
			if _, e = f.s.WidgetAction(f.actor("web", scenario), pub.InteractionRef, q); e == nil {
				t.Fatal("invalid action accepted")
			}
			if widgetCount(t, f, "communication_widget_actions") != 0 {
				t.Fatal("denied action committed")
			}
		})
	}
}

func TestCommunicationWidgetTelegramACKAndCallbackReplayIntegration(t *testing.T) {
	f := setup(t)
	pub, e := f.publish(t, "widget", "communication.widget", multiWidget)
	if e != nil {
		t.Fatal(e)
	}
	// Native send ACK is the only reverse identity; callback text cannot choose it.
	_, e = f.pool.Exec(f.ctx, `UPDATE delivery_outbox SET status='sent',channel_ack='42' WHERE publication_id=$1 AND channel='telegram'`, pub.PublicationRef)
	if e != nil {
		t.Fatal(e)
	}
	var token string
	if e = f.pool.QueryRow(f.ctx, `SELECT callback_token FROM communication_widget_views WHERE interaction_id=$1`, pub.InteractionRef).Scan(&token); e != nil {
		t.Fatal(e)
	}
	binding := communication.Binding{ID: f.bindings["telegram"], Version: 1, Channel: "telegram"}
	callback := "cw:" + token + ":1:1:t0"
	view, e := f.s.TelegramWidgetAction(f.actor("telegram", "toggle"), binding, "42", callback, "")
	if e != nil || view.DraftVersion != 2 {
		t.Fatal(view, e)
	}
	if _, e = f.s.TelegramWidgetAction(f.actor("telegram", "toggle"), binding, "42", callback, ""); e != nil {
		t.Fatal("replayed toggle", e)
	}
	if _, e = f.s.TelegramWidgetAction(f.actor("telegram", "forged"), binding, "43", callback, ""); e == nil {
		t.Fatal("forged message ACK")
	}
	if _, e = f.s.TelegramWidgetAction(f.actor("telegram", "stale"), binding, "42", callback, ""); !errors.Is(e, communication.ErrWidgetStale) {
		t.Fatal(e)
	}
	if _, e = f.s.TelegramWidgetAction(f.actor("telegram", "submit"), binding, "42", "cw:"+token+":1:2:s", ""); e != nil {
		t.Fatal(e)
	}
}

func TestCommunicationWidgetDraftPrivacyErasureIntegration(t *testing.T) {
	f := setup(t)
	pub, e := f.publish(t, "widget", "communication.widget", multiWidget)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.WidgetAction(f.actor("web", "draft"), pub.InteractionRef, widgetAction(f, "web", "draft", 1, "a")); e != nil {
		t.Fatal(e)
	}
	if _, e = f.pool.Exec(f.ctx, `UPDATE communication_interactions SET privacy_redacted=true WHERE interaction_id=$1`, pub.InteractionRef); e != nil {
		t.Fatal(e)
	}
	var raw []byte
	if e = f.pool.QueryRow(f.ctx, `SELECT draft_json FROM communication_widget_views WHERE interaction_id=$1`, pub.InteractionRef).Scan(&raw); e != nil {
		t.Fatal(e)
	}
	var v map[string]any
	_ = json.Unmarshal(raw, &v)
	if len(v) != 0 || widgetCount(t, f, "communication_widget_actions") != 0 {
		t.Fatal("derivative retained private answer")
	}
	f.s.SourceGuard = func(context.Context, pgx.Tx, string) error { return nil }
	if _, e = f.s.WidgetAction(f.actor("web", "stale"), pub.InteractionRef, widgetAction(f, "web", "submit", 2)); e == nil {
		t.Fatal("redacted question accepted")
	}
}

func widgetCount(t *testing.T, f *fixture, table string) int {
	t.Helper()
	var n int
	if e := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM "+pgx.Identifier{table}.Sanitize()).Scan(&n); e != nil {
		t.Fatal(e)
	}
	return n
}

func TestCommunicationWidgetRetainPreparationDoesNotConsumeReadyIntegration(t *testing.T) {
	f := setup(t)
	pub, e := f.publish(t, "widget", "communication.widget", widget)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.WidgetAction(f.actor("web", "answer"), pub.InteractionRef, widgetAction(f, "web", "answer", 1, "b")); e != nil {
		t.Fatal(e)
	}
	if e = f.s.Inputs.SaveInteractionPreparation(f.ctx, f.p.RunID, "interaction:wrong", `{"response":"wrong"}`); e == nil {
		t.Fatal("wrong interaction changed context")
	}
	for n := 0; n < 2; n++ {
		if e = f.s.Inputs.SaveInteractionPreparation(f.ctx, f.p.RunID, pub.InteractionRef, `{"input_items":[{"role":"user","content":"saved preference DATA"}]}`); e != nil {
			t.Fatal(e)
		}
	}
	stage, raw, e := f.s.Inputs.LoadCheckpoint(f.ctx, f.p.RunID)
	if e != nil || stage != "interaction_ready" || !strings.Contains(raw, "saved preference DATA") || !strings.Contains(raw, pub.InteractionRef) {
		t.Fatal("ready marker lost", stage, raw, e)
	}
}
