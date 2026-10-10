package runtime

import (
	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	"github.com/butler/butler/internal/memory/sanitize"
	"github.com/butler/butler/internal/security/redact"
	"net/http"
	"strings"
)

func privateResponseText(response *http.Response, text string, credentials []*runtimev1.ResolvedCredential) string {
	values := []string{}
	for _, credential := range credentials {
		values = append(values, credential.GetValue())
	}
	for _, cookie := range response.Cookies() {
		values = append(values, cookie.Value)
	}
	return sanitize.Text(redact.Values(text, values...))
}

func publicResponseHeaders(response *http.Response, credentials []*runtimev1.ResolvedCredential) map[string]string {
	headers := response.Header
	result := map[string]string{}
	for _, name := range []string{"Content-Type", "Content-Length", "Cache-Control", "Date", "Expires", "Last-Modified"} {
		if value := headers.Get(name); value != "" {
			result[name] = privateResponseText(response, value, credentials)
		}
	}
	return result
}

func callerHeaderAllowed(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" || strings.ContainsAny(name, "\r\n") {
		return false
	}
	switch name {
	case "host", "connection", "transfer-encoding", "content-length", "authorization", "cookie", "set-cookie", "x-api-key", "proxy-authorization", "proxy-connection", "upgrade", "trailer", "te":
		return false
	}
	return !strings.HasPrefix(name, "sec-websocket-")
}
