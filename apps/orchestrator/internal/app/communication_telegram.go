package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/artifacts"
	"github.com/butler/butler/apps/orchestrator/internal/channel/telegram"
	"github.com/butler/butler/apps/orchestrator/internal/channel/telegramrich"
	"github.com/butler/butler/apps/orchestrator/internal/communication"
	"github.com/butler/butler/apps/orchestrator/internal/deliveryoutbox"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/communicationcontrol"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/providerauth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TelegramCommunicationOptions is trusted operator composition, not persisted
// settings, an HTTP request, a model flag or permission to enable production.
// NewForTelegramCommunication accepts only a disposable *_test database.
type TelegramCommunicationOptions struct {
	OwnerID, BindingID, ExpectedBot, TokenFile string
	BotAPIProxyFile                            string
	ChatID                                     int64
	BindingVersion                             int64
	// Web remains opt-in even in this isolated composition. It never enables
	// historical delivery or changes the ordinary product startup.
	WebBindingID      string
	WebBindingVersion int64
	DrainOnly         bool
	// Trusted programmatic reference for a lab that renews the canonical auth
	// row. JSON/operator profiles cannot provide a store or credential material.
	ProviderAuthStore providerauth.SecretStore `json:"-"`
	// Export of private owner material to this precise private bot binding is
	// an explicit operator decision. Public-only is the zero/default policy.
	AllowPrivate bool
}

type telegramCommunication struct {
	store    *communication.Store
	client   *telegramrich.Client
	binding  telegramrich.OwnerBinding
	policy   telegramExportPolicy
	controls *task.Controller
	web      *webCommunication
	draining bool
}

type telegramExportPolicy struct {
	owner        string
	binding      communication.Binding
	allowPrivate bool
	catalog      *catalog.Store
	sourceGuard  func(context.Context, pgx.Tx, string) error
}

func (p telegramExportPolicy) check(ctx context.Context, tx pgx.Tx, run string, b communication.Binding, _ communicationcontrol.Args) error {
	if p.catalog == nil || p.sourceGuard == nil || b.ID != p.binding.ID || b.Version != p.binding.Version || b.Channel != p.binding.Channel || b.Recipient != p.binding.Recipient {
		return communication.ErrDenied
	}
	var owner string
	if err := tx.QueryRow(ctx, `SELECT s.user_id FROM runs r JOIN sessions s USING(session_key) JOIN communication_bindings b ON b.binding_id=$2 AND b.version=$3 AND b.owner_id=s.user_id AND b.channel=$5 AND b.recipient=$4 AND b.enabled AND b.verified WHERE r.run_id=$1 FOR SHARE OF s,b`, run, b.ID, b.Version, b.Recipient, b.Channel).Scan(&owner); err != nil || owner != p.owner {
		return communication.ErrDenied
	}
	if err := p.sourceGuard(ctx, tx, run); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT source_id,source_revision,access_revision,content_hash FROM memory_run_sources WHERE run_id=$1 ORDER BY source_id`, run)
	if err != nil {
		return err
	}
	refs := []catalog.SourceRef{}
	for rows.Next() {
		var ref catalog.SourceRef
		if err = rows.Scan(&ref.ID, &ref.Revision, &ref.AccessRevision, &ref.Hash); err != nil {
			rows.Close()
			return err
		}
		refs = append(refs, ref)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	refs, err = p.catalog.CanonicalSourcesTx(ctx, tx, refs)
	if err != nil {
		return err
	}
	// Channel export reads through the Catalog's current revision/deletion and
	// complete dependency graph checks, without pretending a delivery is a run.
	actor := owneridentity.Actor{OwnerID: p.owner, Channel: b.Channel, SessionKey: b.Recipient, DeliveryID: "communication-export"}
	sources, err := p.catalog.AccessTx(owneridentity.WithActor(ctx, actor), tx, refs, "communication_export")
	if err != nil {
		return err
	}
	for _, source := range sources {
		if source.Origin.Sensitivity != "public" && !p.allowPrivate {
			return communication.ErrDenied
		}
	}
	return p.sourceGuard(ctx, tx, run)
}

func (p telegramExportPolicy) media(ctx context.Context, tx pgx.Tx, run string, b communication.Binding, ref string) (artifacts.Media, error) {
	return artifacts.ResolveMediaTx(ctx, tx, run, ref, func(ctx context.Context, tx pgx.Tx, run, ref string) error {
		if err := p.check(ctx, tx, run, b, communicationcontrol.Args{}); err != nil {
			return err
		}
		var original string
		if err := tx.QueryRow(ctx, `SELECT run_id FROM artifacts WHERE artifact_id=$1 FOR SHARE`, strings.TrimPrefix(ref, "artifact:")).Scan(&original); err != nil {
			return err
		}
		return p.check(ctx, tx, original, b, communicationcontrol.Args{})
	})
}

func newTelegramCommunication(ctx context.Context, pool *pgxpool.Pool, sources *catalog.Store, inputs *durableinput.Store, guard func(context.Context, pgx.Tx, string) error, o TelegramCommunicationOptions) (*telegramCommunication, error) {
	if pool == nil || sources == nil || inputs == nil || guard == nil || !strings.HasSuffix(pool.Config().ConnConfig.Database, "_test") || o.OwnerID == "" || o.BindingID == "" || len(o.BindingID) > 128 || o.ChatID <= 0 || o.ExpectedBot == "" {
		return nil, telegram.ErrExplicitUnsupported
	}
	client, err := telegramrich.NewClientFromTokenFileWithProxyFile(o.TokenFile, o.BotAPIProxyFile)
	if err != nil {
		return nil, err
	}
	me, err := client.GetMe(ctx)
	if err != nil || me.Username != o.ExpectedBot {
		return nil, telegram.ErrExplicitUnsupported
	}
	if o.BindingVersion == 0 {
		o.BindingVersion = 1
	}
	if o.BindingVersion < 1 {
		return nil, telegram.ErrExplicitUnsupported
	}
	b := communication.Binding{ID: o.BindingID, Version: o.BindingVersion, Channel: "telegram", Recipient: "telegram:" + strconv.FormatInt(o.ChatID, 10), Enabled: true, Capabilities: []string{"message", "photo", "report"}}
	core := &communication.Store{Pool: pool, Inputs: inputs, SourceGuard: guard}
	policy := telegramExportPolicy{owner: o.OwnerID, binding: b, allowPrivate: o.AllowPrivate, catalog: sources, sourceGuard: guard}
	core.ChannelGuard = policy.check
	core.PlanDelivery = (telegramrich.Planner{Profile: telegramrich.Capabilities{Message: true, Photo: true, Rich: true, Document: true}, Resolve: policy.media}).Plan
	var owner, channel, recipient string
	var version int64
	var enabled, verified bool
	var capabilities []byte
	err = pool.QueryRow(ctx, `SELECT owner_id,channel,recipient,version,enabled,verified,capabilities FROM communication_bindings WHERE binding_id=$1`, b.ID).Scan(&owner, &channel, &recipient, &version, &enabled, &verified, &capabilities)
	if errors.Is(err, pgx.ErrNoRows) {
		if b.Version != 1 {
			return nil, telegram.ErrExplicitUnsupported
		}
		actor := owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: o.OwnerID, Channel: b.Channel, SessionKey: b.Recipient, DeliveryID: "operator-test-binding"})
		err = core.Bind(actor, b, 0)
	} else if err == nil && (owner != o.OwnerID || channel != b.Channel || recipient != b.Recipient || version != b.Version || !enabled || !verified) {
		err = telegram.ErrExplicitUnsupported
	} else if err == nil {
		var current []string
		if json.Unmarshal(capabilities, &current) != nil || !slices.Equal(current, b.Capabilities) {
			err = telegram.ErrExplicitUnsupported
		}
	}
	if err != nil {
		return nil, err
	}
	if err = pinTelegramBot(ctx, pool, b, me.ID); err != nil {
		return nil, err
	}
	c := &telegramCommunication{store: core, client: client, binding: telegramrich.OwnerBinding{OwnerID: o.OwnerID, BindingID: b.ID, Version: b.Version, ChatID: o.ChatID, UserID: o.ChatID, BotID: me.ID}, policy: policy, draining: o.DrainOnly}
	c.installInputHooks()
	if err = c.installWeb(ctx, o, guard); err != nil {
		return nil, err
	}
	return c, nil
}

// Pin the verified numeric bot before admission/sending. A recreated username
// cannot adopt another bot's binding or an unqualified historical queue.
func pinTelegramBot(ctx context.Context, pool *pgxpool.Pool, b communication.Binding, botID int64) error {
	if botID <= 0 {
		return telegram.ErrExplicitUnsupported
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	var yes bool
	if err = tx.QueryRow(ctx, `SELECT TRUE FROM communication_bindings WHERE binding_id=$1 AND version=$2 AND enabled AND verified AND channel='telegram' FOR UPDATE`, b.ID, b.Version).Scan(&yes); err != nil {
		return telegram.ErrExplicitUnsupported
	}
	var current int64
	err = tx.QueryRow(ctx, `SELECT bot_id FROM communication_telegram_cursors WHERE binding_id=$1`, b.ID).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		var history bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM delivery_outbox WHERE binding_id=$1)`, b.ID).Scan(&history); err != nil {
			return err
		}
		if history {
			return telegram.ErrExplicitUnsupported
		}
		if _, err = tx.Exec(ctx, `INSERT INTO communication_telegram_cursors(bot_id,binding_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, botID, b.ID); err != nil {
			return err
		}
		err = tx.QueryRow(ctx, `SELECT bot_id FROM communication_telegram_cursors WHERE binding_id=$1`, b.ID).Scan(&current)
	}
	if err != nil || current != botID {
		return telegram.ErrExplicitUnsupported
	}
	return tx.Commit(ctx)
}

func (c *telegramCommunication) admit(ctx context.Context, run string) error {
	var owner, key string
	var epoch int
	var proofRaw, eventRaw []byte
	if err := c.store.Pool.QueryRow(ctx, `SELECT s.user_id,r.session_key,i.communication_ingress_epoch,i.communication_ingress_json,i.event_json FROM runs r JOIN sessions s USING(session_key) JOIN accepted_inputs i ON i.run_id=r.run_id AND i.session_key=r.session_key AND i.owner_id=s.user_id WHERE r.run_id=$1`, run).Scan(&owner, &key, &epoch, &proofRaw, &eventRaw); err != nil {
		return err
	}
	// Other sessions never acquire the new profile just because the process can
	// consume epoch 1. Cross-channel canonical replies retain their pinned mode.
	if epoch == 0 {
		if owner == c.binding.OwnerID && (key == c.conversationKey() || c.web != nil && key == c.webConversationKey()) {
			return communication.ErrDenied
		}
		return nil
	}
	var proof durableinput.CommunicationIngress
	var event ingress.InputEvent
	if epoch != 1 || owner != c.binding.OwnerID || json.Unmarshal(proofRaw, &proof) != nil || json.Unmarshal(eventRaw, &event) != nil || proof.OwnerID != owner || proof.DeliveryID != event.EventID || event.SessionKey != key {
		return communication.ErrDenied
	}
	web := c.web != nil && proof.BindingID == c.web.binding.ID && proof.BindingVersion == c.web.binding.Version && proof.BotID == 0 && proof.ActorChannel == "web" && (proof.Kind == "web_message" || proof.Kind == "owner_control")
	telegram := proof.BindingID == c.binding.BindingID && proof.BindingVersion == c.binding.Version && proof.BotID == c.binding.BotID && (proof.Kind == "telegram_message" || proof.Kind == "owner_control")
	if !web && !telegram {
		return communication.ErrDenied
	}
	hash, err := actionauth.ArgumentsHash(event.PayloadJSON)
	if err != nil || hash != proof.PayloadHash {
		return communication.ErrDenied
	}
	return c.store.AdmitRun(ctx, run, false, []string{"communication.message", "communication.photo", "communication.report"})
}

func (c *telegramCommunication) installInputHooks() {
	guard := c.store.SourceGuard
	c.store.SourceGuard = func(ctx context.Context, tx pgx.Tx, run string) error {
		if guard == nil {
			return communication.ErrDenied
		}
		if err := guard(ctx, tx, run); err != nil {
			return err
		}
		if c.web != nil {
			if a, ok := owneridentity.FromContext(ctx); ok && a.Channel == "web" && a.OwnerID == c.binding.OwnerID {
				return c.checkWebBinding(ctx, tx)
			}
			var origin string
			if err := tx.QueryRow(ctx, `SELECT communication_ingress_json->>'BindingID' FROM accepted_inputs WHERE run_id=$1 AND communication_ingress_epoch=1`, run).Scan(&origin); err == nil && origin == c.web.binding.ID {
				return c.checkWebBinding(ctx, tx)
			}
		}
		return c.checkBinding(ctx, tx)
	}
	c.store.ReplyAdmission = func(ctx context.Context, tx pgx.Tx, r durableinput.Record, b communication.Binding) error {
		if c.web != nil && b.ID == c.web.binding.ID && b.Version == c.web.binding.Version && b.Channel == "web" && b.Recipient == c.web.binding.Recipient {
			return c.markWebInput(ctx, tx, r, "web_message")
		}
		if b.ID != c.binding.BindingID || b.Version != c.binding.Version || b.Channel != "telegram" || b.Recipient != c.policy.binding.Recipient {
			return communication.ErrDenied
		}
		return c.markInput(ctx, tx, r, "telegram_message")
	}
	c.store.ControlAdmission = func(ctx context.Context, tx pgx.Tx, id string) error {
		a, ok := owneridentity.FromContext(ctx)
		if !ok || a.OwnerID != c.binding.OwnerID {
			return nil
		}
		r, err := c.store.Inputs.GetTx(ctx, tx, id)
		if err != nil {
			return err
		}
		if c.draining && a.Channel == "telegram" {
			var admitted bool
			webID := ""
			if c.web != nil {
				webID = c.web.binding.ID
			}
			if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM accepted_inputs WHERE task_id=$1 AND communication_ingress_epoch=1 AND communication_ingress_json->>'BindingID' IN ($2,$3))`, r.TaskID, c.binding.BindingID, webID).Scan(&admitted); err != nil {
				return err
			}
			if !admitted {
				return communication.ErrDraining
			}
		}
		if a.Channel != "telegram" {
			var admitted bool
			webID := ""
			if c.web != nil {
				webID = c.web.binding.ID
			}
			if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM accepted_inputs WHERE task_id=$1 AND communication_ingress_epoch=1 AND communication_ingress_json->>'BindingID' IN ($2,$3))`, r.TaskID, c.binding.BindingID, webID).Scan(&admitted); err != nil {
				return err
			}
			if !admitted {
				return nil
			}
			if c.web != nil && a.Channel == "web" {
				if err = c.checkWebBinding(ctx, tx); err != nil {
					return err
				}
				return c.markWebInput(ctx, tx, r, "owner_control")
			}
		}
		if err = c.checkBinding(ctx, tx); err != nil {
			return err
		}
		return c.markInput(ctx, tx, r, "owner_control")
	}
}

func (c *telegramCommunication) markInput(ctx context.Context, tx pgx.Tx, r durableinput.Record, kind string) error {
	a, ok := owneridentity.FromContext(ctx)
	if !ok || a.OwnerID != c.binding.OwnerID {
		return communication.ErrDenied
	}
	if kind != "owner_control" {
		if err := c.allowMessageIngress(ctx, tx, r); err != nil {
			return err
		}
	}
	hash, err := actionauth.ArgumentsHash(r.Event.PayloadJSON)
	if err != nil {
		return err
	}
	return c.store.Inputs.MarkCommunicationTx(ctx, tx, r, durableinput.CommunicationIngress{OwnerID: a.OwnerID, BindingID: c.binding.BindingID, BindingVersion: c.binding.Version, BotID: c.binding.BotID, DeliveryID: r.Event.EventID, PayloadHash: hash, Kind: kind, ActorChannel: a.Channel, ActorDeliveryID: a.DeliveryID})
}

func (c *telegramCommunication) allowMessageIngress(ctx context.Context, tx pgx.Tx, r durableinput.Record) error {
	if !c.draining {
		return nil
	}
	var accepted bool
	if err := tx.QueryRow(ctx, `SELECT communication_ingress_epoch=1 FROM accepted_inputs WHERE input_id=$1`, r.InputID).Scan(&accepted); err != nil {
		return err
	}
	if !accepted {
		return communication.ErrDraining
	}
	return nil
}

func (c *telegramCommunication) conversationKey() string {
	return "telegram-explicit:" + strconv.FormatInt(c.binding.BotID, 10) + ":" + strconv.FormatInt(c.binding.ChatID, 10)
}

func (c *telegramCommunication) conversation(ctx context.Context, u telegramrich.Update) error {
	a, ok := owneridentity.FromContext(ctx)
	if !ok || a.OwnerID != c.binding.OwnerID || a.Channel != "telegram" || a.SessionKey != c.policy.binding.Recipient {
		return communication.ErrDenied
	}
	if kind, policy, target, instruction := telegramOwnerCommand(u.Message.Text); kind != "" {
		if c.controls == nil {
			return telegramrich.ErrUnavailable
		}
		record, err := c.controls.ResolveTarget(ctx, target)
		if errors.Is(err, task.ErrConflict) {
			return communication.ErrAmbiguous
		}
		if err != nil {
			return err
		}
		// A channel address is not the Task's canonical session. The existing
		// Controller derives/locks the actual session and all descendant versions.
		a.SessionKey = ""
		_, err = c.controls.ApplyWithGuard(owneridentity.WithActor(ctx, a), task.ControlRequest{TaskID: record.TaskID, Kind: kind, Instruction: instruction, ExecutionPolicy: policy}, func(ctx context.Context, tx pgx.Tx) error {
			return c.checkBinding(ctx, tx)
		})
		return err
	}
	// Unsupported slash commands cannot silently become instructions to a model.
	if strings.HasPrefix(u.Message.Text, "/") && u.Message.Text != "/start" {
		return telegramrich.ErrRejectedUpdate
	}
	record, _, err := session.NewPostgresRepository(c.store.Pool).CreateSession(ctx, session.CreateSessionParams{SessionKey: c.conversationKey(), UserID: a.OwnerID, Channel: "telegram", MetadataJSON: `{}`})
	if err != nil {
		return err
	}
	if record.UserID != a.OwnerID {
		return communication.ErrDenied
	}
	tx, err := c.store.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if err = c.checkBinding(ctx, tx); err != nil {
		return err
	}
	text := u.Message.Text
	if text == "/start" {
		text = "Начни обычный разговор с владельцем. Кратко представь доступные возможности явным сообщением."
	}
	data, _ := json.Marshal(map[string]any{"text": text, "user_id": a.OwnerID, "intent": "conversation", "origin_binding_ref": c.binding.BindingID})
	r, _, err := c.store.Inputs.AcceptTx(ctx, tx, ingress.InputEvent{SessionKey: record.SessionKey, Source: "telegram", EventID: a.DeliveryID, IdempotencyKey: a.DeliveryID, EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, PayloadJSON: string(data), CreatedAt: time.Now().UTC()}, a.OwnerID, "")
	if err != nil {
		return err
	}
	if err = c.markInput(ctx, tx, r, "telegram_message"); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (c *telegramCommunication) adapter(controls *task.Controller) (*telegram.Adapter, error) {
	if controls == nil {
		return nil, telegram.ErrExplicitUnsupported
	}
	c.controls = controls
	controls.SetInteractionControls(c.store)
	ingress := &telegramrich.Ingress{Store: c.store, Controls: controls, Binding: c.binding, Conversation: c.conversation}
	d := &communication.DeliveryStore{Core: c.store}
	s := &telegramrich.Sender{Client: c.client, Resolve: func(ctx context.Context, m deliveryoutbox.Message, f telegramrich.File) (artifacts.Media, error) {
		tx, err := c.store.Pool.Begin(ctx)
		if err != nil {
			return artifacts.Media{}, err
		}
		defer tx.Rollback(context.Background())
		return c.policy.media(ctx, tx, m.RunID, c.policy.binding, f.Ref)
	}}
	return telegram.NewExplicitAdapter(&telegramrich.Poller{Pool: c.store.Pool, Client: c.client, Acceptor: ingress, Binding: c.binding, Notices: &telegramrich.Notices{Pool: c.store.Pool, Client: c.client, Binding: c.binding}}, d, s)
}

func (c *telegramCommunication) checkBinding(ctx context.Context, tx pgx.Tx) error {
	var valid bool
	return tx.QueryRow(ctx, `SELECT TRUE FROM communication_bindings WHERE binding_id=$1 AND version=$2 AND owner_id=$3 AND recipient=$4 AND channel='telegram' AND enabled AND verified FOR SHARE`, c.binding.BindingID, c.binding.Version, c.binding.OwnerID, c.policy.binding.Recipient).Scan(&valid)
}

func telegramOwnerCommand(text string) (kind, policy, target, instruction string) {
	fields := strings.Fields(text)
	if len(fields) >= 2 {
		switch strings.ToLower(fields[0]) {
		case "/stop", "/pause", "/resume":
			if len(fields) == 2 {
				return strings.TrimPrefix(strings.ToLower(fields[0]), "/"), "", fields[1], text
			}
		case "/amend", "/prepare":
			if len(fields) >= 3 {
				if strings.ToLower(fields[0]) == "/prepare" {
					policy = task.PolicyPrepareOnly
				}
				return task.ControlAmend, policy, fields[1], text
			}
		}
	}
	kind, policy = task.ClassifyOwnerText(text)
	return kind, policy, "", text
}

func communicationHandler(next, private http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == communicationcontrol.Path {
			private.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}
