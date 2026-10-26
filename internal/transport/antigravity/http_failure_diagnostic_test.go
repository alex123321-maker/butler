package antigravity

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/butler/butler/internal/transport"
)

func TestHTTPNoResponseDiagnosticIsNarrowAndNeverRetries(t *testing.T) {
	var calls atomic.Int32
	p := newFixture(t, func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, fmt.Errorf("SYNTHETIC_PRIVATE_URL_TOKEN_ERROR: %w", syscall.ECONNRESET)
	})
	var stage transport.ProviderStage
	ctx := transport.WithProviderStageObserver(t.Context(), func(s transport.ProviderStage) { stage = s })
	stream, err := p.StartRun(ctx, request("no-response"))
	if err != nil {
		t.Fatal(err)
	}
	events := collect(t, stream)
	if len(events) != 2 || events[0].EventType != transport.EventTypeRunStarted || events[1].EventType != transport.EventTypeTransportError {
		t.Fatal("unexpected normalized event chain")
	}
	e := events[1]
	if calls.Load() != 1 || stage != transport.ProviderStageHTTP || e.TransportError == nil || e.TransportError.Type != transport.ErrorTypeTransportConnectionError || e.TransportError.Retryable || transport.DiagnosticReason(e.TransportError) != "http_do_failed_without_response" || e.TransportError.HTTPStatus != 0 || e.Usage != nil || e.ProviderModelVersion != nil {
		t.Fatal("exact no-response branch metadata or one-request boundary changed")
	}
	if strings.Contains(e.PayloadJSON, "SYNTHETIC_PRIVATE") || strings.Contains(e.TransportError.Error(), "SYNTHETIC_PRIVATE") {
		t.Fatal("raw error entered public diagnostics")
	}
}

func TestHTTPNoResponseDiagnosticRejectsCancellationAndReturnedResponse(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := httpExchangeFailure(ctx, nil, syscall.ECONNRESET); !errors.Is(err, context.Canceled) || transport.DiagnosticReason(err) != "" {
		t.Fatal("owning cancellation acquired local connection diagnostic")
	}
	err := httpExchangeFailure(t.Context(), &http.Response{StatusCode: 403}, syscall.ECONNRESET)
	if transport.DiagnosticReason(err) != "" || transport.DiagnosticCode(err) != "transport_connection_error" {
		t.Fatal("returned response acquired no-response diagnostic")
	}
}

func TestHTTPNoResponseDiagnosticClientTimeoutStaysGlobalClass(t *testing.T) {
	var calls atomic.Int32
	p := newFixture(t, func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		<-r.Context().Done()
		return nil, r.Context().Err()
	})
	p.client.Timeout = time.Millisecond
	parent := t.Context()
	stream, err := p.StartRun(parent, request("client-timeout"))
	if err != nil {
		t.Fatal(err)
	}
	e := findError(collect(t, stream))
	if parent.Err() != nil || e == nil || transport.DiagnosticReason(e) != "" || calls.Load() != 1 || e.Retryable {
		t.Fatal("independent client timeout acquired local exception or retry")
	}
}

func TestHTTPNoResponseDiagnosticRejectsDeadlineUnknownAndCertificate(t *testing.T) {
	for _, cause := range []error{nil, context.Canceled, context.DeadlineExceeded, errors.New("arbitrary failure"), x509.UnknownAuthorityError{}, errors.Join(syscall.ECONNRESET, context.DeadlineExceeded), errors.Join(x509.UnknownAuthorityError{}, io.EOF), &tls.CertificateVerificationError{Err: io.EOF}, &httpDiagnosticCycle{}} {
		if transport.DiagnosticReason(httpExchangeFailure(t.Context(), nil, cause)) != "" {
			t.Fatal("non-connection failure acquired local diagnostic")
		}
	}
}

type httpDiagnosticCycle struct{}

func (*httpDiagnosticCycle) Error() string   { return "synthetic cyclic diagnostic" }
func (e *httpDiagnosticCycle) Unwrap() error { return e }
