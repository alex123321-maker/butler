package workspaceexec

import (
	"context"
	"sync"
)

// Local physical exclusion is per opaque copy, acquired before any PG locks.
// RootOwner supplies exclusion across processes. The map retains only live
// callers; it is owned by one Runtime, with no package-global coordination.
type copyLane struct {
	token chan struct{}
	users int
}
type copyLanes struct {
	mu    sync.Mutex
	lanes map[string]*copyLane
}

func (l *copyLanes) acquire(ctx context.Context, id string, try bool) (func(), error) {
	if !opaqueID(id) || ctx.Err() != nil {
		return nil, ErrDenied
	}
	l.mu.Lock()
	if l.lanes == nil {
		l.lanes = map[string]*copyLane{}
	}
	lane := l.lanes[id]
	if lane == nil {
		lane = &copyLane{token: make(chan struct{}, 1)}
		lane.token <- struct{}{}
		l.lanes[id] = lane
	}
	lane.users++
	l.mu.Unlock()
	drop := func() {
		l.mu.Lock()
		lane.users--
		if lane.users == 0 {
			delete(l.lanes, id)
		}
		l.mu.Unlock()
	}
	if try {
		select {
		case <-lane.token:
		default:
			drop()
			return nil, ErrDenied
		}
	} else {
		select {
		case <-ctx.Done():
			drop()
			return nil, ErrDenied
		case <-lane.token:
		}
	}
	return func() { lane.token <- struct{}{}; drop() }, nil
}
