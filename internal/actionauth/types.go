// Package actionauth defines the trusted action authorization boundary shared
// by orchestration, Tool Broker and credential mediation. Models propose args;
// they do not supply trusted context, approval decisions or execution grants.
package actionauth

import "time"

type Effect string

const (
	EffectRead    Effect = "read"
	EffectPrepare Effect = "prepare"
	EffectCommit  Effect = "commit"
	EffectUnknown Effect = "unknown"
)

type CredentialRef struct {
	Alias string `json:"alias"`
	Field string `json:"field"`
}

// ActionBinding is the exact intent approved by the owner. A refreshed worker
// lease does not change this intent, but each dispatch grant pins its live fence.
type ActionBinding struct {
	ActionID               string          `json:"action_id"`
	ActionHash             string          `json:"action_hash"`
	RunID                  string          `json:"run_id"`
	ToolCallID             string          `json:"tool_call_id"`
	ToolName               string          `json:"tool_name"`
	ContractHash           string          `json:"contract_hash"`
	ContractRevision       string          `json:"contract_revision,omitempty"`
	ArgsHash               string          `json:"args_hash"`
	Resource               string          `json:"resource"`
	CredentialRefs         []CredentialRef `json:"credential_refs"`
	CredentialMetadataHash string          `json:"credential_metadata_hash"`
	TaskID                 string          `json:"task_id"`
	TaskRevision           int64           `json:"task_revision"`
	ConstraintsRevision    int64           `json:"constraints_revision"`
	OwnerID                string          `json:"owner_id"`
	Effect                 Effect          `json:"effect"`
	GoalID                 string          `json:"goal_id,omitempty"`
	GoalRevision           int64           `json:"goal_revision,omitempty"`
	MandateID              string          `json:"owner_mandate_id,omitempty"`
	MandateRevision        int64           `json:"owner_mandate_revision,omitempty"`
	Purpose                string          `json:"purpose,omitempty"`
	OwnerPolicyRevision    int64           `json:"owner_policy_revision,omitempty"`
}

// ContextSnapshot must come from Session/Task/Run Service, never tool arguments.
type ContextSnapshot struct {
	RunID               string          `json:"run_id"`
	RunState            string          `json:"run_state"`
	SessionKey          string          `json:"session_key"`
	OwnerID             string          `json:"owner_id"`
	AutonomyMode        int32           `json:"autonomy_mode"`
	MandateID           string          `json:"mandate_id"`
	TaskID              string          `json:"task_id"`
	PinnedTaskRevision  int64           `json:"pinned_task_revision"`
	TaskRevision        int64           `json:"task_revision"`
	ConstraintsRevision int64           `json:"constraints_revision"`
	TaskStatus          string          `json:"task_status"`
	ExecutionPolicy     string          `json:"execution_policy"`
	LeaseEpoch          int64           `json:"lease_epoch"`
	LeaseID             string          `json:"lease_id"`
	LeaseExpiresAt      time.Time       `json:"lease_expires_at"`
	Authority           *Assignment     `json:"authority,omitempty"`
	AuthorityDeadline   time.Time       `json:"authority_deadline,omitempty"`
	ModelPrivacy        string          `json:"model_privacy,omitempty"`
	DenyTools           []string        `json:"deny_tools,omitempty"`
	DenyResources       []ResourceScope `json:"deny_resources,omitempty"`
}

type ApprovalRequest struct {
	ApprovalID string        `json:"approval_id"`
	Binding    ActionBinding `json:"binding"`
}

// ApprovalSnapshot is returned only after approval Service checks its durable
// record, owner, expiry and current task versions against the exact binding.
type ApprovalSnapshot struct {
	ApprovalID  string        `json:"approval_id"`
	Status      string        `json:"status"`
	Binding     ActionBinding `json:"binding"`
	ExpiresAt   time.Time     `json:"expires_at"`
	ResolvedAt  time.Time     `json:"resolved_at"`
	ResolvedVia string        `json:"resolved_via"`
}

type PreparedAction struct {
	Binding          ActionBinding `json:"binding"`
	EffectID         string        `json:"effect_id"`
	RequiresApproval bool          `json:"requires_approval"`
	Summary          string        `json:"summary"`
	ExpiresAt        time.Time     `json:"expires_at"`
}

// Grant is private system metadata; it is never part of model-visible args.
// Claiming a grant records dispatch, not successful completion of an effect.
type Grant struct {
	ID              string        `json:"authorization_id"`
	Binding         ActionBinding `json:"binding"`
	ApprovalID      string        `json:"approval_id"`
	MandateID       string        `json:"mandate_id"`
	MandateRevision int64         `json:"mandate_revision"`
	LeaseID         string        `json:"lease_id"`
	LeaseEpoch      int64         `json:"lease_epoch"`
	AutonomyMode    int32         `json:"autonomy_mode"`
	ExpiresAt       time.Time     `json:"expires_at"`
	State           string        `json:"state"`
}

type Mandate struct {
	ScopeKind           string                `json:"scope_kind,omitempty"`
	GoalID              string                `json:"goal_id,omitempty"`
	GoalRevision        int64                 `json:"goal_revision,omitempty"`
	Status              string                `json:"status,omitempty"`
	Purpose             string                `json:"purpose,omitempty"`
	ResourceScopes      []ResourceScope       `json:"resource_scopes,omitempty"`
	DenyTools           []string              `json:"deny_tools,omitempty"`
	DenyResources       []ResourceScope       `json:"deny_resources,omitempty"`
	Privacy             string                `json:"privacy,omitempty"`
	Budget              MandateBudget         `json:"budget,omitempty"`
	BudgetDeadline      time.Time             `json:"budget_deadline,omitempty"`
	Hours               *ActiveHours          `json:"active_hours,omitempty"`
	AllowInitiative     bool                  `json:"allow_initiative,omitempty"`
	Intervention        []string              `json:"intervention,omitempty"`
	Reporting           string                `json:"reporting,omitempty"`
	Extensions          []ExtensionPermission `json:"extensions,omitempty"`
	Source              map[string]string     `json:"source,omitempty"`
	ID                  string                `json:"mandate_id"`
	OwnerID             string                `json:"owner_id"`
	TaskID              string                `json:"task_id"`
	TaskRevision        int64                 `json:"task_revision"`
	ConstraintsRevision int64                 `json:"constraints_revision"`
	Revision            int64                 `json:"revision"`
	Tools               []string              `json:"tools"`
	Resources           []string              `json:"resources"`
	Effects             []Effect              `json:"effects"`
	CredentialRefs      []CredentialRef       `json:"credential_refs"`
	ExpiresAt           time.Time             `json:"expires_at"`
	RevokedAt           *time.Time            `json:"revoked_at,omitempty"`
}
