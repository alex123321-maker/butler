package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/api"
	"github.com/butler/butler/apps/orchestrator/internal/communication"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/communicationcontrol"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5"
)

type webCommunication struct {
	binding communication.Binding
	policy  telegramExportPolicy
	reader  *communication.WebReader
}

func (c *telegramCommunication) installWeb(ctx context.Context, o TelegramCommunicationOptions, guard func(context.Context, pgx.Tx, string) error) error {
	if o.WebBindingID == "" {
		if o.WebBindingVersion != 0 {
			return communication.ErrDenied
		}
		return nil
	}
	if len(o.WebBindingID) > 128 || o.WebBindingID == o.BindingID {
		return communication.ErrDenied
	}
	if o.WebBindingVersion == 0 {
		o.WebBindingVersion = 1
	}
	if o.WebBindingVersion < 1 {
		return communication.ErrDenied
	}
	b := communication.Binding{ID: o.WebBindingID, Version: o.WebBindingVersion, Channel: "web", Recipient: "web:" + o.OwnerID, Enabled: true, Capabilities: []string{"message", "photo", "report"}}
	var owner, channel, recipient string
	var version int64
	var enabled, verified bool
	var raw []byte
	err := c.store.Pool.QueryRow(ctx, `SELECT owner_id,channel,recipient,version,enabled,verified,capabilities FROM communication_bindings WHERE binding_id=$1`, b.ID).Scan(&owner, &channel, &recipient, &version, &enabled, &verified, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		var history bool
		if err = c.store.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM delivery_outbox WHERE binding_id=$1)`, b.ID).Scan(&history); err != nil {
			return err
		}
		if history || b.Version != 1 {
			return communication.ErrDenied
		}
		a := owneridentity.Actor{OwnerID: o.OwnerID, Channel: "web", SessionKey: b.Recipient, DeliveryID: "operator-test-web-binding"}
		err = c.store.Bind(owneridentity.WithActor(ctx, a), b, 0)
	} else if err == nil {
		var caps []string
		if owner != o.OwnerID || channel != "web" || recipient != b.Recipient || version != b.Version || !enabled || !verified || json.Unmarshal(raw, &caps) != nil || !slices.Equal(caps, b.Capabilities) {
			return communication.ErrDenied
		}
	}
	if err != nil {
		return err
	}
	w := &webCommunication{binding: b, policy: telegramExportPolicy{owner: o.OwnerID, binding: b, allowPrivate: true, catalog: c.policy.catalog, sourceGuard: guard}}
	w.reader = &communication.WebReader{Core: c.store, Binding: b, Resolve: w.policy.media}
	c.web = w
	c.store.WebAvailable = true
	channelGuard, planner := c.store.ChannelGuard, c.store.PlanDelivery
	c.store.ChannelGuard = func(ctx context.Context, tx pgx.Tx, run string, b communication.Binding, a communicationcontrol.Args) error {
		if b.Channel == "web" {
			return w.policy.check(ctx, tx, run, b, a)
		}
		return channelGuard(ctx, tx, run, b, a)
	}
	c.store.PlanDelivery = func(ctx context.Context, tx pgx.Tx, run string, b communication.Binding, a communicationcontrol.Args) ([]string, error) {
		if b.Channel == "web" {
			return w.reader.Plan(ctx, tx, run, b, a)
		}
		return planner(ctx, tx, run, b, a)
	}
	return nil
}

func (c *telegramCommunication) checkWebBinding(ctx context.Context, tx pgx.Tx) error {
	if c.web == nil {
		return communication.ErrDenied
	}
	var yes bool
	b := c.web.binding
	return tx.QueryRow(ctx, `SELECT TRUE FROM communication_bindings WHERE binding_id=$1 AND version=$2 AND owner_id=$3 AND recipient=$4 AND channel='web' AND enabled AND verified FOR SHARE`, b.ID, b.Version, c.binding.OwnerID, b.Recipient).Scan(&yes)
}

func (c *telegramCommunication) markWebInput(ctx context.Context, tx pgx.Tx, r durableinput.Record, kind string) error {
	a, ok := owneridentity.FromContext(ctx)
	if !ok || c.web == nil || a.OwnerID != c.binding.OwnerID || a.Channel != "web" {
		return communication.ErrDenied
	}
	if err := c.checkWebBinding(ctx, tx); err != nil {
		return err
	}
	hash, err := actionauth.ArgumentsHash(r.Event.PayloadJSON)
	if err != nil {
		return err
	}
	return c.store.Inputs.MarkCommunicationTx(ctx, tx, r, durableinput.CommunicationIngress{OwnerID: a.OwnerID, BindingID: c.web.binding.ID, BindingVersion: c.web.binding.Version, DeliveryID: r.Event.EventID, PayloadHash: hash, Kind: kind, ActorChannel: a.Channel, ActorDeliveryID: a.DeliveryID})
}

func (c *telegramCommunication) webConversationKey() string {
	return "web-explicit:" + c.web.binding.ID
}

func (c *telegramCommunication) webConversation(ctx context.Context, text string) (durableinput.Record, error) {
	a, ok := owneridentity.FromContext(ctx)
	if !ok || c.web == nil || a.OwnerID != c.binding.OwnerID || a.Channel != "web" || a.SessionKey != c.web.binding.Recipient {
		return durableinput.Record{}, communication.ErrDenied
	}
	s, _, err := session.NewPostgresRepository(c.store.Pool).CreateSession(ctx, session.CreateSessionParams{SessionKey: c.webConversationKey(), UserID: a.OwnerID, Channel: "web", MetadataJSON: `{}`})
	if err != nil || s.UserID != a.OwnerID {
		return durableinput.Record{}, communication.ErrDenied
	}
	tx, err := c.store.Pool.Begin(ctx)
	if err != nil {
		return durableinput.Record{}, err
	}
	defer tx.Rollback(context.Background())
	if err = c.checkWebBinding(ctx, tx); err != nil {
		return durableinput.Record{}, err
	}
	data, _ := json.Marshal(map[string]any{"text": text, "user_id": a.OwnerID, "intent": "conversation", "origin_binding_ref": c.web.binding.ID})
	r, _, err := c.store.Inputs.AcceptTx(ctx, tx, ingress.InputEvent{SessionKey: s.SessionKey, Source: "web", EventID: a.DeliveryID, IdempotencyKey: a.DeliveryID, EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, PayloadJSON: string(data), CreatedAt: time.Now().UTC()}, a.OwnerID, "")
	if err != nil {
		return durableinput.Record{}, err
	}
	if err = c.markWebInput(ctx, tx, r, "web_message"); err != nil {
		return durableinput.Record{}, err
	}
	return r, tx.Commit(ctx)
}

func (c *telegramCommunication) webHandler(auth *api.OwnerControlAuth) http.Handler {
	if c == nil || c.web == nil {
		return nil
	}
	return api.CommunicationWeb(auth, c.web.reader, c.controls, c.webConversation)
}
