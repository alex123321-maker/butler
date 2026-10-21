package communication

import (
	"context"
	"encoding/json"
	"github.com/butler/butler/internal/communicationcontrol"
	"github.com/jackc/pgx/v5"
)

func (s *Store) LookupTx(ctx context.Context, tx pgx.Tx, p Principal, e communicationcontrol.Envelope) (communicationcontrol.Receipt, error) {
	var out communicationcontrol.Receipt
	if !checkToken(ctx, p) || s.SourceGuard == nil {
		return out, ErrDenied
	}
	if err := s.SourceGuard(ctx, tx, p.RunID); err != nil {
		return out, ErrDenied
	}
	a, err := communicationcontrol.DecodeArgs(e.Tool, e.ArgsJSON)
	if err != nil {
		return out, err
	}
	raw, _ := json.Marshal(a)
	var receipt []byte
	if tx.QueryRow(ctx, `SELECT receipt_json FROM communication_publications WHERE NOT privacy_redacted AND effect_id=$1 AND run_id=$2 AND owner_id=$3 AND input_id=$4 AND tool_call_id=$5 AND payload_hash=$6`, e.EffectID, p.RunID, p.OwnerID, p.InputID, e.ToolCallID, hash(string(raw))).Scan(&receipt) != nil {
		return out, ErrDenied
	}
	err = json.Unmarshal(receipt, &out)
	return out, err
}
