// Package actionauth defines the trusted action authorization boundary shared
// by orchestration, Tool Broker and credential mediation. Models propose args;
// they do not supply trusted context, approval decisions or execution grants.
package actionauth

import (
	"github.com/butler/butler/internal/delegation"
	"time"
)

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

// DataRef is content-free exact Catalog metadata bound by an owned capability
// guard. It grants no access and never contains document/program/secret bytes.
type DataRef struct {
	ID             string `json:"source_id"`
	Revision       int64  `json:"revision"`
	AccessRevision int64  `json:"access_revision"`
	Hash           string `json:"content_hash"`
}

// ActionBinding is the exact intent approved by the owner. A refreshed worker
// lease does not change this intent, but each dispatch grant pins its live fence.
type ActionBinding struct {
	OwnerCommunication            bool            `json:"owner_communication,omitempty"`
	CommunicationDestinationsHash string          `json:"communication_destinations_hash,omitempty"`
	ActionID                      string          `json:"action_id"`
	ActionHash                    string          `json:"action_hash"`
	RunID                         string          `json:"run_id"`
	ToolCallID                    string          `json:"tool_call_id"`
	ToolName                      string          `json:"tool_name"`
	ContractHash                  string          `json:"contract_hash"`
	ContractRevision              string          `json:"contract_revision,omitempty"`
	ArgsHash                      string          `json:"args_hash"`
	Resource                      string          `json:"resource"`
	CredentialRefs                []CredentialRef `json:"credential_refs"`
	CredentialMetadataHash        string          `json:"credential_metadata_hash"`
	TaskID                        string          `json:"task_id"`
	TaskRevision                  int64           `json:"task_revision"`
	ConstraintsRevision           int64           `json:"constraints_revision"`
	OwnerID                       string          `json:"owner_id"`
	Effect                        Effect          `json:"effect"`
	GoalID                        string          `json:"goal_id,omitempty"`
	GoalRevision                  int64           `json:"goal_revision,omitempty"`
	MandateID                     string          `json:"owner_mandate_id,omitempty"`
	MandateRevision               int64           `json:"owner_mandate_revision,omitempty"`
	Purpose                       string          `json:"purpose,omitempty"`
	OwnerPolicyRevision           int64           `json:"owner_policy_revision,omitempty"`
	ExtensionID                   string          `json:"extension_id,omitempty"`
	ExtensionManifestHash         string          `json:"extension_manifest_hash,omitempty"`
	ExtensionGeneration           int64           `json:"extension_generation,omitempty"`
	ExtensionAccessRevision       int64           `json:"extension_access_revision,omitempty"`
	ExtensionAccessExpiresAt      *time.Time      `json:"extension_access_expires_at,omitempty"`
	ConverterID                   string          `json:"converter_id,omitempty"`
	ConverterPackageHash          string          `json:"converter_package_hash,omitempty"`
	ConverterNeedHash             string          `json:"converter_need_hash,omitempty"`
	ConverterGeneration           int64           `json:"converter_generation,omitempty"`
	DataSources                   []DataRef       `json:"data_sources,omitempty"`
}

// ContextSnapshot must come from Session/Task/Run Service, never tool arguments.
type ContextSnapshot struct {
	Delegation          *delegation.Contract `json:"delegation,omitempty"`
	RunID               string               `json:"run_id"`
	RunState            string               `json:"run_state"`
	SessionKey          string               `json:"session_key"`
	OwnerID             string               `json:"owner_id"`
	AutonomyMode        int32                `json:"autonomy_mode"`
	MandateID           string               `json:"mandate_id"`
	TaskID              string               `json:"task_id"`
	PinnedTaskRevision  int64                `json:"pinned_task_revision"`
	TaskRevision        int64                `json:"task_revision"`
	ConstraintsRevision int64                `json:"constraints_revision"`
	TaskStatus          string               `json:"task_status"`
	ExecutionPolicy     string               `json:"execution_policy"`
	LeaseEpoch          int64                `json:"lease_epoch"`
	LeaseID             string               `json:"lease_id"`
	LeaseExpiresAt      time.Time            `json:"lease_expires_at"`
	Authority           *Assignment          `json:"authority,omitempty"`
	AuthorityDeadline   time.Time            `json:"authority_deadline,omitempty"`
	ModelPrivacy        string               `json:"model_privacy,omitempty"`
	// Operator-attested applicability metadata; missing is not compatible.
	ExperienceEnvironmentHash string            `json:"experience_environment_hash,omitempty"`
	DenyTools                 []string          `json:"deny_tools,omitempty"`
	DenyResources             []ResourceScope   `json:"deny_resources,omitempty"`
	ConverterLifecycleReady   bool              `json:"converter_lifecycle_ready,omitempty"`
	DelegationReady           bool              `json:"delegation_ready,omitempty"`
	GuardedTabRequestReady    bool              `json:"guarded_tab_request_ready,omitempty"`
	GuardedTabRuntimeReady    bool              `json:"guarded_tab_runtime_ready,omitempty"`
	MemoryCandidateReady      bool              `json:"memory_candidate_ready,omitempty"`
	CommunicationTargets      map[string]string `json:"communication_targets,omitempty"`
	CommunicationReady        bool              `json:"communication_ready,omitempty"`
	CommunicationMethods      []string          `json:"communication_methods,omitempty"`
	CommunicationMode         string            `json:"communication_mode,omitempty"`
	SchedulesReady            bool              `json:"schedules_ready,omitempty"`
	MemoryReadReady           bool              `json:"memory_read_ready,omitempty"`
	MemoryTemporalReady       bool              `json:"memory_temporal_ready,omitempty"`
	WorkspaceReady            bool              `json:"workspace_ready,omitempty"`
	InputClaimHash            string            `json:"input_claim_hash,omitempty"`
	ConverterDelegationReady  bool              `json:"converter_delegation_ready,omitempty"`
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
	InputClaimHash  string        `json:"input_claim_hash,omitempty"`
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
	Converters          []ConverterPermission `json:"converters,omitempty"`
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
