package orchestrator

import (
	"context"
	"github.com/butler/butler/apps/orchestrator/internal/completion"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/transport"
	"log/slog"
	"time"
)

// Business-flow unit fixtures explicitly supply a fake persistence boundary.
// Production never falls back from a missing PostgreSQL coordinator.
func newFixtureService(sessions SessionRepository, leases session.LeaseManager, runs RunManager, history TranscriptStore, provider transport.ModelProvider, cfg Config, log *slog.Logger) *Service {
	s := NewService(sessions, leases, runs, history, provider, cfg, log)
	if cfg.Completion == nil {
		s.config.Completion = fixtureCompletion{s: s}
	}
	return s
}

type fixtureCompletion struct{ s *Service }

func (f fixtureCompletion) Save(ctx context.Context, q completion.Request) (completion.FinalRecord, error) {
	s := f.s
	if _, err := s.transcript.AppendMessage(ctx, transcript.Message{RunID: q.RunID, SessionKey: q.SessionKey, Role: "assistant", Content: q.Content}); err != nil {
		return completion.FinalRecord{}, err
	}
	if s.config.Artifacts != nil {
		if _, err := s.config.Artifacts.SaveAssistantFinal(ctx, q.RunID, q.SessionKey, q.Content, time.Now()); err != nil {
			return completion.FinalRecord{}, err
		}
	}
	current, err := s.runs.GetRun(ctx, q.RunID)
	if err != nil {
		return completion.FinalRecord{}, err
	}
	if _, err = s.transition(ctx, q.RunID, current.GetCurrentState(), commonv1.RunState_RUN_STATE_COMPLETED, current.GetLeaseId(), "", ""); err != nil {
		return completion.FinalRecord{}, err
	}
	_ = s.finalizeWorkingMemory(ctx, q.SessionKey, q.RunID, commonv1.RunState_RUN_STATE_COMPLETED, q.Content)
	_ = s.finalizeTransientWorkingState(ctx, q.SessionKey, q.RunID, commonv1.RunState_RUN_STATE_COMPLETED, q.Content)
	if s.config.PipelineEnqueuer != nil {
		_ = s.config.PipelineEnqueuer.EnqueuePostRun(ctx, q.RunID, q.SessionKey)
	}
	_ = s.config.Delivery.DeliverAssistantFinal(ctx, DeliveryEvent{RunID: q.RunID, SessionKey: q.SessionKey, Content: q.Content, Final: true})
	return completion.FinalRecord{RunID: q.RunID, ResultID: "fixture:" + q.RunID, Content: q.Content, AnswerState: q.AnswerState, Verdict: "unverified"}, nil
}
