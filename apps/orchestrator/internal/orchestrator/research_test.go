package orchestrator

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/domain"
	"github.com/butler/butler/internal/domain/convert"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/transport"
)

func TestResearchPublicationPreflightIsBoundedAndRunScoped(t *testing.T) {
	now := time.Now()
	h := &memoryTranscriptStore{toolCalls: []transcript.ToolCall{{RunID: "r", ToolName: "web.search", Status: "failed", FinishedAt: &now}}}
	s := newFixtureService(nil, nil, nil, h, &mockProvider{}, Config{}, nil)
	r := &transport.ToolCallRequest{ToolName: "communication.message", ArgsJSON: `{"schema_version":1,"purpose":"result","text":"A precise unsupported date."}`}
	current := &sessionv1.RunRecord{RunId: "r", SessionKey: "chat"}
	if feedback, err := s.researchFeedback(context.Background(), current, r); err != nil || !strings.Contains(feedback, "opened primary") {
		t.Fatal(feedback, err)
	}
	current.RunId = "unrelated"
	if feedback, err := s.researchFeedback(context.Background(), current, r); err != nil || feedback != "" {
		t.Fatal("another run imposed research evidence", feedback, err)
	}
	current.RunId = "r"
	var args map[string]any
	_ = json.Unmarshal([]byte(r.ArgsJSON), &args)
	args["purpose"] = "blocked"
	args["text"] = "Current primary confirmation is unavailable."
	raw, _ := json.Marshal(args)
	r.ArgsJSON = string(raw)
	if feedback, err := s.researchFeedback(context.Background(), current, r); err != nil || feedback != "" {
		t.Fatal("honest bounded failure blocked", feedback, err)
	}
	args["purpose"] = "result"
	raw, _ = json.Marshal(args)
	r.ArgsJSON = string(raw)
	for range 4 {
		h.toolCalls = append(h.toolCalls, transcript.ToolCall{RunID: "r", Status: "validation_rejected"})
	}
	if _, err := s.researchFeedback(context.Background(), current, r); err == nil {
		t.Fatal("unbounded correction loop")
	}
}

func TestResearchGuardedGETPublicationUsesCurrentObservation(t *testing.T) {
	now := time.Now().UTC()
	for _, tool := range []string{"browser.submit_get_form", "browser.follow_link"} {
		h := &memoryTranscriptStore{toolCalls: []transcript.ToolCall{{RunID: "r", ToolName: tool, ArgsJSON: `{"browser_session_id":"context"}`, Status: "completed", FinishedAt: &now, ResultJSON: `{"ok":true,"network_state":"frozen","final_url":"https://issuer.example/record","text":"The record reference is a fresh public observation."}`}}}
		s := newFixtureService(nil, nil, nil, h, &mockProvider{}, Config{}, nil)
		r := &transport.ToolCallRequest{ToolName: "communication.message", ArgsJSON: `{"schema_version":1,"purpose":"result","text":"The record reference is a fresh public observation. https://issuer.example/record","evidence":[{"url":"https://issuer.example/record","quote":"The record reference is a fresh public observation.","role":"primary"}]}`}
		if feedback, err := s.researchFeedback(context.Background(), &sessionv1.RunRecord{RunId: "r", SessionKey: "chat"}, r); err != nil || feedback != "" {
			t.Fatal("valid GET quote required refetch", tool, feedback, err)
		}
		if feedback, err := s.researchFeedback(context.Background(), &sessionv1.RunRecord{RunId: "other", SessionKey: "chat"}, r); err != nil || feedback == "" {
			t.Fatal("previous-run GET quote accepted", tool, feedback, err)
		}
	}
}

func TestResearchCorrectionIsCorrelatedDurableNonDispatch(t *testing.T) {
	store := &memoryTranscriptStore{}
	provider := &mockProvider{}
	runs := newMemoryRunManager()
	current := &sessionv1.RunRecord{RunId: "r", SessionKey: "chat", CurrentState: commonv1.RunState_RUN_STATE_MODEL_RUNNING}
	runs.records["r"] = current
	s := newFixtureService(nil, nil, runs, store, provider, Config{}, nil)
	r := &transport.ToolCallRequest{ToolCallRef: "draft", ToolName: "communication.message", ArgsJSON: `{"schema_version":1,"purpose":"result","text":"Unsupported"}`}
	if _, _, err := s.rejectResearchDraft(context.Background(), current, "lease", r, "Read the actual primary page"); err != nil {
		t.Fatal(err)
	}
	if len(store.toolCalls) != 1 || store.toolCalls[0].Status != "validation_rejected" || len(provider.submittedResults) != 1 || provider.submittedResults[0].ToolCallRef != "draft" {
		t.Fatal("lost durable correlated feedback")
	}
	from := commonv1.RunState_RUN_STATE_MODEL_RUNNING
	for _, to := range runs.history["r"] {
		source, _ := convert.ProtoToRunState(from)
		target, _ := convert.ProtoToRunState(to)
		if err := domain.ValidateRunStateTransition(source, target); err != nil {
			t.Fatal("illegal validation lifecycle", err)
		}
		from = to
	}
	replayed := recoveredActionResultItem("draft", r.ToolName, r.ArgsJSON, recordedToolResultEnvelope(store.toolCalls[0]))
	if !strings.Contains(replayed.Content, `"already_executed":false`) {
		t.Fatal("rejected draft became a committed publication", replayed.Content)
	}
}

func TestResearchMalformedPublicationGetsCorrectableFeedback(t *testing.T) {
	s := newFixtureService(nil, nil, nil, &memoryTranscriptStore{}, &mockProvider{}, Config{}, nil)
	feedback, err := s.researchFeedback(context.Background(), &sessionv1.RunRecord{RunId: "r", SessionKey: "chat"}, &transport.ToolCallRequest{ToolName: "communication.message", ArgsJSON: `{"schema_version":1,"purpose":"result","text":"Fact","evidence":[{"url":"not a URL"}]}`})
	if err != nil || !strings.Contains(feedback, "no message was sent") {
		t.Fatal(feedback, err)
	}
}

func TestResearchDoesNotImposeCitationsOnOrdinaryBrowserOrHTTPActions(t *testing.T) {
	now := time.Now()
	for _, tool := range []string{"browser.snapshot", "http.request"} {
		h := &memoryTranscriptStore{toolCalls: []transcript.ToolCall{{RunID: "r", ToolName: tool, ArgsJSON: `{"method":"POST","url":"https://owner.example/form"}`, Status: "completed", ResultJSON: `{"ok":true}`, FinishedAt: &now}}}
		s := newFixtureService(nil, nil, nil, h, &mockProvider{}, Config{}, nil)
		feedback, err := s.researchFeedback(context.Background(), &sessionv1.RunRecord{RunId: "r", SessionKey: "chat"}, &transport.ToolCallRequest{ToolName: "communication.message", ArgsJSON: `{"schema_version":1,"purpose":"result","text":"The requested form was submitted."}`})
		if err != nil || feedback != "" {
			t.Fatal("non-research action acquired a source requirement", tool, feedback, err)
		}
	}
}
