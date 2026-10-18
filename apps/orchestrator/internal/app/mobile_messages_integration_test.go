package app

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/butler/butler/apps/orchestrator/internal/goal"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/owneridentity"
	"strings"
	"testing"
	"time"

	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	"github.com/butler/butler/internal/memory/readcontrol"
	"github.com/butler/butler/internal/mobile"
)

func TestMobileAgentBrokerSearchRememberNoiseAndDeletionIntegration(t *testing.T) {
	mandateID := ""
	setup := func(f *delegationGuardFixture) {
		manager := goal.New(f.pool, f.store, nil)
		actor := func(id string) context.Context {
			return owneridentity.WithActor(f.ctx, owneridentity.Actor{OwnerID: f.parent.OwnerID, Channel: "web", DeliveryID: id})
		}
		g, err := manager.PutGoal(actor("mobile-goal"), goal.GoalRequest{Goal: actionauth.GoalPolicy{Kind: "objective", Status: "active", Title: "Understand permitted phone messages", Purpose: "mobile_attention", ExpiresAt: time.Now().Add(2 * time.Hour)}})
		if err != nil {
			t.Fatal(err)
		}
		names := []string{}
		resources := []actionauth.ResourceScope{}
		for _, c := range readcontrol.Contracts() {
			names = append(names, c.ToolName)
			resources = append(resources, actionauth.ResourceScope{Origin: "tool:" + c.ToolName})
		}
		u := budget.DefaultPolicy().Mandate
		m, err := manager.PutMandate(actor("mobile-mandate"), goal.MandateRequest{Mandate: actionauth.Mandate{GoalID: g.SubjectID, GoalRevision: g.Revision, Status: "active", Purpose: "mobile_attention", Tools: names, ResourceScopes: resources, Effects: []actionauth.Effect{actionauth.EffectRead, actionauth.EffectPrepare}, Privacy: "local_only", Budget: actionauth.MandateBudget{ModelTurns: u.ModelTurns, Tokens: u.Tokens, ExternalCalls: u.ExternalCalls, Children: u.Children, ActiveMillis: u.ActiveMillis, EgressBytes: u.EgressBytes, ProviderInputBytes: u.ProviderInputBytes}, ExpiresAt: time.Now().Add(time.Hour), Intervention: []string{"budget_exhausted"}, Reporting: "exceptions"}})
		if err != nil {
			t.Fatal(err)
		}
		mandateID = m.SubjectID
		parent, err := manager.CreateOwnerTask(actor("mobile-task"), goal.TaskRequest{SessionKey: f.parent.SessionKey, GoalID: g.SubjectID, MandateID: m.SubjectID, Purpose: "mobile_attention", Goal: "Read and remember phone messages", CompletionCriteria: []string{"source-backed memory"}, InitiativeReason: "owner authorized autonomous message processing", ObservationRef: "fixture:mobile"})
		if err != nil {
			t.Fatal(err)
		}
		f.parent = parent
	}
	f := newNamedToolControlDispatchFixture(t, setup, false, false, readcontrol.Search)
	if _, err := f.f.pool.Exec(f.f.ctx, `UPDATE runs SET autonomy_mode='mode_2' WHERE run_id=$1`, f.call.RunId); err != nil {
		t.Fatal(err)
	}
	store := &mobile.Store{Pool: f.f.pool, Catalog: f.f.sources}
	tx, err := f.f.pool.Begin(f.f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = mobile.ConfigureAgentAccessTx(f.f.ctx, tx, f.f.parent.OwnerID, true); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(f.f.ctx); err != nil {
		t.Fatal(err)
	}
	code, err := store.CreateCode(f.f.owner)
	if err != nil {
		t.Fatal(err)
	}
	pair, err := store.Exchange(f.f.ctx, mobile.PairRequest{Code: code["pairing_code"].(string), Device: "broker_mobile_device", Name: "synthetic", SDK: 36, Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	text := "Вы записаны к барберу 22 октября 2026 года в 15:00."
	noise := "Скидка на поездку по промокоду TEST"
	events := []mobile.Event{}
	for i, body := range []string{text, noise} {
		value := body
		events = append(events, mobile.Event{Schema: 1, ID: fmt.Sprintf("broker_mobile_event_%d", i), Key: fmt.Sprintf("card-%d", i), Package: "example.synthetic", Change: "POSTED", Posted: now, Observed: now, ContentHash: strings.Repeat("a", 64), Visibility: "PRIVATE", Payload: &mobile.Payload{Text: &value}})
	}
	if ack, err := store.Accept(f.f.ctx, pair.Device, pair.Token, mobile.Batch{Schema: 1, ID: "broker_mobile_batch", Sent: now, Events: events}); err != nil || len(ack.Accepted) != 2 {
		t.Fatal(err)
	}
	reads, err := NewMemoryReads(f.f.pool, f.f.sources)
	if err != nil {
		t.Fatal(err)
	}
	h, err := readcontrol.NewHandler("guarded-request-private-process-fixture-token", reads)
	if err != nil {
		t.Fatal(err)
	}
	process := controlBrokerProcess(t, f, h, readcontrol.Path, "BUTLER_MEMORY_READ_HARNESS="+readcontrol.Search)
	seq := 0
	call := func(tool string, args any) readcontrol.Result {
		t.Helper()
		seq++
		raw, _ := json.Marshal(args)
		f.call.ToolName, f.call.ToolCallId, f.call.ArgsJson, f.call.AutonomyMode = tool, fmt.Sprintf("mobile-call-%d", seq), string(raw), commonv1.AutonomyMode_AUTONOMY_MODE_2
		p, err := f.auth.Prepare(f.f.ctx, f.call, readcontrol.Contract(tool))
		if err != nil || p.RequiresApproval {
			t.Fatal("autonomous tool demanded approval", tool, err)
		}
		g, err := f.auth.Authorize(f.f.ctx, f.call, readcontrol.Contract(tool), p.Binding.ActionID, "", mandateID)
		if err != nil {
			t.Fatal(err)
		}
		f.prepared, f.grant = p, g
		prepareMemoryReadCheckpoint(t, f)
		result, err := process.ExecuteAuthorizedToolCall(f.f.ctx, f.call, g.ID)
		if err != nil || result.GetStatus() != "completed" {
			t.Fatal(tool, err, result)
		}
		out, err := readcontrol.DecodeResult([]byte(result.GetResultJson()))
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	found := call(readcontrol.MessageSearch, map[string]any{"query": "барберу"})
	if len(found.Items) != 1 || !strings.Contains(found.Items[0].Text, "15:00") {
		t.Fatal("message search failed", found.Status)
	}
	argsFor := func(item readcontrol.Item, disposition string) map[string]any {
		var loc map[string]any
		_ = json.Unmarshal(item.Locator, &loc)
		return map[string]any{"message_id": loc["message_id"], "source_id": item.Source.ID, "source_revision": item.Source.Revision, "access_revision": item.Source.AccessRevision, "content_hash": item.Source.Hash, "disposition": disposition}
	}
	a := argsFor(found.Items[0], "remember")
	a["note"] = "Запись к барберу: 22 октября 2026, 15:00. Подтверждено уведомлением."
	saved := call(readcontrol.MessageProcess, a)
	if len(saved.Items) != 1 {
		t.Fatal("note not published")
	}
	repeated := call(readcontrol.MessageProcess, a)
	if len(repeated.Diagnostics) != 1 || repeated.Diagnostics[0] != "already_processed" {
		t.Fatal("duplicate note")
	}
	notes := call(readcontrol.Search, map[string]any{"query": "барберу"})
	if len(notes.Items) < 2 {
		t.Fatal("automatic note not searchable", notes.Status, len(notes.Items))
	}
	ads := call(readcontrol.MessageSearch, map[string]any{"query": "Скидка"})
	if len(ads.Items) != 1 {
		t.Fatal("ad missing")
	}
	a = argsFor(ads.Items[0], "noise")
	a["noise_contains"] = "скидка на поездку"
	hidden := call(readcontrol.MessageProcess, a)
	if len(hidden.Items) != 1 {
		t.Fatal("rule missing")
	}
	if out := call(readcontrol.MessageSearch, map[string]any{"query": "Скидка"}); len(out.Items) != 1 {
		t.Fatal("noise erased searchable original")
	}
	if _, err = f.f.sources.Delete(f.f.owner, found.Items[0].Source.ID, "delete", found.Items[0].Source.Revision); err != nil {
		t.Fatal(err)
	}
	var remaining int
	if err = f.f.pool.QueryRow(f.f.ctx, `SELECT count(*) FROM memory_source_revisions WHERE source_id=$1 AND body<>''`, saved.Items[0].Source.ID).Scan(&remaining); err != nil || remaining != 0 {
		t.Fatal("original deletion left remembered text", remaining, err)
	}
}
