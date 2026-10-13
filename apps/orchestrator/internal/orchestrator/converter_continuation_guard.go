package orchestrator

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/generatedtools"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/transport"
)

type converterContinuationProposalKey struct{}

// This marker is routing identity, not authority. Every guarded proposal reloads
// the exact private055 proof under its own current lease, task and sources.
func converterContinuationProposalContext(ctx context.Context, p preparedRun) context.Context {
	if p.SourceInputEvent != nil && p.SourceInputEvent.Source == "converter-return" {
		return context.WithValue(ctx, converterContinuationProposalKey{}, *p.SourceInputEvent)
	}
	return ctx
}

func (s *Service) guardConverterContinuationProposal(ctx context.Context, current *sessionv1.RunRecord, requested *transport.ToolCallRequest) error {
	event, ok := ctx.Value(converterContinuationProposalKey{}).(ingress.InputEvent)
	if !ok {
		return nil
	}
	deny := func(reason string) error { return &budget.LoopError{Reason: reason} }
	if s.config.ConverterContinuations == nil || current == nil || requested == nil {
		return deny("converter_continuation_proposal_proof_unavailable")
	}
	proofCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	c, err := s.config.ConverterContinuations.LoadConverterContinuation(proofCtx, event)
	tok, owned := ownership.TokenFromContext(ctx)
	if err != nil || !owned || c.RunID != current.GetRunId() || c.RunID != tok.RunID || c.SessionKey != current.GetSessionKey() || c.SessionKey != tok.SessionKey || c.SessionKey != event.SessionKey {
		return deny("converter_continuation_proposal_proof_unavailable")
	}
	var binding struct {
		TaskID string `json:"task_id"`
		ID     string `json:"continuation_id"`
	}
	if json.Unmarshal([]byte(event.PayloadJSON), &binding) != nil || binding.TaskID != c.TaskID || binding.ID != c.ID {
		return deny("converter_continuation_proposal_proof_unavailable")
	}
	if c.Purpose == generatedtools.CandidateReturn && c.Activation == nil && len(c.ConfirmedPriorActions) == 0 {
		return nil
	}
	if c.Purpose != generatedtools.ActivatedCatalog || c.Activation == nil {
		return deny("converter_continuation_proposal_proof_unavailable")
	}
	if len(c.ConfirmedPriorActions) == 0 {
		return nil
	}
	if validateConverterContinuationActions(c) != nil {
		return deny("converter_continuation_prior_actions_unknown")
	}
	exact, err := actionauth.ArgumentsHash(requested.ArgsJSON)
	if err != nil {
		return err
	}
	equivalent, err := budget.NormalizedArgumentsHash(requested.ArgsJSON)
	if err != nil {
		return err
	}
	matched, uncertain, priorRead, sameToolRead := false, false, true, true
	for _, prior := range c.ConfirmedPriorActions {
		if prior.ToolName != requested.ToolName {
			continue
		}
		sameToolRead = sameToolRead && prior.Effect == actionauth.EffectRead
		if prior.ArgsHash == exact || prior.EquivalentArgsHash == equivalent {
			matched = true
			priorRead = priorRead && prior.Effect == actionauth.EffectRead
		} else if prior.EquivalentArgsHash == "" {
			uncertain = true
		}
	}
	if !matched && !uncertain {
		return nil
	}
	read, err := s.converterContinuationReadOnly(proofCtx, current.GetRunId(), requested)
	if err != nil {
		return deny("converter_continuation_proposal_proof_unavailable")
	}
	if matched && priorRead && read && !uncertain {
		return nil
	}
	if uncertain && sameToolRead && read {
		return nil
	}
	if uncertain {
		return deny("converter_continuation_prior_arguments_unknown")
	}
	return deny("converter_continuation_equivalent_effect_denied")
}

func (s *Service) converterContinuationReadOnly(ctx context.Context, run string, requested *transport.ToolCallRequest) (bool, error) {
	if s.config.ToolCatalog == nil {
		return false, nil
	}
	var contracts []*toolbrokerv1.ToolContract
	var err error
	if scoped, ok := s.config.ToolCatalog.(interface {
		ListToolsForRun(context.Context, string) ([]*toolbrokerv1.ToolContract, error)
	}); ok {
		contracts, err = scoped.ListToolsForRun(ctx, run)
	} else {
		contracts, err = s.config.ToolCatalog.ListTools(ctx)
	}
	if err != nil {
		return false, err
	}
	for _, c := range contracts {
		if c.GetToolName() != requested.ToolName || c.GetStatus() != "active" || c.GetEffectContract().GetRetrySafety() != "read_only" || actionauth.CatalogEffect(c) != actionauth.EffectRead {
			continue
		}
		if requested.ToolName == "http.request" {
			var args struct {
				Method string `json:"method"`
			}
			if json.Unmarshal([]byte(requested.ArgsJSON), &args) != nil {
				return false, nil
			}
			method := strings.ToUpper(args.Method)
			if method != "" && method != "GET" && method != "HEAD" && method != "OPTIONS" {
				return false, nil
			}
		}
		if requested.ToolName == "browser.read" || requested.ToolName == "browser.open" || requested.ToolName == "browser.navigate" {
			if c.GetContractRevision() != "2" || c.GetEffectContract().GetVerificationMethod() != "read_result" {
				return false, nil
			}
		}
		return true, nil
	}
	return false, nil
}
