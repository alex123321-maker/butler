package semantic

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
)

// CreateTx supports atomic publication of a new owned indexing job. New rows
// acquire no existing task/session locks after catalog/authority locks.
func (s *Store) CreateTx(ctx context.Context, tx pgx.Tx, space Space, scope Scope) (Generation, error) {
	if s == nil || s.pool == nil || tx == nil || space.Validate() != nil || !scope.valid() {
		return Generation{}, ErrInvalid
	}
	g := Generation{ID: freshID(), Space: space, Scope: scope, State: "building"}
	raw, e := json.Marshal(space)
	if e != nil {
		return g, ErrInvalid
	}
	e = tx.QueryRow(ctx, `INSERT INTO semantic_index_generations(id,space_id,space_data,dimensions,scope_type,scope_id) VALUES($1,$2,$3::jsonb,$4,$5,$6) RETURNING created_at`, g.ID, space.ID(), string(raw), space.Dimensions, scope.Type, scope.ID).Scan(&g.CreatedAt)
	return g, e
}
