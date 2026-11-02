package orchestrator

import (
	"context"
	"fmt"
	"log/slog"

	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	"github.com/butler/butler/internal/transport"
)

// Private call-local sink suppresses only the provider continuation for a
// kernel-selected skills.load. The action, Broker, transcript, budgets, source
// and ownership checks remain exactly those of an ordinary tool call.
type initialSkillSinkKey struct{}

func initialSkillSink(ctx context.Context) *string {
	sink, _ := ctx.Value(initialSkillSinkKey{}).(*string)
	return sink
}

func (s *Service) loadInitialSkills(ctx context.Context, log *slog.Logger, current *sessionv1.RunRecord, lease string, p *preparedRun) (*sessionv1.RunRecord, error) {
	if s.config.InitialSkillSelection == nil || s.config.Skills == nil || p.Task == nil {
		return current, nil
	}
	digests, err := s.config.InitialSkillSelection(ctx, current.GetRunId(), p.Task.TaskID)
	if err != nil {
		return current, err
	}
	if len(digests) > 4 {
		return current, fmt.Errorf("too many initial skill selections")
	}
	seen := map[string]bool{}
	for _, digest := range digests {
		if len(digest) != 64 || seen[digest] {
			return current, fmt.Errorf("invalid initial skill selection")
		}
		for _, b := range []byte(digest) {
			if b < '0' || b > '9' && b < 'a' || b > 'f' {
				return current, fmt.Errorf("invalid initial skill digest")
			}
		}
		seen[digest] = true
	}
	for i, digest := range digests {
		var result string
		loadCtx := context.WithValue(ctx, initialSkillSinkKey{}, &result)
		_, next, e := s.handleToolCall(loadCtx, log, current, lease, &transport.ToolCallRequest{
			ToolCallRef: fmt.Sprintf("initial-skill-%s-%d", current.GetRunId(), i), ToolName: "skills.load",
			ArgsJSON: mustMarshalJSON(map[string]any{"digest": digest, "max_bytes": 16384}),
		})
		if next != nil {
			current = next
		}
		if e != nil {
			return current, e
		}
		if result == "" {
			return current, fmt.Errorf("initial skill receipt missing")
		}
	}
	if len(digests) > 0 {
		// Reload live DATA rather than persisting procedure text or inventing a
		// provider function-call/result pair that the provider never produced.
		if err := s.attachSkillContext(ctx, p, true); err != nil {
			return current, err
		}
		if err := s.checkpoint(ctx, current.GetRunId(), "model_started", *p); err != nil {
			return current, err
		}
	}
	return current, nil
}
