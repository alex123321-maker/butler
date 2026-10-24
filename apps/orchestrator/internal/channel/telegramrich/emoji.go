package telegramrich

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
)

// EmojiSticker is public pack metadata, never a credential or download URL.
type EmojiSticker struct {
	FileID        string `json:"file_id"`
	FileUniqueID  string `json:"file_unique_id"`
	CustomEmojiID string `json:"custom_emoji_id"`
	Type          string `json:"type"`
	Width         int    `json:"width"`
	Height        int    `json:"height"`
	Animated      bool   `json:"is_animated"`
	Video         bool   `json:"is_video"`
	Emoji         string `json:"emoji"`
	SetName       string `json:"set_name"`
	Size          int    `json:"file_size"`
}
type EmojiSet struct {
	Name     string         `json:"name"`
	Title    string         `json:"title"`
	Type     string         `json:"sticker_type"`
	Stickers []EmojiSticker `json:"stickers"`
}

var emojiSetName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,63}$`)
var emojiID = regexp.MustCompile(`^[0-9]{1,24}$`)

// GetCustomEmojiSet is an operator-only public catalog read. No tool registration,
// owner-message read, polling cursor or delivery operation is installed.
func (c *Client) GetCustomEmojiSet(ctx context.Context, name string) (EmojiSet, error) {
	var s EmojiSet
	if !emojiSetName.MatchString(name) {
		return s, ErrUnavailable
	}
	b, _ := json.Marshal(map[string]string{"name": name})
	e, status, err := c.call(ctx, "getStickerSet", b, "application/json")
	if err != nil || status != 200 || e.OK == nil || !*e.OK || json.Unmarshal(e.Result, &s) != nil ||
		!strings.EqualFold(s.Name, name) || s.Type != "custom_emoji" || len(s.Stickers) == 0 || len(s.Stickers) > 2000 {
		return EmojiSet{}, ErrUnavailable
	}
	for _, v := range s.Stickers {
		if v.Type != "custom_emoji" || !emojiID.MatchString(v.CustomEmojiID) || v.FileID == "" || len(v.FileID) > 512 ||
			strings.ContainsAny(v.FileID, "\r\n\x00") || v.Width < 1 || v.Height < 1 || v.Width > 512 || v.Height > 512 ||
			v.Size < 0 || v.Size > 2*1024*1024 || v.Animated && v.Video ||
			v.SetName != "" && !strings.EqualFold(v.SetName, name) {
			return EmojiSet{}, ErrUnavailable
		}
	}
	return s, nil
}

// DownloadCustomEmoji accepts only metadata returned by the operator catalog.
// It allows a fixed HTTPS host and bounded bytes; remote paths remain private.
func (c *Client) DownloadCustomEmoji(ctx context.Context, s EmojiSticker) ([]byte, string, error) {
	if c == nil || c.http == nil || s.Type != "custom_emoji" || !emojiID.MatchString(s.CustomEmojiID) ||
		s.FileID == "" || len(s.FileID) > 512 || strings.ContainsAny(s.FileID, "\r\n\x00") ||
		s.Size < 0 || s.Size > 2*1024*1024 || s.Animated && s.Video {
		return nil, "", ErrUnavailable
	}
	b, _ := json.Marshal(map[string]string{"file_id": s.FileID})
	e, status, err := c.call(ctx, "getFile", b, "application/json")
	var f struct {
		ID   string `json:"file_id"`
		Path string `json:"file_path"`
		Size int    `json:"file_size"`
	}
	if err != nil || status != 200 || e.OK == nil || !*e.OK || json.Unmarshal(e.Result, &f) != nil ||
		f.ID != s.FileID || f.Size < 1 || f.Size > 2*1024*1024 || s.Size > 0 && f.Size != s.Size {
		return nil, "", ErrUnavailable
	}
	if f.Path == "" || len(f.Path) > 512 || path.Clean(f.Path) != f.Path || strings.HasPrefix(f.Path, "/") ||
		strings.Contains(f.Path, "..") || strings.ContainsAny(f.Path, "\\?#%:\r\n\x00") {
		return nil, "", ErrUnavailable
	}
	for _, seg := range strings.Split(f.Path, "/") {
		if seg == "" || url.PathEscape(seg) != seg {
			return nil, "", ErrUnavailable
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/file/bot"+c.token+"/"+f.Path, nil)
	if err != nil {
		return nil, "", ErrUnavailable
	}
	client := *c.http
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, "", ErrUnavailable
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, int64(f.Size)+1))
	if err != nil || len(raw) != f.Size {
		return nil, "", ErrUnavailable
	}
	mime := ""
	switch {
	case s.Animated && len(raw) > 2 && raw[0] == 0x1f && raw[1] == 0x8b:
		mime = "application/x-tgsticker"
	case s.Video && bytes.HasPrefix(raw, []byte{0x1a, 0x45, 0xdf, 0xa3}):
		mime = "video/webm"
	case !s.Animated && !s.Video && len(raw) >= 12 && string(raw[:4]) == "RIFF" && string(raw[8:12]) == "WEBP":
		mime = "image/webp"
	case !s.Animated && !s.Video && bytes.HasPrefix(raw, []byte{137, 80, 78, 71, 13, 10, 26, 10}):
		mime = "image/png"
	default:
		return nil, "", ErrUnavailable
	}
	return raw, mime, nil
}
