package actionauth

import (
	"context"
	"encoding/json"
	"time"

	"github.com/butler/butler/internal/browserpage"
	"github.com/butler/butler/internal/budget"
	"github.com/jackc/pgx/v5"
)

// PageOutcomeSource validates identity only. It grants no observation, report,
// replay or execution authority. Acknowledged post-effect refs come from Broker.
func PageOutcomeSource(b ActionBinding, argsRaw, resultRaw string) (browserpage.Args, browserpage.Observation, error) {
	a, err := browserpage.Parse(b.ToolName, argsRaw)
	o, observeErr := browserpage.ReadObservation(resultRaw)
	hash, hashErr := ArgumentsHash(argsRaw)
	if err != nil || observeErr != nil || hashErr != nil || hash != b.ArgsHash || !browserpage.OutcomeGesture(b.ToolName) || b.ContractRevision != browserpage.Revision || b.Effect != EffectUnknown || len(b.CredentialRefs) != 0 || b.BrowserPage == nil || !b.BrowserPage.Valid(true) || a.Page == nil || *a.Page != b.BrowserPage.Ref || b.Resource != "browser:"+a.SessionID+":" || o.SessionID != a.SessionID || o.Page.ID != a.Page.ID || o.Page.Version <= a.Page.Version {
		return browserpage.Args{}, browserpage.Observation{}, ErrOwnerOutcome
	}
	return a, o, nil
}

// pageOutcomeReadTx joins immutable runtime evidence to both exact page
// observations and the current context fence. Titles, another page's frozen
// snapshot, a model receipt and a refreshed owner epoch cannot satisfy it.
func pageOutcomeReadTx(ctx context.Context, tx pgx.Tx, effectID string, b ActionBinding, argsRaw, resultRaw, readCall string, at time.Time) (time.Time, string, error) {
	a, post, err := PageOutcomeSource(b, argsRaw, resultRaw)
	if err != nil || readCall == "" {
		return time.Time{}, "", ErrOwnerOutcome
	}
	captured := func(effect, action, call, body, verdict string) bool {
		normalized, e := budget.NormalizeResult(body)
		if e != nil || normalized == "" {
			return false
		}
		var ok bool
		e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM action_effect_evidence WHERE action_id=$1 AND source_action_id=$2 AND evidence->>'source'='runtime_response' AND evidence->>'verdict'=$6 AND evidence->>'source_run_id'=$3 AND evidence->>'source_tool_call_id'=$4 AND evidence->>'normalized_result_digest'=$5)`, effect, action, b.RunID, call, normalized, verdict).Scan(&ok)
		return e == nil && ok
	}
	if !captured(effectID, b.ActionID, b.ToolCallID, resultRaw, string(StateUnknown)) {
		return time.Time{}, "", ErrOwnerOutcome
	}
	var rawBinding, rawArgs, proof, readEffect, readAction string
	err = tx.QueryRow(ctx, `SELECT a.binding::text,c.args::text,c.result::text,e.action_id,a.action_id FROM tool_calls c JOIN action_intents a ON a.run_id=c.run_id AND a.tool_call_id=c.tool_call_id JOIN action_effects e ON e.action_id=a.effect_id WHERE c.run_id=$1 AND c.tool_call_id=$2 AND c.tool_name=$3 AND c.status='completed' AND c.finished_at>=(SELECT finished_at FROM tool_calls WHERE run_id=$1 AND tool_call_id=$4) AND e.state='effect_confirmed' AND e.policy->>'retry_safety'='read_only' AND e.policy->>'verification_method'='read_result'`, b.RunID, readCall, browserpage.Freeze, b.ToolCallID).Scan(&rawBinding, &rawArgs, &proof, &readEffect, &readAction)
	var read ActionBinding
	q, parseErr := browserpage.Parse(browserpage.Freeze, rawArgs)
	view, viewErr := browserpage.ReadObservation(proof)
	argsHash, hashErr := ArgumentsHash(rawArgs)
	if err != nil || json.Unmarshal([]byte(rawBinding), &read) != nil || parseErr != nil || viewErr != nil || hashErr != nil {
		return time.Time{}, "", ErrOwnerOutcome
	}
	if read.ArgsHash != argsHash || read.ToolName != browserpage.Freeze || read.ContractRevision != browserpage.Revision || read.Effect != EffectRead || len(read.CredentialRefs) != 0 || read.RunID != b.RunID || read.ToolCallID != readCall || read.OwnerID != b.OwnerID || read.Resource != b.Resource {
		return time.Time{}, "", ErrOwnerOutcome
	}
	if read.TaskID != b.TaskID || read.TaskRevision != b.TaskRevision || read.ConstraintsRevision != b.ConstraintsRevision || read.GoalID != b.GoalID || read.GoalRevision != b.GoalRevision || read.MandateID != b.MandateID || read.MandateRevision != b.MandateRevision || read.OwnerPolicyRevision != b.OwnerPolicyRevision || read.Purpose != b.Purpose {
		return time.Time{}, "", ErrOwnerOutcome
	}
	if read.BrowserPage == nil || !read.BrowserPage.Valid(true) || q.SessionID != a.SessionID || *q.Page != *post.Page || read.BrowserPage.Ref != *post.Page || read.BrowserPage.URL != post.URL || read.BrowserPage.Epoch != b.BrowserPage.Epoch || read.BrowserPage.RuntimeVersion <= b.BrowserPage.RuntimeVersion {
		return time.Time{}, "", ErrOwnerOutcome
	}
	if view.SessionID != a.SessionID || view.Page.ID != post.Page.ID || view.Page.DocumentID != post.Page.DocumentID || view.Page.Version <= post.Page.Version || view.URL != post.URL || view.Network != "frozen" || !view.Expires.After(at) || !captured(readEffect, readAction, readCall, proof, string(StateConfirmed)) {
		return time.Time{}, "", ErrOwnerOutcome
	}
	// Lock Task before the context/page rows, matching final page release. The
	// owner report grants no execution; resumption independently rechecks policy.
	if b.TaskID != "" {
		var valid bool
		if tx.QueryRow(ctx, `SELECT TRUE FROM runs r JOIN tasks t ON t.task_id=r.task_id WHERE r.run_id=$1 AND t.task_id=$2 AND t.owner_id=$3 AND r.task_revision=$4 AND t.revision=$4 AND t.constraints_revision=$5 AND t.status IN ('active','waiting_for_user') FOR SHARE OF r,t`, b.RunID, b.TaskID, b.OwnerID, b.TaskRevision, b.ConstraintsRevision).Scan(&valid) != nil || !valid {
			return time.Time{}, "", ErrOwnerOutcome
		}
	}
	if CheckTaskActionTx(ctx, tx, b, at) != nil || CheckTaskActionTx(ctx, tx, read, at) != nil {
		return time.Time{}, "", ErrOwnerOutcome
	}
	var expires time.Time
	if tx.QueryRow(ctx, `SELECT s.expires_at FROM server_browser_sessions s JOIN server_browser_pages p USING(browser_session_id) WHERE s.browser_session_id=$1 AND s.run_id=$2 AND s.owner_id=$3 AND s.task_id=$4 AND s.page_mode AND s.mode='agent' AND s.epoch=$5 AND s.expires_at>clock_timestamp() AND p.page_id=$6 AND p.document_id=$7 AND p.observation_version=$8 AND p.observed_epoch=s.epoch AND p.state='ready' AND p.observed_url=$9 AND p.runtime_version>$10 FOR SHARE OF s,p`, a.SessionID, b.RunID, b.OwnerID, b.TaskID, b.BrowserPage.Epoch, view.Page.ID, view.Page.DocumentID, view.Page.Version, view.URL, read.BrowserPage.RuntimeVersion).Scan(&expires) != nil {
		return time.Time{}, "", ErrOwnerOutcome
	}
	return expires, post.URL + " [page " + post.Page.ID + "; document " + post.Page.DocumentID + "]", nil
}
