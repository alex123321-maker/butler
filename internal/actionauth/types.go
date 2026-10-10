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
	ArgsHash               string          `json:"args_hash"`
	Resource               string          `json:"resource"`
	CredentialRefs         []CredentialRef `json:"credential_refs"`
	CredentialMetadataHash string          `json:"credential_metadata_hash"`
	TaskID                 string          `json:"task_id"`
	TaskRevision           int64           `json:"task_revision"`
	ConstraintsRevision    int64           `json:"constraints_revision"`
	OwnerID                string          `json:"owner_id"`
	Effect                 Effect          `json:"effect"`
}

// ContextSnapshot must come from Session/Task/Run Service, never tool arguments.
type ContextSnapshot struct {
	RunID               string    `json:"run_id"`
	RunState            string    `json:"run_state"`
	SessionKey          string    `json:"session_key"`
	OwnerID             string    `json:"owner_id"`
	AutonomyMode        int32     `json:"autonomy_mode"`
	TaskID              string    `json:"task_id"`
	PinnedTaskRevision  int64     `json:"pinned_task_revision"`
	TaskRevision        int64     `json:"task_revision"`
	ConstraintsRevision int64     `json:"constraints_revision"`
	TaskStatus          string    `json:"task_status"`
	LeaseEpoch          int64     `json:"lease_epoch"`
	LeaseID             string    `json:"lease_id"`
	LeaseExpiresAt      time.Time `json:"lease_expires_at"`
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
	ID                  string          `json:"mandate_id"`
	OwnerID             string          `json:"owner_id"`
	TaskID              string          `json:"task_id"`
	TaskRevision        int64           `json:"task_revision"`
	ConstraintsRevision int64           `json:"constraints_revision"`
	Revision            int64           `json:"revision"`
	Tools               []string        `json:"tools"`
	Resources           []string        `json:"resources"`
	Effects             []Effect        `json:"effects"`
	CredentialRefs      []CredentialRef `json:"credential_refs"`
	ExpiresAt           time.Time       `json:"expires_at"`
	RevokedAt           *time.Time      `json:"revoked_at,omitempty"`
}
