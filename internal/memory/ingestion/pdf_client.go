package ingestion

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"
)

// NewPDFClient only addresses the dedicated no-egress parser service. It cannot
// fetch caller URLs, follow redirects or forward owner/provider credentials.
type PDFClient struct{ client *http.Client }

func NewPDFClient() *PDFClient {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil // operator proxy settings must not reroute private documents
	return &PDFClient{client: &http.Client{Transport: transport, Timeout: 16 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}
func (p *PDFClient) call(ctx context.Context, mode string, raw []byte) ([]Page, error) {
	if len(raw) == 0 || len(raw) > MaxBytes {
		return nil, ErrLimit
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, "http://memory-parser:9091/"+mode, bytes.NewReader(raw))
	if e != nil {
		return nil, ErrCorrupt
	}
	req.Header.Set("Content-Type", "application/pdf")
	r, e := p.client.Do(req)
	if e != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrFormat
	}
	defer r.Body.Close()
	if r.StatusCode == http.StatusRequestEntityTooLarge {
		return nil, ErrLimit
	}
	if r.StatusCode != 200 {
		return nil, ErrCorrupt
	}
	b, e := io.ReadAll(io.LimitReader(r.Body, 16<<20+1))
	if e != nil || len(b) > 16<<20 {
		return nil, ErrLimit
	}
	var pages []Page
	if json.Unmarshal(b, &pages) != nil || len(pages) > 16 {
		return nil, ErrCorrupt
	}
	return pages, nil
}
func (p *PDFClient) TextLayer(ctx context.Context, raw []byte) ([]Page, error) {
	return p.call(ctx, "text", raw)
}
func (p *PDFClient) Recognize(ctx context.Context, raw []byte) ([]Page, error) {
	return p.call(ctx, "ocr", raw)
}
