package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	"github.com/butler/butler/internal/memory/sanitize"
	memoryservice "github.com/butler/butler/internal/memory/service"
)

// workingMemoryContext holds the in-flight working memory state for a run.
type workingMemoryContext struct {
	Goal           string
	ActiveEntities any
	PendingSteps   any
	Scratch        map[string]any
	Status         string
	Policy         WorkingMemoryPolicy
}

func (w *workingMemoryContext) withInitialGoal(userMessage string) *workingMemoryContext {
	if w == nil {
		return &workingMemoryContext{Goal: strings.TrimSpace(userMessage), Status: "active", Scratch: map[string]any{}}
	}
	if strings.TrimSpace(w.Goal) == "" {
		w.Goal = strings.TrimSpace(userMessage)
	}
	w.Status = normalizeWorkingStatus(w.Status)
	if w.Status == "idle" && strings.TrimSpace(w.Goal) != "" {
		w.Status = "active"
	}
	if w.ActiveEntities == nil {
		w.ActiveEntities = map[string]any{}
	}
	if w.PendingSteps == nil {
		w.PendingSteps = []any{}
	}
	if w.Scratch == nil {
		w.Scratch = map[string]any{}
	}
	return w
}

func (w *workingMemoryContext) bundleMap() map[string]any {
	if w == nil {
		return nil
	}
	return map[string]any{
		"goal":            w.Goal,
		"active_entities": w.ActiveEntities,
		"pending_steps":   w.PendingSteps,
		"working_status":  w.Status,
	}
}

func workingMemoryIsEmpty(w *workingMemoryContext) bool {
	if w == nil {
		return true
	}
	if strings.TrimSpace(w.Goal) != "" {
		return false
	}
	if !isEmptyJSONValue(w.ActiveEntities) {
		return false
	}
	if !isEmptyJSONValue(w.PendingSteps) {
		return false
	}
	return strings.TrimSpace(normalizeWorkingStatus(w.Status)) == "idle"
}

func workingMemoryFromBundle(bundle memoryservice.WorkingContext, policy WorkingMemoryPolicy) *workingMemoryContext {
	return (&workingMemoryContext{
		Goal:           strings.TrimSpace(bundle.Goal),
		ActiveEntities: bundle.ActiveEntities,
		PendingSteps:   bundle.PendingSteps,
		Scratch:        bundle.Scratch,
		Status:         normalizeWorkingStatus(bundle.Status),
		Policy:         policy,
	}).withInitialGoal("")
}

func normalizeWorkingStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "idle":
		return "idle"
	case "active", "running", "completed", "abandoned", "failed", "cancelled", "timed_out", "retained":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

// savePreparedWorkingMemory persists the initial working memory snapshot at run start.
func (s *Service) savePreparedWorkingMemory(ctx context.Context, runID, sessionKey string, prepared preparedRun) error {
	if s.config.WorkingStore == nil || prepared.WorkingMemory == nil {
		return nil
	}
	updated := prepared.WorkingMemory.withInitialGoal(prepared.UserMessage)
	_, err := s.config.WorkingStore.Save(ctx, WorkingMemorySnapshot{
		MemoryType:       "working",
		SessionKey:       sessionKey,
		RunID:            runID,
		Goal:             sanitize.Text(updated.Goal),
		EntitiesJSON:     sanitize.JSON(mustMarshalJSON(updated.ActiveEntities)),
		PendingStepsJSON: sanitize.JSON(mustMarshalJSON(updated.PendingSteps)),
		ScratchJSON:      sanitize.JSON(mustMarshalJSON(updated.Scratch)),
		Status:           normalizeWorkingStatus(updated.Status),
		SourceType:       "run",
		SourceID:         runID,
	})
	if err != nil {
		return err
	}
	return nil
}

// updateWorkingMemoryCheckpoint saves a checkpoint during tool execution.
func (s *Service) updateWorkingMemoryCheckpoint(ctx context.Context, sessionKey, runID, status, toolName, payload string) error {
	if s.config.WorkingStore == nil {
		return nil
	}
	working, err := s.loadWorkingMemory(ctx, sessionKey)
	if err != nil {
		return err
	}
	if working.Scratch == nil {
		working.Scratch = map[string]any{}
	}
	working.Status = normalizeWorkingStatus(status)
	if strings.TrimSpace(toolName) != "" {
		working.Scratch["last_tool"] = map[string]any{"name": toolName, "payload": sanitize.JSON(normalizeJSON(payload, "{}"))}
	}
	_, err = s.config.WorkingStore.Save(ctx, WorkingMemorySnapshot{
		MemoryType:       "working",
		SessionKey:       sessionKey,
		RunID:            runID,
		Goal:             sanitize.Text(working.Goal),
		EntitiesJSON:     sanitize.JSON(mustMarshalJSON(working.ActiveEntities)),
		PendingStepsJSON: sanitize.JSON(mustMarshalJSON(working.PendingSteps)),
		ScratchJSON:      sanitize.JSON(mustMarshalJSON(working.Scratch)),
		Status:           working.Status,
		SourceType:       "run",
		SourceID:         runID,
	})
	return err
}

// finalizeWorkingMemory applies the configured working memory policy at run terminal state.
func (s *Service) finalizeWorkingMemory(ctx context.Context, sessionKey, runID string, state commonv1.RunState, note string) error {
	if s.config.WorkingStore == nil || strings.TrimSpace(sessionKey) == "" {
		return nil
	}
	action := s.workingMemoryAction(state)
	if action == "clear" {
		err := s.config.WorkingStore.Clear(ctx, sessionKey)
		if err != nil && !errors.Is(err, ErrWorkingMemoryNotFound) {
			return err
		}
		return nil
	}
	working, err := s.loadWorkingMemory(ctx, sessionKey)
	if err != nil {
		return err
	}
	if working.Scratch == nil {
		working.Scratch = map[string]any{}
	}
	if trimmed := strings.TrimSpace(note); trimmed != "" {
		working.Scratch["final_note"] = sanitize.Text(trimmed)
	}
	working.Status = normalizeWorkingStatus(action)
	_, err = s.config.WorkingStore.Save(ctx, WorkingMemorySnapshot{
		MemoryType:       "working",
		SessionKey:       sessionKey,
		RunID:            runID,
		Goal:             sanitize.Text(working.Goal),
		EntitiesJSON:     sanitize.JSON(mustMarshalJSON(working.ActiveEntities)),
		PendingStepsJSON: sanitize.JSON(mustMarshalJSON(working.PendingSteps)),
		ScratchJSON:      sanitize.JSON(mustMarshalJSON(working.Scratch)),
		Status:           working.Status,
		SourceType:       string(runStateSourceType(state)),
		SourceID:         runID,
	})
	return err
}

func runStateSourceType(state commonv1.RunState) string {
	switch state {
	case commonv1.RunState_RUN_STATE_COMPLETED, commonv1.RunState_RUN_STATE_FAILED, commonv1.RunState_RUN_STATE_CANCELLED, commonv1.RunState_RUN_STATE_TIMED_OUT:
		return "run"
	default:
		return "system_event"
	}
}

func (s *Service) workingMemoryAction(state commonv1.RunState) string {
	switch state {
	case commonv1.RunState_RUN_STATE_COMPLETED:
		return strings.ToLower(strings.TrimSpace(s.config.WorkingPolicy.OnCompleted))
	case commonv1.RunState_RUN_STATE_CANCELLED:
		return strings.ToLower(strings.TrimSpace(s.config.WorkingPolicy.OnCancelled))
	case commonv1.RunState_RUN_STATE_TIMED_OUT:
		return strings.ToLower(strings.TrimSpace(s.config.WorkingPolicy.OnTimedOut))
	default:
		return strings.ToLower(strings.TrimSpace(s.config.WorkingPolicy.OnFailed))
	}
}

// saveTransientWorkingState persists short-lived execution state in the transient store.
func (s *Service) saveTransientWorkingState(ctx context.Context, runID, sessionKey, status string, scratch map[string]any) error {
	if s.config.TransientStore == nil {
		return nil
	}
	if scratch == nil {
		scratch = map[string]any{}
	}
	_, err := s.config.TransientStore.Save(ctx, TransientWorkingState{
		SessionKey:  sessionKey,
		RunID:       runID,
		Status:      normalizeWorkingStatus(status),
		ScratchJSON: mustMarshalJSON(scratch),
		UpdatedAt:   time.Now().UTC().Format(time.RFC3339Nano),
	}, s.config.TransientTTL)
	return err
}

// finalizeTransientWorkingState applies the working memory policy to transient state.
func (s *Service) finalizeTransientWorkingState(ctx context.Context, sessionKey, runID string, state commonv1.RunState, note string) error {
	if s.config.TransientStore == nil {
		return nil
	}
	action := s.workingMemoryAction(state)
	switch action {
	case "clear":
		err := s.config.TransientStore.Clear(ctx, sessionKey, runID)
		if err != nil && !errors.Is(err, ErrTransientWorkingStateNotFound) {
			return err
		}
		return nil
	default:
		scratch := map[string]any{}
		if trimmed := strings.TrimSpace(note); trimmed != "" {
			scratch["final_note"] = trimmed
		}
		return s.saveTransientWorkingState(ctx, runID, sessionKey, action, scratch)
	}
}

// loadWorkingMemory retrieves the current working memory context for a session.
func (s *Service) loadWorkingMemory(ctx context.Context, sessionKey string) (*workingMemoryContext, error) {
	working := &workingMemoryContext{Status: "idle", Policy: s.config.WorkingPolicy, Scratch: map[string]any{}}
	if s.config.WorkingStore == nil {
		return working, nil
	}
	snapshot, err := s.config.WorkingStore.Get(ctx, sessionKey)
	if err != nil {
		if errors.Is(err, ErrWorkingMemoryNotFound) {
			return working, nil
		}
		return nil, fmt.Errorf("load working memory: %w", err)
	}
	working.Goal = strings.TrimSpace(snapshot.Goal)
	working.Status = normalizeWorkingStatus(snapshot.Status)
	working.ActiveEntities = decodeJSONValue(snapshot.EntitiesJSON, map[string]any{})
	working.PendingSteps = decodeJSONValue(snapshot.PendingStepsJSON, []any{})
	working.Scratch = decodeJSONObject(snapshot.ScratchJSON)
	return working, nil
}
