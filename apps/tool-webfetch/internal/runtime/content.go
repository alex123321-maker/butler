package runtime

import (
	"bytes"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"unicode/utf8"
)

const maxContentBytes = 2 * 1024 * 1024

var (
	errUnsupportedContent = errors.New("web resource is not supported UTF-8 text")
	errResponseLimit      = errors.New("web resource response exceeds the supported size limit")
)

// Read one additional byte so an exact-limit response is distinguishable from
// an incomplete response. The additional byte is never published.
func readContent(body io.Reader) ([]byte, bool, error) {
	data, err := io.ReadAll(io.LimitReader(body, maxContentBytes+1))
	if err != nil {
		return nil, false, err
	}
	truncated := len(data) > maxContentBytes
	if truncated {
		data = data[:maxContentBytes]
	}
	return data, truncated, nil
}

func textMIME(raw string, sample []byte) (string, error) {
	if strings.TrimSpace(raw) == "" {
		raw = http.DetectContentType(sample)
	}
	kind, _, err := mime.ParseMediaType(raw)
	if err != nil {
		return "", errUnsupportedContent
	}
	if strings.HasPrefix(kind, "text/") || kind == "application/json" || kind == "application/xml" || kind == "application/xhtml+xml" || strings.HasSuffix(kind, "+json") || strings.HasSuffix(kind, "+xml") {
		return kind, nil
	}
	return "", errUnsupportedContent
}

func contentText(body []byte, truncated bool) (string, error) {
	// Cutting a valid multibyte rune at the byte limit is not malformed input.
	if truncated && !utf8.Valid(body) {
		for start := len(body) - 1; start >= 0 && start >= len(body)-utf8.UTFMax; start-- {
			if utf8.RuneStart(body[start]) {
				if !utf8.FullRune(body[start:]) {
					body = body[:start]
				}
				break
			}
		}
	}
	if !utf8.Valid(body) || bytes.IndexByte(body, 0) >= 0 {
		return "", errUnsupportedContent
	}
	return string(body), nil
}
