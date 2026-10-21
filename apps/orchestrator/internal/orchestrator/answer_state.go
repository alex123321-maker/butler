package orchestrator

import (
	"context"
	"errors"
	"strings"
)

var ErrResultPersistencePending = errors.New("final result persistence pending; saved answer only may be retried")

type answerKey struct{}
type answerSnapshot struct {
	state                   string
	finalSeen, terminalSeen bool
}

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

func resetAnswerState(ctx context.Context) {
	if s, ok := ctx.Value(answerKey{}).(*answerSnapshot); ok {
		s.state = "final"
		s.finalSeen = false
		s.terminalSeen = false
	}
}

// Providers retain their native finish reason in the logical final event.
// A token limit or explicitly incomplete response is still a saved result,
// but must never be presented as a complete answer.
func partialFinishReason(reason string) bool {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "max_tokens", "length", "incomplete", "content_filter", "cancelled":
		return true
	}
	return false
}

func markFinalSeen(ctx context.Context) {
	if s, ok := ctx.Value(answerKey{}).(*answerSnapshot); ok {
		s.finalSeen = true
	}
}
func markTerminalSeen(ctx context.Context) {
	if s, ok := ctx.Value(answerKey{}).(*answerSnapshot); ok {
		s.terminalSeen = true
	}
}
func finalTerminalSeen(ctx context.Context) bool {
	s, ok := ctx.Value(answerKey{}).(*answerSnapshot)
	return ok && s.finalSeen && s.terminalSeen
}
