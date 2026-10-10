package redact

import (
	"encoding/base64"
	"net/url"
	"sort"
	"strings"
)

// Values is an ephemeral transform. Secret values and derived encodings are
// never persisted as redaction metadata or logged.
func Values(text string, values ...string) string {
	variants := map[string]bool{}
	for _, value := range values {
		if value == "" {
			continue
		}
		for _, variant := range []string{value, url.QueryEscape(value), url.PathEscape(value), base64.StdEncoding.EncodeToString([]byte(value)), base64.RawStdEncoding.EncodeToString([]byte(value)), base64.URLEncoding.EncodeToString([]byte(value)), base64.RawURLEncoding.EncodeToString([]byte(value))} {
			variants[variant] = true
		}
	}
	ordered := make([]string, 0, len(variants))
	for value := range variants {
		ordered = append(ordered, value)
	}
	sort.Slice(ordered, func(i, j int) bool { return len(ordered[i]) > len(ordered[j]) })
	for _, value := range ordered {
		text = strings.ReplaceAll(text, value, "[REDACTED]")
	}
	return text
}
