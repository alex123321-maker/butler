package actionauth

import (
	"encoding/json"
	"fmt"
	"time"

	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/security/egress"
	"github.com/butler/butler/internal/websearch"
)

// NetworkReleaseRequired treats every model-selected path/query/header/body as
// potentially private. Public routing eligibility is not a data-release grant.
// No DNS or credential material is touched during this check.
func NetworkReleaseRequired(call *toolbrokerv1.ToolCall, contract *toolbrokerv1.ToolContract) (bool, error) {
	var args map[string]any
	if call == nil || contract == nil || json.Unmarshal([]byte(call.GetArgsJson()), &args) != nil {
		return false, fmt.Errorf("release policy requires validated arguments")
	}
	var resources []string
	if contract.GetExtension() != nil {
		resource, ok := args["url"].(string)
		if !ok || resource == "" {
			return false, fmt.Errorf("extension resource is invalid")
		}
		resources = append(resources, resource)
	}
	switch call.GetToolName() {
	case "web.search":
		query, _ := args["query"].(string)
		resource, err := websearch.URL(query)
		if err != nil {
			return false, err
		}
		resources = append(resources, resource)
	case "http.request", "http.download", "http.parse_html", "web.fetch", "web.extract", "browser.read", "browser.open", "browser.navigate":
		if resource, ok := args["url"].(string); ok && resource != "" {
			resources = append(resources, resource)
		}
	case "web.fetch_batch":
		urls, ok := args["urls"].([]any)
		if !ok {
			return false, fmt.Errorf("release resources are invalid")
		}
		for _, value := range urls {
			resource, ok := value.(string)
			if !ok {
				return false, fmt.Errorf("release resources are invalid")
			}
			resources = append(resources, resource)
		}
	case "browser.fill", "browser.type", "browser.prepare_fill", "browser.prepare_type", "browser.click", "browser.set_cookie", "browser.restore_storage_state":
		if resource, ok := args["expected_url"].(string); ok && resource != "" {
			resources = append(resources, resource)
		}
	}
	requires := call.GetToolName() == "web.search"
	for _, resource := range resources {
		if _, err := (egress.Policy{}).ValidateURL(resource); err != nil {
			return false, fmt.Errorf("network resource is outside public execution policy")
		}
		// The registry is operator owned and included in ContractHash. An empty
		// declaration is routing compatibility, never permission to disclose.
		if len(contract.GetAllowedDomains()) == 0 {
			requires = true
			continue
		}
		if _, err := (egress.Policy{AllowedDomains: contract.GetAllowedDomains()}).ValidateURL(resource); err != nil {
			requires = true
		}
	}
	return requires, nil
}

func releaseMandateAllowed(m Mandate, b ActionBinding, requires bool, at time.Time) bool {
	if !mandateAllows(m, b, at) {
		return false
	}
	// Broad release needs the current confirmed-purpose assignment, not a
	// caller-selected legacy origin-only mandate.
	return !requires || m.ScopeKind == "owner_goal" && b.Purpose != "" && b.GoalID != ""
}
