package broker

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/butler/butler/apps/tool-broker/internal/registry"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/extensions"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

// TaskCatalog uses the same private live projection as action authorization.
// Search ranks only contracts that survived current capability filtering.
type TaskCatalog struct {
	Registry *registry.Registry
	Contexts actionauth.ContextReader
	Mandates interface {
		GetMandate(context.Context, string) (actionauth.Mandate, error)
	}
	Extensions *extensions.Store
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
		if strings.HasPrefix(contract.GetToolName(), "skills.") && a.TaskID == "" {
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
	sort.Slice(result, func(i, j int) bool { return result[i].GetToolName() < result[j].GetToolName() })
	if limit <= 0 || limit > 64 {
		limit = 64
	}
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}
