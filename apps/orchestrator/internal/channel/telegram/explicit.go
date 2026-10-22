package telegram

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/channel/telegramrich"
	"github.com/butler/butler/apps/orchestrator/internal/communication"
	"github.com/butler/butler/apps/orchestrator/internal/deliveryoutbox"
)

var ErrExplicitUnsupported = errors.New("explicit Telegram channel operation unavailable")

type explicitChannel struct {
	poller *telegramrich.Poller
	worker deliveryoutbox.Worker
	sender telegramrich.Sender
}

// NewExplicitAdapter composes the existing Telegram adapter with durable
// publication delivery and authenticated reply ingress. Bootstrap never calls
// it. It has no provider/executor/final-send fallback or runtime enable flag.
func NewExplicitAdapter(p *telegramrich.Poller, d *communication.DeliveryStore, s *telegramrich.Sender) (*Adapter, error) {
	if p == nil || d == nil || s == nil || s.Client == nil || s.Resolve == nil || d.Core == nil || d.Core.Pool == nil || d.Core.Inputs == nil || d.Core.SourceGuard == nil || d.Core.ChannelGuard == nil || p.Pool != d.Core.Pool || p.Client != s.Client {
		return nil, ErrExplicitUnsupported
	}
	i, ok := p.Acceptor.(*telegramrich.Ingress)
	if !ok || i == nil || i.Controls == nil || i.Store != d.Core || i.Binding != p.Binding {
		return nil, ErrExplicitUnsupported
	}
	b := p.Binding
	if p.Notices != nil && (p.Notices.Pool != p.Pool || p.Notices.Client != s.Client || p.Notices.Binding != b) {
		return nil, ErrExplicitUnsupported
	}
	if b.OwnerID == "" || b.BindingID == "" || b.Version < 1 || b.BotID <= 0 || b.ChatID <= 0 || b.UserID != b.ChatID {
		return nil, ErrExplicitUnsupported
	}
	e := &explicitChannel{poller: p, sender: *s}
	scoped := d.ForBinding(communication.Binding{ID: b.BindingID, Version: b.Version, Channel: "telegram", Recipient: "telegram:" + strconv.FormatInt(b.ChatID, 10)})
	e.sender.Authorize = scoped.AuthorizeSend // Caller hooks cannot weaken the owning guard.
	e.sender.Widget = d.Core.DeliveryWidget
	e.worker = deliveryoutbox.Worker{Store: scoped, Sender: &e.sender}
	return &Adapter{explicit: e, allowedChatIDs: map[string]struct{}{strconv.FormatInt(b.ChatID, 10): {}}}, nil
}

func (e *explicitChannel) run(ctx context.Context) error {
	if ctx.Err() != nil {
		return nil
	}
	delay := time.Second
	for {
		me, err := e.sender.Client.GetMe(ctx)
		if err == nil && me.IsBot && me.ID == e.poller.Binding.BotID {
			break
		}
		// A channel outage or failed identity check cannot take down Web/Core.
		// No worker sends anything until this exact immutable client is verified.
		if !explicitWait(ctx, delay) {
			return nil
		}
		delay = min(delay*2, 30*time.Second)
	}
	run, cancel := context.WithCancel(ctx)
	defer cancel()
	finished := make(chan error, 2)
	go func() { finished <- e.worker.Run(run) }()
	go func() {
		finished <- runExplicitPolling(run, func(ctx context.Context) error {
			_, err := e.poller.ProcessOnce(ctx)
			if err != nil {
				return err
			}
			return e.poller.RefreshWidgets(ctx)
		}, time.Second)
	}()
	<-finished
	cancel()
	<-finished
	if ctx.Err() != nil {
		return nil
	}
	return ErrExplicitUnsupported
}

func explicitWait(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func runExplicitPolling(ctx context.Context, poll func(context.Context) error, interval time.Duration) error {
	delay := interval
	for ctx.Err() == nil {
		if err := poll(ctx); err != nil && ctx.Err() == nil {
			slog.Warn("explicit Telegram polling unavailable; durable cursor retained", "component", "telegram")
			if !explicitWait(ctx, delay) {
				break
			}
			delay = min(delay*2, 30*time.Second)
			continue
		}
		delay = interval
		if !explicitWait(ctx, delay) {
			break
		}
	}
	return ctx.Err()
}
