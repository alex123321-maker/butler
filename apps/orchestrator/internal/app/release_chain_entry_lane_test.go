package app

import (
	"context"
	"sync"
	"time"
)

// This fixture-only lane serializes actual provider entries without holding a
// database transaction or lock while waiting or calling the provider. Durable
// window/history checks remain authoritative after acquiring the lane.
type releaseChainHighEntryLane struct {
	gate   chan struct{}
	mu     sync.Mutex
	holder *releaseChainHighLaneHolder
}

type releaseChainHighLaneHolder struct {
	ctx   context.Context
	check func(context.Context) error
}

func releaseChainHighEntryLaneFor(policy releaseChainHighWindowPolicy) *releaseChainHighEntryLane {
	if !policy.SerializedEntries {
		return nil
	}
	return &releaseChainHighEntryLane{gate: make(chan struct{}, 1)}
}

func (l *releaseChainHighEntryLane) acquire(ctx context.Context, current func(context.Context) error) (func(), error) {
	if l == nil || l.gate == nil || current == nil {
		return nil, errNativeFixtureDenied
	}
	check := func(owned context.Context, f func(context.Context) error) error {
		guard, cancel := context.WithTimeout(owned, 2*time.Second)
		defer cancel()
		return f(guard)
	}
	timer := time.NewTicker(50 * time.Millisecond)
	defer timer.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := check(ctx, current); err != nil {
			return nil, err
		}
		select {
		case l.gate <- struct{}{}:
			l.mu.Lock()
			l.holder = &releaseChainHighLaneHolder{ctx: ctx, check: current}
			l.mu.Unlock()
			var once sync.Once
			release := func() { once.Do(func() { l.mu.Lock(); l.holder = nil; l.mu.Unlock(); <-l.gate }) }
			if err := check(ctx, current); err != nil {
				release()
				return nil, err
			}
			return release, nil
		default:
		}
		l.mu.Lock()
		holder := l.holder
		l.mu.Unlock()
		if holder != nil {
			// Observe the holder with its own exact lease/token. This context
			// is never supplied to the queued caller or its provider invocation.
			if err := check(holder.ctx, holder.check); err != nil {
				l.mu.Lock()
				stillHeld := l.holder == holder
				l.mu.Unlock()
				if stillHeld {
					return nil, err
				}
				// A released/completed holder cannot deny a live queued caller.
				// Retry its own proof and the durable admission checks.
				continue
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func releaseChainV9Policy() releaseChainHighWindowPolicy {
	p := releaseChainV8Policy()
	p.SerializedEntries = true
	return p
}
