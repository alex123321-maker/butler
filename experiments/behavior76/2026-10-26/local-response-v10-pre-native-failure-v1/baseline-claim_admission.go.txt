package input

import (
	"context"
	"errors"
	"log/slog"
)

// ErrClaimAdmissionClosed stops this worker's claim loop. Existing claimed
// executions finish under their original context, authority and deadlines.
var ErrClaimAdmissionClosed = errors.New("input claim admission closed")

// NewServiceWithClaimAdmission installs an immutable trusted fence around queue
// expiry and claim acquisition. It grants no execution authority. Model/input
// data cannot install it. A successful callback must return a release function.
// NewService retains its ordinary unfenced lifecycle.
func NewServiceWithClaimAdmission(store *Store, sessions SessionWriter, executor Executor, runs RunReader, log *slog.Logger, admit func(context.Context) (func(), error)) (*Service, error) {
	if admit == nil {
		return nil, errors.New("claim admission callback required")
	}
	s := NewService(store, sessions, executor, runs, log)
	s.claimAdmission = admit
	s.claimWorkersDone = make(chan struct{})
	s.claimWorkersFailed = make(chan struct{})
	return s, nil
}

func (s *Service) claim(ctx context.Context) (Record, error) {
	if s.claimAdmission != nil {
		if err := s.ClaimAdmissionError(); err != nil {
			return Record{}, err
		}
		release, err := s.claimAdmission(ctx)
		if err != nil {
			return Record{}, err
		}
		if release == nil {
			return Record{}, errors.New("claim admission returned no release")
		}
		defer release()
		// A different worker may fail while this callback waits for its fence.
		// Recheck the broadcast under the acquired fence before queue writes.
		if err := s.ClaimAdmissionError(); err != nil {
			return Record{}, err
		}
	}
	if err := s.store.ExpireQueued(ctx); err != nil && ctx.Err() == nil {
		if s.claimAdmission != nil {
			return Record{}, err
		}
		s.log.Warn("expire queued inputs", "error", err.Error())
	}
	return s.store.Claim(ctx, s.claimTTL)
}

// These immutable broadcast signals expose worker lifecycle, not authority.
// Both are nil for ordinary construction. Failure does not cancel an already
// executing process; the caller still joins workers before claiming quiescence.
func (s *Service) ClaimWorkersDone() <-chan struct{}     { return s.claimWorkersDone }
func (s *Service) ClaimAdmissionFailed() <-chan struct{} { return s.claimWorkersFailed }
func (s *Service) ClaimAdmissionError() error {
	s.claimWorkersMu.Lock()
	defer s.claimWorkersMu.Unlock()
	return s.claimWorkersErr
}

// WaitClaimWorkers joins actual claim/process goroutines, including owning
// finish, without cancelling their execution contexts. Run and maintenance keep
// their usual parent context. Call only after closing the installed claim fence.
func (s *Service) WaitClaimWorkers(ctx context.Context) error {
	if s.claimWorkersDone == nil {
		return errors.New("claim admission lifecycle not configured")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.claimWorkersDone:
	}
	s.claimWorkersMu.Lock()
	defer s.claimWorkersMu.Unlock()
	return s.claimWorkersErr
}
