package singletabcontrol

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/resourceexec"
	"github.com/butler/butler/internal/resources"
	"github.com/jackc/pgx/v5"
)

func randomID() (string, error) {
	var v [24]byte
	_, e := rand.Read(v[:])
	return hex.EncodeToString(v[:]), e
}
func (c *Controller) begin(ctx context.Context, b resources.Binding) (pgx.Tx, error) {
	a, ok := owneridentity.FromContext(ctx)
	if c == nil || !ok || a.OwnerID != b.OwnerID || (a.SessionKey != "" && a.SessionKey != b.SessionKey) {
		return nil, ErrDenied
	}
	tx, e := c.pool.Begin(ctx)
	if e != nil {
		return nil, e
	}
	// All owning selection writers start with the same exclusive owning barrier.
	// Broker uses SHARE here before candidates. No candidate holder waits backward
	// for ownership, so select/revoke and resource dispatch have one serial order.
	var session string
	e = tx.QueryRow(ctx, `SELECT session_key FROM session_ownership WHERE session_key=$1 FOR UPDATE`, b.SessionKey).Scan(&session)
	if e == nil {
		e = c.guard.CheckTx(ctx, tx, b)
	}
	if e == nil {
		e = c.sources(ctx, tx, b.RunID)
	}
	if e != nil {
		_ = tx.Rollback(context.Background())
		return nil, ErrDenied
	}
	return tx, nil
}
func (c *Controller) preflight(ctx context.Context, b resources.Binding) error {
	tx, e := c.begin(ctx, b)
	if e != nil {
		return e
	}
	defer tx.Rollback(context.Background())
	return tx.Commit(ctx)
}

// finalCheck repeats the finite binding deadline after every domain/authority
// lock wait. Query errors never become a successful owner decision.
func (c *Controller) finalCheck(ctx context.Context, tx pgx.Tx, b resources.Binding, expiry time.Time) error {
	if c.guard.CheckTx(ctx, tx, b) != nil {
		return ErrDenied
	}
	var at time.Time
	if tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&at) != nil || !b.Deadline.After(at) || !expiry.After(at) {
		return ErrDenied
	}
	return nil
}
func validTab(t Tab, host string) bool {
	origin, e := resourceexec.SingleTabOrigin(t.Origin)
	if e != nil || len(origin) > 512 || host == "" || len(host) > 128 || origin != t.Origin || t.BrowserInstance != host || t.Ref == "" || len(t.Ref) > 10 {
		return false
	}
	for _, v := range t.Ref {
		if v < '0' || v > '9' {
			return false
		}
	}
	return t.Ref[0] != '0'
}

// Request obtains bounded actual native metadata outside every PG transaction,
// then rechecks current owning authority before atomic pending publication.
// b is resolved by trusted composition, never an HTTP/model binding argument.
func (c *Controller) Request(ctx context.Context, b resources.Binding) (Selection, error) {
	var out Selection
	if c.preflight(ctx, b) != nil {
		return out, ErrDenied
	}
	tabs, e := c.metadata.List(ctx)
	if e != nil || len(tabs) < 1 || len(tabs) > MaxTabs {
		return out, ErrDenied
	}
	refs := map[string]bool{}
	for _, t := range tabs {
		if !validTab(t, c.host) || refs[t.Ref] {
			return out, ErrDenied
		}
		refs[t.Ref] = true
	}
	tx, e := c.begin(ctx, b)
	if e != nil {
		return out, e
	}
	defer tx.Rollback(context.Background())
	var busy bool
	if tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM single_tab_sessions WHERE session_key=$1 AND status='ACTIVE') OR EXISTS(SELECT 1 FROM guarded_single_tab_requests WHERE session_key=$1 AND state='pending' AND expires_at>clock_timestamp())`, b.SessionKey).Scan(&busy) != nil || busy {
		return out, ErrDenied
	}
	id, e := randomID()
	if e != nil {
		return out, e
	}
	var at time.Time
	if tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&at) != nil || !b.Deadline.After(at) {
		return out, ErrDenied
	}
	expiry := at.Add(15 * time.Minute)
	if b.Deadline.Before(expiry) {
		expiry = b.Deadline
	}
	_, e = tx.Exec(ctx, `INSERT INTO approvals(approval_id,run_id,session_key,tool_call_id,approval_type,status,requested_via,tool_name,args_json,payload_json,risk_level,summary,details_json,requested_at,expires_at,updated_at)
 VALUES($1,$2,$3,$1,'browser_tab_selection','pending',$4,$5,'{}','{}','medium','Select one guarded browser origin','{}',$6,$7,$6)`, id, b.RunID, b.SessionKey, actorChannel(ctx), BindTool, at, expiry)
	if e != nil {
		return out, e
	}
	_, e = tx.Exec(ctx, `INSERT INTO guarded_single_tab_requests(request_id,approval_id,owner_id,run_id,session_key,task_id,task_revision,constraints_revision,browser_instance,created_at,expires_at,state) VALUES($1,$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'pending')`, id, b.OwnerID, b.RunID, b.SessionKey, b.TaskID, b.TaskRevision, b.ConstraintsRevision, c.host, at, expiry)
	if e != nil {
		return out, e
	}
	for i, t := range tabs {
		token, e := randomID()
		if e != nil {
			return out, e
		}
		_, e = tx.Exec(ctx, `INSERT INTO approval_tab_candidates(approval_id,candidate_token,internal_tab_ref,domain,current_url,display_label,status,created_at,expires_at) VALUES($1,$2,$3,$4,$4,$4,'available',$5,$6)`, id, token, t.Ref, t.Origin, at, expiry)
		if e != nil {
			return out, e
		}
		_, e = tx.Exec(ctx, `INSERT INTO guarded_single_tab_choices(request_id,candidate_token,origin,ordinal,active_at_request) VALUES($1,$2,$3,$4,$5)`, id, token, t.Origin, i+1, t.Active)
		if e != nil {
			return out, e
		}
		out.Candidates = append(out.Candidates, Candidate{Token: token, Origin: t.Origin, Ordinal: i + 1, Active: t.Active})
	}
	if e = c.finalCheck(ctx, tx, b, expiry); e != nil {
		return Selection{}, ErrDenied
	}
	out.ApprovalID, out.ExpiresAt = id, expiry
	if c.checkpoints != nil && c.checkpoints.SuspendSelectionTx(ctx, tx, b, out) != nil {
		return Selection{}, ErrDenied
	}
	if e = tx.Commit(ctx); e != nil {
		return Selection{}, e
	}
	out.ApprovalID = id
	out.ExpiresAt = expiry
	return out, nil
}
func actorChannel(ctx context.Context) string {
	a, _ := owneridentity.FromContext(ctx)
	return a.Channel
}

// lockSelection locks the complete sorted candidate set, then approval, then
// the owning request. It is called only after current ownership/source locks.
func (c *Controller) lockSelection(ctx context.Context, tx pgx.Tx, b resources.Binding, id string) (string, time.Time, error) {
	rows, e := tx.Query(ctx, `SELECT candidate_token FROM approval_tab_candidates WHERE approval_id=$1 ORDER BY candidate_token FOR UPDATE`, id)
	if e != nil {
		return "", time.Time{}, e
	}
	count := 0
	for rows.Next() {
		count++
	}
	e = rows.Err()
	rows.Close()
	if e != nil || count < 1 || count > MaxTabs {
		return "", time.Time{}, ErrDenied
	}
	var valid bool
	var approvalExpiry time.Time
	if tx.QueryRow(ctx, `SELECT run_id=$2 AND session_key=$3 AND approval_type='browser_tab_selection' AND tool_name=$4 AND status IN ('pending','approved'),expires_at FROM approvals WHERE approval_id=$1 FOR UPDATE`, id, b.RunID, b.SessionKey, BindTool).Scan(&valid, &approvalExpiry) != nil || !valid {
		return "", time.Time{}, ErrDenied
	}
	var state string
	var expiry time.Time
	if tx.QueryRow(ctx, `SELECT state,expires_at,owner_id=$2 AND run_id=$3 AND session_key=$4 AND task_id=$5 AND task_revision=$6 AND constraints_revision=$7 AND browser_instance=$8 FROM guarded_single_tab_requests WHERE approval_id=$1 FOR UPDATE`, id, b.OwnerID, b.RunID, b.SessionKey, b.TaskID, b.TaskRevision, b.ConstraintsRevision, c.host).Scan(&state, &expiry, &valid) != nil || !valid {
		return "", time.Time{}, ErrDenied
	}
	if approvalExpiry.Before(expiry) {
		expiry = approvalExpiry
	}
	var now time.Time
	if tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now) != nil || !expiry.After(now) || !b.Deadline.After(now) {
		return "", time.Time{}, ErrDenied
	}
	return state, expiry, nil
}

// Select revalidates the actual selected native tab outside PG locks and then
// atomically approves exactly one candidate and creates the guarded binding.
// Native metadata is not approval and can never invoke a browser action.
func (c *Controller) Select(ctx context.Context, b resources.Binding, id, token string) (string, error) {
	if c.preflight(ctx, b) != nil {
		return "", ErrDenied
	}
	var tab Tab
	e := c.pool.QueryRow(ctx, `SELECT q.browser_instance,p.internal_tab_ref,p.current_url FROM guarded_single_tab_requests q JOIN approval_tab_candidates p ON p.approval_id=q.approval_id WHERE q.approval_id=$1 AND p.candidate_token=$2 AND q.owner_id=$3 AND q.run_id=$4 AND q.session_key=$5`, id, token, b.OwnerID, b.RunID, b.SessionKey).Scan(&tab.BrowserInstance, &tab.Ref, &tab.Origin)
	if e != nil || !validTab(tab, c.host) || c.metadata.Check(ctx, tab) != nil {
		return "", ErrDenied
	}
	tx, e := c.begin(ctx, b)
	if e != nil {
		return "", e
	}
	defer tx.Rollback(context.Background())
	state, expiry, e := c.lockSelection(ctx, tx, b, id)
	if e != nil {
		return "", e
	}
	var currentRef, currentURL, candidateState string
	if tx.QueryRow(ctx, `SELECT internal_tab_ref,current_url,status FROM approval_tab_candidates WHERE approval_id=$1 AND candidate_token=$2`, id, token).Scan(&currentRef, &currentURL, &candidateState) != nil || currentRef != tab.Ref || currentURL != tab.Origin {
		return "", ErrDenied
	}
	if state == "selected" {
		var existing string
		if candidateState != "selected" || tx.QueryRow(ctx, `SELECT session_id FROM guarded_single_tab_selections WHERE request_id=$1 AND candidate_token=$2`, id, token).Scan(&existing) != nil {
			return "", ErrDenied
		}
		selected, e := readSelected(ctx, tx, b, existing, c.host)
		if e != nil {
			return "", e
		}
		if selected.ExpiresAt.Before(expiry) {
			expiry = selected.ExpiresAt
		}
		if e = c.finalCheck(ctx, tx, b, expiry); e != nil {
			return "", ErrDenied
		}
		return existing, tx.Commit(ctx)
	}
	if state != "pending" || candidateState != "available" {
		return "", ErrDenied
	}
	var busy bool
	if tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM single_tab_sessions WHERE session_key=$1 AND status='ACTIVE')`, b.SessionKey).Scan(&busy) != nil || busy {
		return "", ErrDenied
	}
	sid, e := randomID()
	if e != nil {
		return "", e
	}
	_, e = tx.Exec(ctx, `UPDATE approval_tab_candidates SET status=CASE WHEN candidate_token=$2 THEN 'selected' ELSE 'cancelled' END,selected_at=CASE WHEN candidate_token=$2 THEN clock_timestamp() ELSE NULL END WHERE approval_id=$1`, id, token)
	if e != nil {
		return "", e
	}
	a, _ := owneridentity.FromContext(ctx)
	_, e = tx.Exec(ctx, `UPDATE approvals SET status='approved',resolved_via=$2,resolved_by=$3,resolution_reason='guarded tab selected by authenticated owner',resolved_at=clock_timestamp(),updated_at=clock_timestamp() WHERE approval_id=$1`, id, a.Channel, a.OwnerID)
	if e != nil {
		return "", e
	}
	_, e = tx.Exec(ctx, `INSERT INTO single_tab_sessions(single_tab_session_id,session_key,created_by_run_id,approval_id,status,bound_tab_ref,browser_instance_id,selected_via,selected_by,current_url,status_reason,created_at,activated_at,expires_at,updated_at) VALUES($1,$2,$3,$4,'ACTIVE',$5,$6,$7,$8,$9,'guarded owner selection',clock_timestamp(),clock_timestamp(),$10,clock_timestamp())`, sid, b.SessionKey, b.RunID, id, tab.Ref, c.host, a.Channel, a.OwnerID, tab.Origin, expiry)
	if e != nil {
		return "", e
	}
	_, e = tx.Exec(ctx, `INSERT INTO guarded_single_tab_selections(session_id,request_id,candidate_token,tab_ref,origin,created_at) VALUES($1,$2,$3,$4,$5,clock_timestamp())`, sid, id, token, tab.Ref, tab.Origin)
	if e != nil {
		return "", e
	}
	_, e = tx.Exec(ctx, `UPDATE guarded_single_tab_requests SET state='selected' WHERE request_id=$1`, id)
	if e != nil {
		return "", e
	}
	selected, e := readSelected(ctx, tx, b, sid, c.host)
	if e != nil {
		return "", e
	}
	if selected.ExpiresAt.Before(expiry) {
		expiry = selected.ExpiresAt
	}
	if e = c.finalCheck(ctx, tx, b, expiry); e != nil {
		return "", ErrDenied
	}
	if c.checkpoints != nil && c.checkpoints.ResolveSelectionTx(ctx, tx, b, id, false) != nil {
		return "", ErrDenied
	}
	if e = c.finalCheck(ctx, tx, b, expiry); e != nil {
		return "", ErrDenied
	}
	if e = tx.Commit(ctx); e != nil {
		return "", e
	}
	return sid, nil
}

// Revoke is current owner control, not external-effect reconciliation. It does
// not clear the browser's uncertain persistent lane or replay any prior action.
func (c *Controller) Revoke(ctx context.Context, b resources.Binding, id string) error {
	tx, e := c.begin(ctx, b)
	if e != nil {
		return e
	}
	defer tx.Rollback(context.Background())
	state, expiry, e := c.lockSelection(ctx, tx, b, id)
	if e != nil {
		return e
	}
	if state != "pending" && state != "selected" {
		return ErrDenied
	}
	_, e = tx.Exec(ctx, `UPDATE single_tab_sessions SET status='REVOKED_BY_USER',released_at=clock_timestamp(),updated_at=clock_timestamp() WHERE approval_id=$1`, id)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `UPDATE guarded_single_tab_requests SET state='revoked' WHERE approval_id=$1`, id)
	if e != nil {
		return e
	}
	if e = c.finalCheck(ctx, tx, b, expiry); e != nil {
		return ErrDenied
	}
	if c.checkpoints != nil && c.checkpoints.ResolveSelectionTx(ctx, tx, b, id, true) != nil {
		return ErrDenied
	}
	if e = c.finalCheck(ctx, tx, b, expiry); e != nil {
		return ErrDenied
	}
	return tx.Commit(ctx)
}
