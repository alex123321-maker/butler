package broker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/butler/butler/apps/tool-broker/internal/runtimeclient"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/security/egress/egresstest"
)

func TestBrowserSessionRecoveryAfterRuntimeLossChromiumIntegration(t *testing.T) {
	if os.Getenv("BUTLER_BROWSER_E2E") != "1" {
		t.Skip("isolated PG and actual Chromium required")
	}
	var posts atomic.Int32
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.Method == "POST" {
			posts.Add(1)
			fmt.Fprint(w, "saved")
			return
		}
		fmt.Fprint(w, `<form method=post><input name=title><button id=submit>Send</button></form>`)
	}))
	defer site.Close()
	t.Setenv("BUTLER_RESOURCE_BROWSER_FIXTURE", site.URL)
	f := resourceBroker(t, nil, false)
	old, url, version := browserSafetyOpen(t, f)
	prep := func(id, session string, v int64) *toolbrokerv1.ExecuteToolCallRequest {
		return f.callRequest(t, &toolbrokerv1.ToolCall{RunId: "run", ToolCallId: id, ToolName: "browser.prepare_fill", AutonomyMode: 3, ArgsJson: fmt.Sprintf(`{"browser_session_id":%q,"expected_url":%q,"observation_version":%d,"selector":"input[name=title]","value":"ordinary"}`, session, url, v)})
	}
	original := prep("before-loss", old, version)
	f.stopRuntime()
	f.startRuntime()
	router := runtimeclient.New(nil)
	defer router.Close()
	contract, _ := f.server.registry.Get("browser.open")
	if err := router.SetTargetAuthentication(contract.RuntimeTarget, strings.Repeat("fixture-private-resource-", 3)); err != nil {
		t.Fatal(err)
	}
	executor, err := NewResourceExecutor(f.store, router)
	if err != nil {
		t.Fatal(err)
	}
	f.server.executor = executor
	ctx := context.Background()
	r, err := f.server.ExecuteToolCall(ctx, original)
	if err != nil || r.GetResult().GetStatus() != "failed" || r.GetResult().GetEffectState() != "effect_absent" || !strings.Contains(r.GetResult().GetError().GetMessage(), "Open a new browser session") || posts.Load() != 0 {
		t.Fatal("lost pre-input context was not safely absent", r.GetResult().GetStatus(), r.GetResult().GetEffectState(), r.GetResult().GetError())
	}
	var reservations, dispatches int
	if f.pool.QueryRow(ctx, `SELECT count(*) FROM execution_budget_reservations`).Scan(&reservations) != nil || reservations != 2 {
		t.Fatal("lost preparation spending disappeared")
	}
	if f.pool.QueryRow(ctx, `SELECT count(*) FROM action_effect_dispatches WHERE authorization_id=$1`, original.AuthorizationId).Scan(&dispatches) != nil || dispatches != 1 {
		t.Fatal("original dispatch identity changed")
	}
	_, _ = f.server.ExecuteToolCall(ctx, original)
	if posts.Load() != 0 {
		t.Fatal("old consumed grant dispatched again")
	}
	fresh, url, version := browserSafetyOpenNamed(t, f, "recovery-open")
	if fresh == old {
		t.Fatal("DOM context silently restored")
	}
	r, err = f.server.ExecuteToolCall(ctx, prep("recovery-prepare", fresh, version))
	var view struct {
		Version int64 `json:"observation_version"`
	}
	if err != nil || r.GetResult().GetStatus() != "completed" || r.GetResult().GetEffectState() != "effect_confirmed" || json.Unmarshal([]byte(r.GetResult().GetResultJson()), &view) != nil {
		t.Fatal("fresh preparation did not recover", r.GetResult().GetError())
	}
	click := f.callRequest(t, &toolbrokerv1.ToolCall{RunId: "run", ToolCallId: "recovery-click", ToolName: "browser.click", AutonomyMode: 3, ArgsJson: fmt.Sprintf(`{"browser_session_id":%q,"expected_url":%q,"observation_version":%d,"selector":"#submit"}`, fresh, url, view.Version)})
	r, err = f.server.ExecuteToolCall(ctx, click)
	if err != nil || r.GetResult().GetEffectState() != "effect_unknown" || posts.Load() != 1 {
		t.Fatal("recovered context did not submit exactly once", r.GetResult().GetError())
	}
	// A real earlier unknown write still blocks further preparation. A new
	// pre-input absence protocol cannot manufacture reconciliation authority.
	next := prep("after-unknown", fresh, view.Version)
	r, err = f.server.ExecuteToolCall(ctx, next)
	if err != nil || r.GetResult().GetStatus() == "completed" || posts.Load() != 1 {
		t.Fatal("recovery cleared unknown POST")
	}
	t.Log("actual_runtime_restart=true old_preparation=effect_absent old_dispatches=1 retained_costs=true distinct_fresh_context=true fresh_preparation=effect_confirmed recipient_posts=1 post_effect=effect_unknown successor_blocked=true")
}

func TestBrowserFrozenNavigationRecoveryChromiumIntegration(t *testing.T) {
	if os.Getenv("BUTLER_BROWSER_E2E") != "1" {
		t.Skip("isolated PG and actual Chromium required")
	}
	var reads, posts atomic.Int32
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.Method == "POST" {
			posts.Add(1)
		}
		if r.URL.Path == "/read" {
			reads.Add(1)
			fmt.Fprint(w, "Recovered reference")
			return
		}
		fmt.Fprint(w, `<form><input name=title></form>`)
	}))
	defer site.Close()
	t.Setenv("BUTLER_RESOURCE_BROWSER_FIXTURE", site.URL)
	f := resourceBroker(t, nil, false)
	id, url, version := browserSafetyOpen(t, f)
	ctx := context.Background()
	frozen := f.callRequest(t, &toolbrokerv1.ToolCall{RunId: "run", ToolCallId: "freeze", ToolName: "browser.freeze_snapshot", AutonomyMode: 3, ArgsJson: fmt.Sprintf(`{"browser_session_id":%q}`, id)})
	r, err := f.server.ExecuteToolCall(ctx, frozen)
	if err != nil || r.GetResult().GetStatus() != "completed" {
		t.Fatal("freeze failed")
	}
	var view struct {
		Version int64 `json:"observation_version"`
	}
	if json.Unmarshal([]byte(r.GetResult().GetResultJson()), &view) != nil {
		t.Fatal("freeze observation missing")
	}
	version = view.Version
	nav := f.callRequest(t, &toolbrokerv1.ToolCall{RunId: "run", ToolCallId: "frozen-navigation", ToolName: "browser.navigate", AutonomyMode: 3, ArgsJson: fmt.Sprintf(`{"browser_session_id":%q,"expected_url":%q,"observation_version":%d,"url":%q}`, id, url, version, egresstest.Origin+"/read")})
	r, err = f.server.ExecuteToolCall(ctx, nav)
	if err != nil || r.GetResult().GetStatus() != "failed" || r.GetResult().GetEffectState() != "effect_absent" || reads.Load() != 0 || posts.Load() != 0 {
		t.Fatal("frozen navigation lacked exact non-dispatch proof", r.GetResult().GetError())
	}
	open := f.callRequest(t, &toolbrokerv1.ToolCall{RunId: "run", ToolCallId: "fresh-independent-read", ToolName: "browser.open", AutonomyMode: 3, ArgsJson: `{"url":"` + egresstest.Origin + `/read"}`})
	r, err = f.server.ExecuteToolCall(ctx, open)
	var fresh struct {
		ID string `json:"browser_session_id"`
	}
	_ = json.Unmarshal([]byte(r.GetResult().GetResultJson()), &fresh)
	if err != nil || r.GetResult().GetStatus() != "completed" || fresh.ID == id || fresh.ID == "" || reads.Load() != 1 || posts.Load() != 0 {
		t.Fatal("new context failed to continue read")
	}
	_, _ = f.server.ExecuteToolCall(ctx, nav)
	if reads.Load() != 1 {
		t.Fatal("old navigation grant replayed")
	}
	t.Log("frozen_navigation=effect_absent rejected_target_requests=0 fresh_context_read=effect_confirmed total_target_reads=1 recipient_posts=0 consumed_grant_not_replayed=true")
}
