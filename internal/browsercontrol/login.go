package browsercontrol

import (
	"context"
	"encoding/json"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/browserauth"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"time"
)

// A login may follow a redirect. Check its observed resource against the same
// current task/goal constraints before releasing any page bytes. This stricter
// release projection authorizes no additional request or POST.
func (s *Store) checkLoginRelease(ctx context.Context, call *toolbrokerv1.ToolCall, b actionauth.ActionBinding, result *toolbrokerv1.ToolResult) error {
	deny := func() error { result.ResultJson = "{}"; return ErrDenied }
	a, err := browserauth.Parse(call.GetArgsJson())
	if err != nil {
		return deny()
	}
	var out struct {
		URL string `json:"final_url"`
		ID  string `json:"browser_session_id"`
	}
	if json.Unmarshal([]byte(result.ResultJson), &out) != nil || out.ID != a.Browser || browserauth.Origin(out.URL) != browserauth.Origin(a.TargetURL) {
		return deny()
	}
	epoch, ok := admittedEpoch(ctx)
	if !ok {
		return deny()
	}
	b.Resource = "browser:" + a.Browser + ":" + out.URL
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return deny()
	}
	defer tx.Rollback(ctx)
	var at time.Time
	if tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&at) != nil || actionauth.CheckTaskActionTx(ctx, tx, b, at) != nil {
		return deny()
	}
	var current bool
	if tx.QueryRow(ctx, `SELECT TRUE FROM server_browser_sessions WHERE browser_session_id=$1 AND run_id=$2 AND owner_id=$3 AND task_id=$4 AND epoch=$5 AND mode='agent' AND expires_at>clock_timestamp() FOR SHARE`, a.Browser, b.RunID, b.OwnerID, b.TaskID, epoch).Scan(&current) != nil || !current {
		return deny()
	}
	if tx.Commit(ctx) != nil {
		return deny()
	}
	return nil
}
