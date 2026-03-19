package orchestrator

import (
	"context"

	memoryservice "github.com/butler/butler/internal/memory/service"
)

// memoryBundleWorkingStore adapts WorkingMemoryStore to the memoryservice.WorkingStore interface.
// This adapter is necessary because WorkingMemoryStore has Save and Clear methods
// that the read-only memoryservice.WorkingStore does not.
type memoryBundleWorkingStore struct{ store WorkingMemoryStore }

func (a memoryBundleWorkingStore) Get(ctx context.Context, sessionKey string) (memoryservice.WorkingSnapshot, error) {
	if a.store == nil {
		return memoryservice.WorkingSnapshot{}, ErrWorkingMemoryNotFound
	}
	snapshot, err := a.store.Get(ctx, sessionKey)
	if err != nil {
		return memoryservice.WorkingSnapshot{}, err
	}
	return memoryservice.WorkingSnapshot{Goal: snapshot.Goal, EntitiesJSON: snapshot.EntitiesJSON, PendingStepsJSON: snapshot.PendingStepsJSON, ScratchJSON: snapshot.ScratchJSON, Status: snapshot.Status}, nil
}
