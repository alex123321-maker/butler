package api

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

func NewExtensionAuthMiddleware(tokens []string) func(http.Handler) http.Handler {
	normalized := make([]string, 0, len(tokens))
	for _, token := range tokens {
		trimmed := strings.TrimSpace(token)
		if trimmed == "" {
			continue
		}
		normalized = append(normalized, trimmed)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(normalized) == 0 {
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{
					"error": "extension API is disabled",
					"code":  "extension_api_disabled",
				})
				return
			}

			authorization := r.Header.Get("Authorization")
			token, ok := parseBearerAuthorization(authorization)
			if !ok || !isTokenAuthorized(token, normalized) {
				w.Header().Set("WWW-Authenticate", "Bearer")
				writeJSON(w, http.StatusUnauthorized, map[string]string{
					"error": "extension authorization failed",
					"code":  "unauthorized",
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func parseBearerAuthorization(raw string) (string, bool) {
	parts := strings.Fields(strings.TrimSpace(raw))
	if len(parts) != 2 {
		return "", false
	}
	if !strings.EqualFold(parts[0], "Bearer") {
		return "", false
	}
	if strings.TrimSpace(parts[1]) == "" {
		return "", false
	}
	return parts[1], true
}

func isTokenAuthorized(token string, allowed []string) bool {
	for _, candidate := range allowed {
		if subtle.ConstantTimeCompare([]byte(token), []byte(candidate)) == 1 {
			return true
		}
	}
	return false
}
