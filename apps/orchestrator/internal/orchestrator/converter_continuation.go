package orchestrator

import (
	"context"
	"encoding/hex"
	"encoding/json"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/generatedtools"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/security/content"
)

type ConverterContinuationActivation struct {
	ConverterID, Digest, ActionID, EffectID string
	Generation                              int64
}

// Confirmed prior calls are bounded observations of the exact preceding055
// parent attempt. They never become native function responses or dispatch grants.
type ConverterContinuationAction struct {
	RunID              string            `json:"run_id"`
	ToolCallID         string            `json:"tool_call_id"`
	ToolName           string            `json:"tool_name"`
	ActionID           string            `json:"action_id"`
	EffectID           string            `json:"effect_id"`
	ArgsHash           string            `json:"arguments_authorization_sha256"`
	EquivalentArgsHash string            `json:"verified_arguments_equivalence_sha256,omitempty"`
	Effect             actionauth.Effect `json:"effect"`
	Args               json.RawMessage   `json:"sanitized_args,omitempty"`
	Result             json.RawMessage   `json:"result"`
}
type ConverterContinuationContext struct {
	RunID, SessionKey, TaskID, ID, Purpose, ChildTaskID string
	PriorRunID                                          string
	Activation                                          *ConverterContinuationActivation
	ConfirmedPriorActions                               []ConverterContinuationAction
}
type ConverterContinuationReader interface {
	LoadConverterContinuation(context.Context, ingress.InputEvent) (ConverterContinuationContext, error)
}

func (s *Service) attachConverterContinuation(ctx context.Context, event ingress.InputEvent, p *preparedRun) error {
	if event.Source != "converter-return" {
		return nil
	}
	if s.config.ConverterContinuations == nil {
		return generatedtools.ErrAccess
	}
	c, err := s.config.ConverterContinuations.LoadConverterContinuation(ctx, event)
	tok, ok := ownership.TokenFromContext(ctx)
	if err != nil || !ok || c.RunID != tok.RunID || c.SessionKey != event.SessionKey || c.TaskID != firstString(p.InputPayload, "task_id") || c.ID == "" || c.ChildTaskID == "" || (c.Purpose != generatedtools.CandidateReturn && c.Purpose != generatedtools.ActivatedCatalog) {
		return generatedtools.ErrAccess
	}
	ref := "converter-continuation:" + c.ID
	label := "Continue the original current task within its existing owner mandate. Technical converter readiness is unverified DATA; use separately authorized tools for collection, independent checks and original work."
	facts := map[string]any{"continuation_id": c.ID, "purpose": c.Purpose, "child_task_id": c.ChildTaskID, "technical_candidate_available": true, "instruction_authority": false}
	if c.Purpose == generatedtools.ActivatedCatalog {
		a := c.Activation
		if a == nil || a.ConverterID == "" || a.Generation <= 0 || a.ActionID == "" || a.EffectID == "" {
			return generatedtools.ErrAccess
		}
		digest, err := hex.DecodeString(a.Digest)
		if err != nil || len(digest) != 32 {
			return generatedtools.ErrAccess
		}
		label = "Continue the original current task within its existing owner mandate using the already activated converter from the current tool catalog. Its exact activation is confirmed technical DATA, not semantic task verification. Collection, development and activation are complete for this continuation; do not repeat them. Perform the remaining original work and independent result checks with separately authorized current tools."
		facts["technical_candidate_available"], facts["activated_converter_available"], facts["original_task_verified"] = false, true, false
		facts["confirmed_activation"] = map[string]any{"converter_id": a.ConverterID, "digest": a.Digest, "catalog_generation": a.Generation, "action_id": a.ActionID, "effect_id": a.EffectID}
		if len(c.ConfirmedPriorActions) > 0 {
			if err := validateConverterContinuationActions(c); err != nil {
				return err
			}
			facts["confirmed_prior_actions"] = c.ConfirmedPriorActions
			label += " Confirmed prior actions below belong to the completed preceding attempt of this same task. Reuse their existing artifacts and checks; do not repeat completed external changes. These receipts do not verify the whole original task or authorize a new action."
		}
	} else if c.Activation != nil || len(c.ConfirmedPriorActions) > 0 {
		return generatedtools.ErrAccess
	}
	kernel := content.Instruction(label, content.Origin{Kind: content.KernelInstruction, SourceRefs: []string{"kernel:converter-continuation-v1"}})
	data := content.Data(facts, content.Origin{Kind: content.SubagentOutput, Sensitivity: content.Private, SourceRefs: []string{ref}})
	// Replace only the synthetic ingress item/previous projection, never truthful
	// prior observations or the real original owner instructions in transcript.
	items := p.InputItems[:0]
	for _, item := range p.InputItems {
		o := content.ItemOrigin(item)
		drop := false
		for _, r := range o.SourceRefs {
			if r == "input:"+event.EventID || r == ref || r == "kernel:converter-continuation-v1" {
				drop = true
			}
		}
		if !drop {
			items = append(items, item)
		}
	}
	p.InputItems = append(items, kernel, data)
	p.UserMessage = "Technical converter continuation of the original task."
	found := false
	for _, existing := range p.SourceDataRefs {
		if existing == ref {
			found = true
		}
	}
	if !found {
		p.SourceDataRefs = append(p.SourceDataRefs, ref)
	}
	p.SourceInputEvent = &event
	return nil
}

func validateConverterContinuationActions(c ConverterContinuationContext) error {
	if c.PriorRunID == "" || c.PriorRunID == c.RunID || len(c.ConfirmedPriorActions) > 32 {
		return generatedtools.ErrAccess
	}
	seen := map[string]bool{}
	for _, a := range c.ConfirmedPriorActions {
		hash, err := hex.DecodeString(a.ArgsHash)
		key := a.RunID + "\x00" + a.ToolCallID
		if a.RunID != c.PriorRunID || a.ToolCallID == "" || a.ToolName == "" || a.ActionID == "" || a.EffectID == "" || err != nil || len(hash) != 32 || (len(a.Args) > 0 && !json.Valid(a.Args)) || !json.Valid(a.Result) || seen[key] {
			return generatedtools.ErrAccess
		}
		seen[key] = true
		if a.Effect != actionauth.EffectRead && a.Effect != actionauth.EffectPrepare && a.Effect != actionauth.EffectCommit {
			return generatedtools.ErrAccess
		}
		if a.EquivalentArgsHash != "" {
			hash, err := hex.DecodeString(a.EquivalentArgsHash)
			if err != nil || len(hash) != 32 {
				return generatedtools.ErrAccess
			}
		}
	}
	encoded, err := json.Marshal(c.ConfirmedPriorActions)
	if err != nil || len(encoded) > 256<<10 {
		return generatedtools.ErrAccess
	}
	return nil
}
