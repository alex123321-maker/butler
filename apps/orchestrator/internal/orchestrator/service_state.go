package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/observability"
	runservice "github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/transport"
)

func (s *Service) persistProviderSessionRef(ctx context.Context, current *sessionv1.RunRecord, ref *transport.ProviderSessionRef, runLog *slog.Logger) (*sessionv1.RunRecord, error) {
	if ref == nil {
		return current, nil
	}
	encoded, err := transport.MarshalProviderSessionRef(ref)
	if err != nil {
		return current, fmt.Errorf("marshal provider session ref: %w", err)
	}
	if strings.TrimSpace(encoded) == strings.TrimSpace(current.GetProviderSessionRef()) {
		return current, nil
	}
	updated, err := s.runs.PersistProviderSessionRef(ctx, current.GetRunId(), encoded)
	if err != nil {
		return current, fmt.Errorf("persist provider session ref: %w", err)
	}
	runLog.Info("provider session ref persisted", slog.String("run_id", current.GetRunId()))
	return updated, nil
}

type leaseRenewerKey struct{}

// This per-run handle only joins supervision. It neither cancels the owning
// execution context nor grants permission for terminal writes. Those writes
// retain their original context, token and database fences.
type leaseRenewerControl struct {
	requested chan struct{}
	done      chan struct{}
	once      sync.Once
}

func (c *leaseRenewerControl) quiesce() {
	c.once.Do(func() { close(c.requested) })
	<-c.done
}

func quiesceLeaseRenewer(ctx context.Context) {
	if control, ok := ctx.Value(leaseRenewerKey{}).(*leaseRenewerControl); ok {
		control.quiesce()
	}
}

func (s *Service) startLeaseRenewer(parent context.Context, lease session.LeaseRecord, runLog *slog.Logger) (context.Context, context.CancelFunc, <-chan error) {
	return s.startLeaseRenewerWithTaskGate(parent, lease, runLog, nil)
}

// Only canonical interaction recovery supplies taskReady. Lease observation,
// renewal and cancellation remain active while its waiting->active transaction
// runs; only the active-only Task check waits for the resolver's success.
func (s *Service) startLeaseRenewerWithTaskGate(parent context.Context, lease session.LeaseRecord, runLog *slog.Logger, taskReady <-chan struct{}) (context.Context, context.CancelFunc, <-chan error) {
	runCtx, cancelCause := context.WithCancelCause(parent)
	control := &leaseRenewerControl{requested: make(chan struct{}), done: make(chan struct{})}
	runCtx = context.WithValue(runCtx, leaseRenewerKey{}, control)
	cancel := func() { cancelCause(context.Canceled) }
	errCh := make(chan error, 1)
	ttl := time.Duration(s.config.LeaseTTL) * time.Second
	if ttl <= 0 {
		ttl = time.Minute
	}
	interval := ttl / 2
	if interval < time.Second {
		interval = time.Second
	}
	go func() {
		defer close(control.done)
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		nextRenew := time.Now().Add(interval)
		for {
			select {
			case <-control.requested:
				return
			default:
			}
			select {
			case <-runCtx.Done():
				return
			case <-control.requested:
				return
			case <-ticker.C:
				if lease.FenceEpoch > 0 {
					checkCtx, checkCancel := context.WithTimeout(runCtx, 750*time.Millisecond)
					var current session.LeaseRecord
					var checkErr error
					checkPhase := "lease"
					checkStarted := time.Now()
					if reader, ok := s.leases.(interface {
						GetLease(context.Context, string) (session.LeaseRecord, error)
					}); ok {
						current, checkErr = reader.GetLease(checkCtx, lease.LeaseID)
					} else {
						checkErr = ownership.ErrStale
					}
					checkTask := true
					if taskReady != nil {
						select {
						case <-taskReady:
						default:
							checkTask = false
						}
					}
					if checkErr == nil && checkTask {
						if guard, ok := s.config.Tasks.(interface {
							ValidateRunAuthority(context.Context, string) error
						}); ok {
							checkPhase = "task_authority"
							checkErr = guard.ValidateRunAuthority(checkCtx, lease.RunID)
						}
					}
					checkCancel()
					// A requested terminal join does not discard an in-flight
					// check or its failure. Own terminal mutations wait for done.
					if runCtx.Err() != nil {
						return
					}
					if checkErr != nil || current.RunID != lease.RunID || current.SessionKey != lease.SessionKey || current.FenceEpoch != lease.FenceEpoch {
						reason := "execution_ownership_changed"
						if checkErr != nil {
							reason = "execution_authority_check_failed"
						}
						classification := "changed"
						if checkErr != nil {
							classification = "unavailable"
							if errors.Is(checkErr, context.DeadlineExceeded) {
								classification = "deadline"
							} else if errors.Is(checkErr, session.ErrLeaseNotFound) || errors.Is(checkErr, ownership.ErrStale) {
								classification = "stale"
							}
						}
						fields := []any{slog.String("phase", checkPhase), slog.String("classification", classification), slog.Int64("elapsed_ms", time.Since(checkStarted).Milliseconds())}
						var observation *session.LeaseObservationError
						if errors.As(checkErr, &observation) {
							fields = append(fields, slog.String("lease_phase", observation.Phase), slog.Int("pool_acquired", int(observation.Acquired)), slog.Int("pool_idle", int(observation.Idle)), slog.Int("pool_total", int(observation.Total)))
						}
						runLog.Warn("execution authority observation stopped", fields...)
						cause := &ownedCancellation{reason: reason, failure: ownership.ErrStale}
						if checkPhase == "lease" && classification == "deadline" {
							cause.reason = "execution_authority_observation_timeout"
							cause.observationTimeout = true
						}
						select {
						case errCh <- cause:
						default:
						}
						cancelCause(cause)
						cleanup, cleanupCancel := context.WithTimeout(context.Background(), 2*time.Second)
						_, cancelErr := s.provider.CancelRun(cleanup, transport.CancelRunRequest{RunID: lease.RunID, Reason: "owner authority revoked or unavailable"})
						cleanupCancel()
						if cancelErr != nil {
							runLog.Warn("provider cancellation unavailable", slog.String("run_id", lease.RunID))
						}
						return
					}
				}
				if time.Now().Before(nextRenew) {
					continue
				}
				callCtx, callCancel := context.WithTimeout(runCtx, 5*time.Second)
				_, err := s.leases.RenewLease(callCtx, lease.LeaseID, ttl)
				callCancel()
				if runCtx.Err() != nil {
					return
				}
				nextRenew = time.Now().Add(interval)
				if err != nil {
					cause := &ownedCancellation{reason: "execution_lease_renewal_failed", failure: errLeaseRenewalFailed}
					runLog.Error("lease renew failed", slog.String("lease_id", lease.LeaseID), slog.String("reason", cause.reason))
					select {
					case errCh <- cause:
					default:
					}
					cancelCause(cause)
					return
				}
			}
		}
	}()
	return runCtx, func() {
		cancel()
		control.quiesce()
	}, errCh
}

func drainRenewError(errCh <-chan error) error {
	select {
	case err := <-errCh:
		return err
	default:
		return nil
	}
}

func (s *Service) transition(ctx context.Context, runID string, from, to commonv1.RunState, leaseID, errorType, errorMessage string) (*sessionv1.RunRecord, error) {
	resp, err := s.runs.TransitionRun(ctx, &sessionv1.UpdateRunStateRequest{
		RunId:        runID,
		FromState:    from,
		ToState:      to,
		LeaseId:      leaseID,
		ErrorType:    toErrorClass(errorType),
		ErrorMessage: errorMessage,
	})
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	fromStr := runStateString(from)
	toStr := runStateString(to)
	if s.config.TransitionLogger != nil {
		metaPayload := map[string]any{}
		if leaseID != "" {
			metaPayload["lease_id"] = leaseID
		}
		if errorType != "" {
			metaPayload["error_type"] = errorType
		}
		if errorMessage != "" {
			metaPayload["error_message"] = errorMessage
		}
		if logErr := s.config.TransitionLogger.InsertTransition(ctx, runservice.StateTransition{
			RunID:          runID,
			FromState:      fromStr,
			ToState:        toStr,
			TriggeredBy:    "orchestrator",
			MetadataJSON:   mustMarshalJSON(metaPayload),
			TransitionedAt: now,
		}); logErr != nil {
			s.log.Warn("failed to log state transition", slog.String("run_id", runID), slog.String("error", logErr.Error()))
		}
	}
	s.emitEvent(ctx, runID, resp.GetSessionKey(), observability.EventStateTransition, map[string]any{"from_state": fromStr, "to_state": toStr})
	return resp, nil
}
