package housekeeping

import (
	"context"
	"testing"
	"time"
)

type stubWorkingPruner struct{ deleted int64 }

func (s stubWorkingPruner) DeleteStaleBefore(context.Context, time.Time) (int64, error) {
	return s.deleted, nil
}

type stubProfilePruner struct{ deleted int64 }

func (s stubProfilePruner) DeleteInactiveBefore(context.Context, time.Time) (int64, error) {
	return s.deleted, nil
}

type stubEpisodePruner struct{ deleted int64 }

func (s stubEpisodePruner) Prune(context.Context, time.Time, int) (int64, error) {
	return s.deleted, nil
}

type stubChunkPruner struct{ deleted int64 }

func (s stubChunkPruner) Prune(context.Context, time.Time, int) (int64, error) { return s.deleted, nil }

func TestRunOnceAggregatesPruneResults(t *testing.T) {
	t.Parallel()
	svc := New(stubWorkingPruner{deleted: 1}, stubProfilePruner{deleted: 2}, stubEpisodePruner{deleted: 3}, stubChunkPruner{deleted: 4}, Config{})
	result, err := svc.RunOnce(context.Background(), time.Now().UTC())
	if err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if result.WorkingDeleted != 1 || result.ProfileDeleted != 2 || result.EpisodeDeleted != 3 || result.ChunkDeleted != 4 {
		t.Fatalf("unexpected result %+v", result)
	}
}
