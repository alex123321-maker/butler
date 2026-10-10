package actionauth

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

// ValidateMandate accepts a bounded owner decision. A model cannot create a
// mandate through a tool. Changes create a new revision and invalidate grants.
func ValidateMandate(m Mandate, now time.Time) error {
	if m.ScopeKind == "owner_goal" {
		return ValidateOwnerMandate(m, now)
	}
	if m.ID == "" || m.OwnerID == "" || m.TaskID == "" || m.TaskRevision <= 0 || m.ConstraintsRevision <= 0 || m.Revision <= 0 {
		return fmt.Errorf("mandate identity and current task versions are required")
	}
	if !m.ExpiresAt.After(now) || m.ExpiresAt.After(now.Add(7*24*time.Hour)) {
		return fmt.Errorf("mandate expiry must be within seven days")
	}
	if len(m.Tools) == 0 || len(m.Tools) > 64 || len(m.Resources) == 0 || len(m.Resources) > 64 || len(m.Effects) == 0 {
		return fmt.Errorf("bounded tools, resources and effects are required")
	}
	for _, tool := range m.Tools {
		if tool == "" || strings.ContainsAny(tool, "*? ") || !strings.Contains(tool, ".") {
			return fmt.Errorf("mandate tools must be exact names")
		}
	}
	for _, resource := range m.Resources {
		if strings.HasPrefix(resource, "tool:") {
			if strings.ContainsAny(resource, "*? ") {
				return fmt.Errorf("tool resource must be exact")
			}
			continue
		}
		u, err := url.Parse(resource)
		if err != nil || u.User != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.Path != "" && u.Path != "/" || u.RawQuery != "" || u.Fragment != "" || strings.ContainsAny(u.Host, "*?") {
			return fmt.Errorf("mandate resources must be exact HTTP(S) origins or exact tool resources")
		}
	}
	for _, effect := range m.Effects {
		switch effect {
		case EffectRead, EffectPrepare, EffectCommit, EffectUnknown:
		default:
			return fmt.Errorf("unsupported mandate effect")
		}
	}
	for _, ref := range m.CredentialRefs {
		if ref.Alias == "" || ref.Field == "" || strings.ContainsAny(ref.Alias+ref.Field, "*?") {
			return fmt.Errorf("mandate credential fields must be exact")
		}
	}
	return nil
}
func mandateAllows(m Mandate, b ActionBinding, now time.Time) bool {
	if m.ScopeKind == "owner_goal" {
		return ownerMandateAllows(m, b, now)
	}
	if m.RevokedAt != nil || !m.ExpiresAt.After(now) || m.OwnerID != b.OwnerID || m.TaskID != b.TaskID || m.TaskRevision != b.TaskRevision || m.ConstraintsRevision != b.ConstraintsRevision {
		return false
	}
	if !contains(m.Tools, b.ToolName) {
		return false
	}
	effectOK := false
	for _, effect := range m.Effects {
		if effect == b.Effect {
			effectOK = true
		}
	}
	if !effectOK {
		return false
	}
	resource := b.Resource
	if strings.HasPrefix(resource, "browser:") || strings.HasPrefix(resource, "single_tab:") {
		if i := strings.Index(resource, "http"); i >= 0 {
			resource = resource[i:]
		}
	}
	origin := resource
	if u, err := url.Parse(resource); err == nil && u.Hostname() != "" {
		origin = strings.ToLower(u.Scheme + "://" + u.Host)
	}
	resourceOK := false
	for _, allowed := range m.Resources {
		if strings.TrimSuffix(strings.ToLower(allowed), "/") == strings.TrimSuffix(origin, "/") {
			resourceOK = true
		}
	}
	if !resourceOK {
		return false
	}
	for _, ref := range b.CredentialRefs {
		found := false
		for _, allowed := range m.CredentialRefs {
			if ref == allowed {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	return true
}
func contains(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
