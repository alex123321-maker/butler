package credentialcontrol

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/butler/butler/internal/budget"
	"github.com/jackc/pgx/v5"
)

// StoppedAccessIdentity comes from the journal's original terminal fences.
type StoppedAccessIdentity struct {
	OwnerID, SessionKey, RunID        string
	TaskRevision, ConstraintsRevision int64
}

// StoppedAccessAbsentTx is used only under the journal's original terminal
// session/run/Input locks. The pinned CreateRequest protocol takes those same
// run/session locks, atomically creates its request and audit, and has no other
// effect. No row can arrive late after this absence check. A same-call row in
// any run conservatively prevents this proof.
func StoppedAccessAbsentTx(ctx context.Context, tx pgx.Tx, call string) (bool, error) {
	var absent bool
	err := tx.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM credential_vault_requests WHERE tool_call_id=$1)`, call).Scan(&absent)
	return absent, err
}

// StoppedAccessReceiptTx verifies creation only, never permission, secret release,
// notice delivery or login. The caller must hold the original terminal run and
// Input fences. This deliberately excludes approved, OTP and notified requests.
// No ciphertext, PIN, unlock token or credential value is read.
func StoppedAccessReceiptTx(ctx context.Context, tx pgx.Tx, p StoppedAccessIdentity, call string) (argsJSON, receiptDigest string, err error) {
	var id, record, origin, browser, purpose, fields, expected string
	err = tx.QueryRow(ctx, `SELECT q.id,q.record_id,q.target_origin,q.browser_session_id,q.purpose,q.fields::text
 FROM credential_vault_requests q
 WHERE q.owner_id=$1 AND q.session_key=$2 AND q.run_id=$3 AND q.task_id=$4 AND q.task_revision=$5 AND q.constraints_revision=$6 AND q.tool_call_id=$7
 AND q.kind='access' AND q.state='expired' AND q.expires_at<=clock_timestamp() AND q.record_id<>'' AND q.record_revision>0 AND q.parent_request_id=''
 AND q.notice_state='pending' AND q.notice_ack='' AND q.notice_started_at IS NULL
 AND (SELECT count(*) FROM credential_vault_audit a WHERE a.request_id=q.id)=1
 AND EXISTS(SELECT 1 FROM credential_vault_audit a WHERE a.request_id=q.id AND a.owner_id=q.owner_id AND a.record_id=q.record_id AND a.operation='requested')
 FOR SHARE OF q`, p.OwnerID, p.SessionKey, p.RunID, "", p.TaskRevision, p.ConstraintsRevision, call).Scan(&id, &record, &origin, &browser, &purpose, &fields)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", nil
	}
	if err != nil {
		return "", "", err
	}
	expected, err = expiredRequestBrowserTx(ctx, tx, p, browser)
	if err != nil || expected == "" {
		return "", "", err
	}
	var names []string
	if json.Unmarshal([]byte(fields), &names) != nil || len(names) == 0 || len(names) > 2 {
		return "", "", nil
	}
	seen := map[string]bool{}
	for _, name := range names {
		if (name != "username" && name != "password") || seen[name] {
			return "", "", nil
		}
		seen[name] = true
	}
	u, parseErr := url.Parse(expected)
	if parseErr != nil || u.Hostname() == "" || u.User != nil || u.Opaque != "" || (u.Scheme != "https" && u.Scheme != "http") || strings.ToLower(u.Scheme+"://"+u.Host) != origin || purpose == "" || len(purpose) > 500 {
		return "", "", nil
	}
	raw, err := json.Marshal(map[string]any{"record_id": record, "browser_session_id": browser, "expected_url": expected, "purpose": purpose, "fields": names})
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256([]byte(id + "\x00" + string(raw) + "\x00expired_unapproved_access_request"))
	return string(raw), hex.EncodeToString(sum[:]), nil
}

// Expired session rows are normally pruned. Only a separately journaled,
// confirmed original open can replace that missing context; an existing foreign
// or live row cannot. This is provenance for metadata hashing, never a receipt
// for credential release or site acceptance.
func expiredRequestBrowserTx(ctx context.Context, tx pgx.Tx, p StoppedAccessIdentity, browser string) (string, error) {
	var expected string
	var expiry time.Time
	err := tx.QueryRow(ctx, `SELECT observed_url,expires_at FROM server_browser_sessions WHERE browser_session_id=$1 AND owner_id=$2 AND run_id=$3 AND mode='agent' FOR SHARE`, browser, p.OwnerID, p.RunID).Scan(&expected, &expiry)
	if err == nil {
		if expiry.After(time.Now()) {
			return "", nil
		}
		return expected, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	var exists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM server_browser_sessions WHERE browser_session_id=$1)`, browser).Scan(&exists); err != nil || exists {
		return "", err
	}
	var raw, digest string
	err = tx.QueryRow(ctx, `SELECT c.result::text,v.evidence->>'normalized_result_digest'
 FROM tool_calls c JOIN action_intents a ON a.run_id=c.run_id AND a.tool_call_id=c.tool_call_id
 JOIN action_effects e ON e.action_id=a.effect_id AND e.initial_binding=a.binding
 JOIN action_effect_evidence v ON v.action_id=e.action_id AND v.source_action_id=a.action_id
 WHERE c.run_id=$1 AND c.tool_name='browser.open' AND c.status='completed' AND c.result->>'browser_session_id'=$2 AND c.args->>'url'=c.result->>'final_url'
 AND e.state='effect_confirmed' AND e.initial_binding->>'owner_id'=$3 AND e.initial_binding->>'run_id'=$1 AND e.initial_binding->>'tool_call_id'=c.tool_call_id
 AND e.initial_binding->>'tool_name'='browser.open' AND e.initial_binding->>'contract_revision'='3' AND e.initial_binding->>'effect'='read' AND e.initial_binding->'credential_refs'='[]'::jsonb
 AND e.initial_binding->>'resource'=c.result->>'final_url' AND e.policy->>'retry_safety'='read_only' AND e.policy->>'verification_method'='read_result'
 AND v.evidence->>'source'='runtime_response' AND v.evidence->>'verdict'='effect_confirmed' AND v.evidence->>'source_run_id'=c.run_id AND v.evidence->>'source_tool_call_id'=c.tool_call_id
 ORDER BY c.finished_at LIMIT 1 FOR SHARE OF c,a,e,v`, p.RunID, browser, p.OwnerID).Scan(&raw, &digest)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var open struct {
		ID     string    `json:"browser_session_id"`
		URL    string    `json:"final_url"`
		Expiry time.Time `json:"expires_at"`
		OK     bool      `json:"ok"`
	}
	actual, err := budget.NormalizeResult(raw)
	if err != nil || actual != digest || json.Unmarshal([]byte(raw), &open) != nil || !open.OK || open.ID != browser || open.Expiry.IsZero() || open.Expiry.After(time.Now()) {
		return "", nil
	}
	return open.URL, nil
}
