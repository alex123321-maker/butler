package orchestrator

import (
	"context"
	"errors"
)

var ErrResultPersistencePending = errors.New("final result persistence pending; saved answer only may be retried")

type answerKey struct{}
type answerSnapshot struct{ state string }

func withAnswerState(ctx context.Context, state string) context.Context {
	if state == "" {
		state = "final"
	}
	return context.WithValue(ctx, answerKey{}, &answerSnapshot{state: state})
}
func answerState(ctx context.Context) string {
	if s, ok := ctx.Value(answerKey{}).(*answerSnapshot); ok {
		return s.state
	}
	return "final"
}
func setPartialAnswer(ctx context.Context) {
	if s, ok := ctx.Value(answerKey{}).(*answerSnapshot); ok {
		s.state = "partial"
	}
}
