package input

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/butler/butler/internal/owneridentity"
)

// ReconciledObservation contains only the owning database's typed identity,
// enum and hash projection. It grants no execution permission and contains no
// checkpoint, owner instruction, final or tool response body.
type ReconciledObservation struct {
	Context json.RawMessage
	Hash    string
}

func (s *Store) ObserveReconciled(ctx context.Context, run, snapshot, control string) (ReconciledObservation, error) {
	var out ReconciledObservation
	a, ok := owneridentity.FromContext(ctx)
	if !ok || a.OwnerID == "" || a.DeliveryID == "" || s == nil || run == "" || snapshot == "" || control == "" {
		return out, fmt.Errorf("explicit owner reconciled-attempt observation required")
	}
	err := s.pool.QueryRow(ctx, `WITH proof AS(SELECT butler_memory_reconciled_context($1,$2,$3,$4) AS p) SELECT p,encode(sha256(convert_to(p::text,'UTF8')),'hex') FROM proof WHERE p IS NOT NULL AND jsonb_array_length(p->'heads') BETWEEN 10 AND 33`, run, snapshot, control, a.OwnerID).Scan(&out.Context, &out.Hash)
	if err != nil || len(out.Context) > 16384 || len(out.Hash) != 64 {
		return ReconciledObservation{}, fmt.Errorf("current separately reconciled attempt evidence unavailable")
	}
	return out, nil
}
