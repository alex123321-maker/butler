package config

import "strings"

const fullMask = "••••••••"

func MaskForDisplay(key, value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	normalizedKey := strings.ToLower(strings.TrimSpace(key))

	if isTelegramToken(normalizedKey, trimmed) {
		if len(trimmed) <= 9 {
			return fullMask
		}
		return trimmed[:6] + ":..." + trimmed[len(trimmed)-3:]
	}
	if isFullyMaskedSecret(normalizedKey, trimmed) {
		return fullMask
	}
	if isAPIKey(normalizedKey) {
		if len(trimmed) <= 4 {
			return "..." + trimmed
		}
		return "..." + trimmed[len(trimmed)-4:]
	}
	return fullMask
}

func isTelegramToken(key, value string) bool {
	return strings.Contains(key, "telegram") && strings.Contains(value, ":")
}

func isFullyMaskedSecret(key, value string) bool {
	if strings.Contains(value, "://") {
		return true
	}
	for _, marker := range []string{"password", "dsn", "url", "connection", "secret"} {
		if strings.Contains(key, marker) {
			return true
		}
	}
	return false
}

func isAPIKey(key string) bool {
	return strings.Contains(key, "api") || strings.Contains(key, "key")
}
