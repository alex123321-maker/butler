package approval

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/butler/butler/internal/actionauth"
)

type OwnerActionFacts struct {
	Summary                  string `json:"summary"`
	Effect                   string `json:"effect"`
	Resource                 string `json:"resource"`
	TaskID                   string `json:"task_id,omitempty"`
	TaskRevision             int64  `json:"task_revision"`
	ConstraintsRevision      int64  `json:"constraints_revision"`
	GoalID                   string `json:"goal_id,omitempty"`
	GoalRevision             int64  `json:"goal_revision"`
	MandateID                string `json:"mandate_id,omitempty"`
	MandateRevision          int64  `json:"mandate_revision"`
	ContractHash             string `json:"contract_hash"`
	ArgsHash                 string `json:"args_hash"`
	CredentialReferenceCount int    `json:"credential_reference_count"`
}

var ownerFactID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.:-]{0,255}$`)
var ownerFactHash = regexp.MustCompile(`^[a-f0-9]{64}$`)

// OwnerFactsForRecord reads only the service-owned durable binding. Raw args,
// continuation, credentials, actor/lease capabilities and free-form details are
// never projected. Saved versions describe the proposal, not live permission.
func OwnerFactsForRecord(record Record) *OwnerActionFacts {
	var details struct {
		Binding actionauth.ActionBinding `json:"action_binding"`
	}
	if json.Unmarshal([]byte(record.DetailsJSON), &details) != nil {
		return nil
	}
	b := details.Binding
	if !ownerFactID.MatchString(b.ActionID) || b.RunID != record.RunID || b.ToolCallID != record.ToolCallID || b.ToolName != record.ToolName || !ownerFactID.MatchString(b.ToolName) || !ownerFactHash.MatchString(b.ContractHash) || !ownerFactHash.MatchString(b.ArgsHash) {
		return nil
	}
	for _, ref := range []string{b.TaskID, b.GoalID, b.MandateID} {
		if ref != "" && !ownerFactID.MatchString(ref) {
			return nil
		}
	}
	resource := ""
	if parsed, err := url.Parse(b.Resource); err == nil && (parsed.Scheme == "https" || parsed.Scheme == "http") && parsed.Hostname() != "" {
		parsed.User = nil
		parsed.RawQuery = ""
		parsed.ForceQuery = false
		parsed.Fragment = ""
		parsed.RawFragment = ""
		resource = parsed.String()
	} else if strings.HasPrefix(b.Resource, "workspace:") && ownerFactID.MatchString(strings.TrimPrefix(b.Resource, "workspace:")) {
		resource = b.Resource
	}
	effect := string(b.Effect)
	if effect != "read" && effect != "prepare" && effect != "commit" {
		effect = "unknown"
	}
	target := resource
	if target == "" {
		target = "resource metadata unavailable"
	}
	return &OwnerActionFacts{Summary: fmt.Sprintf("%s (%s) on %s", b.ToolName, effect, target), Effect: effect, Resource: resource, TaskID: b.TaskID, TaskRevision: b.TaskRevision, ConstraintsRevision: b.ConstraintsRevision, GoalID: b.GoalID, GoalRevision: b.GoalRevision, MandateID: b.MandateID, MandateRevision: b.MandateRevision, ContractHash: b.ContractHash, ArgsHash: b.ArgsHash, CredentialReferenceCount: len(b.CredentialRefs)}
}
