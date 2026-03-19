package activity

import (
	"context"
	"encoding/json"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/observability"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Record(ctx context.Context, params CreateParams) error {
	if s == nil || s.repo == nil {
		return nil
	}
	_, err := s.repo.CreateActivity(ctx, params)
	return err
}

func (s *Service) RecordFromObservabilityEvent(ctx context.Context, event observability.Event) error {
	if s == nil || s.repo == nil {
		return nil
	}
	activityType, title, summary, actorType, severity := mapEvent(event)
	detailsJSON := "{}"
	if event.Payload != nil {
		if raw, err := json.Marshal(event.Payload); err == nil {
			detailsJSON = string(raw)
		}
	}
	_, err := s.repo.CreateActivity(ctx, CreateParams{
		RunID:        event.RunID,
		SessionKey:   event.SessionKey,
		ActivityType: activityType,
		Title:        title,
		Summary:      summary,
		DetailsJSON:  detailsJSON,
		ActorType:    actorType,
		Severity:     severity,
		CreatedAt:    event.Timestamp.UTC(),
	})
	return err
}

func mapEvent(event observability.Event) (activityType, title, summary, actorType, severity string) {
	activityType = event.EventType
	title = event.EventType
	summary = event.EventType
	actorType = "system"
	severity = SeverityInfo

	switch event.EventType {
	case observability.EventStateTransition:
		title = "State transition"
		summary = "Run state changed"
	case observability.EventApprovalRequested:
		activityType = TypeApprovalRequested
		title = "Approval requested"
		summary = "Task is waiting for approval"
		actorType = "agent"
	case observability.EventApprovalResolved:
		activityType = TypeApprovalResolved
		title = "Approval resolved"
		summary = "Approval decision received"
	case observability.EventRunCompleted:
		activityType = TypeTaskCompleted
		title = "Task completed"
		summary = "Task finished successfully"
	case observability.EventRunError:
		activityType = TypeTaskFailed
		title = "Task failed"
		summary = "Task finished with an error"
		severity = SeverityError
	case observability.EventToolStarted:
		title = "Tool started"
		summary = "Agent started tool execution"
		actorType = "agent"
	case observability.EventToolCompleted:
		title = "Tool completed"
		summary = "Agent finished tool execution"
		actorType = "agent"
	case observability.EventAssistantFinal:
		title = "Assistant final response"
		summary = "Assistant returned final output"
		actorType = "agent"
	case observability.EventMemoryLoaded:
		title = "Task received"
		summary = "Task context prepared"
		activityType = TypeTaskReceived
	case observability.EventPromptAssembled:
		title = "Model started"
		summary = "Model input assembled"
		activityType = TypeModelStarted
	}

	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	return activityType, title, summary, actorType, severity
}
