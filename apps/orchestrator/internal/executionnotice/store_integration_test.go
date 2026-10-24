package executionnotice_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/api"
	"github.com/butler/butler/apps/orchestrator/internal/communication"
	"github.com/butler/butler/apps/orchestrator/internal/deliveryoutbox"
	"github.com/butler/butler/apps/orchestrator/internal/executionnotice"
	"github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/internal/actionauth"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fixture struct {
	pool     *pgxpool.Pool
	ctx      context.Context
	store    *executionnotice.Store
	core     *communication.Store
	input    input.Record
	bindings []executionnotice.Binding
}

func TestExecutionNoticeScheduledChecksAreQuietButFailuresVisibleIntegration(t *testing.T) {
	f := setup(t)
	e := ingress.InputEvent{SessionKey: f.input.SessionKey, Source: "schedule", EventID: "scheduled-check", IdempotencyKey: "scheduled-check", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_SCHEDULED_INTERNAL_EVENT, PayloadJSON: `{"text":"check"}`, CreatedAt: time.Now()}
	r, _, err := input.NewStore(f.pool).Accept(f.ctx, e, "owner", "")
	if err != nil {
		t.Fatal(err)
	}
	hash, _ := actionauth.ArgumentsHash(e.PayloadJSON)
	proof, _ := json.Marshal(input.CommunicationIngress{OwnerID: "owner", BindingID: "web", BindingVersion: 1, DeliveryID: e.EventID, PayloadHash: hash, Kind: "scheduled_occurrence", ActorChannel: "schedule", ActorDeliveryID: e.EventID})
	// This test seeds only the already-qualified receipt projection. The owning
	// schedule/admission tests prove its creation and reject forged public inputs.
	if _, err = f.pool.Exec(f.ctx, `UPDATE accepted_inputs SET communication_ingress_epoch=1,communication_ingress_json=$2 WHERE input_id=$1`, r.InputID, proof); err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{"queued", "running", "completed"} {
		if _, err = f.pool.Exec(f.ctx, `UPDATE accepted_inputs SET status=$2 WHERE input_id=$1`, r.InputID, state); err != nil {
			t.Fatal(err)
		}
		if err = f.store.Sweep(f.ctx); err != nil {
			t.Fatal(err)
		}
		items, err := f.store.List(f.actor("owner"), "web")
		if err != nil {
			t.Fatal(err)
		}
		for _, n := range items {
			if n.InputID == r.InputID {
				t.Fatal("quiet check produced lifecycle spam", n)
			}
		}
	}
	if _, err = f.pool.Exec(f.ctx, `UPDATE accepted_inputs SET status='blocked',error_code='provider_unavailable' WHERE input_id=$1`, r.InputID); err != nil {
		t.Fatal(err)
	}
	if err = f.store.Sweep(f.ctx); err != nil {
		t.Fatal(err)
	}
	items, err := f.store.List(f.actor("owner"), "web")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, n := range items {
		found = found || n.InputID == r.InputID && n.Reason == "model_unavailable"
	}
	if !found {
		t.Fatal("scheduled outage vanished")
	}
}

func setup(t *testing.T) *fixture {
	t.Helper()
	pool := memorytest.Database(t)
	ctx := context.Background()
	f := &fixture{pool: pool, ctx: ctx, core: &communication.Store{Pool: pool}, bindings: []executionnotice.Binding{
		{ID: "web", Version: 1, OwnerID: "owner", Channel: "web", Recipient: "web:owner"},
		{ID: "telegram", Version: 1, OwnerID: "owner", Channel: "telegram", Recipient: "telegram:42"},
	}}
	for _, b := range f.bindings {
		a := owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: "owner", Channel: b.Channel, SessionKey: b.Recipient, DeliveryID: "bind"})
		if err := f.core.Bind(a, communication.Binding{ID: b.ID, Version: 1, Channel: b.Channel, Recipient: b.Recipient, Enabled: true, Capabilities: []string{"message"}}, 0); err != nil {
			t.Fatal(err)
		}
	}
	var err error
	f.store, err = executionnotice.New(ctx, pool, f.bindings)
	if err != nil {
		t.Fatal(err)
	}
	key := "web-explicit:fixture"
	if _, _, err = session.NewPostgresRepository(pool).CreateSession(ctx, session.CreateSessionParams{SessionKey: key, UserID: "owner", Channel: "web", MetadataJSON: `{}`}); err != nil {
		t.Fatal(err)
	}
	in := input.NewStore(pool)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	e := ingress.InputEvent{SessionKey: key, Source: "web", EventID: "notice-input", IdempotencyKey: "notice-input", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, PayloadJSON: `{"text":"synthetic-private-message"}`, CreatedAt: time.Now()}
	f.input, _, err = in.AcceptTx(ctx, tx, e, "owner", "")
	if err != nil {
		t.Fatal(err)
	}
	a := owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: "owner", Channel: "web", SessionKey: "web:owner", DeliveryID: e.EventID})
	hash, _ := actionauth.ArgumentsHash(e.PayloadJSON)
	err = in.MarkCommunicationTx(a, tx, f.input, input.CommunicationIngress{OwnerID: "owner", BindingID: "web", BindingVersion: 1, DeliveryID: e.EventID, PayloadHash: hash, Kind: "web_message", ActorChannel: "web", ActorDeliveryID: e.EventID})
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return f
}
func (f *fixture) actor(owner string) context.Context {
	return owneridentity.WithActor(f.ctx, owneridentity.Actor{OwnerID: owner, Channel: "web", SessionKey: "web:" + owner, DeliveryID: "read"})
}
func (f *fixture) state(t *testing.T, status, code string) {
	t.Helper()
	if _, err := f.pool.Exec(f.ctx, `UPDATE accepted_inputs SET status=$2,error_code=$3,error_message='synthetic-private-provider-error',updated_at=clock_timestamp() WHERE input_id=$1`, f.input.InputID, status, code); err != nil {
		t.Fatal(err)
	}
}

func TestExecutionNoticeDurableFailureAndRestartIntegration(t *testing.T) {
	f := setup(t)
	f.state(t, "blocked", "budget_provider_input_bytes_conversation")
	if err := f.store.Sweep(f.ctx); err != nil {
		t.Fatal(err)
	}
	items, err := f.store.List(f.actor("owner"), "web")
	if err != nil || len(items) != 1 || items[0].Reason != "budget" || items[0].DeliveryState != "available" {
		t.Fatalf("items=%+v err=%v", items, err)
	}
	if strings.Contains(items[0].Text, "synthetic-private") {
		t.Fatal("reflected private context")
	}
	restarted, err := executionnotice.New(f.ctx, f.pool, f.bindings)
	if err != nil {
		t.Fatal(err)
	}
	m, err := restarted.Claim(f.ctx, time.Minute)
	if err != nil || m == nil {
		t.Fatalf("claim=%v err=%v", m, err)
	}
	if err = restarted.AuthorizeSend(f.ctx, *m); err != nil {
		t.Fatal(err)
	}
	if err = restarted.Finish(f.ctx, *m, deliveryoutbox.Outcome{Status: "sent", ACK: "17"}); err != nil {
		t.Fatal(err)
	}
	if m, err = restarted.Claim(f.ctx, time.Minute); err != nil || m != nil {
		t.Fatalf("resent acknowledged notice: %v %v", m, err)
	}
	if _, err = f.store.List(f.actor("another-owner"), "web"); err == nil {
		t.Fatal("cross-owner state disclosed")
	}
}

func TestExecutionNoticeUnknownAndStaleStateCannotResendIntegration(t *testing.T) {
	f := setup(t)
	m, err := f.store.Claim(f.ctx, time.Minute)
	if err != nil || m == nil {
		t.Fatalf("claim=%v %v", m, err)
	}
	if _, err = f.pool.Exec(f.ctx, `UPDATE execution_notices SET claim_until=clock_timestamp()-interval '1 second' WHERE notice_id=$1`, m.NotificationID); err != nil {
		t.Fatal(err)
	}
	if again, err := f.store.Claim(f.ctx, time.Minute); err != nil || again != nil {
		t.Fatalf("unknown send retried: %v %v", again, err)
	}
	var state string
	if err = f.pool.QueryRow(f.ctx, `SELECT status FROM execution_notices WHERE notice_id=$1`, m.NotificationID).Scan(&state); err != nil || state != "delivery_unknown" {
		t.Fatal(state, err)
	}
	f.state(t, "waiting_for_user", "")
	waiting, err := f.store.Claim(f.ctx, time.Minute)
	if err != nil || waiting == nil {
		t.Fatal(waiting, err)
	}
	f.state(t, "blocked", "effect_unknown")
	if err = f.store.AuthorizeSend(f.ctx, *waiting); err == nil {
		t.Fatal("sent obsolete waiting state")
	}
	current, err := f.store.Claim(f.ctx, time.Minute)
	if err != nil || current == nil || !strings.Contains(current.Content, "не подтверждён") {
		t.Fatalf("current=%v err=%v", current, err)
	}
}

func TestExecutionNoticeRebindingDoesNotAdoptHistoryIntegration(t *testing.T) {
	f := setup(t)
	m, err := f.store.Claim(f.ctx, time.Minute)
	if err != nil || m == nil {
		t.Fatal(m, err)
	}
	b := f.bindings[1]
	a := owneridentity.WithActor(f.ctx, owneridentity.Actor{OwnerID: "owner", Channel: "telegram", SessionKey: b.Recipient, DeliveryID: "rebind"})
	if err = f.core.Bind(a, communication.Binding{ID: b.ID, Channel: b.Channel, Recipient: b.Recipient, Enabled: true, Capabilities: []string{"message"}}, 1); err != nil {
		t.Fatal(err)
	}
	if err = f.store.AuthorizeSend(f.ctx, *m); err == nil {
		t.Fatal("old target version retained permission")
	}
	b.Version = 2
	next, err := executionnotice.New(f.ctx, f.pool, []executionnotice.Binding{b})
	if err != nil {
		t.Fatal(err)
	}
	if m, err = next.Claim(f.ctx, time.Minute); err != nil || m != nil {
		t.Fatalf("new binding adopted old input: %v %v", m, err)
	}
}

func TestExecutionNoticeResumeChangesStateWithoutDuplicateHeartbeatIntegration(t *testing.T) {
	f := setup(t)
	f.state(t, "running", "")
	if err := f.store.Sweep(f.ctx); err != nil {
		t.Fatal(err)
	}
	f.state(t, "running", "")
	if err := f.store.Sweep(f.ctx); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM execution_notices WHERE binding_id='web'`).Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
	f.state(t, "waiting_for_user", "")
	if err := f.store.Sweep(f.ctx); err != nil {
		t.Fatal(err)
	}
	f.state(t, "running", "")
	if err := f.store.Sweep(f.ctx); err != nil {
		t.Fatal(err)
	}
	items, err := f.store.List(f.actor("owner"), "web")
	if err != nil || len(items) != 1 || items[0].Code != "working" {
		t.Fatal(items, err)
	}
}

func TestExecutionNoticeOwnerHTTPReadWithoutPublicationIntegration(t *testing.T) {
	f := setup(t)
	f.state(t, "blocked", "budget_provider_input_bytes_conversation")
	if err := f.store.Sweep(f.ctx); err != nil {
		t.Fatal(err)
	}
	f.core.ExecutionNotices = f.store
	const token = "synthetic-notice-owner-proof-12345678901234567890"
	handler := api.CommunicationWeb(api.NewOwnerControlAuth(token, "owner"), &communication.WebReader{Core: f.core, Binding: communication.Binding{ID: "web", Version: 1, Channel: "web", Recipient: "web:owner"}}, nil, nil)
	for _, tt := range []struct {
		path, proof string
		status      int
	}{
		{"/api/v3/communications/states", "", 401},
		{"/api/v3/communications/states", token, 200},
		{"/api/v3/communications/states?cursor=0", token, 400},
	} {
		request := httptest.NewRequest("GET", tt.path, nil)
		if tt.proof != "" {
			request.Header.Set("Authorization", "Bearer "+tt.proof)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != tt.status {
			t.Fatal(tt.path, response.Code, response.Body.String())
		}
		if response.Code == 200 && (!strings.Contains(response.Body.String(), `"reason":"budget"`) || strings.Contains(response.Body.String(), "synthetic-private")) {
			t.Fatal("incorrect public status", response.Body.String())
		}
	}
}

func TestExecutionNoticeModelFailureIsSpecificAndSourceFreeIntegration(t *testing.T) {
	f := setup(t)
	f.state(t, "blocked", "provider_unavailable")
	if err := f.store.Sweep(f.ctx); err != nil {
		t.Fatal(err)
	}
	items, err := f.store.List(f.actor("owner"), "web")
	if err != nil || len(items) != 1 || items[0].Reason != "model_unavailable" || !strings.Contains(items[0].Text, "сервис модели") || strings.Contains(items[0].Text, "synthetic-private") {
		t.Fatal(items, err)
	}
}
