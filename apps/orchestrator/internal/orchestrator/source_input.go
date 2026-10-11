package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/butler/butler/internal/delegation"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/security/content"
	"github.com/butler/butler/internal/transport"
)

func opaqueSourceInput(event ingress.InputEvent) bool {
	return event.Source == "delegation" || strings.HasPrefix(event.EventID, "voice-reviewed:") || strings.HasPrefix(event.EventID, "delegated-owned:")
}
func (s *Service) prepareSourceInput(ctx context.Context, event ingress.InputEvent) (ingress.InputEvent, error) {
	if event.Source == "delegation" {
		if s.config.DelegationContexts == nil {
			return ingress.InputEvent{}, delegation.ErrDenied
		}
		return event, nil // Current DB-selected DATA is loaded by prepareRun.
	}
	if s.config.InputSourcePreparation == nil {
		if opaqueSourceInput(event) {
			return ingress.InputEvent{}, errors.New("current input source unavailable")
		}
		return event, nil
	}
	resolved, e := s.config.InputSourcePreparation(ctx, event)
	if e != nil {
		return ingress.InputEvent{}, e
	}
	if resolved.EventID != event.EventID || resolved.SessionKey != event.SessionKey || resolved.Source != event.Source || resolved.EventType != event.EventType || resolved.IdempotencyKey != event.IdempotencyKey || !resolved.CreatedAt.Equal(event.CreatedAt) {
		return ingress.InputEvent{}, errors.New("input source changed trusted identity")
	}
	return resolved, nil
}

func durableSourcePreparation(prepared preparedRun) preparedRun {
	if prepared.SourceInputEvent == nil && len(prepared.SourceDataRefs) == 0 {
		return prepared
	}
	out := prepared
	if prepared.SourceInputEvent != nil {
		out.UserMessage = ""
		out.InputPayload = map[string]any{}
		_ = json.Unmarshal([]byte(prepared.SourceInputEvent.PayloadJSON), &out.InputPayload)
	}
	out.InputItems = append([]transport.InputItem(nil), prepared.InputItems...)
	selected := make(map[string]bool, len(prepared.SourceDataRefs))
	for _, ref := range prepared.SourceDataRefs {
		selected[ref] = true
	}
	for i, item := range out.InputItems {
		o := content.ItemOrigin(item)
		if o.IsInstruction() || o.Kind == content.ToolOutput || item.Role == "tool" || item.Name != "" {
			continue
		}
		for _, ref := range o.SourceRefs {
			if prepared.SourceInputEvent != nil && o.Kind == content.OwnerInput && ref == "input:"+prepared.SourceInputEvent.EventID {
				out.InputItems[i] = content.CurrentOwnerMessage("Current owner-reviewed source: "+prepared.SourceInputEvent.EventID, ref)
				break
			}
			if o.Kind != content.OwnerInput && selected[ref] {
				out.InputItems[i] = content.Data(map[string]any{"reload_current_data": true, "source_refs": o.SourceRefs}, o)
				break
			}
		}
	}
	return out
}
func (s *Service) rehydrateSourcePreparation(ctx context.Context, prepared *preparedRun) error {
	if prepared.SourceInputEvent == nil {
		return s.rehydrateDelegatedContext(ctx, prepared)
	}
	if prepared.SourceInputEvent.Source == "delegation" {
		d, err := s.delegatedContext(ctx, prepared.SourceInputEvent.SessionKey)
		if err != nil || d == nil {
			return delegation.ErrDenied
		}
		if _, err = s.verifiedDelegatedInput(ctx, *prepared.SourceInputEvent, d); err != nil {
			return err
		}
		prepared.UserMessage = delegatedInputLabel
		return s.rehydrateDelegatedContext(ctx, prepared)
	}
	resolved, e := s.prepareSourceInput(ctx, *prepared.SourceInputEvent)
	if e != nil {
		return e
	}
	var payload map[string]any
	if json.Unmarshal([]byte(resolved.PayloadJSON), &payload) != nil {
		return errors.New("resolved input unavailable")
	}
	text := extractUserMessage(payload)
	if strings.TrimSpace(text) == "" {
		return errors.New("resolved input unavailable")
	}
	prepared.UserMessage = text
	prepared.InputPayload = payload
	inputRef := "input:" + resolved.EventID
	for i, item := range prepared.InputItems {
		o := content.ItemOrigin(item)
		for _, ref := range o.SourceRefs {
			if o.Kind == content.OwnerInput && ref == inputRef {
				prepared.InputItems[i] = content.CurrentOwnerMessage(text, inputRef)
			}
		}
	}
	return s.rehydrateDelegatedContext(ctx, prepared)
}

func validateSourceCheckpoint(event ingress.InputEvent, prepared preparedRun) error {
	if !opaqueSourceInput(event) {
		return nil
	}
	if prepared.SourceInputEvent == nil || prepared.SourceInputEvent.EventID != event.EventID || prepared.SourceInputEvent.SessionKey != event.SessionKey || prepared.SourceInputEvent.Source != event.Source || prepared.SourceInputEvent.EventType != event.EventType || prepared.SourceInputEvent.IdempotencyKey != event.IdempotencyKey || prepared.SourceInputEvent.PayloadJSON != event.PayloadJSON || !prepared.SourceInputEvent.CreatedAt.Equal(event.CreatedAt) {
		return errors.New("current input source checkpoint unavailable")
	}
	return nil
}
