package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/observability"
	runservice "github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	"github.com/butler/butler/internal/transport"
)

func (s *Service) attachReusableProviderSessionRef(ctx context.Context, current *sessionv1.RunRecord, runLog *slog.Logger) *sessionv1.RunRecord {
	if current == nil || s.runs == nil {
		return current
	}
	if strings.TrimSpace(current.GetProviderSessionRef()) != "" {
		return current
	}
	runs, err := s.runs.ListRunsBySessionKey(ctx, current.GetSessionKey())
	if err != nil {
		runLog.Warn("provider session reuse skipped; run lookup failed", slog.String("error", err.Error()))
		return current
	}
	ref, sourceRunID := latestReusableProviderSessionRef(runs, current.GetRunId(), s.config.ProviderName)
	if ref == nil {
		return current
	}
	encoded, err := transport.MarshalProviderSessionRef(ref)
	if err != nil {
		runLog.Warn("provider session reuse skipped; marshal failed", slog.String("source_run_id", sourceRunID), slog.String("error", err.Error()))
		return current
	}
	updated, err := s.runs.PersistProviderSessionRef(ctx, current.GetRunId(), encoded)
	if err != nil {
		runLog.Warn("provider session reuse skipped; persist failed", slog.String("source_run_id", sourceRunID), slog.String("error", err.Error()))
		return current
	}
	runLog.Info("provider session ref reused for new run", slog.String("source_run_id", sourceRunID), slog.String("run_id", current.GetRunId()))
	return updated
}

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

func (s *Service) startLeaseRenewer(parent context.Context, lease session.LeaseRecord, runLog *slog.Logger) (context.Context, context.CancelFunc, <-chan error) {
	runCtx, cancel := context.WithCancel(parent)
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
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case <-ticker.C:
				callCtx, callCancel := context.WithTimeout(context.Background(), 5*time.Second)
				_, err := s.leases.RenewLease(callCtx, lease.LeaseID, ttl)
				callCancel()
				if err != nil {
					runLog.Error("lease renew failed", slog.String("lease_id", lease.LeaseID), slog.String("error", err.Error()))
					select {
					case errCh <- fmt.Errorf("%w: %v", errLeaseRenewalFailed, err):
					default:
					}
					cancel()
					return
				}
			}
		}
	}()
	return runCtx, cancel, errCh
}

func drainRenewError(errCh <-chan error) error {
	select {
	case err := <-errCh:
		return err
	default:
		return nil
	}
}

func latestReusableProviderSessionRef(runs []*sessionv1.RunRecord, currentRunID, providerName string) (*transport.ProviderSessionRef, string) {
	var selected *transport.ProviderSessionRef
	selectedRunID := ""
	selectedAt := time.Time{}
	for _, runRecord := range runs {
		if runRecord == nil || strings.TrimSpace(runRecord.GetRunId()) == strings.TrimSpace(currentRunID) {
			continue
		}
		if providerName != "" && !strings.EqualFold(strings.TrimSpace(runRecord.GetModelProvider()), strings.TrimSpace(providerName)) {
			continue
		}
		if strings.TrimSpace(runRecord.GetProviderSessionRef()) == "" {
			continue
		}
		ref, err := transport.ParseProviderSessionRef(runRecord.GetProviderSessionRef())
		if err != nil || ref == nil {
			continue
		}
		if providerName != "" && strings.TrimSpace(ref.ProviderName) != "" && !strings.EqualFold(strings.TrimSpace(ref.ProviderName), strings.TrimSpace(providerName)) {
			continue
		}
		candidateAt := runRecordTime(runRecord)
		if selected == nil || candidateAt.After(selectedAt) {
			copied := *ref
			selected = &copied
			selectedRunID = runRecord.GetRunId()
			selectedAt = candidateAt
		}
	}
	return selected, selectedRunID
}

func runRecordTime(runRecord *sessionv1.RunRecord) time.Time {
	if runRecord == nil {
		return time.Time{}
	}
	for _, value := range []string{runRecord.GetStartedAt(), runRecord.GetUpdatedAt(), runRecord.GetFinishedAt()} {
		parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(value))
		if err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
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
	s.emitEvent(runID, resp.GetSessionKey(), observability.EventStateTransition, map[string]any{"from_state": fromStr, "to_state": toStr})
	return resp, nil
}
