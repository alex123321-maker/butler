package jobs

import (
	"context"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/jackc/pgx/v5"
)

// ModelScheduleHandler is code-owned preparation under the exact current owner
// policy. It cannot enlarge its stable source identities or access permissions.
// A bounded reason defers work without reserving an inference attempt.
type ModelScheduleHandler interface {
	PrepareSourcesTx(context.Context, pgx.Tx, *catalog.Store, ModelPolicy, []catalog.Source) ([]catalog.Source, string, error)
}
