package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/butler/butler/internal/memory/catalog"
)

type experienceSourceTimingKey struct{}
type experienceSourceTimingRecord struct {
	Phase        string `json:"phase"`
	Micros       int64  `json:"micros"`
	Failure      string `json:"failure,omitempty"`
	PoolAcquired int32  `json:"pool_acquired"`
	PoolIdle     int32  `json:"pool_idle"`
}
type experienceSourceTimings struct {
	mu      sync.Mutex
	runtime *experienceNativeRuntime
	records []experienceSourceTimingRecord
}

func newExperienceSourceTimings(t *testing.T, r *experienceNativeRuntime, directory string) *experienceSourceTimings {
	t.Helper()
	x := &experienceSourceTimings{runtime: r}
	if proof := os.Getenv("BUTLER_EXPERIENCE_PROCESS_PROOF"); filepath.IsAbs(proof) {
		t.Cleanup(func() {
			x.mu.Lock()
			raw, err := json.Marshal(x.records)
			x.mu.Unlock()
			if err != nil || os.MkdirAll(proof, 0700) != nil {
				t.Error("source timing evidence unavailable")
				return
			}
			file, err := os.OpenFile(filepath.Join(proof, "source-timing-"+experienceDigest(directory)+".json"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if err != nil {
				t.Error("source timing evidence already used/unavailable")
				return
			}
			_, writeErr := file.Write(raw)
			closeErr := file.Close()
			if writeErr != nil || closeErr != nil {
				t.Error("source timing evidence write failed")
			}
		})
	}
	return x
}

func (x *experienceSourceTimings) record(ctx context.Context, phase string, started time.Time, err error) {
	if x == nil {
		return
	}
	stat := x.runtime.pool.Stat()
	r := experienceSourceTimingRecord{Phase: phase, Micros: time.Since(started).Microseconds(), PoolAcquired: stat.AcquiredConns(), PoolIdle: stat.IdleConns()}
	if err != nil {
		r.Failure = "denied_or_storage_error"
		if errors.Is(err, context.DeadlineExceeded) || ctx.Err() == context.DeadlineExceeded {
			r.Failure = "deadline"
		} else if errors.Is(err, context.Canceled) || ctx.Err() == context.Canceled {
			r.Failure = "cancelled"
		}
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	if len(x.records) < 16384 {
		x.records = append(x.records, r)
	}
}

// Transparent metadata-only observations of the unchanged production methods.
// They preserve deadlines, contexts, errors and ordering, and write no files
// until test cleanup. No SQL, arguments, bodies, IDs or raw errors are recorded.
type experienceTimedCatalog struct {
	*catalog.Store
	timing *experienceSourceTimings
}

func (s experienceTimedCatalog) CheckRunSources(ctx context.Context, run string) (err error) {
	start := time.Now()
	defer func() { s.timing.record(ctx, "base_catalog", start, err) }()
	ctx = catalog.WithRunSourceObserver(ctx, func(p catalog.RunSourceProbe) {
		s.timing.mu.Lock()
		defer s.timing.mu.Unlock()
		if len(s.timing.records) < 16384 {
			s.timing.records = append(s.timing.records, experienceSourceTimingRecord{Phase: "catalog." + p.Phase, Micros: p.Millis * 1000, Failure: p.ErrorKind, PoolAcquired: p.PoolAcquired, PoolIdle: p.PoolIdle})
		}
	})
	return s.Store.CheckRunSources(ctx, run)
}

type experienceTimedRunSources struct {
	experienceRunSources
	timing *experienceSourceTimings
}

func (s experienceTimedRunSources) CheckRunSources(ctx context.Context, run string) (err error) {
	start := time.Now()
	defer func() { s.timing.record(ctx, "compound_source", start, err) }()
	return s.experienceRunSources.CheckRunSources(context.WithValue(ctx, experienceSourceTimingKey{}, true), run)
}
