package transport

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type forbiddenErrorBody struct{ closed bool }

func (*forbiddenErrorBody) Read([]byte) (int, error) {
	panic("untrusted error body must not be parsed")
}
func (b *forbiddenErrorBody) Close() error { b.closed = true; return nil }

func TestProviderErrorsNeverRetainResponseOrCredentialText(t *testing.T) {
	body := &forbiddenErrorBody{}
	err := ProviderHTTPError(&http.Response{StatusCode: 429, Body: body})
	normalized := NormalizeError(err, "fixture")
	if !body.closed || normalized.Type != ErrorTypeRateLimited || !normalized.Retryable || normalized.ProviderDetails != nil || normalized.ProviderCode != "" {
		t.Fatal("HTTP classification or safe error boundary lost")
	}
	secret := "synthetic-private-provider-token"
	for _, err := range []error{errors.New(secret), &HTTPStatusError{StatusCode: 400, Message: secret, Code: secret, Details: map[string]any{"token": secret}}} {
		event := NewTransportErrorEvent("run", "fixture", err)
		if strings.Contains(event.TransportError.Error(), secret) || event.TransportError.ProviderDetails != nil || event.TransportError.ProviderCode != "" || strings.Contains(event.PayloadJSON, secret) {
			t.Fatal("untrusted error became persistent transport state")
		}
	}
	response := &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("success"))}
	if ProviderHTTPError(response) != nil {
		t.Fatal("successful response rejected")
	}
}
