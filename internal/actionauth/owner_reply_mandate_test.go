package actionauth

import (
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/communicationcontrol"
)

func TestOwnerReplyMandateRetainsExactMethodAndAuthority(t *testing.T) {
	now := time.Now()
	for _, scenario := range []string{"allowed", "not_listed", "deny_tool", "no_prepare", "revoked", "expired", "stale_revision", "foreign_owner", "wrong_purpose", "foreign_resource", "changed_contract", "missing_destinations", "credentials", "unknown_effect", "browser_click"} {
		t.Run(scenario, func(t *testing.T) {
			m := testOwnerMandate(now)
			m.Tools = append(m.Tools, "communication.message")
			m.Effects = append(m.Effects, EffectPrepare)
			hash, _ := ContractHash(communicationcontrol.Contract("communication.message"))
			b := ActionBinding{OwnerID: m.OwnerID, GoalID: m.GoalID, GoalRevision: m.GoalRevision, MandateID: m.ID, MandateRevision: m.Revision, Purpose: m.Purpose, ToolName: "communication.message", Resource: "tool:communication.message", ContractHash: hash, Effect: EffectPrepare, CommunicationDestinationsHash: strings.Repeat("a", 64)}
			switch scenario {
			case "not_listed":
				m.Tools = m.Tools[:len(m.Tools)-1]
			case "deny_tool":
				m.DenyTools = []string{b.ToolName}
			case "no_prepare":
				m.Effects = []Effect{EffectRead}
			case "revoked":
				m.Status = "revoked"
			case "expired":
				m.ExpiresAt = now.Add(-time.Second)
			case "stale_revision":
				b.MandateRevision++
			case "foreign_owner":
				b.OwnerID = "other"
			case "wrong_purpose":
				b.Purpose = "other"
			case "foreign_resource":
				b.Resource = "https://other.invalid/"
			case "changed_contract":
				b.ContractHash = strings.Repeat("f", 64)
			case "missing_destinations":
				b.CommunicationDestinationsHash = ""
			case "credentials":
				b.CredentialRefs = []CredentialRef{{Alias: "secret", Field: "password"}}
			case "unknown_effect":
				b.Effect = EffectUnknown
			case "browser_click":
				b.ToolName = "browser.click"
				m.Tools = append(m.Tools, b.ToolName)
			}
			if (CheckAssignedAction(m, b, now) == nil) != (scenario == "allowed") {
				t.Fatal("owner reply widened exact mandate or external resource")
			}
		})
	}
}
