package transport

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type ProviderSessionRef struct {
	ProviderName string
	SessionRef   string
	ResponseRef  string
	CreatedAt    time.Time
	LastUsedAt   time.Time
}

type TransportRunContext struct {
	RunID                  string
	SessionKey             string
	ProviderName           string
	ModelName              string
	ProviderSessionRef     *ProviderSessionRef
	SupportsStreaming      bool
	SupportsToolCalls      bool
	SupportsStatefulResume bool
}

type InputItem struct {
	Role        string
	Content     string
	ContentType string
	Name        string
	Metadata    map[string]any
}

type ToolDefinition struct {
	Name        string
	Description string
	SchemaJSON  string
}

type StartRunRequest struct {
	Context              TransportRunContext
	InputItems           []InputItem
	ToolDefinitions      []ToolDefinition
	StreamingEnabled     bool
	TransportOptions     map[string]any
	TransportOptionsJSON string
}

type ContinueRunRequest struct {
	RunID                string
	ProviderSessionRef   *ProviderSessionRef
	InputItems           []InputItem
	TransportOptions     map[string]any
	TransportOptionsJSON string
}

type SubmitToolResultRequest struct {
	RunID                string
	ProviderSessionRef   *ProviderSessionRef
	ToolCallRef          string
	ToolResultJSON       string
	TransportOptions     map[string]any
	TransportOptionsJSON string
}

type CancelRunRequest struct {
	RunID              string
	ProviderSessionRef *ProviderSessionRef
	Reason             string
}

type TransportCommandType string

const (
	CommandTypeStartRun         TransportCommandType = "start_run"
	CommandTypeContinueRun      TransportCommandType = "continue_run"
	CommandTypeSubmitToolResult TransportCommandType = "submit_tool_result"
	CommandTypeCancelRun        TransportCommandType = "cancel_run"
)

type TransportCommand struct {
	CommandID    string
	RunID        string
	CommandType  TransportCommandType
	ProviderName string
	PayloadJSON  string
	CreatedAt    time.Time
}

func (r TransportRunContext) Validate() error {
	if strings.TrimSpace(r.RunID) == "" {
		return fmt.Errorf("run_id is required")
	}
	if strings.TrimSpace(r.SessionKey) == "" {
		return fmt.Errorf("session_key is required")
	}
	if strings.TrimSpace(r.ProviderName) == "" {
		return fmt.Errorf("provider_name is required")
	}
	if strings.TrimSpace(r.ModelName) == "" {
		return fmt.Errorf("model_name is required")
	}
	return nil
}

func (r StartRunRequest) Validate() error {
	if err := r.Context.Validate(); err != nil {
		return err
	}
	return normalizeTransportOptions(r.TransportOptionsJSON)
}

func (r ContinueRunRequest) Validate() error {
	if strings.TrimSpace(r.RunID) == "" {
		return fmt.Errorf("run_id is required")
	}
	return normalizeTransportOptions(r.TransportOptionsJSON)
}

func (r SubmitToolResultRequest) Validate() error {
	if strings.TrimSpace(r.RunID) == "" {
		return fmt.Errorf("run_id is required")
	}
	if strings.TrimSpace(r.ToolCallRef) == "" {
		return fmt.Errorf("tool_call_ref is required")
	}
	if strings.TrimSpace(r.ToolResultJSON) == "" {
		return fmt.Errorf("tool_result_json is required")
	}
	if !json.Valid([]byte(r.ToolResultJSON)) {
		return fmt.Errorf("tool_result_json must be valid JSON")
	}
	return normalizeTransportOptions(r.TransportOptionsJSON)
}

func (r CancelRunRequest) Validate() error {
	if strings.TrimSpace(r.RunID) == "" {
		return fmt.Errorf("run_id is required")
	}
	return nil
}

func normalizeTransportOptions(value string) error {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	if !json.Valid([]byte(value)) {
		return fmt.Errorf("transport_options_json must be valid JSON")
	}
	return nil
}
