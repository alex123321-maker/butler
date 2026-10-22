package communication

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/deliveryoutbox"
	"github.com/butler/butler/internal/communicationcontrol"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5"
)

var ErrWidgetStale = errors.New("interaction projection is stale or closed")
var ErrWidgetAnswer = errors.New("invalid interaction answer")

type WidgetView struct {
	Ref          string                      `json:"interaction_ref"`
	Version      int64                       `json:"version"`
	State        string                      `json:"state"`
	DraftVersion int64                       `json:"draft_version"`
	Draft        communicationcontrol.Answer `json:"draft"`
	Answer       communicationcontrol.Answer `json:"answer"`
	ExpiresAt    *time.Time                  `json:"expires_at,omitempty"`
	Token        string                      `json:"-"`
	Question     communicationcontrol.Args   `json:"-"`
}

func widgetViewTx(ctx context.Context, tx pgx.Tx, publication string) (WidgetView, error) {
	var v WidgetView
	var draft, answer, question []byte
	err := tx.QueryRow(ctx, `SELECT x.interaction_id,x.version,CASE WHEN x.state='open' AND x.expires_at<=clock_timestamp() THEN 'expired' ELSE x.state END,x.expires_at,x.response_json,x.question_json,w.callback_token,w.draft_version,w.draft_json FROM communication_interactions x JOIN communication_widget_views w USING(interaction_id) WHERE x.publication_id=$1 AND NOT x.privacy_redacted`, publication).Scan(&v.Ref, &v.Version, &v.State, &v.ExpiresAt, &answer, &question, &v.Token, &v.DraftVersion, &draft)
	if err != nil {
		return v, err
	}
	if json.Unmarshal(question, &v.Question) != nil || json.Unmarshal(draft, &v.Draft) != nil {
		return v, ErrDenied
	}
	if len(answer) > 0 && json.Unmarshal(answer, &v.Answer) != nil {
		return v, ErrDenied
	}
	return v, nil
}

type WidgetAction struct {
	BindingID       string                      `json:"binding_id"`
	BindingVersion  int64                       `json:"binding_version"`
	ExpectedVersion int64                       `json:"expected_version"`
	DraftVersion    int64                       `json:"draft_version"`
	Operation       string                      `json:"operation"` // draft, submit, answer
	OptionID        string                      `json:"option_id,omitempty"`
	Answer          communicationcontrol.Answer `json:"answer"`
}

// WidgetActionTx is a control transaction, never a queued conversational input.
// Shared drafts have their own CAS; only submit/answer calls the existing resume.
func (s *Store) WidgetActionTx(ctx context.Context, tx pgx.Tx, id string, q WidgetAction) (WidgetView, error) {
	actor, ok := owneridentity.FromContext(ctx)
	if !ok || actor.DeliveryID == "" || len(actor.DeliveryID) > 512 {
		return WidgetView{}, ErrDenied
	}
	v, _, err := s.lockInteractionTx(ctx, tx, id, q.BindingID, q.BindingVersion)
	if err != nil {
		return WidgetView{}, err
	}
	view, err := widgetViewTx(ctx, tx, v.PublicationID)
	if err != nil {
		return view, err
	}
	raw, _ := json.Marshal(q)
	digest := hash(string(raw))
	delivery := actor.Channel + ":" + actor.DeliveryID
	var oldHash string
	var oldReceipt []byte
	err = tx.QueryRow(ctx, `SELECT payload_hash,receipt FROM communication_widget_actions WHERE interaction_id=$1 AND delivery_id=$2`, id, delivery).Scan(&oldHash, &oldReceipt)
	if err == nil {
		if oldHash != digest {
			return view, ErrConflict
		}
		if json.Unmarshal(oldReceipt, &view) != nil {
			return view, ErrDenied
		}
		return view, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return view, err
	}
	if view.State == "resolved" {
		return view, ErrResolved
	}
	if view.State != "open" || view.Version != q.ExpectedVersion || v.Question.Input == nil {
		return view, ErrWidgetStale
	}
	input := *v.Question.Input
	switch q.Operation {
	case "draft", "toggle":
		if input.Type != "multi_choice" || q.DraftVersion != view.DraftVersion {
			return view, ErrWidgetStale
		}
		if q.Operation == "toggle" {
			if q.Answer.Text != "" || len(q.Answer.OptionIDs) > 0 {
				return view, ErrWidgetAnswer
			}
			q.Answer.OptionIDs = append([]string{}, view.Draft.OptionIDs...)
			if at := slices.Index(q.Answer.OptionIDs, q.OptionID); at >= 0 {
				q.Answer.OptionIDs = slices.Delete(q.Answer.OptionIDs, at, at+1)
			} else {
				q.Answer.OptionIDs = append(q.Answer.OptionIDs, q.OptionID)
			}
		}
		draftInput := input
		draftInput.Min = 0
		if communicationcontrol.ValidateAnswer(draftInput, q.Answer) != nil {
			return view, ErrWidgetAnswer
		}
		// Canonical option order makes a set independent of click/channel ordering.
		q.Answer.OptionIDs = canonicalOptions(input, q.Answer.OptionIDs)
		data, _ := json.Marshal(q.Answer)
		if _, err = tx.Exec(ctx, `UPDATE communication_widget_views SET draft_version=draft_version+1,draft_json=$2 WHERE interaction_id=$1`, id, data); err != nil {
			return view, err
		}
	case "submit":
		if input.Type != "multi_choice" || q.DraftVersion != view.DraftVersion || q.Answer.Text != "" || len(q.Answer.OptionIDs) > 0 {
			return view, ErrWidgetStale
		}
		if communicationcontrol.ValidateAnswer(input, view.Draft) != nil {
			return view, ErrWidgetAnswer
		}
		if _, err = s.RespondTx(ctx, tx, id, q.BindingID, q.BindingVersion, q.ExpectedVersion, view.Draft); err != nil {
			return view, err
		}
	case "answer":
		if input.Type == "multi_choice" || communicationcontrol.ValidateAnswer(input, q.Answer) != nil {
			return view, ErrWidgetAnswer
		}
		if _, err = s.RespondTx(ctx, tx, id, q.BindingID, q.BindingVersion, q.ExpectedVersion, q.Answer); err != nil {
			return view, err
		}
	default:
		return view, ErrWidgetAnswer
	}
	view, err = widgetViewTx(ctx, tx, v.PublicationID)
	if err != nil {
		return view, err
	}
	receipt, _ := json.Marshal(view)
	if _, err = tx.Exec(ctx, `INSERT INTO communication_widget_actions(interaction_id,delivery_id,payload_hash,receipt) VALUES($1,$2,$3,$4)`, id, delivery, digest, receipt); err != nil {
		return view, err
	}
	if err = s.eventTx(ctx, tx, v.OwnerID, v.SessionKey, id, "interaction.updated"); err != nil {
		return view, err
	}
	return view, nil
}

func canonicalOptions(q communicationcontrol.QuestionInput, ids []string) []string {
	out := []string{}
	for _, o := range q.Options {
		if slices.Contains(ids, o.ID) {
			out = append(out, o.ID)
		}
	}
	return out
}

func (s *Store) WidgetAction(ctx context.Context, id string, q WidgetAction) (WidgetView, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return WidgetView{}, err
	}
	defer tx.Rollback(context.Background())
	view, err := s.WidgetActionTx(ctx, tx, id, q)
	if err != nil {
		return view, err
	}
	return view, tx.Commit(ctx)
}

// TelegramWidgetAction proves the callback through an actual sent message ACK.
// Short callback tokens are locators only; neither they nor message text grant authority.
func (s *Store) TelegramWidgetAction(ctx context.Context, b Binding, ack, callback, text string) (WidgetView, error) {
	actor, ok := owneridentity.FromContext(ctx)
	if !ok || actor.Channel != "telegram" || b.Channel != "telegram" || ack == "" {
		return WidgetView{}, ErrDenied
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return WidgetView{}, err
	}
	defer tx.Rollback(context.Background())
	var pub string
	rows, err := tx.Query(ctx, `SELECT DISTINCT p.publication_id FROM delivery_outbox d JOIN communication_publications p USING(publication_id) JOIN communication_bindings b ON b.binding_id=d.binding_id AND b.version=d.binding_version WHERE b.binding_id=$1 AND b.version=$2 AND b.owner_id=$3 AND b.channel='telegram' AND b.recipient=$4 AND b.enabled AND b.verified AND d.recipient_snapshot=b.recipient AND d.status='sent' AND d.channel_ack=$5 AND p.kind='widget' AND NOT p.privacy_redacted AND NOT d.privacy_redacted LIMIT 2`, b.ID, b.Version, actor.OwnerID, actor.SessionKey, ack)
	if err != nil {
		return WidgetView{}, err
	}
	n := 0
	for rows.Next() {
		n++
		if err = rows.Scan(&pub); err != nil {
			break
		}
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return WidgetView{}, err
	}
	if n != 1 {
		return WidgetView{}, ErrInteractionRequired
	}
	view, err := widgetViewTx(ctx, tx, pub)
	if err != nil {
		return view, err
	}
	q := WidgetAction{BindingID: b.ID, BindingVersion: b.Version, ExpectedVersion: 1, Operation: "answer", Answer: communicationcontrol.Answer{Text: text}}
	if callback != "" {
		parts := strings.Split(callback, ":")
		if len(callback) > 64 || len(parts) != 5 || parts[0] != "cw" || parts[1] != view.Token {
			return view, ErrWidgetStale
		}
		q.ExpectedVersion, err = strconv.ParseInt(parts[2], 36, 64)
		if err != nil {
			return view, ErrWidgetStale
		}
		q.DraftVersion, err = strconv.ParseInt(parts[3], 36, 64)
		if err != nil {
			return view, ErrWidgetStale
		}
		op := parts[4]
		q.Answer = communicationcontrol.Answer{}
		if op == "s" {
			q.Operation = "submit"
		} else {
			if len(op) < 2 {
				return view, ErrWidgetStale
			}
			index, e := strconv.Atoi(op[1:])
			if e != nil || view.Question.Input == nil || index < 0 || index >= len(view.Question.Input.Options) {
				return view, ErrWidgetStale
			}
			id := view.Question.Input.Options[index].ID
			if op[0] == 'o' {
				q.Answer.OptionIDs = []string{id}
			} else if op[0] == 't' {
				q.Operation = "toggle"
				q.OptionID = id
			} else {
				return view, ErrWidgetStale
			}
		}
	}
	view, err = s.WidgetActionTx(ctx, tx, view.Ref, q)
	if err != nil {
		return view, err
	}
	return view, tx.Commit(ctx)
}

// DeliveryWidget permits rendering only the exact stored destination. It is
// followed by the sender's existing claim authorization immediately before I/O.
func (s *Store) DeliveryWidget(ctx context.Context, m deliveryoutbox.Message) (WidgetView, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return WidgetView{}, err
	}
	defer tx.Rollback(context.Background())
	var owner, session string
	err = tx.QueryRow(ctx, `SELECT p.owner_id,p.session_key FROM delivery_outbox d JOIN communication_publications p USING(publication_id) JOIN communication_bindings b ON b.binding_id=d.binding_id AND b.version=d.binding_version WHERE d.notification_id=$1 AND d.publication_id=$2 AND d.binding_id=$3 AND d.binding_version=$4 AND d.recipient_snapshot=$5 AND d.channel='telegram' AND b.recipient=d.recipient_snapshot AND b.owner_id=p.owner_id AND b.enabled AND b.verified AND NOT p.privacy_redacted AND NOT d.privacy_redacted`, m.NotificationID, m.PublicationID, m.BindingID, m.BindingVersion, m.Recipient).Scan(&owner, &session)
	if err != nil {
		return WidgetView{}, err
	}
	// Owning response/resume locks session before run. SourceGuard may lock
	// the run, and ChannelGuard locks the session; establish the same order
	// before either guard so a projection cannot deadlock that continuation.
	var yes bool
	if err = tx.QueryRow(ctx, `SELECT TRUE FROM sessions WHERE session_key=$1 AND user_id=$2 FOR SHARE`, session, owner).Scan(&yes); err != nil {
		return WidgetView{}, err
	}
	v, err := widgetViewTx(ctx, tx, m.PublicationID)
	if err != nil {
		return v, err
	}
	b := Binding{ID: m.BindingID, Version: m.BindingVersion, Channel: m.Channel, Recipient: m.Recipient}
	if s.SourceGuard == nil || s.ChannelGuard == nil || s.SourceGuard(ctx, tx, m.RunID) != nil || s.ChannelGuard(ctx, tx, m.RunID, b, v.Question) != nil {
		return v, ErrDenied
	}
	// Re-read state/expiry after policy waits, just before renderer I/O.
	v, err = widgetViewTx(ctx, tx, m.PublicationID)
	if err != nil {
		return v, err
	}
	if m.ClaimID != "" {
		var live bool
		if tx.QueryRow(ctx, `SELECT TRUE FROM delivery_outbox WHERE notification_id=$1 AND claim_id=$2 AND status='sending' AND claim_until>clock_timestamp() AND NOT privacy_redacted`, m.NotificationID, m.ClaimID).Scan(&live) != nil {
			return v, ErrDenied
		}
	}
	return v, tx.Commit(ctx)
}
