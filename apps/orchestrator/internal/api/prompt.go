package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	promptmgmt "github.com/butler/butler/apps/orchestrator/internal/prompt"
)

type PromptManager interface {
	GetPromptConfig(context.Context) (promptmgmt.ConfigState, error)
	UpdatePromptConfig(context.Context, promptmgmt.UpdateRequest) (promptmgmt.ConfigState, error)
	PreviewPrompt(context.Context, flow.PromptPreviewRequest) (flow.PromptPreviewResult, error)
}

type PromptServer struct {
	manager PromptManager
}

func NewPromptServer(manager PromptManager) *PromptServer {
	return &PromptServer{manager: manager}
}

func (s *PromptServer) HandleConfig() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			state, err := s.manager.GetPromptConfig(r.Context())
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"prompt": toPromptConfigDTO(state)})
		case http.MethodPut:
			var request struct {
				BasePrompt string `json:"base_prompt"`
				Enabled    *bool  `json:"enabled"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON request body"})
				return
			}
			state, err := s.manager.UpdatePromptConfig(r.Context(), promptmgmt.UpdateRequest{BasePrompt: request.BasePrompt, Enabled: request.Enabled})
			if err != nil {
				status := http.StatusInternalServerError
				if strings.Contains(err.Error(), "secret-like") || strings.Contains(err.Error(), "exceeds") {
					status = http.StatusBadRequest
				}
				writeJSON(w, status, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"prompt": toPromptConfigDTO(state)})
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
}

func (s *PromptServer) HandlePreview() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var request struct {
			SessionKey  string `json:"session_key"`
			UserID      string `json:"user_id"`
			UserMessage string `json:"user_message"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON request body"})
			return
		}
		preview, err := s.manager.PreviewPrompt(r.Context(), flow.PromptPreviewRequest{SessionKey: request.SessionKey, UserID: request.UserID, UserMessage: request.UserMessage})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"preview": toPromptPreviewResponseDTO(preview)})
	})
}

type promptConfigDTO struct {
	ConfiguredPrompt      string   `json:"configured_prompt"`
	EffectivePrompt       string   `json:"effective_prompt"`
	Enabled               bool     `json:"enabled"`
	Source                string   `json:"source"`
	UpdatedAt             string   `json:"updated_at,omitempty"`
	UpdatedBy             string   `json:"updated_by,omitempty"`
	AvailablePlaceholders []string `json:"available_placeholders"`
}

type promptSectionDTO struct {
	Name          string `json:"name"`
	Label         string `json:"label"`
	Content       string `json:"content"`
	Inserted      bool   `json:"inserted"`
	Truncated     bool   `json:"truncated"`
	Omitted       bool   `json:"omitted"`
	OmittedReason string `json:"omitted_reason,omitempty"`
}

type promptPreviewResponseDTO struct {
	Prompt              promptConfigDTO    `json:"prompt"`
	ConfiguredPrompt    string             `json:"configured_prompt"`
	EffectiveBasePrompt string             `json:"effective_base_prompt"`
	FinalPrompt         string             `json:"final_prompt"`
	UnknownPlaceholders []string           `json:"unknown_placeholders,omitempty"`
	Truncated           bool               `json:"truncated"`
	Sections            []promptSectionDTO `json:"sections"`
}

func toPromptConfigDTO(state promptmgmt.ConfigState) promptConfigDTO {
	item := promptConfigDTO{
		ConfiguredPrompt:      state.ConfiguredPrompt,
		EffectivePrompt:       state.EffectivePrompt,
		Enabled:               state.Enabled,
		Source:                state.Source,
		UpdatedBy:             state.UpdatedBy,
		AvailablePlaceholders: []string{"{{session_summary}}", "{{working_memory}}", "{{profile_memory}}", "{{episodic_memory}}", "{{document_chunks}}", "{{tool_summary}}"},
	}
	if !state.UpdatedAt.IsZero() {
		item.UpdatedAt = state.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z07:00")
	}
	return item
}

func toPromptPreviewResponseDTO(result flow.PromptPreviewResult) promptPreviewResponseDTO {
	sections := make([]promptSectionDTO, 0, len(result.Sections))
	for _, section := range result.Sections {
		sections = append(sections, promptSectionDTO{
			Name:          section.Name,
			Label:         section.Label,
			Content:       section.Content,
			Inserted:      section.Inserted,
			Truncated:     section.Truncated,
			Omitted:       section.Omitted,
			OmittedReason: section.OmittedReason,
		})
	}
	return promptPreviewResponseDTO{
		Prompt:              toPromptConfigDTO(result.Config),
		ConfiguredPrompt:    result.ConfiguredPrompt,
		EffectiveBasePrompt: result.EffectiveBasePrompt,
		FinalPrompt:         result.FinalPrompt,
		UnknownPlaceholders: append([]string(nil), result.UnknownPlaceholders...),
		Truncated:           result.Truncated,
		Sections:            sections,
	}
}
