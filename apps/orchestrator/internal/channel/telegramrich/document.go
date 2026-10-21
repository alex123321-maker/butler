package telegramrich

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/butler/butler/apps/orchestrator/internal/artifacts"
)

// VerifyDocument reads an explicitly selected uploaded document through this
// bot's fixed file endpoint and compares it to retained bytes. It emits no
// credential URL, remote path/body or private file ID. It is an operator/channel
// check, not a model tool or arbitrary network/file access capability.
func (c *Client) VerifyDocument(ctx context.Context, fileID string, want artifacts.Media) error {
	if c == nil || len(fileID) == 0 || len(fileID) > 512 || strings.ContainsAny(fileID, "\r\n\x00") || want.Validate() != nil {
		return ErrUnavailable
	}
	body, _ := json.Marshal(map[string]string{"file_id": fileID})
	e, status, err := c.call(ctx, "getFile", body, "application/json")
	var file struct {
		ID   string `json:"file_id"`
		Path string `json:"file_path"`
		Size int    `json:"file_size"`
	}
	if err != nil || status != 200 || !*e.OK || json.Unmarshal(e.Result, &file) != nil || file.ID != fileID || file.Size != len(want.Data) {
		return ErrUnavailable
	}
	// Telegram supplies the path. Treat it as DATA: no authority to change host,
	// escape the fixed endpoint, inject query/fragment or normalize dot segments.
	if file.Path == "" || len(file.Path) > 512 || path.Clean(file.Path) != file.Path || strings.HasPrefix(file.Path, "/") || strings.Contains(file.Path, "..") || strings.ContainsAny(file.Path, "\\?#%:\r\n\x00") {
		return ErrUnavailable
	}
	for _, segment := range strings.Split(file.Path, "/") {
		if segment == "" || url.PathEscape(segment) != segment {
			return ErrUnavailable
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/file/bot"+c.token+"/"+file.Path, nil)
	if err != nil {
		return ErrUnavailable
	}
	client := *c.http
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		return ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return ErrUnavailable
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, int64(len(want.Data))+1))
	if err != nil || !bytes.Equal(raw, want.Data) {
		return ErrUnavailable
	}
	return nil
}
