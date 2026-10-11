package voice

import (
	"context"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/owneridentity"
	"time"
)

// Present is an explicit owner request, independent of a completed run lease.
// The text remains in its original catalog source if speech fails or is stopped.
func (p *Processor) Present(ctx context.Context, source catalog.SourceRef, allowance budget.Units) ([]byte, error) {
	if _, ok := owneridentity.FromContext(ctx); !ok {
		return nil, ErrDenied
	}
	until, e := p.store.presentationDeadline(ctx, source)
	if e != nil {
		return nil, e
	}
	deadline := time.Now().Add(2 * time.Minute)
	if until != nil && until.Before(deadline) {
		deadline = *until
	}
	id, e := budget.NewOperationID()
	if e != nil {
		return nil, e
	}
	_, e = p.store.Jobs.Enqueue(ctx, jobs.Request{ID: id, Kind: "ingest", Purpose: "voice_tts", Sources: []catalog.SourceRef{source}, Allowance: allowance, Deadline: deadline})
	if e != nil {
		return nil, e
	}
	j, ok, e := p.store.Jobs.ClaimOwner(ctx, id, "ingest", "voice_tts")
	if e != nil || !ok {
		return nil, ErrDenied
	}
	wav, e := p.Synthesize(ctx, j, source)
	code := ""
	if e != nil {
		code = "voice_tts_unavailable"
	}
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	finish := p.store.Jobs.Finish(finishCtx, j, code, false)
	if e != nil {
		return nil, e
	}
	if finish != nil {
		return nil, finish
	}
	return wav, nil
}

func (p *Processor) Run(ctx context.Context) error {
	for ctx.Err() == nil {
		worked, e := p.ProcessOne(ctx)
		delay := 5 * time.Second
		if worked {
			delay = 100 * time.Millisecond
		}
		if e != nil {
			delay = time.Second
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return ctx.Err()
}
