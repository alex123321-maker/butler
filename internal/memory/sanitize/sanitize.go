package sanitize

import (
	"encoding/json"
	"regexp"
	"strings"
)

var (
	bearerPattern         = regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/=-]+`)
	credentialURLPattern  = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.-]*://)([^/@\s:]+):([^@\s]+)@`)
	keyValueSecretPattern = regexp.MustCompile(`(?i)\b(access_token|refresh_token|api_key|apikey|token|secret|client_secret|password|passwd|pwd|cookie|set-cookie|storage_state|connection_string|dsn)\b\s*[:=]\s*("[^"]*"|'[^']*'|[^\s,;]+)`)
	commonTokenPattern    = regexp.MustCompile(`\b(?:sk-[A-Za-z0-9_-]{8,}|ghp_[A-Za-z0-9]{12,}|github_pat_[A-Za-z0-9_]{20,}|xox[baprs]-[A-Za-z0-9-]{10,})\b`)
	jwtLikePattern        = regexp.MustCompile(`\beyJ[A-Za-z0-9_-]+\.[A-Za-z0-9._-]+\.[A-Za-z0-9._-]+\b`)
	passwordInlinePattern = regexp.MustCompile(`(?i)(password\s+is\s+)(\S+)`)
	cookieInlinePattern   = regexp.MustCompile(`(?i)(cookie\s+value\s+is\s+)(\S+)`)
)

func Text(input string) string {
	if strings.TrimSpace(input) == "" {
		return input
	}
	output := input
	output = credentialURLPattern.ReplaceAllString(output, `${1}[REDACTED_DSN]@`)
	output = bearerPattern.ReplaceAllString(output, `Bearer [REDACTED_TOKEN]`)
	output = commonTokenPattern.ReplaceAllString(output, `[REDACTED_TOKEN]`)
	output = jwtLikePattern.ReplaceAllString(output, `[REDACTED_TOKEN]`)
	output = passwordInlinePattern.ReplaceAllString(output, `${1}[REDACTED_PASSWORD]`)
	output = cookieInlinePattern.ReplaceAllString(output, `${1}[REDACTED_COOKIE]`)
	output = keyValueSecretPattern.ReplaceAllStringFunc(output, func(match string) string {
		parts := keyValueSecretPattern.FindStringSubmatch(match)
		if len(parts) < 2 {
			return `[REDACTED]`
		}
		return parts[1] + `=` + placeholderForKey(parts[1])
	})
	return output
}

func JSON(input string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return input
	}
	if !json.Valid([]byte(trimmed)) {
		return Text(input)
	}
	var decoded any
	if err := json.Unmarshal([]byte(trimmed), &decoded); err != nil {
		return Text(input)
	}
	sanitized := value(decoded, "")
	encoded, err := json.Marshal(sanitized)
	if err != nil {
		return Text(input)
	}
	return string(encoded)
}

func value(input any, parentKey string) any {
	switch typed := input.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			if isSensitiveKey(key) {
				result[key] = placeholderForKey(key)
				continue
			}
			result[key] = value(item, key)
		}
		return result
	case []any:
		result := make([]any, 0, len(typed))
		for _, item := range typed {
			result = append(result, value(item, parentKey))
		}
		return result
	case string:
		if isSensitiveKey(parentKey) {
			return placeholderForKey(parentKey)
		}
		return Text(typed)
	default:
		return input
	}
}

func isSensitiveKey(key string) bool {
	lowered := strings.ToLower(strings.TrimSpace(key))
	for _, marker := range []string{"password", "passwd", "pwd", "token", "secret", "api_key", "apikey", "access_token", "refresh_token", "client_secret", "cookie", "set-cookie", "storage_state", "connection_string", "dsn"} {
		if lowered == marker || strings.Contains(lowered, marker) {
			return true
		}
	}
	return false
}

func placeholderForKey(key string) string {
	lowered := strings.ToLower(strings.TrimSpace(key))
	switch {
	case strings.Contains(lowered, "password"), strings.Contains(lowered, "passwd"), strings.Contains(lowered, "pwd"):
		return "[REDACTED_PASSWORD]"
	case strings.Contains(lowered, "cookie"), strings.Contains(lowered, "storage_state"):
		return "[REDACTED_COOKIE]"
	case strings.Contains(lowered, "dsn"), strings.Contains(lowered, "connection"):
		return "[REDACTED_DSN]"
	default:
		return "[REDACTED_TOKEN]"
	}
}

func TranscriptMessageContent(content string) string {
	return Text(content)
}

func TranscriptMetadataJSON(metadata string) string {
	return JSON(metadata)
}

func TranscriptToolArgsJSON(args string) string {
	return JSON(args)
}

func TranscriptToolResultJSON(result string) string {
	return JSON(result)
}

func TranscriptToolErrorJSON(err string) string {
	return JSON(err)
}
