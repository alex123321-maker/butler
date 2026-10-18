package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/butler/butler/apps/orchestrator/internal/observability"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/security/content"
	"github.com/butler/butler/internal/toolcatalog"
	"github.com/butler/butler/internal/transport"
	"google.golang.org/protobuf/proto"
)

// Completed load receipts in the existing durable tool journal are the selection
// record. Neither a model name nor saved provider definitions can load a schema.
func (s *Service) loadedToolRefs(ctx context.Context, runID string) ([]string, error) {
	if s.transcript == nil {
		return nil, fmt.Errorf("tool discovery requires durable transcript")
	}
	run, err := s.runs.GetRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	// The write-fencing adapter does not alter reads. Use the production store's
	// run-scoped reader so selection does not rescan weeks of session history.
	historyStore := s.transcript
	if fenced, ok := historyStore.(fencedTranscript); ok {
		historyStore = fenced.TranscriptStore
	}
	var history transcript.Transcript
	if reader, ok := historyStore.(interface {
		GetRunTranscript(context.Context, string) (transcript.Transcript, error)
	}); ok {
		history, err = reader.GetRunTranscript(ctx, runID)
	} else {
		history, err = historyStore.GetTranscript(ctx, run.GetSessionKey())
	}
	if err != nil {
		return nil, err
	}
	byName := map[string]string{}
	for _, c := range history.ToolCalls {
		if c.RunID != runID || c.ToolName != toolcatalog.Load || c.FinishedAt == nil || c.Status != "completed" {
			continue
		}
		var result toolcatalog.Result
		if json.Unmarshal([]byte(c.ResultJSON), &result) != nil || result.Status != "ok" {
			continue
		}
		for _, ref := range result.Loaded {
			if i := strings.LastIndexByte(ref, '@'); i > 0 {
				byName[ref[:i]] = ref
			}
		}
	}
	refs := make([]string, 0, len(byName))
	for _, r := range byName {
		refs = append(refs, r)
	}
	sort.Strings(refs)
	return refs, nil
}

func (s *Service) selectToolContracts(ctx context.Context, runID string, contracts []*toolbrokerv1.ToolContract) ([]*toolbrokerv1.ToolContract, []string, error) {
	refs, err := s.loadedToolRefs(ctx, runID)
	if err != nil {
		return nil, nil, err
	}
	result := []*toolbrokerv1.ToolContract{}
	for _, c := range contracts {
		if toolcatalog.IsTool(c.ToolName) && proto.Equal(c, toolcatalog.Contract(c.ToolName)) {
			result = append(result, c)
		}
	}
	if len(result) != 2 {
		return nil, nil, fmt.Errorf("tool discovery requires matching Broker catalog controls")
	}
	stale := []string{}
	for _, ref := range refs {
		if resolved, ok := toolcatalog.Resolve(contracts, []string{ref}); ok {
			result = append(result, resolved...)
		} else {
			stale = append(stale, ref)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ToolName < result[j].ToolName })
	if !selectionFits(result) {
		return nil, nil, fmt.Errorf("loaded tool schema budget exceeded")
	}
	return result, stale, nil
}

func selectionFits(contracts []*toolbrokerv1.ToolContract) bool {
	if len(contracts) > toolcatalog.MaxLoaded+2 {
		return false
	}
	defs := toolDefinitionsFromContracts(contracts)
	b, err := json.Marshal(defs)
	return err == nil && len(b) <= toolcatalog.MaxSchemaBytes
}

func (s *Service) currentToolContracts(ctx context.Context, runID string) ([]*toolbrokerv1.ToolContract, error) {
	catalog, ok := s.config.ToolCatalog.(interface {
		ListToolsForRun(context.Context, string) ([]*toolbrokerv1.ToolContract, error)
	})
	if !ok {
		return nil, fmt.Errorf("tool discovery requires a current scoped catalog")
	}
	return catalog.ListToolsForRun(ctx, runID)
}

// Validate the candidate selection before writing its durable load receipt. A
// rejected selection leaves earlier loaded methods intact and explains why.
func (s *Service) validateToolLoad(ctx context.Context, runID string, result *toolbrokerv1.ToolResult) error {
	if !s.config.ToolDiscoveryEnabled || result.GetToolName() != toolcatalog.Load || result.GetStatus() != "completed" {
		return nil
	}
	var receipt toolcatalog.Result
	if err := json.Unmarshal([]byte(result.ResultJson), &receipt); err != nil {
		return err
	}
	if receipt.Status != "ok" {
		return nil
	}
	contracts, err := s.currentToolContracts(ctx, runID)
	if err != nil {
		return err
	}
	current, _, err := s.selectToolContracts(ctx, runID, contracts)
	if err != nil {
		return err
	}
	candidate, ok := toolcatalog.Resolve(contracts, receipt.Loaded)
	if !ok {
		receipt = toolcatalog.Result{Status: "unavailable_or_stale"}
	} else {
		byName := map[string]*toolbrokerv1.ToolContract{}
		for _, c := range append(current, candidate...) {
			byName[c.ToolName] = c
		}
		all := []*toolbrokerv1.ToolContract{}
		for _, c := range byName {
			all = append(all, c)
		}
		if !selectionFits(all) {
			receipt = toolcatalog.Result{Status: "schema_budget_exceeded"}
		}
	}
	raw, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	result.ResultJson = string(raw)
	return nil
}

func (s *Service) toolSetForContinuation(ctx context.Context, runID string) (*transport.ToolSetSnapshot, error) {
	if !s.config.ToolDiscoveryEnabled {
		return nil, nil
	}
	contracts, err := s.currentToolContracts(ctx, runID)
	if err != nil {
		return nil, err
	}
	selected, _, err := s.selectToolContracts(ctx, runID, contracts)
	if err != nil {
		return nil, err
	}
	set := transport.NewToolSetSnapshot(toolDefinitionsFromContracts(selected))
	encoded, _ := json.Marshal(set.Definitions)
	run, err := s.runs.GetRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	s.emitEvent(ctx, runID, run.GetSessionKey(), observability.EventToolsLoaded, map[string]any{"mode": "deferred", "phase": "continuation", "tool_count": len(set.Definitions), "schema_bytes": len(encoded), "tool_set_revision": set.Revision})
	return set, nil
}

func discoveryInstructions() transport.InputItem {
	return content.Instruction(toolcatalog.Instructions, content.Origin{Kind: content.OperatorInstruction, SourceRefs: []string{"kernel:tool-discovery-v1"}, Sensitivity: content.Private})
}

func durableCatalogObservation(result *toolbrokerv1.ToolResult) *toolbrokerv1.ToolResult {
	if result.GetToolName() != toolcatalog.Search || result.GetStatus() != "completed" {
		return result
	}
	copy := proto.Clone(result).(*toolbrokerv1.ToolResult)
	var data toolcatalog.Result
	if json.Unmarshal([]byte(result.ResultJson), &data) != nil {
		copy.ResultJson = `{"status":"catalog_data_unavailable"}`
		return copy
	}
	for i := range data.Items {
		data.Items[i].Description = ""
	}
	copy.ResultJson = mustMarshalJSON(data)
	return copy
}
