package app

import (
	"context"
	"errors"
	"time"

	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	"github.com/butler/butler/internal/memory/chunks"
	"github.com/butler/butler/internal/memory/episodic"
	"github.com/butler/butler/internal/memory/profile"
	memoryservice "github.com/butler/butler/internal/memory/service"
	"github.com/butler/butler/internal/memory/working"
)

type profileStoreAdapter struct{ store *profile.Store }

func (a profileStoreAdapter) GetByScope(ctx context.Context, scopeType, scopeID string) ([]memoryservice.ProfileEntry, error) {
	entries, err := a.store.GetByScope(ctx, scopeType, scopeID)
	if err != nil {
		return nil, err
	}
	result := make([]memoryservice.ProfileEntry, 0, len(entries))
	for _, entry := range entries {
		result = append(result, entry)
	}
	return result, nil
}

type episodicStoreAdapter struct{ store *episodic.Store }

type memoryEpisodeExactMatch struct{ episodic.Episode }

func (m memoryEpisodeExactMatch) EpisodeSummary() string   { return m.Summary }
func (m memoryEpisodeExactMatch) EpisodeDistance() float64 { return 0.25 }

func (a episodicStoreAdapter) Search(ctx context.Context, scopeType, scopeID string, embedding []float32, limit int) ([]memoryservice.Episode, error) {
	entries, err := a.store.Search(ctx, scopeType, scopeID, embedding, limit)
	if err != nil {
		return nil, err
	}
	result := make([]memoryservice.Episode, 0, len(entries))
	for _, entry := range entries {
		result = append(result, entry)
	}
	return result, nil
}

func (a episodicStoreAdapter) FindBySummary(ctx context.Context, scopeType, scopeID, summary string) ([]memoryservice.Episode, error) {
	entries, err := a.store.FindBySummary(ctx, scopeType, scopeID, summary)
	if err != nil {
		return nil, err
	}
	result := make([]memoryservice.Episode, 0, len(entries))
	for _, entry := range entries {
		result = append(result, memoryEpisodeExactMatch{Episode: entry})
	}
	return result, nil
}

type chunkStoreAdapter struct{ store *chunks.Store }

type memoryChunkExactMatch struct{ chunks.Chunk }

func (m memoryChunkExactMatch) ChunkTitle() string     { return m.Title }
func (m memoryChunkExactMatch) ChunkSummary() string   { return m.Summary }
func (m memoryChunkExactMatch) ChunkDistance() float64 { return 0.25 }

func (a chunkStoreAdapter) Search(ctx context.Context, scopeType, scopeID string, embedding []float32, limit int) ([]memoryservice.Chunk, error) {
	entries, err := a.store.Search(ctx, scopeType, scopeID, embedding, limit)
	if err != nil {
		return nil, err
	}
	result := make([]memoryservice.Chunk, 0, len(entries))
	for _, entry := range entries {
		result = append(result, entry)
	}
	return result, nil
}

func (a chunkStoreAdapter) FindByTitle(ctx context.Context, scopeType, scopeID, title string, limit int) ([]memoryservice.Chunk, error) {
	entries, err := a.store.FindByTitle(ctx, scopeType, scopeID, title, limit)
	if err != nil {
		return nil, err
	}
	result := make([]memoryservice.Chunk, 0, len(entries))
	for _, entry := range entries {
		result = append(result, memoryChunkExactMatch{Chunk: entry})
	}
	return result, nil
}

type workingStoreAdapter struct{ store *working.Store }

func (a workingStoreAdapter) Get(ctx context.Context, sessionKey string) (flow.WorkingMemorySnapshot, error) {
	snapshot, err := a.store.Get(ctx, sessionKey)
	if err != nil {
		if errors.Is(err, working.ErrSnapshotNotFound) {
			return flow.WorkingMemorySnapshot{}, flow.ErrWorkingMemoryNotFound
		}
		return flow.WorkingMemorySnapshot{}, err
	}
	return flow.WorkingMemorySnapshot{
		MemoryType:       snapshot.MemoryType,
		SessionKey:       snapshot.SessionKey,
		RunID:            snapshot.RunID,
		Goal:             snapshot.Goal,
		EntitiesJSON:     snapshot.EntitiesJSON,
		PendingStepsJSON: snapshot.PendingStepsJSON,
		ScratchJSON:      snapshot.ScratchJSON,
		Status:           snapshot.Status,
		SourceType:       snapshot.SourceType,
		SourceID:         snapshot.SourceID,
		ProvenanceJSON:   snapshot.ProvenanceJSON,
	}, nil
}

func (a workingStoreAdapter) Save(ctx context.Context, snapshot flow.WorkingMemorySnapshot) (flow.WorkingMemorySnapshot, error) {
	saved, err := a.store.Save(ctx, working.Snapshot{
		MemoryType:       snapshot.MemoryType,
		SessionKey:       snapshot.SessionKey,
		RunID:            snapshot.RunID,
		Goal:             snapshot.Goal,
		EntitiesJSON:     snapshot.EntitiesJSON,
		PendingStepsJSON: snapshot.PendingStepsJSON,
		ScratchJSON:      snapshot.ScratchJSON,
		Status:           snapshot.Status,
		SourceType:       snapshot.SourceType,
		SourceID:         snapshot.SourceID,
		ProvenanceJSON:   snapshot.ProvenanceJSON,
	})
	if err != nil {
		return flow.WorkingMemorySnapshot{}, err
	}
	return flow.WorkingMemorySnapshot{
		MemoryType:       saved.MemoryType,
		SessionKey:       saved.SessionKey,
		RunID:            saved.RunID,
		Goal:             saved.Goal,
		EntitiesJSON:     saved.EntitiesJSON,
		PendingStepsJSON: saved.PendingStepsJSON,
		ScratchJSON:      saved.ScratchJSON,
		Status:           saved.Status,
		SourceType:       saved.SourceType,
		SourceID:         saved.SourceID,
		ProvenanceJSON:   saved.ProvenanceJSON,
	}, nil
}

func (a workingStoreAdapter) Clear(ctx context.Context, sessionKey string) error {
	err := a.store.Clear(ctx, sessionKey)
	if err != nil && errors.Is(err, working.ErrSnapshotNotFound) {
		return flow.ErrWorkingMemoryNotFound
	}
	return err
}

type transientWorkingStoreAdapter struct{ store *working.TransientStore }

func (a transientWorkingStoreAdapter) Get(ctx context.Context, sessionKey, runID string) (flow.TransientWorkingState, error) {
	state, err := a.store.Get(ctx, sessionKey, runID)
	if err != nil {
		if errors.Is(err, working.ErrTransientStateNotFound) {
			return flow.TransientWorkingState{}, flow.ErrTransientWorkingStateNotFound
		}
		return flow.TransientWorkingState{}, err
	}
	return flow.TransientWorkingState{
		SessionKey:  state.SessionKey,
		RunID:       state.RunID,
		Status:      state.Status,
		ScratchJSON: state.ScratchJSON,
		UpdatedAt:   state.UpdatedAt,
	}, nil
}

func (a transientWorkingStoreAdapter) Save(ctx context.Context, state flow.TransientWorkingState, ttl time.Duration) (flow.TransientWorkingState, error) {
	saved, err := a.store.Save(ctx, working.TransientState{
		SessionKey:  state.SessionKey,
		RunID:       state.RunID,
		Status:      state.Status,
		ScratchJSON: state.ScratchJSON,
		UpdatedAt:   state.UpdatedAt,
	}, ttl)
	if err != nil {
		return flow.TransientWorkingState{}, err
	}
	return flow.TransientWorkingState{
		SessionKey:  saved.SessionKey,
		RunID:       saved.RunID,
		Status:      saved.Status,
		ScratchJSON: saved.ScratchJSON,
		UpdatedAt:   saved.UpdatedAt,
	}, nil
}

func (a transientWorkingStoreAdapter) Clear(ctx context.Context, sessionKey, runID string) error {
	err := a.store.Clear(ctx, sessionKey, runID)
	if err != nil && errors.Is(err, working.ErrTransientStateNotFound) {
		return flow.ErrTransientWorkingStateNotFound
	}
	return err
}
