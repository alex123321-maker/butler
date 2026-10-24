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

// MarkScheduledCommunicationTx is an owning-kernel continuation receipt. It
// copies only the original current binding, never pretends a timer is a human.
// Public payloads and service callers cannot choose identity, origin or target.
func (s *Store) MarkScheduledCommunicationTx(ctx context.Context, tx pgx.Tx, occurrence string) error {
	var id, owner, sourceRun string
	var raw []byte
	var epoch int
	err := tx.QueryRow(ctx, `SELECT o.input_id,j.owner_id,j.source_run_id,COALESCE(i.communication_ingress_epoch,0),i.communication_ingress_json FROM scheduled_occurrences o JOIN scheduled_jobs j USING(schedule_id) LEFT JOIN accepted_inputs i ON i.run_id=j.source_run_id AND i.owner_id=j.owner_id AND i.session_key=j.session_key WHERE o.occurrence_id=$1 AND o.schedule_revision=j.revision AND j.spec_json->>'mode'='agent_task' AND j.state='active' AND j.expires_at>clock_timestamp() FOR SHARE OF j`, occurrence).Scan(&id, &owner, &sourceRun, &epoch, &raw)
	if err != nil {
		return err
	}
	if epoch == 0 {
		return nil
	} // Legacy jobs retain their existing delivery profile.
	var p CommunicationIngress
	if epoch != 1 || json.Unmarshal(raw, &p) != nil || p.OwnerID != owner || (p.Kind != "web_message" && p.Kind != "telegram_message" && p.Kind != "owner_control") {
		return errors.New("scheduled origin unavailable")
	}
	r, err := s.GetTx(ctx, tx, id)
	if err != nil {
		return err
	}
	var valid bool
	if tx.QueryRow(ctx, `SELECT TRUE FROM scheduled_occurrences o JOIN scheduled_jobs j USING(schedule_id) JOIN tasks t ON t.task_id=o.task_id JOIN communication_bindings b ON b.binding_id=$4 AND b.version=$5 AND b.owner_id=j.owner_id AND b.enabled AND b.verified WHERE o.occurrence_id=$1 AND o.input_id=$2 AND o.task_id=$3 AND j.source_run_id=$6 AND t.owner_id=j.owner_id AND t.session_key=j.session_key AND t.source_json->>'occurrence_id'=o.occurrence_id AND t.source_json->>'schedule_id'=j.schedule_id FOR SHARE OF b,t`, occurrence, id, r.TaskID, p.BindingID, p.BindingVersion, sourceRun).Scan(&valid) != nil {
		return errors.New("scheduled identity unavailable")
	}
	p.Kind = "scheduled_occurrence"
	p.ActorChannel = "schedule"
	p.ActorDeliveryID = occurrence
	p.DeliveryID = r.Event.EventID
	p.PayloadHash, err = actionauth.ArgumentsHash(r.Event.PayloadJSON)
	if err != nil || r.Source != "schedule" || r.Event.EventID != occurrence || r.OwnerID != owner {
		return errors.New("scheduled input mismatch")
	}
	raw, err = json.Marshal(p)
	if err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE accepted_inputs SET communication_ingress_epoch=1,communication_ingress_json=$2::jsonb WHERE input_id=$1 AND communication_ingress_epoch=0 AND source='schedule'`, id, raw)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("scheduled input already pinned")
	}
	return nil
}

// Source binding changes fence future ticks as well as later delivery. Legacy
// schedules without an explicit source receipt keep their original profile.
func (s *Store) CheckScheduledCommunicationSourceTx(ctx context.Context, tx pgx.Tx, run string) error {
	var epoch int
	var raw []byte
	var owner string
	err := tx.QueryRow(ctx, `SELECT communication_ingress_epoch,communication_ingress_json,owner_id FROM accepted_inputs WHERE run_id=$1 FOR SHARE`, run).Scan(&epoch, &raw, &owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if epoch == 0 {
		return nil
	}
	var p CommunicationIngress
	if epoch != 1 || json.Unmarshal(raw, &p) != nil || p.OwnerID != owner || (p.Kind != "web_message" && p.Kind != "telegram_message" && p.Kind != "owner_control") {
		return errors.New("scheduled source receipt unavailable")
	}
	var valid bool
	if tx.QueryRow(ctx, `SELECT TRUE FROM communication_bindings WHERE binding_id=$1 AND version=$2 AND owner_id=$3 AND enabled AND verified FOR SHARE`, p.BindingID, p.BindingVersion, owner).Scan(&valid) != nil {
		return errors.New("scheduled source binding changed")
	}
	return nil
}
