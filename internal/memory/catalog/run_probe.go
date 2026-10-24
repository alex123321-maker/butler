package catalog

import (
	"context"
	"errors"
	"time"
)

// RunSourceProbe contains fixed phases and storage health only. Source IDs,
// query text/parameters, error messages and credential material are excluded.
type RunSourceProbe struct {
	Phase        string `json:"phase"`
	Millis       int64  `json:"millis"`
	ErrorKind    string `json:"error_kind,omitempty"`
	SQLState     string `json:"sqlstate,omitempty"`
	PoolAcquired int32  `json:"pool_acquired"`
	PoolIdle     int32  `json:"pool_idle"`
	PoolTotal    int32  `json:"pool_total"`
}

type runSourceObserverKey struct{}
type runSourceProbeKey struct{}
type runSourceProbeState struct {
	ctx     context.Context
	store   *Store
	observe func(RunSourceProbe)
	phase   string
	started time.Time
}

// WithRunSourceObserver is a trusted diagnostic seam, not a model/tool option.
// The observer must return promptly and handle concurrent guard observations.
func WithRunSourceObserver(ctx context.Context, observe func(RunSourceProbe)) context.Context {
	return context.WithValue(ctx, runSourceObserverKey{}, observe)
}

func beginRunSourceProbe(ctx context.Context, s *Store) (context.Context, func(error)) {
	observe, _ := ctx.Value(runSourceObserverKey{}).(func(RunSourceProbe))
	if observe == nil {
		return ctx, func(error) {}
	}
	p := &runSourceProbeState{ctx: ctx, store: s, observe: observe}
	return context.WithValue(ctx, runSourceProbeKey{}, p), p.finish
}

func runSourcePhase(ctx context.Context, phase string) {
	if p, _ := ctx.Value(runSourceProbeKey{}).(*runSourceProbeState); p != nil {
		p.finish(nil)
		p.phase, p.started = phase, time.Now()
	}
}

func (p *runSourceProbeState) finish(err error) {
	if p.phase == "" {
		return
	}
	stat := p.store.RunObservationPool(p.ctx).Stat()
	o := RunSourceProbe{Phase: p.phase, Millis: time.Since(p.started).Milliseconds(), PoolAcquired: stat.AcquiredConns(), PoolIdle: stat.IdleConns(), PoolTotal: stat.TotalConns()}
	if err != nil {
		o.ErrorKind = "denied_or_storage_error"
		if errors.Is(err, context.DeadlineExceeded) || p.ctx.Err() == context.DeadlineExceeded {
			o.ErrorKind = "deadline"
		} else if errors.Is(err, context.Canceled) || p.ctx.Err() == context.Canceled {
			o.ErrorKind = "cancelled"
		}
		var state interface{ SQLState() string }
		if errors.As(err, &state) {
			code := state.SQLState()
			if len(code) == 5 {
				valid := true
				for _, c := range code {
					valid = valid && (c >= '0' && c <= '9' || c >= 'A' && c <= 'Z')
				}
				if valid {
					o.SQLState = code
				}
			}
		}
	}
	p.phase = ""
	p.observe(o)
}
