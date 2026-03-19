package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	promptmgmt "github.com/butler/butler/apps/orchestrator/internal/prompt"
)

func TestPromptServerHandleConfigReturnsPromptState(t *testing.T) {
	t.Parallel()

	server := NewPromptServer(fakePromptManager{state: promptmgmt.ConfigState{ConfiguredPrompt: "Operator prompt", EffectivePrompt: "Operator prompt", Enabled: true, Source: "db", UpdatedAt: time.Date(2026, 3, 16, 12, 0, 0, 0, time.UTC), UpdatedBy: "unit-test"}})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/prompts/system", nil)
	rr := httptest.NewRecorder()

	server.HandleConfig().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var payload struct {
		Prompt promptConfigDTO `json:"prompt"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Prompt.ConfiguredPrompt != "Operator prompt" {
		t.Fatalf("unexpected prompt payload %+v", payload.Prompt)
	}
	if len(payload.Prompt.AvailablePlaceholders) == 0 {
		t.Fatalf("expected available placeholders, got %+v", payload.Prompt)
	}
}

func TestPromptServerHandleConfigRejectsInvalidPrompt(t *testing.T) {
	t.Parallel()

	server := NewPromptServer(fakePromptManager{updateErr: errors.New("base prompt contains secret-like content")})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/prompts/system", strings.NewReader(`{"base_prompt":"password=secret"}`))
	rr := httptest.NewRecorder()

	server.HandleConfig().ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestPromptServerHandlePreviewReturnsAssembledPrompt(t *testing.T) {
	t.Parallel()

	server := NewPromptServer(fakePromptManager{preview: flow.PromptPreviewResult{
		Config: promptmgmt.ConfigState{ConfiguredPrompt: "Base", EffectivePrompt: "Base", Enabled: true, Source: "db"},
		Assembly: promptmgmt.Assembly{
			ConfiguredPrompt:    "Base",
			EffectiveBasePrompt: "Base",
			FinalPrompt:         "Base\n\nSession summary:\nCurrent task",
			Sections:            []promptmgmt.Section{{Name: "session_summary", Label: "Session summary", Content: "Session summary:\nCurrent task", Inserted: false}},
		},
	}})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/prompts/system/preview", strings.NewReader(`{"session_key":"session-1","user_message":"hello"}`))
	rr := httptest.NewRecorder()

	server.HandlePreview().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var payload struct {
		Preview promptPreviewResponseDTO `json:"preview"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Preview.FinalPrompt == "" || !strings.Contains(payload.Preview.FinalPrompt, "Current task") {
		t.Fatalf("unexpected preview payload %+v", payload.Preview)
	}
}

type fakePromptManager struct {
	state     promptmgmt.ConfigState
	preview   flow.PromptPreviewResult
	updateErr error
}

func (f fakePromptManager) GetPromptConfig(context.Context) (promptmgmt.ConfigState, error) {
	return f.state, nil
}

func (f fakePromptManager) UpdatePromptConfig(context.Context, promptmgmt.UpdateRequest) (promptmgmt.ConfigState, error) {
	if f.updateErr != nil {
		return promptmgmt.ConfigState{}, f.updateErr
	}
	return f.state, nil
}

func (f fakePromptManager) PreviewPrompt(context.Context, flow.PromptPreviewRequest) (flow.PromptPreviewResult, error) {
	return f.preview, nil
}

var _ PromptManager = fakePromptManager{}
