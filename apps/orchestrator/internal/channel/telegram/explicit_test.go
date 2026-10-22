package telegram

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/artifacts"
	"github.com/butler/butler/apps/orchestrator/internal/channel/telegramrich"
	"github.com/butler/butler/apps/orchestrator/internal/communication"
	"github.com/butler/butler/apps/orchestrator/internal/deliveryoutbox"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/communicationcontrol"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCommunicationExplicitPollingOutageDoesNotEndOwningRuntime(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	healthy := make(chan struct{})
	done := make(chan error, 1)
	calls := 0
	go func() {
		done <- runExplicitPolling(ctx, func(context.Context) error {
			calls++
			if calls < 4 {
				return errors.New("synthetic transport or storage outage")
			}
			if calls == 4 {
				close(healthy)
			}
			return nil
		}, time.Millisecond)
	}()
	select {
	case <-healthy:
	case <-done:
		t.Fatal("channel outage terminated owning application")
	case <-time.After(time.Second):
		t.Fatal("polling did not recover")
	}
	select {
	case <-done:
		t.Fatal("successful polling terminated owning application")
	default:
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("backoff ignored cancellation")
	}
}

func TestCommunicationExplicitAdapterSuppressesImplicitOutput(t *testing.T) {
	file := filepath.Join(t.TempDir(), "synthetic-token")
	if err := os.WriteFile(file, []byte("100001:"+strings.Repeat("a", 35)), 0600); err != nil {
		t.Fatal(err)
	}
	client, err := telegramrich.NewClientFromTokenFile(file)
	if err != nil {
		t.Fatal(err)
	}
	// No DB or network call is allowed by these output paths. A legacy fallback
	// would dereference the absent old client and fail this test.
	pool := &pgxpool.Pool{}
	core := &communication.Store{Pool: pool, Inputs: durableinput.NewStore(pool), SourceGuard: func(context.Context, pgx.Tx, string) error { return nil }, ChannelGuard: func(context.Context, pgx.Tx, string, communication.Binding, communicationcontrol.Args) error {
		return nil
	}}
	b := telegramrich.OwnerBinding{OwnerID: "owner", BindingID: "bot-binding", Version: 1, BotID: 100001, ChatID: 42, UserID: 42}
	p := &telegramrich.Poller{Pool: pool, Client: client, Binding: b, Acceptor: &telegramrich.Ingress{Store: core, Controls: &task.Controller{}, Binding: b}}
	s := &telegramrich.Sender{Client: client, Resolve: func(context.Context, deliveryoutbox.Message, telegramrich.File) (artifacts.Media, error) {
		return artifacts.Media{}, telegramrich.ErrUnavailable
	}}
	a, err := NewExplicitAdapter(p, &communication.DeliveryStore{Core: core}, s)
	if err != nil {
		t.Fatal(err)
	}
	// A foreign row must fail the adapter's bound guard before touching the
	// deliberately absent DB, even when a caller supplied a permissive hook.
	if a.explicit.sender.Authorize(context.Background(), deliveryoutbox.Message{BindingID: "other-bot", BindingVersion: b.Version, Channel: "telegram", Recipient: "telegram:42"}) == nil {
		t.Fatal("adapter failed to bind its owning delivery scope")
	}
	event := flow.DeliveryEvent{SessionKey: "telegram:chat:42", RunID: "run", Content: "Internal model final must not be sent"}
	if a.DeliverAssistantFinal(context.Background(), event) != nil || a.DeliverAssistantDelta(context.Background(), event) != nil {
		t.Fatal("implicit output failed")
	}
	if a.DeliverStatusEvent(context.Background(), flow.StatusEvent{}) != nil || a.DeliverToolCallEvent(context.Background(), flow.ToolCallEvent{}) != nil {
		t.Fatal("implicit status publication")
	}
	if a.DeliverApprovalRequest(context.Background(), flow.ApprovalRequest{}) == nil {
		t.Fatal("unsupported approval silently accepted")
	}
	if out := a.SendStored(context.Background(), deliveryoutbox.Message{Channel: "telegram", Content: "legacy"}); out.Status != "absent" {
		t.Fatal("mixed legacy delivery accepted", out)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err = a.Run(cancelled); err != nil {
		t.Fatal(err)
	}
	p.Acceptor = &telegramrich.Ingress{Store: core, Binding: b}
	if _, err = NewExplicitAdapter(p, &communication.DeliveryStore{Core: core}, s); err == nil {
		t.Fatal("missing owner controls accepted")
	}
}

func TestCommunicationLegacyAdapterRejectsPublicationRows(t *testing.T) {
	a := &Adapter{}
	if out := a.SendStored(context.Background(), deliveryoutbox.Message{PublicationID: "publication:test", Channel: "telegram", SessionKey: "telegram:chat:42", Content: "must not use legacy sender"}); out.Status != "absent" || out.ErrorCode != "telegram_unconfigured" {
		t.Fatal(out)
	}
}
