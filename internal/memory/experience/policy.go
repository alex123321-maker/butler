package experience

import (
	"context"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/jobs"
)

// The owner selects an existing completed/stopped attempt and finite independent policy,
// not evidence contents, environment, verdict or model-written source parents.
type PolicyRequest struct {
	RunID          string                  `json:"run_id"`
	Policy         jobs.ModelPolicyRequest `json:"policy"`
	Reconciliation *ReconciliationRequest  `json:"reconciliation,omitempty"`
}

// The owner selects existing immutable witness/control identities, never
// reconciliation contents, response bytes, verdict or a synthetic final.
type ReconciliationRequest struct {
	OriginalSnapshotID string `json:"original_snapshot_id"`
	ControlID          string `json:"control_id"`
}
type PolicyCreator interface {
	SealObservation(context.Context, string) (catalog.SourceRef, error)
	Create(context.Context, PolicyRequest) (jobs.ModelPolicy, error)
}
