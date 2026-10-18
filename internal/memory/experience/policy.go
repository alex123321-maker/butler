package experience

import (
	"context"
	"github.com/butler/butler/internal/memory/jobs"
)

// The owner selects an existing completed/stopped attempt and finite independent policy,
// not evidence contents, environment, verdict or model-written source parents.
type PolicyRequest struct {
	RunID  string                  `json:"run_id"`
	Policy jobs.ModelPolicyRequest `json:"policy"`
}
type PolicyCreator interface {
	Create(context.Context, PolicyRequest) (jobs.ModelPolicy, error)
}
