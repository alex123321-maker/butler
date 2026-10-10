package transport

import "net/http"

// ProviderHTTPError retains only the HTTP classification. Remote error bodies
// may reflect request content or credentials and must never enter run state,
// logs, delivery or a later model request. Closing without parsing also bounds
// resources for malformed, oversized and streaming error responses.
func ProviderHTTPError(response *http.Response) error {
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return nil
	}
	if response.Body != nil {
		_ = response.Body.Close()
	}
	return &HTTPStatusError{StatusCode: response.StatusCode}
}

// ProviderProtocolError deliberately does not accept a remote payload.
func ProviderProtocolError(providerName string) error {
	return &Error{Type: ErrorTypeProviderProtocolError, Message: "provider rejected the operation", ProviderName: providerName}
}
