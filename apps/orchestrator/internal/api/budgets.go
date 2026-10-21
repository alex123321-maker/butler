package api

import (
	"net/http"

	"github.com/butler/butler/internal/budget"
)

// This read model requires the same owner proof as task mutations. A query/body
// owner ID cannot select another owner's budget or authorize spending.
func NewBudgetHandler(ledger *budget.Ledger, auth *OwnerControlAuth, ownerDayEnabled bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		actor, ok := auth.Authenticate(r)
		if !ok {
			http.Error(w, "owner authentication required", http.StatusUnauthorized)
			return
		}
		s, err := ledger.SnapshotOwner(r.Context(), actor.OwnerID)
		if err != nil {
			http.Error(w, "budget observations unavailable", http.StatusServiceUnavailable)
			return
		}
		writeJSON(w, http.StatusOK, struct {
			budget.Snapshot
			OwnerDayEnabled bool `json:"owner_day_enabled"`
		}{s, ownerDayEnabled})
	})
}
