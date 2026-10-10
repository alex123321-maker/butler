package api

import (
	"context"
	"errors"
	"github.com/butler/butler/apps/orchestrator/internal/completion"
	"github.com/butler/butler/apps/orchestrator/internal/deliveryoutbox"
	"net/http"
)

type finalResultReader interface {
	GetByRun(context.Context, string) (completion.FinalRecord, error)
}
type outboxReader interface {
	ListByRun(context.Context, string) ([]deliveryoutbox.Message, error)
}

func (s *TaskViewServer) SetFinalResults(results finalResultReader, outbox outboxReader) {
	s.finalResults = results
	s.outbox = outbox
}
func (s *OverviewServer) SetOutbox(outbox outboxReader) { s.outbox = outbox }

func (s *TaskViewServer) loadFinalResult(r *http.Request, runID string) any {
	if s.finalResults == nil {
		return nil
	}
	record, err := s.finalResults.GetByRun(r.Context(), runID)
	if errors.Is(err, completion.ErrNotFound) {
		return nil
	}
	if err != nil {
		return map[string]string{"state": "result_read_unavailable"}
	}
	return record
}
func (s *TaskViewServer) loadOutboxState(r *http.Request, runID string) (map[string]any, bool) {
	if s.outbox == nil {
		return nil, false
	}
	records, err := s.outbox.ListByRun(r.Context(), runID)
	if err != nil {
		return map[string]any{"state": "delivery_read_unavailable", "error_message": "delivery_read_failed"}, true
	}
	if len(records) == 0 {
		return nil, false
	}
	return map[string]any{"channel": records[0].Channel, "state": deliveryoutbox.State(records), "delivery_type": "assistant_final", "result_id": records[0].ResultID, "notifications": records, "ack_semantics": "Telegram sent means Bot API accepted message_id; Web result_ready means stored result; client read unobservable"}, true
}
func (s *OverviewServer) outboxStatus(r *http.Request, runID string) string {
	if s.outbox == nil {
		return ""
	}
	records, err := s.outbox.ListByRun(r.Context(), runID)
	if err != nil {
		return "delivery_read_unavailable"
	}
	return deliveryoutbox.State(records)
}
