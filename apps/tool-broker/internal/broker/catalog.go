package broker

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/butler/butler/apps/tool-broker/internal/registry"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/delegation"
	"github.com/butler/butler/internal/extensions"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/generatedtools"
	"github.com/butler/butler/internal/memory/readcontrol"
	"github.com/butler/butler/internal/schedulecontrol"
	"github.com/butler/butler/internal/workspaceexec"
)

// TaskCatalog uses the same private live projection as action authorization.
// Search ranks only contracts that survived current capability filtering.
type TaskCatalog struct {
	Registry                    *registry.Registry
	Contexts                    actionauth.ContextReader
	GuardedTabOnly              bool
	BrowserResourcesUnavailable bool
	Mandates                    interface {
		GetMandate(context.Context, string) (actionauth.Mandate, error)
	}
	Extensions *extensions.Store
	Converters *generatedtools.Store
}

func (c *TaskCatalog) ListForRun(ctx context.Context, run, query string, limit int) ([]*toolbrokerv1.ToolContract, error) {
	if c.Contexts == nil || c.Registry == nil || len(query) > 256 {
		return nil, fmt.Errorf("trusted catalog unavailable")
	}
	a, err := c.Contexts.GetActionContext(ctx, run)
	if err != nil || a.RunID != run || a.OwnerID == "" || a.LeaseID == "" || a.LeaseEpoch <= 0 || !a.LeaseExpiresAt.After(time.Now().UTC()) || a.AutonomyMode < 1 || a.AutonomyMode > 4 {
		return nil, fmt.Errorf("current catalog ownership unavailable")
	}
	if a.TaskID != "" && (a.TaskStatus != "active" || a.TaskRevision != a.PinnedTaskRevision) {
		return nil, fmt.Errorf("current catalog task unavailable")
	}
	switch a.RunState {
	case "RUN_STATE_PREPARING", "RUN_STATE_MODEL_RUNNING", "RUN_STATE_AWAITING_MODEL_RESUME", "RUN_STATE_TOOL_PENDING", "RUN_STATE_TOOL_RUNNING", "RUN_STATE_AWAITING_APPROVAL", "preparing", "model_running", "awaiting_model_resume", "tool_pending", "tool_running", "awaiting_approval":
	default:
		return nil, fmt.Errorf("inactive run catalog unavailable")
	}
	var allowed map[string]bool
	if a.Authority != nil {
		if c.Mandates == nil {
			return nil, actionauth.ErrOwnerAuthority
		}
		m, e := c.Mandates.GetMandate(ctx, a.Authority.MandateID)
		if e != nil || m.Revision != a.Authority.MandateRevision || m.Status != "active" || m.RevokedAt != nil || !m.ExpiresAt.After(time.Now().UTC()) || !actionauth.WithinActiveHours(m.Hours, time.Now().UTC()) {
			return nil, actionauth.ErrOwnerAuthority
		}
		allowed = map[string]bool{}
		for _, name := range m.Tools {
			allowed[name] = true
		}
	}
	denied := map[string]bool{}
	for _, name := range a.DenyTools {
		denied[name] = true
	}
	terms := strings.Fields(strings.ToLower(query))
	result := []*toolbrokerv1.ToolContract{}
	for _, contract := range c.Registry.List("", false) {
		if schedulecontrol.IsTool(contract.GetToolName()) && (!a.SchedulesReady || a.Delegation != nil) {
			continue
		}
		if contract.GetToolName() == "converters.collect" {
			// A queued developer receipt is not a candidate return. Advertising
			// collection early invites a call that correctly fails Prepare.
			// This is availability only; actual Prepare/Claim repeat all guards.
			if c.Converters == nil {
				continue
			}
			ready, e := c.Converters.CollectionAvailable(ctx, a)
			if e != nil {
				return nil, e
			}
			if !ready {
				continue
			}
		}
		if c.BrowserResourcesUnavailable && (strings.HasPrefix(contract.GetToolName(), "browser.") || strings.HasPrefix(contract.GetToolName(), "single_tab.")) && contract.GetToolName() != "single_tab.bind.guarded" {
			continue
		}
		if workspaceexec.Protected(contract.GetToolName()) && (a.TaskID == "" || !a.WorkspaceReady || !workspaceexec.ExactContract(contract)) {
			continue
		}
		if c.GuardedTabOnly && strings.HasPrefix(contract.GetToolName(), "browser.") {
			continue
		}
		if contract.GetContractRevision() == "guarded-1" && (contract.GetToolName() == "single_tab.status" || contract.GetToolName() == "single_tab.click") && (a.TaskID == "" || !a.GuardedTabRuntimeReady) {
			continue
		}
		if contract.GetToolName() == "memory.propose_candidate" && (a.TaskID == "" || !a.MemoryCandidateReady) {
			continue
		}
		if readcontrol.IsTool(contract.GetToolName()) && (a.TaskID == "" || !a.MemoryReadReady) {
			continue
		}
		if contract.GetToolName() == "memory.facts" && (a.TaskID == "" || !a.MemoryReadReady || !a.MemoryTemporalReady) {
			continue
		}
		if contract.GetToolName() == "single_tab.bind.guarded" && (a.TaskID == "" || !a.GuardedTabRequestReady) {
			continue
		}
		if contract.GetToolName() == delegation.AdmissionTool && (a.TaskID == "" || !a.DelegationReady) {
			continue
		}
		if contract.GetToolName() == generatedtools.DeveloperAdmissionTool && (a.TaskID == "" || !a.DelegationReady || !a.ConverterDelegationReady || !a.ConverterLifecycleReady) {
			continue
		}
		if (strings.HasPrefix(contract.GetToolName(), "skills.") || strings.HasPrefix(contract.GetToolName(), "converters.")) && a.TaskID == "" {
			continue
		}
		effect := actionauth.CatalogEffect(contract)
		if actionauth.CheckExecutionPolicy(a, effect) != nil || a.AutonomyMode == 1 && effect != actionauth.EffectRead {
			continue
		}
		if strings.HasPrefix(contract.GetToolName(), "extension.") || denied[contract.GetToolName()] || allowed != nil && !allowed[contract.GetToolName()] {
			continue
		}
		matches := true
		haystack := strings.ToLower(contract.GetToolName() + " " + contract.GetDescription())
		for _, term := range terms {
			if !strings.Contains(haystack, term) {
				matches = false
				break
			}
		}
		if matches {
			result = append(result, contract)
		}
	}
	if c.Extensions != nil {
		installed, e := c.Extensions.Catalog(ctx, a, query, 64)
		if e != nil {
			return nil, e
		}
		result = append(result, installed...)
	}
	if c.Converters != nil {
		installed, e := c.Converters.Catalog(ctx, a, query, 64)
		if e != nil {
			return nil, e
		}
		result = append(result, installed...)
	}
	if a.Delegation != nil {
		filtered := result[:0]
		for _, contract := range result {
			hash, e := actionauth.ContractHash(contract)
			if e != nil {
				continue
			}
			for _, p := range a.Delegation.Permissions {
				if p.Tool == contract.GetToolName() && p.ContractHash == hash && len(p.Resources) > 0 && len(p.Effects) > 0 && delegation.Allows(a.Delegation, p.Tool, hash, p.Resources[0], p.Effects[0], nil) {
					filtered = append(filtered, contract)
					break
				}
			}
		}
		result = filtered
	}
	sort.Slice(result, func(i, j int) bool { return result[i].GetToolName() < result[j].GetToolName() })
	if limit <= 0 || limit > 64 {
		limit = 64
	}
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}
