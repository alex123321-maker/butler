package browsercontrol

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/browserpage"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/jackc/pgx/v5"
)

func pageOrigin(raw string) string {
	u, e := url.Parse(raw)
	if e != nil || u.Hostname() == "" || u.User != nil || u.Scheme != "http" && u.Scheme != "https" {
		return ""
	}
	host := strings.ToLower(u.Host)
	if u.Scheme == "https" && u.Port() == "443" {
		host = strings.TrimSuffix(host, ":443")
	}
	if u.Scheme == "http" && u.Port() == "80" {
		host = strings.TrimSuffix(host, ":80")
	}
	return u.Scheme + "://" + host
}

func (s *Store) BrowserPageScope(ctx context.Context, b actionauth.ActionBinding, a browserpage.Args) (*browserpage.Binding, error) {
	p := PageContext{SessionID: a.SessionID, RunID: b.RunID, OwnerID: b.OwnerID, TaskID: b.TaskID}
	if s == nil || s.Pool == nil || b.Resource != "browser:"+a.SessionID+":" {
		return nil, ErrDenied
	}
	var mode bool
	var target string
	if s.Pool.QueryRow(ctx, `SELECT page_mode,epoch,observed_url FROM server_browser_sessions WHERE browser_session_id=$1 AND run_id=$2 AND owner_id=$3 AND task_id=$4 AND mode='agent' AND expires_at>clock_timestamp()`, p.SessionID, p.RunID, p.OwnerID, p.TaskID).Scan(&mode, &p.Epoch, &target) != nil || !mode {
		return nil, ErrDenied
	}
	tx, e := s.pageTx(ctx, p)
	if e != nil {
		return nil, ErrDenied
	}
	defer tx.Rollback(ctx)
	bound := &browserpage.Binding{URL: target, Epoch: p.Epoch}
	if browserpage.Bound(b.ToolName) {
		if a.Page == nil {
			return nil, ErrDenied
		}
		state, e := readPage(ctx, tx, p.SessionID, *a.Page)
		if e != nil || state.State != "ready" || state.Epoch != p.Epoch {
			return nil, ErrDenied
		}
		bound.Ref, bound.URL = state.PageRef, state.URL
		if tx.QueryRow(ctx, `SELECT runtime_version FROM server_browser_pages WHERE browser_session_id=$1 AND page_id=$2`, p.SessionID, state.ID).Scan(&bound.RuntimeVersion) != nil || bound.RuntimeVersion <= 0 {
			return nil, ErrDenied
		}
	}
	if commitPageTx(ctx, tx, p) != nil {
		return nil, ErrDenied
	}
	return bound, nil
}

func (s *Store) beforePage(ctx context.Context, call *toolbrokerv1.ToolCall, epoch int64) (context.Context, error) {
	a, e := browserpage.Parse(call.ToolName, call.ArgsJson)
	if e != nil {
		return ctx, ErrDenied
	}
	var owner, task string
	if s.Pool.QueryRow(ctx, `SELECT owner_id,task_id FROM server_browser_sessions WHERE browser_session_id=$1 AND run_id=$2 AND page_mode AND mode='agent' AND epoch=$3 AND expires_at>clock_timestamp()`, a.SessionID, call.RunId, epoch).Scan(&owner, &task) != nil {
		return ctx, ErrDenied
	}
	if !browserpage.Bound(call.ToolName) {
		return WithDispatchEpoch(ctx, epoch), nil
	}
	b, e := s.BrowserPageScope(ctx, actionauth.ActionBinding{ToolName: call.ToolName, Resource: "browser:" + a.SessionID + ":", RunID: call.RunId, OwnerID: owner, TaskID: task}, a)
	if e != nil || b.Epoch != epoch {
		return ctx, ErrDenied
	}
	return WithPageDispatch(WithDispatchEpoch(ctx, epoch), *b), nil
}

// recordPageExecution performs final current-authority/data-release checks and
// translates private executor counters to PostgreSQL-owned public versions.
// Any rejection scrubs all returned bytes. No effect is acknowledged here.
func (s *Store) recordPageExecution(ctx context.Context, call *toolbrokerv1.ToolCall, b actionauth.ActionBinding, result *toolbrokerv1.ToolResult) error {
	deny := func() error { result.ResultJson = "{}"; return ErrDenied }
	if s == nil || s.Pool == nil {
		return deny()
	}
	var raw map[string]json.RawMessage
	var report struct {
		ID       string               `json:"browser_session_id"`
		Expires  time.Time            `json:"expires_at"`
		OK       bool                 `json:"ok"`
		Pages    []browserpage.Report `json:"private_pages"`
		Selected string               `json:"selected_page_id"`
	}
	if json.Unmarshal([]byte(result.ResultJson), &report) != nil || !report.OK || !pageID(report.ID) || len(report.Pages) == 0 || len(report.Pages) > MaxLifetimePages || json.Unmarshal([]byte(result.ResultJson), &raw) != nil {
		return deny()
	}
	a, e := browserpage.Parse(call.ToolName, call.ArgsJson)
	if e != nil {
		return deny()
	}
	p := PageContext{SessionID: report.ID, RunID: b.RunID, OwnerID: b.OwnerID, TaskID: b.TaskID}
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return deny()
	}
	defer tx.Rollback(ctx)
	if b.TaskID != "" {
		var current bool
		if tx.QueryRow(ctx, `SELECT TRUE FROM runs r JOIN tasks t ON t.task_id=r.task_id WHERE r.run_id=$1 AND t.task_id=$2 AND t.owner_id=$3 AND r.task_revision=$4 AND t.revision=$4 AND t.constraints_revision=$5 AND t.status NOT IN ('cancelled','completed','paused','blocked') AND r.current_state IN ('tool_running','tool_pending','awaiting_approval','RUN_STATE_TOOL_RUNNING','RUN_STATE_TOOL_PENDING','RUN_STATE_AWAITING_APPROVAL') FOR SHARE OF r,t`, b.RunID, b.TaskID, b.OwnerID, b.TaskRevision, b.ConstraintsRevision).Scan(&current) != nil || !current {
			return deny()
		}
	}
	var origin string
	if call.ToolName == browserpage.Open {
		if !report.Expires.After(time.Now()) || report.Expires.After(time.Now().Add(16*time.Minute)) || len(report.Pages) != 1 || report.Pages[0].OpenerID != "" {
			return deny()
		}
		origin = pageOrigin(a.URL)
		target, e := pageURL(a.URL)
		if e != nil || origin == "" {
			return deny()
		}
		if _, e = tx.Exec(ctx, `INSERT INTO server_browser_sessions(browser_session_id,run_id,owner_id,task_id,expires_at,observed_url,page_mode,page_origin) VALUES($1,$2,$3,$4,$5,$6,true,$7)`, p.SessionID, p.RunID, p.OwnerID, p.TaskID, report.Expires, target, origin); e != nil {
			return deny()
		}
	} else {
		if a.SessionID != report.ID || b.BrowserPage == nil {
			return deny()
		}
		p.Epoch = b.BrowserPage.Epoch
		if tx.QueryRow(ctx, `SELECT page_origin FROM server_browser_sessions WHERE browser_session_id=$1 AND run_id=$2 AND owner_id=$3 AND task_id=$4 AND epoch=$5 AND page_mode AND mode='agent' AND expires_at>clock_timestamp() FOR UPDATE`, p.SessionID, p.RunID, p.OwnerID, p.TaskID, p.Epoch).Scan(&origin) != nil {
			return deny()
		}
		if a.Page != nil {
			old, e := readPage(ctx, tx, p.SessionID, *a.Page)
			if e != nil || old.Epoch != p.Epoch {
				return deny()
			}
		}
	}
	seen := map[string]bool{}
	views := []browserpage.View{}
	var selected *browserpage.Ref
	selectedURL := ""
	for _, r := range report.Pages {
		if !pageID(r.ID) || seen[r.ID] || r.OpenerID != "" && !pageID(r.OpenerID) {
			return deny()
		}
		seen[r.ID] = true
		if r.State != "ready" && r.State != "pending" && r.State != "closed" && r.State != "lost" {
			return deny()
		}
		clean := ""
		if r.State == "ready" {
			if !pageID(r.DocumentID) || r.RuntimeVersion <= 0 || pageOrigin(r.URL) != origin {
				return deny()
			}
			clean, e = pageURL(r.URL)
			if e != nil || clean != r.URL {
				return deny()
			}
			check := b
			copy := browserpage.Binding{Ref: browserpage.Ref{ID: r.ID, DocumentID: r.DocumentID}, URL: clean, Epoch: p.Epoch}
			check.BrowserPage = &copy
			var at time.Time
			if tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&at) != nil || actionauth.CheckTaskActionTx(ctx, tx, check, at) != nil {
				return deny()
			}
		} else if r.State == "pending" && (r.DocumentID != "" || r.RuntimeVersion != 0 || r.URL != "") {
			return deny()
		}
		var version, runtimeVersion, oldEpoch int64
		var oldDoc, oldURL, state, opener string
		e = tx.QueryRow(ctx, `SELECT observation_version,runtime_version,document_id,observed_url,state,COALESCE(opener_page_id,''),observed_epoch FROM server_browser_pages WHERE browser_session_id=$1 AND page_id=$2`, p.SessionID, r.ID).Scan(&version, &runtimeVersion, &oldDoc, &oldURL, &state, &opener, &oldEpoch)
		if e == pgx.ErrNoRows {
			var total, live int
			if tx.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE state IN ('pending','ready')) FROM server_browser_pages WHERE browser_session_id=$1`, p.SessionID).Scan(&total, &live) != nil || total >= MaxLifetimePages || live >= MaxLivePages || total > 0 && r.OpenerID == "" {
				return deny()
			}
			var parent any
			if r.OpenerID != "" {
				parent = r.OpenerID
			}
			if _, e = tx.Exec(ctx, `INSERT INTO server_browser_pages(browser_session_id,page_id,opener_page_id) VALUES($1,$2,$3)`, p.SessionID, r.ID, parent); e != nil {
				return deny()
			}
			state, opener, oldEpoch = "pending", r.OpenerID, -1
		} else if e != nil {
			return deny()
		}
		if opener != r.OpenerID {
			return deny()
		}
		if state == "closed" || state == "lost" {
			if state != r.State {
				return deny()
			}
			continue
		}
		changed := r.State != state || r.RuntimeVersion != runtimeVersion || r.DocumentID != oldDoc || clean != oldURL || r.State == "ready" && oldEpoch != p.Epoch
		if r.State == "ready" && (r.RuntimeVersion < runtimeVersion || r.RuntimeVersion == runtimeVersion && (r.DocumentID != oldDoc || clean != oldURL)) {
			return deny()
		}
		if changed {
			version++
			doc, target, epoch := oldDoc, oldURL, oldEpoch
			if r.State == "ready" {
				doc, target, epoch = r.DocumentID, clean, p.Epoch
			}
			if _, e = tx.Exec(ctx, `UPDATE server_browser_pages SET document_id=$3,observation_version=$4,observed_url=$5,observed_epoch=$6,state=$7,runtime_version=$8,updated_at=clock_timestamp() WHERE browser_session_id=$1 AND page_id=$2`, p.SessionID, r.ID, doc, version, target, epoch, r.State, r.RuntimeVersion); e != nil {
				return deny()
			}
		}
		if r.State == "ready" {
			ref := browserpage.Ref{ID: r.ID, DocumentID: r.DocumentID, Version: version}
			views = append(views, browserpage.View{Ref: ref, OpenerID: r.OpenerID, URL: clean, State: r.State})
			if r.ID == report.Selected {
				copy := ref
				selected = &copy
				selectedURL = clean
			}
		}
	}
	// Every known live page must be represented; omission cannot erase a
	// hidden window or turn another page's freshness into this one's proof.
	rows, e := tx.Query(ctx, `SELECT page_id FROM server_browser_pages WHERE browser_session_id=$1 AND state IN ('pending','ready')`, p.SessionID)
	if e != nil {
		return deny()
	}
	omitted := false
	for rows.Next() {
		var id string
		if rows.Scan(&id) != nil || !seen[id] {
			omitted = true
		}
	}
	rows.Close()
	if rows.Err() != nil || omitted {
		return deny()
	}
	if browserpage.Bound(call.ToolName) && call.ToolName != browserpage.Close && (selected == nil || selected.ID != a.Page.ID) {
		return deny()
	}
	var finalURL string
	if value, ok := raw["final_url"]; ok && (json.Unmarshal(value, &finalURL) != nil || selected == nil || finalURL != selectedURL) {
		return deny()
	}
	if call.ToolName == browserpage.List || call.ToolName == browserpage.Close {
		for _, key := range []string{"text", "title", "links", "forms"} {
			if _, ok := raw[key]; ok {
				return deny()
			}
		}
	}
	if _, e = tx.Exec(ctx, `UPDATE server_browser_sessions SET fresh_needed=EXISTS(SELECT 1 FROM server_browser_pages p WHERE p.browser_session_id=$1 AND p.state IN ('pending','ready') AND (p.state<>'ready' OR p.observed_epoch<>$2)) WHERE browser_session_id=$1`, p.SessionID, p.Epoch); e != nil {
		return deny()
	}
	if commitPageTx(ctx, tx, p) != nil {
		return deny()
	}
	delete(raw, "private_pages")
	delete(raw, "selected_page_id")
	delete(raw, "observation_version")
	encoded, _ := json.Marshal(views)
	raw["pages"] = encoded
	if selected != nil {
		encoded, _ = json.Marshal(selected)
		raw["page"] = encoded
	}
	encoded, _ = json.Marshal(raw)
	result.ResultJson = string(encoded)
	return nil
}
