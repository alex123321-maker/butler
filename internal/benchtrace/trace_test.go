package benchtrace

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/butler/butler/internal/transport"
)

func TestEnrollmentAndLogicalModelTrace(t *testing.T) {
	root := t.TempDir()
	event := "bench-" + strings.Repeat("1", 24) + "-0001-01"
	session, run := "web:synthetic", "run-1"
	if err := Enroll(root, session, event); err != nil {
		t.Fatal(err)
	}
	if err := Enroll(root, session, event); err == nil {
		t.Fatal("reused enrollment")
	}
	r := &transport.StartRunRequest{Context: transport.TransportRunContext{ProviderSessionRef: &transport.ProviderSessionRef{SessionRef: "opaque-do-not-save"}},
		InputItems:      []transport.InputItem{{Role: "system", Content: "test policy"}, {Role: "user", Content: "synthetic request"}},
		ToolDefinitions: []transport.ToolDefinition{{Name: "example", SchemaJSON: "{}"}}, TransportOptions: map[string]any{"token": "opaque-do-not-save"}}
	for _, bad := range []struct{ source, session, event, id string }{{"mobile", session, event, run}, {"web", "other", event, run}, {"web", session, "../outside", run}, {"web", session, event, "foreign-run"}} {
		w, err := Begin(Bind(t.Context(), root, bad.source, bad.session, bad.event, run), bad.id, "op", "provider", "model", r)
		if err != nil || w != nil {
			t.Fatal("unrelated traffic captured")
		}
	}
	ctx := Bind(t.Context(), root, "web", session, event, run)
	w, err := Begin(ctx, run, "op", "provider", "model", r)
	if err != nil || w == nil {
		t.Fatal(err)
	}
	phase := false
	ctx = transport.WithProviderStageObserver(ctx, func(transport.ProviderStage) { phase = true })
	transport.ObserveProviderStage(w.ObserveStages(ctx), transport.ProviderStageHTTP)
	if !phase {
		t.Fatal("parent observer lost")
	}
	w.Event(transport.NewProviderSessionBoundEvent(run, "provider", *r.Context.ProviderSessionRef))
	w.Event(transport.NewAssistantDeltaEvent(run, "provider", transport.AssistantDelta{DeltaType: "text", Content: "visible"}))
	w.Event(transport.NewAssistantDeltaEvent(run, "provider", transport.AssistantDelta{DeltaType: "internal", Content: "opaque-do-not-save"}))
	usage := &transport.Usage{InputTokens: 8, OutputTokens: 4, TotalTokens: 13}
	ver := "fixture-v1"
	e := transport.NewAssistantFinalEvent(run, "provider", transport.AssistantFinal{Content: "visible", Usage: usage})
	e.ProviderModelVersion = &ver
	w.Event(e)
	if err := w.End("stream_closed"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, event, Filename(run, "op")))
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"synthetic request", "test policy", "example", "http_round_trip", "visible", "fixture-v1", "TotalTokens"} {
		if !strings.Contains(string(raw), s) {
			t.Fatalf("missing %s", s)
		}
	}
	if bytes.Contains(raw, []byte("opaque-do-not-save")) {
		t.Fatal("opaque transport state retained")
	}
	end, err := Inspect(bytes.NewReader(raw), run, "op", "provider", "model")
	if err != nil || end.FirstOutputUS == nil || *end.FirstOutputUS > end.ElapsedUS {
		t.Fatal("missing timing", err)
	}
	if _, err = Inspect(bytes.NewReader(raw), run, "other-op", "provider", "model"); err == nil {
		t.Fatal("cross-operation trace accepted")
	}
	if _, err = Inspect(bytes.NewReader(bytes.TrimSuffix(raw, []byte("\n"))), run, "op", "provider", "model"); err == nil {
		t.Fatal("truncated trace accepted")
	}
	for _, v := range []struct {
		req  any
		test func(Request) bool
	}{
		{&transport.ContinueRunRequest{InputItems: r.InputItems, ToolSet: &transport.ToolSetSnapshot{Definitions: []transport.ToolDefinition{}}}, func(p Request) bool { return len(p.Items) == 2 && p.Tools != nil && len(p.Tools.Definitions) == 0 }},
		{&transport.SubmitToolResultRequest{ToolCallRef: "call", ToolResultJSON: "{}"}, func(p Request) bool { return p.ToolResult == "{}" && p.ToolRef == "call" && p.Tools == nil }},
	} {
		b, _ := json.Marshal(request(v.req))
		var p Request
		if json.Unmarshal(b, &p) != nil || !v.test(p) {
			t.Fatal("continuation data", string(b))
		}
	}
}
