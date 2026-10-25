package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

const defaultTextCharacters = 12000
const maxTextCharacters = 24000

var errTextWindow = errors.New("text window invalid or page changed; read again from text_offset=0")

type TextWindow struct {
	Offset     int    `json:"offset"`
	Returned   int    `json:"returned_characters"`
	Available  int    `json:"available_characters"`
	NextOffset *int   `json:"next_offset,omitempty"`
	TextHash   string `json:"text_hash"`
}

func validWindow(offset, limit int, expected string) bool {
	if offset < 0 || offset > 2*1024*1024 || limit != 0 && (limit < 256 || limit > maxTextCharacters) {
		return false
	}
	if expected != "" {
		b, e := hex.DecodeString(expected)
		if e != nil || len(b) != 32 {
			return false
		}
	}
	return true
}

// The upstream body limit protects IO. This separate window protects model
// context from archives/catalogs while retaining an explicit continuation and
// content hash so later windows cannot silently stitch different page versions.
func windowFetch(r FetchResult, offset, limit int, expected string) (FetchResult, error) {
	if !validWindow(offset, limit, expected) {
		return FetchResult{}, errTextWindow
	}
	if limit == 0 {
		limit = defaultTextCharacters
	}
	sum := sha256.Sum256([]byte(r.ContentText))
	digest := hex.EncodeToString(sum[:])
	if expected != "" && expected != digest {
		return FetchResult{}, errTextWindow
	}
	text := []rune(r.ContentText)
	if offset > len(text) {
		return FetchResult{}, errTextWindow
	}
	end := min(len(text), offset+limit)
	w := &TextWindow{Offset: offset, Returned: end - offset, Available: len(text), TextHash: digest}
	if end < len(text) {
		next := end
		w.NextOffset = &next
	}
	if r.Metadata == nil {
		r.Metadata = map[string]any{}
	}
	upstream, _ := r.Metadata["truncated"].(bool)
	r.Metadata["upstream_truncated"] = upstream
	if offset > 0 || end < len(text) {
		r.Metadata["truncated"] = true
	}
	r.ContentText = string(text[offset:end])
	r.Window = w
	if html := []rune(r.ContentHTML); len(html) > limit {
		r.ContentHTML = string(html[:limit])
		r.Metadata["html_truncated"] = true
	}
	return r, nil
}
