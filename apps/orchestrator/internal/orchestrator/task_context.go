package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/memory/sanitize"
	"github.com/butler/butler/internal/security/content"
	"github.com/butler/butler/internal/transport"
)

func (s *Service) attachTaskContext(ctx context.Context, sessionKey string, prepared *preparedRun) error {
	id := firstString(prepared.InputPayload, "task_id")
	if s.config.Tasks == nil {
		if id != "" {
			return fmt.Errorf("task service is not configured")
		}
		return nil
	}
	// Session transcript/profile/episodes remain available. Task goals and plans
	// never inherit the old session-scoped working snapshot, even in conversation.
	delete(prepared.MemoryBundle, "working")
	prepared.WorkingMemory = &workingMemoryContext{Status: "idle", Scratch: map[string]any{}, Policy: s.config.WorkingPolicy}
	if id == "" {
		return nil
	}
	record, err := s.config.Tasks.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("load explicit task: %w", err)
	}
	if record.SessionKey != sessionKey || record.OwnerID != prepared.SessionUserID || record.Status != "active" {
		return fmt.Errorf("%w: task is not active for this session owner", task.ErrConflict)
	}
	prepared.Task = &record
	if err := s.attachOwnerGoalContext(ctx, record.TaskID, prepared); err != nil {
		return err
	}
	prepared.WorkingMemory = &workingMemoryContext{Goal: record.Goal, PendingSteps: record.Working.Plan, ActiveEntities: map[string]any{}, Scratch: record.Working.Scratch, Status: "active", Policy: s.config.WorkingPolicy}
	prepared.MemoryBundle["working"] = prepared.WorkingMemory.bundleMap()
	ref := fmt.Sprintf("task:%s:revision:%d:constraints:%d", record.TaskID, record.Revision, record.ConstraintsRevision)
	policy := content.Instruction("Butler task authority (authenticated durable owner state; a final model response only ends this attempt):\n"+sanitize.JSON(mustMarshalJSON(map[string]any{
		"task_id": record.TaskID, "revision": record.Revision, "goal": sanitize.Text(record.Goal), "constraints": record.Constraints, "constraints_revision": record.ConstraintsRevision,
		"completion_criteria": record.CompletionCriteria, "execution_policy": record.ExecutionPolicy,
	})), content.Origin{Kind: content.OwnerPolicy, SourceRefs: []string{ref}, Sensitivity: content.Private})
	working := content.Data(map[string]any{"task_working": record.Working, "task_source": record.Source}, content.Origin{Kind: content.MemoryDerived, SourceRefs: []string{ref}, Sensitivity: content.Private})
	prepared.InputItems = append([]transport.InputItem{policy, working}, prepared.InputItems...)
	return nil
}

func (s *Service) checkpointTaskWorking(ctx context.Context, runID, status, toolName, payload string) error {
	record, err := s.config.Tasks.GetByRun(ctx, runID)
	if errors.Is(err, task.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	state := record.Working
	if state.Scratch == nil {
		state.Scratch = map[string]any{}
	}
	state.Scratch["run_stage"] = status
	if strings.TrimSpace(toolName) != "" {
		state.Scratch["last_tool"] = map[string]any{"name": toolName, "payload": sanitize.JSON(normalizeJSON(payload, "{}"))}
	}
	_, err = s.config.Tasks.CheckpointRun(ctx, runID, record.WorkingRevision, state)
	return err
}
