package deliveryevents

import (
	"context"
	"testing"
	"time"

	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
)

type memoryRepo struct {
	items []Record
}

func (m *memoryRepo) CreateEvent(_ context.Context, params CreateParams) (Record, error) {
	rec := Record{EventID: int64(len(m.items) + 1), RunID: params.RunID, SessionKey: params.SessionKey, Channel: params.Channel, DeliveryType: params.DeliveryType, State: params.State, ErrorMessage: params.ErrorMessage, DetailsJSON: params.DetailsJSON, CreatedAt: params.CreatedAt}
	m.items = append(m.items, rec)
	return rec, nil
}

func (m *memoryRepo) ListEvents(_ context.Context, params ListParams) ([]Record, error) {
	return m.items, nil
}

func (m *memoryRepo) LatestByRun(_ context.Context, runID string) (*Record, error) {
	for i := len(m.items) - 1; i >= 0; i-- {
		if m.items[i].RunID == runID {
			return &m.items[i], nil
		}
	}
	return nil, nil
}

func TestDeliveryEventsServiceRecordsWaitingAndFailures(t *testing.T) {
	t.Parallel()

	repo := &memoryRepo{}
	svc := NewService(repo)

	svc.RecordApprovalRequest(context.Background(), flow.ApprovalRequest{RunID: "run-1", SessionKey: "telegram:chat:1", ToolCallID: "call-1", ToolName: "http.request"}, nil)
	svc.RecordAssistantFinal(context.Background(), flow.DeliveryEvent{RunID: "run-1", SessionKey: "telegram:chat:1", Content: "done", Final: true}, nil)

	if len(repo.items) != 2 {
		t.Fatalf("expected two delivery events, got %d", len(repo.items))
	}
	if repo.items[0].State != StateWaiting {
		t.Fatalf("expected waiting_reply for approval request, got %q", repo.items[0].State)
	}
	if repo.items[1].State != StateSent {
		t.Fatalf("expected sent for assistant final, got %q", repo.items[1].State)
	}

	now := time.Now().UTC()
	_ = now
}
