package actionauth

import (
	"strings"
	"testing"
	"time"
)

func testOwnerMandate(now time.Time) Mandate {
	return Mandate{ID: "mandate_synthetic", OwnerID: "fixture_owner", ScopeKind: "owner_goal", GoalID: "goal_synthetic", GoalRevision: 1, Revision: 1, Status: "active", Purpose: "service_maintenance", Tools: []string{"http.request", "browser.read"}, ResourceScopes: []ResourceScope{{Origin: "https://example.invalid", PathPrefix: "/test-service"}}, Effects: []Effect{EffectRead, EffectCommit, EffectUnknown}, CredentialRefs: []CredentialRef{{Alias: "test_service", Field: "password"}}, ExpiresAt: now.Add(24 * time.Hour), BudgetDeadline: now.Add(24 * time.Hour), Budget: MandateBudget{ModelTurns: 20, Tokens: 20000, ActiveMillis: 600000, ExternalCalls: 10}, Privacy: "cloud_allowed", AllowInitiative: true, Intervention: []string{"effect_unknown", "scope_change", "budget_exhausted"}, Reporting: "exceptions"}
}
func TestOwnerMandateScopeAndRevision(t *testing.T) {
	now := time.Now()
	m := testOwnerMandate(now)
	if err := ValidateMandate(m, now); err != nil {
		t.Fatal(err)
	}
	b := ActionBinding{OwnerID: m.OwnerID, GoalID: m.GoalID, GoalRevision: 1, MandateID: m.ID, MandateRevision: 1, Purpose: m.Purpose, ToolName: "http.request", Effect: EffectCommit, Resource: "https://example.invalid/test-service/recovery", CredentialRefs: []CredentialRef{{"test_service", "password"}}}
	if err := CheckAssignedAction(m, b, now); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*ActionBinding){"other_account": func(b *ActionBinding) { b.Resource = "https://example.invalid/work-account/recovery" }, "other_origin": func(b *ActionBinding) { b.Resource = "https://foreign.invalid/test-service/recovery" }, "old_goal": func(b *ActionBinding) { b.GoalRevision = 2 }, "old_mandate": func(b *ActionBinding) { b.MandateRevision = 2 }, "model_purpose": func(b *ActionBinding) { b.Purpose = "unreviewed" }, "credential": func(b *ActionBinding) { b.CredentialRefs = []CredentialRef{{"work_account", "password"}} }, "tool": func(b *ActionBinding) { b.ToolName = "shell.exec" }}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			c := b
			change(&c)
			if CheckAssignedAction(m, c, now) == nil {
				t.Fatal("scope escape authorized")
			}
		})
	}
	m.DenyResources = []ResourceScope{{"https://example.invalid", "/test-service/recovery"}}
	if CheckAssignedAction(m, b, now) == nil {
		t.Fatal("deny override ignored")
	}
}
func TestOwnerMandateResourceAmbiguityDenied(t *testing.T) {
	scope := ResourceScope{"https://example.invalid", "/test-service"}
	for _, raw := range []string{"https://example.invalid/test-service/../work", "https://example.invalid/test-service/%2E%2E/work", "https://example.invalid/test-service%2fwork", "https://example.invalid/test-service-other", "https://user@example.invalid/test-service", "https://example.invalid/test-service/\\work"} {
		if resourceWithin(scope, raw) {
			t.Fatalf("ambiguous resource accepted: %s", raw)
		}
	}
	for _, raw := range []string{"https://example.invalid/test-service", "https://example.invalid/test-service/", "https://example.invalid/test-service/recovery?q=diagnostic"} {
		if !resourceWithin(scope, raw) {
			t.Fatalf("valid bounded resource refused: %s", raw)
		}
	}
}
func TestOwnerMandateHoursAndExactExtension(t *testing.T) {
	now := time.Date(2026, 10, 12, 10, 30, 0, 0, time.UTC)
	m := testOwnerMandate(now)
	m.Hours = &ActiveHours{Timezone: "UTC", StartMinute: 600, EndMinute: 660, Weekdays: []int{1}}
	if !WithinActiveHours(m.Hours, now) || WithinActiveHours(m.Hours, now.Add(time.Hour)) || WithinActiveHours(m.Hours, now.Add(24*time.Hour)) {
		t.Fatal("active hours not enforced")
	}
	m.Extensions = []ExtensionPermission{{Source: "operator-reviewed", ExtensionID: "test-service-tools", ManifestHash: strings.Repeat("a", 64), Tools: []string{"http.request"}}}
	if err := ValidateMandate(m, now); err != nil {
		t.Fatal(err)
	}
	if err := CheckExtensionAttachment(m, "operator-reviewed", "test-service-tools", strings.Repeat("a", 64), []string{"http.request"}, now); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		hash  string
		tools []string
	}{{strings.Repeat("b", 64), []string{"http.request"}}, {strings.Repeat("a", 64), []string{"shell.exec"}}} {
		if CheckExtensionAttachment(m, "operator-reviewed", "test-service-tools", c.hash, c.tools, now) == nil {
			t.Fatal("unreviewed manifest/capability allowed")
		}
	}
	m.Status = "revoked"
	if CheckExtensionAttachment(m, "operator-reviewed", "test-service-tools", strings.Repeat("a", 64), []string{"http.request"}, now) == nil {
		t.Fatal("revoked extension permission allowed")
	}
}
