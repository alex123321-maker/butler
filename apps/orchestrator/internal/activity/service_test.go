package activity

import (
	"context"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/observability"
)

type memoryRepo struct {
	items []Record
}

func (m *memoryRepo) CreateActivity(_ context.Context, params CreateParams) (Record, error) {
	rec := Record{
		ActivityID:   int64(len(m.items) + 1),
		RunID:        params.RunID,
		SessionKey:   params.SessionKey,
		ActivityType: params.ActivityType,
		Title:        params.Title,
		Summary:      params.Summary,
		DetailsJSON:  params.DetailsJSON,
		ActorType:    params.ActorType,
		Severity:     params.Severity,
		CreatedAt:    params.CreatedAt,
	}
	m.items = append(m.items, rec)
	return rec, nil
}

func (m *memoryRepo) ListActivities(_ context.Context, params ListParams) ([]Record, error) {
	return m.items, nil
}

func TestActivityServiceMapsObservabilityEvents(t *testing.T) {
	t.Parallel()

	repo := &memoryRepo{}
	svc := NewService(repo)
	now := time.Date(2026, 3, 19, 17, 30, 0, 0, time.UTC)
	err := svc.RecordFromObservabilityEvent(context.Background(), observability.Event{
		RunID:      "run-1",
		SessionKey: "telegram:chat:1",
		EventType:  observability.EventRunError,
		Timestamp:  now,
		Payload:    map[string]any{"error_message": "timeout"},
	})
	if err != nil {
		t.Fatalf("RecordFromObservabilityEvent returned error: %v", err)
	}
	if len(repo.items) != 1 {
		t.Fatalf("expected one activity record, got %d", len(repo.items))
	}
	if repo.items[0].ActivityType != TypeTaskFailed {
		t.Fatalf("expected task_failed, got %q", repo.items[0].ActivityType)
	}
	if repo.items[0].Severity != SeverityError {
		t.Fatalf("expected error severity, got %q", repo.items[0].Severity)
	}
}
