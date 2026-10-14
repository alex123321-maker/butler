package app

import "github.com/butler/butler/internal/transport"
import "time"

// Only permitted output/measurement metadata is retained, never raw HTTP,
// credential envelopes, headers or transport error payloads.
type memoryEvalEvent struct {
	ID           string                    `json:"event_id"`
	Run          string                    `json:"run_id"`
	Provider     string                    `json:"provider"`
	Time         time.Time                 `json:"timestamp"`
	Type         string                    `json:"type"`
	Version      *string                   `json:"version"`
	Usage        *transport.Usage          `json:"usage"`
	Final        *transport.AssistantFinal `json:"final"`
	ToolProposed bool                      `json:"tool_proposed"`
}
