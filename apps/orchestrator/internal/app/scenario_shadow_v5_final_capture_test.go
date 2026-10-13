package app

import (
	"context"
	"fmt"
	"github.com/butler/butler/internal/transport"
	"os"
	"path/filepath"
	"sync"
)

// Forensic retention matches the provider's complete16MiB body bound. Visible
// acceptance stays8192; larger finals fail and only separate private bytes survive.
const shadowV5ForensicMaximumBytes = 16 << 20

func captureShadowV5PrivateFinal(private string, ordinal int, content string) (privateFinalMetadata, error) {
	raw := []byte(content)
	m := privateFinalFacts(raw)
	if len(raw) > shadowV5ForensicMaximumBytes {
		return m, errNativeFixtureDenied
	}
	stat, err := os.Lstat(private)
	if ordinal <= 0 || !filepath.IsAbs(private) || err != nil || !stat.IsDir() || stat.Mode()&os.ModeSymlink != 0 {
		return m, errNativeFixtureDenied
	}
	file, err := os.OpenFile(filepath.Join(private, fmt.Sprintf("final-%03d.raw", ordinal)), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return m, errNativeFixtureDenied
	}
	written, writeErr := file.Write(raw)
	syncErr := file.Sync()
	closeErr := file.Close()
	if writeErr != nil || written != len(raw) || syncErr != nil || closeErr != nil {
		return m, errNativeFixtureDenied
	}
	m.RawRetained = true
	return m, nil
}

// Future diagnostic-only observer. It neither admits a provider call nor
// repairs/rewraps its output. Only a matching filtered AssistantFinal is captured;
// deltas, thoughts, signatures and provider transport frames are never stored.
type shadowV5FinalCaptureProvider struct {
	transport.ModelProvider
	private          string
	ordinal          int
	mu               sync.Mutex
	metadata         *privateFinalMetadata
	captureFailed    bool
	raw              []byte // Bounded, private, never serialized; retained only until SQL persistence.
	persistAttempted bool
}

func (p *shadowV5FinalCaptureProvider) finalMetadata() *privateFinalMetadata {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.metadata == nil {
		return nil
	}
	copy := *p.metadata
	return &copy
}

func (p *shadowV5FinalCaptureProvider) captureFailure() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.captureFailed
}

// Called by the trusted driver only after owning SQL spend is persisted. File
// I/O can fail the diagnostic but cannot block or rewrite measured packet data.
func (p *shadowV5FinalCaptureProvider) persistFinal() error {
	p.mu.Lock()
	if p.metadata == nil {
		p.mu.Unlock()
		return errNativeFixtureDenied
	}
	if p.persistAttempted {
		p.mu.Unlock()
		return errNativeFixtureDenied
	}
	p.persistAttempted = true
	meta := *p.metadata
	raw := append([]byte(nil), p.raw...)
	previousFailure := p.captureFailed
	p.mu.Unlock()
	if meta.Bytes > shadowV5ForensicMaximumBytes {
		p.mu.Lock()
		p.captureFailed = true
		p.mu.Unlock()
		return errNativeFixtureDenied
	}
	retained, err := captureShadowV5PrivateFinal(p.private, p.ordinal, string(raw))
	p.mu.Lock()
	if retained.Bytes != meta.Bytes || retained.SHA256 != meta.SHA256 || retained.Reason != meta.Reason {
		err = errNativeFixtureDenied
	}
	p.metadata = &retained
	p.captureFailed = p.captureFailed || err != nil
	p.raw = nil
	p.mu.Unlock()
	if err != nil || previousFailure {
		return errNativeFixtureDenied
	}
	return nil
}

func (*shadowV5FinalCaptureProvider) ContinueRun(context.Context, transport.ContinueRunRequest) (transport.EventStream, error) {
	return nil, errNativeFixtureDenied
}
func (*shadowV5FinalCaptureProvider) SubmitToolResult(context.Context, transport.SubmitToolResultRequest) (transport.EventStream, error) {
	return nil, errNativeFixtureDenied
}

func (p *shadowV5FinalCaptureProvider) StartRun(ctx context.Context, req transport.StartRunRequest) (transport.EventStream, error) {
	stream, err := p.ModelProvider.StartRun(ctx, req)
	if err != nil {
		return nil, err
	}
	out := make(chan transport.TransportEvent)
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case event, open := <-stream:
				if !open {
					return
				}
				if event.RunID == req.Context.RunID && event.EventType == transport.EventTypeAssistantFinal && event.AssistantFinal != nil {
					p.mu.Lock()
					seen := p.metadata != nil
					p.mu.Unlock()
					if seen {
						p.mu.Lock()
						p.captureFailed = true
						p.mu.Unlock()
					} else {
						m := privateFinalFacts([]byte(event.AssistantFinal.Content))
						p.mu.Lock()
						p.metadata = &m
						if m.Bytes <= shadowV5ForensicMaximumBytes {
							p.raw = append([]byte(nil), event.AssistantFinal.Content...)
						} else {
							p.captureFailed = true
						}
						p.mu.Unlock()
					}
				}
				select {
				case out <- event:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out, nil
}
