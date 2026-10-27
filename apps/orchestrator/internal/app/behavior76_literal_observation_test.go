package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/prompt"
)

func TestBehavior76LiteralBusinessDeadlineRecipientIntegration(t *testing.T) {
	t.Setenv("BUTLER_BEHAVIOR76_NATIVE_ENABLE", "disabled")
	t.Setenv(prompt.SettingKeyBasePrompt, prompt.DefaultBasePrompt)
	t.Setenv(prompt.SettingKeyBasePromptEnabled, "true")
	for _, late := range []bool{false, true} {
		name := "on_time"
		if late {
			name = "late_confirmed_is_not_on_time"
		}
		t.Run(name, func(t *testing.T) {
			f := newBehavior76LiteralFixture(t, context.Background(), nil)
			value := behavior76PublicLiteralInterface()[0]
			value.Episode.Initial.Tasks[0].ExecutionPolicy = "normal"
			value.Episode.Initial.Resources[0].Body = "original"
			value.Deadlines = []behavior76LiteralDeadline{{ResourceKey: "object", DeadlineAt: time.Now().UTC().Add(48 * time.Hour)}}
			date := time.Now().UTC().Add(24 * time.Hour)
			if late {
				date = time.Now().UTC().Add(-24 * time.Hour)
			}
			value.DeadlineChanges = []behavior76LiteralDeadlineChange{{BeforeStepID: "turn-1", ResourceKey: "object", DeadlineAt: date, Provenance: "synthetic_service_configuration"}}
			f.seedLiteral(value)
			receipts, err := f.applyLiteralServiceChanges(value, "turn-1")
			if err != nil || len(receipts) != 1 {
				t.Fatal("actual service date amendment unavailable", err)
			}
			provider := &behavior76Scripted{initiativeProjectScriptedProvider: &initiativeProjectScriptedProvider{}, commit: true, resourceKey: "object"}
			outcome, err := f.evaluateLiteral(provider, value.Episode.Steps[0], value.OwnerTaskSelections[0])
			if err != nil || outcome.State != "completed" || outcome.KernelResponse != nil || outcome.OwnerRunID == "" {
				t.Fatal("public authorized synthetic commit unavailable", err)
			}
			var raw []byte
			if err = f.pool.QueryRow(f.ctx, `SELECT observation FROM behavior76_business_observations WHERE resource_key='object' AND kind='commit'`).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			var observation behavior76LiteralDeadlineObservation
			if json.Unmarshal(raw, &observation) != nil || observation.PublicationLate == nil || *observation.PublicationLate != late || observation.DeadlinePassed != late || observation.SourceRevision != 2 || observation.SourceRef == "" || observation.ObservedAt.IsZero() || observation.ConfiguredAt.After(observation.ObservedAt) {
				t.Fatal("actual clock/source/late observation was lost")
			}
			object, err := behavior76ObjectQuery(f.ctx, f.pool, "object", "")
			if err != nil || object.CommitCount != 1 || object.CommittedBody == nil || *object.CommittedBody != "public committed bytes" {
				t.Fatal("late observation concealed actual confirmed synthetic write", err)
			}
			var receipt string
			if err = f.pool.QueryRow(f.ctx, `SELECT r.receipt::text FROM behavior76_recipient_receipts r JOIN behavior76_business_observations o USING(action_id) WHERE o.kind='commit'`).Scan(&receipt); err != nil || !strings.Contains(receipt, `confirmed`) {
				t.Fatal("actual confirmed receipt missing", err)
			}
			snapshot := f.snapshot()
			if snapshot.LiteralService == nil || len(snapshot.LiteralService.Deadlines) != 1 || len(snapshot.LiteralService.DeadlineEvents) != 2 || len(snapshot.LiteralService.Observations) != 1 {
				t.Fatal("literal source/effect snapshot incomplete")
			}
		})
	}
}

func TestBehavior76LiteralManualChangeContextIntegration(t *testing.T) {
	t.Setenv("BUTLER_BEHAVIOR76_NATIVE_ENABLE", "disabled")
	t.Setenv(prompt.SettingKeyBasePrompt, prompt.DefaultBasePrompt)
	t.Setenv(prompt.SettingKeyBasePromptEnabled, "true")
	f := newBehavior76LiteralFixture(t, context.Background(), nil)
	value := behavior76PublicLiteralInterface()[0]
	f.seedLiteral(value)
	for _, body := range []string{"public manual change one", "public manual change two"} {
		if _, err := f.apply(behavior76Step{Event: behavior76Event{Type: "manual_resource_change", ResourceKey: "object", NewBody: body, Provenance: "independently_observed"}}); err != nil {
			t.Fatal(err)
		}
	}
	provider := &behavior76SetupProvider{initiativeProjectScriptedProvider: &initiativeProjectScriptedProvider{}, fixture: f}
	outcome, err := f.evaluateLiteral(provider, value.Episode.Steps[0], value.OwnerTaskSelections[0])
	if err != nil || outcome.State != "completed" || len(provider.requests) != 1 {
		t.Fatal("public actual context preparation unavailable", err)
	}
	r, err := f.tasks.Get(f.ctx, f.lookup("task", "work"))
	if err != nil {
		t.Fatal(err)
	}
	var events []struct {
		Kind      string           `json:"kind"`
		SourceRef string           `json:"source_ref"`
		Before    behavior76Object `json:"before_state"`
		After     behavior76Object `json:"after_state"`
	}
	if json.Unmarshal([]byte(behavior76JSON(r.Working.Scratch["synthetic_fixture_manual_changes"])), &events) != nil || len(events) != 2 || events[0].After.SHA256 != events[1].Before.SHA256 || events[0].Before.Version != 1 || events[1].After.Version != 3 || events[1].After.Body != "public manual change two" || events[0].Kind != "observed_manual_change_data" || events[0].SourceRef == events[1].SourceRef {
		t.Fatal("manual context lacks actual event chronology")
	}
	// One actual owner exchange only: manual receipts never fabricate feedback
	// messages, accepted Profile changes or extra owner/model opportunities.
	var ownerMessages int
	if err = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM messages WHERE session_key=$1 AND role='user'`, r.SessionKey).Scan(&ownerMessages); err != nil || ownerMessages != 1 {
		t.Fatal("manual changes invented owner history", err)
	}
}

func TestBehavior76LiteralBusinessDatesBoundToActualWindow(t *testing.T) {
	start := time.Date(2026, 10, 27, 1, 0, 0, 0, time.UTC)
	end := start.Add(4 * time.Hour)
	values := behavior76PublicLiteralInterface()
	if err := behavior76LiteralBusinessDatesOutsideWindow(values, start, end); err != nil {
		t.Fatal(err)
	}
	for i := range values {
		if len(values[i].Deadlines) > 0 {
			values[i].Deadlines[0].DeadlineAt = start.Add(time.Hour)
			if behavior76LiteralBusinessDatesOutsideWindow(values, start, end) == nil {
				t.Fatal("crossing business date qualified as unambiguous on-time measurement")
			}
			break
		}
	}
}
