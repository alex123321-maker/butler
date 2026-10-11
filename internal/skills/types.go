package skills

import (
	"context"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/memory/catalog"
	"time"
)

// SourceProof comes from the private task/result verifier, never a submitted
// model verdict. Text/effect evidence is omitted: only stable references enter.
type SourceProof struct {
	EvidenceRef
	OwnerID            string              `json:"owner_id"`
	Verifier           string              `json:"verifier"`
	Verdict            string              `json:"verdict"`
	EvidenceRefs       []string            `json:"evidence_refs"`
	ConfirmedTools     []string            `json:"confirmed_tools"`
	ConfirmedContracts []Dependency        `json:"confirmed_contracts"`
	ConfirmedActions   []string            `json:"confirmed_actions"`
	Failures           int64               `json:"failures"`
	ModelAttempts      int64               `json:"model_attempts"`
	ModelUsage         *budget.ModelUsage  `json:"model_usage,omitempty"`
	ContentParents     []catalog.SourceRef `json:"content_parents"`
}
type EvidenceVerifier interface {
	VerifySkillSources(context.Context, string, []EvidenceRef) ([]SourceProof, error)
}
type ContractReader interface {
	GetSkillContract(context.Context, string) (*toolbrokerv1.ToolContract, error)
}
type RunEnvironment interface {
	GetActionContext(context.Context, string) (actionauth.ContextSnapshot, error)
	ContractsForRun(context.Context, string) ([]*toolbrokerv1.ToolContract, error)
}
type Validation struct {
	Validator string        `json:"validator"`
	Passed    bool          `json:"passed"`
	Checks    []string      `json:"checks"`
	Sources   []SourceProof `json:"sources"`
}
type Record struct {
	ID            string            `json:"skill_id"`
	OwnerID       string            `json:"owner_id"`
	Digest        string            `json:"digest"`
	Generation    int64             `json:"generation"`
	Status        string            `json:"status"`
	Active        bool              `json:"active"`
	Package       Package           `json:"package"`
	Version       string            `json:"version"`
	BodySource    catalog.SourceRef `json:"body_source"`
	BodyAvailable bool              `json:"body_available"`
	dependencies  []Dependency
	shape         string
}
type Descriptor struct {
	ID          string `json:"skill_id"`
	Digest      string `json:"digest"`
	Version     string `json:"version"`
	Description string `json:"description"`
}
type Usage struct {
	RunID               string    `json:"run_id"`
	TaskID              string    `json:"task_id"`
	TaskRevision        int64     `json:"task_revision"`
	ConstraintsRevision int64     `json:"constraints_revision"`
	Digest              string    `json:"digest"`
	LoadedAt            time.Time `json:"loaded_at"`
}
type CorrectionGrant struct {
	ID              string    `json:"grant_id"`
	SkillID         string    `json:"skill_id"`
	ParentDigest    string    `json:"parent_digest"`
	Class           string    `json:"class"`
	MandateID       string    `json:"mandate_id"`
	MandateRevision int64     `json:"mandate_revision"`
	Purpose         string    `json:"purpose"`
	ExpiresAt       time.Time `json:"expires_at"`
}

type Outcome struct {
	Usage             Usage              `json:"usage"`
	ResultID          string             `json:"result_id,omitempty"`
	Verdict           string             `json:"verdict"`
	KnownToolFailures int64              `json:"known_tool_failures"`
	ModelUsage        *budget.ModelUsage `json:"model_usage,omitempty"`
}
type OutcomeReader interface {
	ReadSkillOutcomes(context.Context, string, []Usage) ([]Outcome, error)
}
type QualityReport struct {
	Digest             string        `json:"digest"`
	Baseline           []SourceProof `json:"baseline"`
	Outcomes           []Outcome     `json:"outcomes"`
	VerifiedSuccesses  int           `json:"verified_successes"`
	UnverifiedAttempts int           `json:"unverified_attempts"`
	KnownToolFailures  int64         `json:"known_tool_failures"`
}
