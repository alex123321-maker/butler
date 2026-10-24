package telegramrich

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmojiCatalogAndDownloadBoundaries(t *testing.T) {
	for _, bad := range []string{"../evil", "https://evil", "", strings.Repeat("x", 65)} {
		if _, err := (&Client{}).GetCustomEmojiSet(context.Background(), bad); err == nil {
			t.Fatal("invalid set accepted")
		}
	}
	for _, p := range []string{"stickers/a.webp", "../a", "/a", "a/../b", "a%2fb", "a?x", "a#x", "https://evil", "a\\b"} {
		t.Run(p, func(t *testing.T) {
			downloads := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "getFile") {
					json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": map[string]any{"file_id": "f", "file_size": 12, "file_path": p}})
				} else {
					downloads++
					w.Write([]byte("RIFF0000WEBP"))
				}
			}))
			defer srv.Close()
			c := &Client{token: "private", base: srv.URL, http: srv.Client()}
			b, m, err := c.DownloadCustomEmoji(context.Background(), EmojiSticker{FileID: "f", CustomEmojiID: "123", Type: "custom_emoji", Size: 12})
			if p == "stickers/a.webp" {
				if err != nil || len(b) != 12 || m != "image/webp" {
					t.Fatal("valid failed")
				}
			} else if err == nil || downloads != 0 {
				t.Fatal("unsafe path reached network")
			}
		})
	}
}
func TestEmojiRejectsInvalidCatalogAndFile(t *testing.T) {
	for _, kind := range []string{"regular", "wrong_id", "oversize", "wrong_format", "redirect", "secret_error"} {
		t.Run(kind, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if kind == "secret_error" {
					w.WriteHeader(500)
					w.Write([]byte("private"))
					return
				}
				if strings.HasSuffix(r.URL.Path, "getStickerSet") {
					ty := "custom_emoji"
					if kind == "regular" {
						ty = "regular"
					}
					json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": EmojiSet{Name: "Test", Type: ty, Stickers: []EmojiSticker{{FileID: "f", CustomEmojiID: "123", Type: ty, Width: 100, Height: 100}}}})
					return
				}
				if strings.HasSuffix(r.URL.Path, "getFile") {
					id, size := "f", 12
					if kind == "wrong_id" {
						id = "other"
					}
					if kind == "oversize" {
						size = 3 * 1024 * 1024
					}
					json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": map[string]any{"file_id": id, "file_size": size, "file_path": "a.webp"}})
					return
				}
				if kind == "redirect" {
					http.Redirect(w, r, "https://example.invalid", 302)
					return
				}
				w.Write([]byte("not an image"))
			}))
			defer srv.Close()
			c := &Client{token: "private", base: srv.URL, http: srv.Client()}
			if kind == "regular" || kind == "secret_error" {
				_, err := c.GetCustomEmojiSet(context.Background(), "Test")
				if err != ErrUnavailable {
					t.Fatal("catalog accepted")
				}
				return
			}
			_, _, err := c.DownloadCustomEmoji(context.Background(), EmojiSticker{FileID: "f", CustomEmojiID: "123", Type: "custom_emoji"})
			if err != ErrUnavailable {
				t.Fatal("file accepted")
			}
		})
	}
}
