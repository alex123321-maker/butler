package singletabcontrol

import (
	"context"
	"github.com/butler/butler/internal/resourceexec"
	"github.com/butler/butler/internal/resources"
	"github.com/jackc/pgx/v5"
	"time"
)

func readSelected(ctx context.Context, tx pgx.Tx, b resources.Binding, id, host string) (resourceexec.SingleTabSession, error) {
	var out resourceexec.SingleTabSession
	if id == "" || host == "" {
		return out, ErrDenied
	}
	// Mapping is database-immutable. Every mutable row is locked below in order.
	var request, token, tab, origin string
	if tx.QueryRow(ctx, `SELECT request_id,candidate_token,tab_ref,origin FROM guarded_single_tab_selections WHERE session_id=$1`, id).Scan(&request, &token, &tab, &origin) != nil {
		return out, ErrDenied
	}
	var live bool
	var url string
	if tx.QueryRow(ctx, `SELECT status='selected' AND internal_tab_ref=$3,current_url FROM approval_tab_candidates WHERE approval_id=$1 AND candidate_token=$2 FOR SHARE`, request, token, tab).Scan(&live, &url) != nil || !live {
		return out, ErrDenied
	}
	canonical, e := resourceexec.SingleTabOrigin(url)
	if e != nil || canonical != origin || url != origin {
		return out, ErrDenied
	}
	var approvalExpiry time.Time
	if tx.QueryRow(ctx, `SELECT status='approved' AND approval_type='browser_tab_selection' AND tool_name=$4 AND run_id=$2 AND session_key=$3,expires_at FROM approvals WHERE approval_id=$1 FOR SHARE`, request, b.RunID, b.SessionKey, BindTool).Scan(&live, &approvalExpiry) != nil || !live {
		return out, ErrDenied
	}
	var expiry time.Time
	if tx.QueryRow(ctx, `SELECT expires_at,state='selected' AND owner_id=$2 AND run_id=$3 AND session_key=$4 AND task_id=$5 AND task_revision=$6 AND constraints_revision=$7 AND browser_instance=$8 FROM guarded_single_tab_requests WHERE request_id=$1 FOR SHARE`, request, b.OwnerID, b.RunID, b.SessionKey, b.TaskID, b.TaskRevision, b.ConstraintsRevision, host).Scan(&expiry, &live) != nil || !live {
		return out, ErrDenied
	}
	var sessionExpiry time.Time
	if tx.QueryRow(ctx, `SELECT LEAST(COALESCE(expires_at,created_at+INTERVAL '15 minutes'),created_at+INTERVAL '15 minutes'),status='ACTIVE' AND session_key=$2 AND created_by_run_id=$3 AND approval_id=$4 AND bound_tab_ref=$5 AND browser_instance_id=$6 AND released_at IS NULL AND current_url=$7 FROM single_tab_sessions WHERE single_tab_session_id=$1 FOR SHARE`, id, b.SessionKey, b.RunID, request, tab, host, origin).Scan(&sessionExpiry, &live) != nil || !live {
		return out, ErrDenied
	}
	if sessionExpiry.Before(expiry) {
		expiry = sessionExpiry
	}
	if approvalExpiry.Before(expiry) {
		expiry = approvalExpiry
	}
	var at time.Time
	if tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&at) != nil || !expiry.After(at) || !b.Deadline.After(at) {
		return out, ErrDenied
	}
	return resourceexec.SingleTabSession{ID: id, BrowserInstance: host, TabRef: tab, Origin: origin, ExpiresAt: expiry}, nil
}
