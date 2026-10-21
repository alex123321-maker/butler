package communication

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
)

type bindingReader interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

// DestinationHash contains no exposed recipient data. Exact authorization pins
// it before execution; rebind/disable/addition invalidates the prepared action.
func DestinationHash(ctx context.Context, q bindingReader, owner, kind string) (string, error) {
	rows, e := q.Query(ctx, `SELECT binding_id,channel,recipient,version FROM communication_bindings WHERE owner_id=$1 AND enabled AND verified AND capabilities ? $2 ORDER BY binding_id`, owner, kind)
	if e != nil {
		return "", e
	}
	defer rows.Close()
	type target struct {
		ID, Channel, Recipient string
		Version                int64
	}
	var values []target
	for rows.Next() {
		var t target
		if e = rows.Scan(&t.ID, &t.Channel, &t.Recipient, &t.Version); e != nil {
			return "", e
		}
		values = append(values, t)
	}
	if e = rows.Err(); e != nil {
		return "", e
	}
	if len(values) == 0 {
		return "", ErrDenied
	}
	raw, _ := json.Marshal(values)
	return hash(string(raw)), nil
}
