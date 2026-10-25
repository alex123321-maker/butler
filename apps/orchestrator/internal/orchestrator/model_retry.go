package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/transport"
)

var ErrModelRetryPending = errors.New("one durable model retry queued")

type modelRetryRecoveryKey struct{}

type modelTurnTimeout struct{ operationID string }

func (*modelTurnTimeout) Error() string { return "model_turn_timeout" }
func (*modelTurnTimeout) Unwrap() error { return &budget.LoopError{Reason: "model_turn_timeout"} }

type modelRetryStore interface {
	ClaimModelRetry(context.Context, string, string) error
	ValidateModelRetry(context.Context, string, string) error
	MarkModelRetryStarted(context.Context, string, string) error
}

type modelRetryCheckpoint struct {
	Count             int       `json:"count"`
	Reason            string    `json:"reason"`
	PriorOperationID  string    `json:"prior_operation_id"`
	State             string    `json:"state"`
	NotBefore         time.Time `json:"not_before"`
	ClaimedAt         time.Time `json:"claimed_at"`
	ExecutionDeadline time.Time `json:"execution_deadline,omitempty"`
}

func modelTurnRetryBound(ctx context.Context, a budget.ModelAdmission) bool {
	if !a.ModelTurnLimited || ctx.Err() != nil {
		return false
	}
	if parent, ok := ctx.Deadline(); ok && !parent.After(a.Deadline) {
		return false
	}
	return true
}

func modelRetryTimeoutEvent(event transport.TransportEvent, nominal, parentActive bool) transport.TransportEvent {
	if event.EventType == transport.EventTypeTransportError && event.TransportError != nil && event.TransportError.Type == "model_turn_timeout" {
		event.TransportError = &transport.Error{Type: transport.ErrorTypeProviderProtocolError, Message: "provider_protocol_error"}
		return event
	}
	if event.EventType == transport.EventTypeRunTimedOut {
		event.EventType = transport.EventTypeTransportError
		event.TransportError = &transport.Error{Type: "execution_budget_deadline", Message: "execution_budget_deadline"}
		if nominal && parentActive {
			event.TransportError = &transport.Error{Type: "model_turn_timeout", Message: "model_turn_timeout"}
		}
	} else if event.EventType == transport.EventTypeTransportError && event.TransportError != nil && event.TransportError.Type == transport.ErrorTypeProviderTimeout && nominal && parentActive {
		event.TransportError = &transport.Error{Type: "model_turn_timeout", Message: "model_turn_timeout"}
	}
	return event
}

type modelRetryStageTrace struct {
	mu     sync.Mutex
	last   transport.ProviderStage
	sealed bool
}

func (p *modelRetryStageTrace) observe(stage transport.ProviderStage) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.sealed {
		p.last = stage
	}
}
func (p *modelRetryStageTrace) inferencePhase() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sealed = true
	return p.last == transport.ProviderStageHTTP || p.last == transport.ProviderStageStream
}
func withModelRetryStages(ctx context.Context) (context.Context, *modelRetryStageTrace) {
	trace := &modelRetryStageTrace{}
	parent := ctx
	return transport.WithProviderStageObserver(ctx, func(stage transport.ProviderStage) {
		trace.observe(stage)
		transport.ObserveProviderStage(parent, stage)
	}), trace
}

func (s *Service) modelTimeoutFromCheckpoint(ctx context.Context, runID string) error {
	if s.config.Budgets == nil || s.config.Checkpoints == nil {
		return &budget.LoopError{Reason: "model_turn_timeout"}
	}
	stage, raw, err := s.config.Checkpoints.LoadCheckpoint(ctx, runID)
	var c struct {
		Data struct {
			Operation string `json:"operation_id"`
			Origin    string `json:"deadline_origin"`
		} `json:"checkpoint_data"`
	}
	if err != nil || stage != "model_budget_reserved" || json.Unmarshal([]byte(raw), &c) != nil || c.Data.Origin != "model_turn" || c.Data.Operation == "" {
		return &budget.LoopError{Reason: "model_turn_timeout"}
	}
	return &modelTurnTimeout{operationID: c.Data.Operation}
}

// Eligibility is private trusted admission data. No provider Retryable bit,
// model text or owner/global/source failure can claim this one allowance.
func (s *Service) scheduleModelRetry(ctx context.Context, current *sessionv1.RunRecord, err error) bool {
	var timeout *modelTurnTimeout
	store, ok := s.config.Checkpoints.(modelRetryStore)
	if !ok || !errors.As(err, &timeout) || timeout.operationID == "" || s.config.Budgets == nil || s.config.MemorySources == nil || s.transcript == nil || current == nil || ctx.Err() != nil {
		return false
	}
	if s.config.MemorySources.CheckRunSources(ctx, current.GetRunId()) != nil {
		return false
	}
	if _, readErr := s.modelRetryRecordedInputs(ctx, current, nil); readErr != nil {
		return false
	}
	if store.ClaimModelRetry(ctx, current.GetRunId(), timeout.operationID) != nil {
		return false
	}
	// The marker is already durable. Cleanup cannot refund it; Resume repeats
	// cleanup before the fresh session. No terminal task/run or delivery occurs.
	_ = s.discardProviderContext(ctx, current.GetRunId())
	return true
}

func (s *Service) modelRetryRecordedInputs(ctx context.Context, current *sessionv1.RunRecord, inputs []transport.InputItem) ([]transport.InputItem, error) {
	history, err := s.transcript.GetTranscript(ctx, current.GetSessionKey())
	if err != nil {
		return nil, err
	}
	result := append([]transport.InputItem(nil), inputs...)
	for _, call := range history.ToolCalls {
		if call.RunID != current.GetRunId() {
			continue
		}
		if call.FinishedAt == nil || call.Status == "effect_unknown" || call.ResultJSON == "" && call.ErrorJSON == "" {
			return nil, ErrActionOutcomeUnknown
		}
		result = append(result, recoveredActionResultItem(call.ToolCallID, call.ToolName, call.ArgsJSON, recordedToolResultEnvelope(call)))
	}
	return result, nil
}

func (s *Service) resumeModelRetry(ctx context.Context, current *sessionv1.RunRecord, prepared preparedRun, retry modelRetryCheckpoint) (transport.EventStream, error) {
	store, ok := s.config.Checkpoints.(modelRetryStore)
	if !ok || s.config.Budgets == nil || s.config.MemorySources == nil || retry.Count != 1 || (retry.Reason != "model_turn_timeout" && retry.Reason != "authority_observation_timeout") || retry.State != "claimed" || retry.PriorOperationID == "" || retry.ClaimedAt.IsZero() || !retry.NotBefore.Equal(retry.ClaimedAt.Add(2*time.Second)) {
		return nil, errors.New("model_retry_not_resumable")
	}
	if current.GetCurrentState() != commonv1.RunState_RUN_STATE_MODEL_RUNNING && current.GetCurrentState() != commonv1.RunState_RUN_STATE_AWAITING_MODEL_RESUME {
		return nil, errors.New("model_retry_run_not_live")
	}
	if err := store.ValidateModelRetry(ctx, current.GetRunId(), retry.PriorOperationID); err != nil {
		return nil, err
	}
	if err := s.config.MemorySources.CheckRunSources(ctx, current.GetRunId()); err != nil {
		return nil, err
	}
	refreshed, err := s.refreshRecoveredToolContext(ctx, current.GetRunId(), prepared.InputItems, prepared.ToolDefs)
	if err != nil {
		return nil, err
	}
	inputs, err := s.modelRetryRecordedInputs(ctx, current, refreshed.InputItems)
	if err != nil {
		return nil, err
	}
	if err = s.discardProviderContext(ctx, current.GetRunId()); err != nil {
		return nil, err
	}
	ctx, release, err := ownership.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	return s.provider.StartRun(ctx, transport.StartRunRequest{Context: transport.TransportRunContext{RunID: current.GetRunId(), SessionKey: current.GetSessionKey(), ProviderName: s.config.ProviderName, ModelName: s.config.ModelName, SupportsStreaming: true, SupportsToolCalls: true}, InputItems: inputs, ToolDefinitions: refreshed.ToolDefs, StreamingEnabled: true})
}

// Retry recovery does not create a new intentional identical write. New call
// IDs and equivalent JSON numbers/ordering cannot replay a committed receipt.
// Normal explicit repeated intents outside this private recovery keep their
// existing slot semantics. Missing/unknown catalog metadata is never read-only.
func (s *Service) guardModelRetryProposal(ctx context.Context, current *sessionv1.RunRecord, requested *transport.ToolCallRequest) error {
	if recovered, _ := ctx.Value(modelRetryRecoveryKey{}).(bool); !recovered {
		return nil
	}
	hash, err := modelRetryArgumentsHash(requested.ArgsJSON)
	if err != nil {
		return err
	}
	history, err := s.transcript.GetTranscript(ctx, current.GetSessionKey())
	if err != nil {
		return err
	}
	equivalent := false
	for _, call := range history.ToolCalls {
		if call.RunID != current.GetRunId() {
			continue
		}
		if call.Status == "validation_rejected" {
			continue
		} // Known local non-dispatch.
		if call.FinishedAt == nil || call.Status == "effect_unknown" {
			return ErrActionOutcomeUnknown
		}
		prior, err := modelRetryArgumentsHash(call.ArgsJSON)
		if err != nil {
			return err
		}
		if call.ToolName == requested.ToolName && prior == hash {
			equivalent = true
		}
	}
	if !equivalent {
		return nil
	}
	if s.config.ToolCatalog == nil {
		return &budget.LoopError{Reason: "model_retry_equivalent_effect_denied"}
	}
	var contracts []*toolbrokerv1.ToolContract
	if scoped, ok := s.config.ToolCatalog.(interface {
		ListToolsForRun(context.Context, string) ([]*toolbrokerv1.ToolContract, error)
	}); ok {
		contracts, err = scoped.ListToolsForRun(ctx, current.GetRunId())
	} else {
		contracts, err = s.config.ToolCatalog.ListTools(ctx)
	}
	if err != nil {
		return err
	}
	for _, contract := range contracts {
		if contract.GetToolName() != requested.ToolName || contract.GetStatus() != "active" || contract.GetEffectContract().GetRetrySafety() != "read_only" || actionauth.CatalogEffect(contract) != actionauth.EffectRead {
			continue
		}
		if requested.ToolName == "http.request" {
			var args struct {
				Method string `json:"method"`
			}
			if json.Unmarshal([]byte(requested.ArgsJSON), &args) != nil {
				break
			}
			method := strings.ToUpper(args.Method)
			if method != "" && method != "GET" && method != "HEAD" && method != "OPTIONS" {
				break
			}
		}
		if requested.ToolName == "browser.read" || requested.ToolName == "browser.open" || requested.ToolName == "browser.navigate" {
			if contract.GetContractRevision() != "2" || contract.GetEffectContract().GetVerificationMethod() != "read_result" {
				break
			}
		}
		return nil
	}
	return &budget.LoopError{Reason: "model_retry_equivalent_effect_denied"}
}

func modelRetryArgumentsHash(raw string) (string, error) {
	// Authorization validates unique keys/bounds but preserves numeric lexemes.
	// Reuse the existing exact-rational loop identity only for retry equivalence.
	if _, err := actionauth.ArgumentsHash(raw); err != nil {
		return "", err
	}
	return budget.NormalizedArgumentsHash(raw)
}
