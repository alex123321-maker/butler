package communication

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/communicationcontrol"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5"
)

var ErrAmbiguous = errors.New("explicit reply target required")
var ErrInteractionRequired = errors.New("typed interaction response required")
var ErrReplyTargetUnavailable = fmt.Errorf("%w: reply target unavailable", ErrDenied)

func replyLookupError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrReplyTargetUnavailable
	}
	return ErrDenied
}

// ChannelACK is private adapter DATA, never an instruction or execution grant.
// It is accepted only with the current verified binding and a stored sent ACK.
type ReplyRequest struct {
	BindingID      string `json:"binding_id"`
	BindingVersion int64  `json:"binding_version"`
	PublicationRef string `json:"publication_ref,omitempty"`
	ChannelACK     string `json:"channel_ack,omitempty"`
	Text           string `json:"text"`
}

func (s *Store) Reply(ctx context.Context, q ReplyRequest) (input.Record, error) {
	a, ok := owneridentity.FromContext(ctx)
	if !ok || s == nil || s.Inputs == nil || s.SourceGuard == nil || s.ChannelGuard == nil || a.SessionKey == "" || len(a.DeliveryID) > 512 || !utf8.ValidString(q.Text) || strings.TrimSpace(q.Text) == "" || utf8.RuneCountInString(q.Text) > 4000 || len(q.ChannelACK) > 512 {
		return input.Record{}, ErrDenied
	}
	if (q.PublicationRef == "") == (q.ChannelACK == "") {
		return input.Record{}, ErrAmbiguous
	}
	if kind, _ := task.ClassifyOwnerText(q.Text); kind != "" {
		return input.Record{}, ErrOwnerControlRequired
	}
	if q.BindingID == "" || len(q.BindingID) > 128 || q.BindingVersion < 1 || len(q.PublicationRef) > 128 {
		return input.Record{}, ErrDenied
	}
	if a.Channel == "telegram" && q.ChannelACK == "" || a.Channel == "web" && q.PublicationRef == "" {
		return input.Record{}, ErrDenied
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return input.Record{}, err
	}
	defer tx.Rollback(context.Background())
	// Resolve through durable delivery identity; never choose the owner's last run.
	rows, err := tx.Query(ctx, `SELECT DISTINCT p.publication_id,p.session_key FROM communication_publications p JOIN delivery_outbox d USING(publication_id) JOIN communication_bindings b ON b.binding_id=d.binding_id AND b.version=d.binding_version WHERE b.binding_id=$1 AND b.version=$2 AND b.owner_id=$3 AND b.channel=$4 AND b.recipient=$5 AND b.enabled AND b.verified AND d.recipient_snapshot=b.recipient AND NOT p.privacy_redacted AND NOT d.privacy_redacted AND (($6<>'' AND p.publication_id=$6) OR ($7<>'' AND d.status='sent' AND d.channel_ack=$7)) LIMIT 2`, q.BindingID, q.BindingVersion, a.OwnerID, a.Channel, a.SessionKey, q.PublicationRef, q.ChannelACK)
	if err != nil {
		return input.Record{}, err
	}
	var pub, key string
	n := 0
	for rows.Next() {
		n++
		if err = rows.Scan(&pub, &key); err != nil {
			rows.Close()
			return input.Record{}, err
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return input.Record{}, err
	}
	if n == 0 {
		return input.Record{}, ErrReplyTargetUnavailable
	}
	if n != 1 {
		return input.Record{}, ErrAmbiguous
	}
	var yes bool
	if err = tx.QueryRow(ctx, `SELECT TRUE FROM sessions WHERE session_key=$1 AND user_id=$2 FOR UPDATE`, key, a.OwnerID).Scan(&yes); err != nil {
		return input.Record{}, replyLookupError(err)
	}
	var runID, taskID, kind string
	var rev, con int64
	var payload []byte
	if tx.QueryRow(ctx, `SELECT p.run_id,COALESCE(p.task_id,''),p.task_revision,p.constraints_revision,p.kind,p.payload FROM communication_publications p JOIN delivery_outbox d USING(publication_id) JOIN communication_bindings b ON b.binding_id=d.binding_id AND b.version=d.binding_version WHERE p.publication_id=$1 AND b.binding_id=$2 AND b.version=$3 AND b.owner_id=$4 AND b.channel=$5 AND b.recipient=$6 AND b.enabled AND b.verified AND d.recipient_snapshot=b.recipient AND NOT p.privacy_redacted AND NOT d.privacy_redacted AND ($7='' OR (d.status='sent' AND d.channel_ack=$7)) FOR SHARE OF b,p,d`, pub, q.BindingID, q.BindingVersion, a.OwnerID, a.Channel, a.SessionKey, q.ChannelACK).Scan(&runID, &taskID, &rev, &con, &kind, &payload) != nil {
		return input.Record{}, ErrDenied
	}
	if kind == "widget" {
		return input.Record{}, ErrInteractionRequired
	}
	if taskID != "" {
		if err = tx.QueryRow(ctx, `SELECT TRUE FROM tasks WHERE task_id=$1 AND owner_id=$2 AND session_key=$3 AND revision=$4 AND constraints_revision=$5 AND status='active' FOR SHARE`, taskID, a.OwnerID, key, rev, con).Scan(&yes); err != nil {
			return input.Record{}, replyLookupError(err)
		}
		var now time.Time
		if tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now) != nil {
			return input.Record{}, ErrDenied
		}
		if _, err = actionauth.CheckTaskAuthorityTx(ctx, tx, taskID, a.OwnerID, nil, now); err != nil {
			return input.Record{}, ErrDenied
		}
	}
	if err = s.SourceGuard(ctx, tx, runID); err != nil {
		return input.Record{}, ErrDenied
	}
	args, err := communicationcontrol.DecodeArgs("communication."+kind, string(payload))
	if err != nil {
		return input.Record{}, ErrDenied
	}
	if err = s.ChannelGuard(ctx, tx, runID, Binding{ID: q.BindingID, Version: q.BindingVersion, Channel: a.Channel, Recipient: a.SessionKey}, args); err != nil {
		return input.Record{}, ErrDenied
	}
	if taskID != "" {
		var now time.Time
		if tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now) != nil {
			return input.Record{}, ErrDenied
		}
		if _, err = actionauth.CheckTaskAuthorityTx(ctx, tx, taskID, a.OwnerID, nil, now); err != nil {
			return input.Record{}, ErrDenied
		}
	}
	if err = s.SourceGuard(ctx, tx, runID); err != nil {
		return input.Record{}, ErrDenied
	}
	data, _ := json.Marshal(map[string]any{"text": q.Text, "user_id": a.OwnerID, "task_id": taskID, "reply_to_publication_ref": pub, "origin_channel": a.Channel, "origin_binding_ref": q.BindingID})
	event := ingress.InputEvent{SessionKey: key, Source: a.Channel, EventID: a.DeliveryID, IdempotencyKey: a.DeliveryID, EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, PayloadJSON: string(data), CreatedAt: time.Now().UTC()}
	r, _, err := s.Inputs.AcceptTx(ctx, tx, event, a.OwnerID, taskID)
	if err != nil {
		if errors.Is(err, input.ErrDeliveryConflict) {
			return input.Record{}, ErrConflict
		}
		return input.Record{}, err
	}
	if s.ReplyAdmission != nil {
		if err = s.ReplyAdmission(ctx, tx, r, Binding{ID: q.BindingID, Version: q.BindingVersion, Channel: a.Channel, Recipient: a.SessionKey}); err != nil {
			return input.Record{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return input.Record{}, err
	}
	return r, nil
}

// Bindings intentionally omits recipients/proof; only the owning actor can read.
func (s *Store) Bindings(ctx context.Context) ([]Binding, error) {
	a, ok := owneridentity.FromContext(ctx)
	if !ok || s == nil || s.Pool == nil {
		return nil, ErrDenied
	}
	rows, err := s.Pool.Query(ctx, `SELECT binding_id,channel,version,enabled,capabilities FROM communication_bindings WHERE owner_id=$1 ORDER BY binding_id`, a.OwnerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Binding{}
	for rows.Next() {
		var b Binding
		var raw []byte
		if err = rows.Scan(&b.ID, &b.Channel, &b.Version, &b.Enabled, &raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &b.Capabilities); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
