package input

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5"
)

// CommunicationIngress is a trusted transport receipt. Public event payloads
// cannot set these columns; old workers cannot claim an epoch-1 ingress.
type CommunicationIngress struct {
	OwnerID, BindingID, DeliveryID, PayloadHash string
	Kind, ActorChannel, ActorDeliveryID         string
	BindingVersion, BotID                       int64
}

func (s *Store) MarkCommunicationTx(ctx context.Context, tx pgx.Tx, r Record, p CommunicationIngress) error {
	a, ok := owneridentity.FromContext(ctx)
	hash, err := actionauth.ArgumentsHash(r.Event.PayloadJSON)
	if !ok || tx == nil || a.OwnerID != r.OwnerID || p.OwnerID != r.OwnerID || p.ActorChannel != a.Channel || p.ActorDeliveryID != a.DeliveryID || p.BindingID == "" || p.BindingVersion < 1 || p.BotID < 0 || p.DeliveryID != r.Event.EventID || err != nil || p.PayloadHash != hash {
		return errors.New("verified communication input required")
	}
	switch p.Kind {
	case "telegram_message":
		if a.Channel != "telegram" || r.Source != "telegram" || a.DeliveryID != r.Event.EventID || p.BotID <= 0 {
			return errors.New("verified Telegram delivery required")
		}
	case "web_message":
		if a.Channel != "web" || r.Source != "web" || a.DeliveryID != r.Event.EventID || p.BotID != 0 {
			return errors.New("verified Web delivery required")
		}
	case "owner_control":
		var valid bool
		if tx.QueryRow(ctx, `SELECT TRUE FROM task_control_events WHERE control_id||':continue'=$1 AND owner_id=$2 AND source=$3 AND delivery_id=$4 AND task_id=$5`, r.Event.EventID, a.OwnerID, a.Channel, a.DeliveryID, r.TaskID).Scan(&valid) != nil {
			return errors.New("owning control continuation required")
		}
	default:
		return errors.New("unsupported communication ingress proof")
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE accepted_inputs SET communication_ingress_epoch=1,communication_ingress_json=$2::jsonb WHERE input_id=$1 AND owner_id=$3 AND source=$7 AND event_id=$4 AND session_key=$5 AND event_json->>'PayloadJSON'=$6 AND (communication_ingress_epoch=0 OR communication_ingress_epoch=1 AND communication_ingress_json=$2::jsonb)`, r.InputID, raw, r.OwnerID, p.DeliveryID, r.SessionKey, r.Event.PayloadJSON, r.Source)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("communication input identity mismatch")
	}
	return nil
}

func (s *Store) GetTx(ctx context.Context, tx pgx.Tx, id string) (Record, error) {
	return scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM accepted_inputs WHERE input_id=$1`, id))
}
