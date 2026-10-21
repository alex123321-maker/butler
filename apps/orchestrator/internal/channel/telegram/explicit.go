package telegram

import (
	"context"
	"errors"
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
	if b.OwnerID == "" || b.BindingID == "" || b.Version < 1 || b.BotID <= 0 || b.ChatID <= 0 || b.UserID != b.ChatID {
		return nil, ErrExplicitUnsupported
	}
	e := &explicitChannel{poller: p, sender: *s}
	e.sender.Authorize = d.AuthorizeSend // Caller hooks cannot weaken the owning guard.
	e.worker = deliveryoutbox.Worker{Store: d, Sender: &e.sender}
	return &Adapter{explicit: e, allowedChatIDs: map[string]struct{}{strconv.FormatInt(b.ChatID, 10): {}}}, nil
}

func (e *explicitChannel) run(ctx context.Context) error {
	if ctx.Err() != nil {
		return nil
	}
	me, err := e.sender.Client.GetMe(ctx)
	if err != nil || !me.IsBot || me.ID != e.poller.Binding.BotID {
		return ErrExplicitUnsupported
	}
	run, cancel := context.WithCancel(ctx)
	defer cancel()
	finished := make(chan error, 2)
	go func() { finished <- e.worker.Run(run) }()
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			if _, err := e.poller.ProcessOnce(run); err != nil {
				finished <- err
				return
			}
			select {
			case <-run.Done():
				finished <- run.Err()
				return
			case <-ticker.C:
			}
		}
	}()
	<-finished
	cancel()
	<-finished
	if ctx.Err() != nil {
		return nil
	}
	return ErrExplicitUnsupported
}
