// Package utilitytrace connects the prospective utility scorer to a closed,
// owning Core run and permanent Broker evidence. It never executes a tool/model.
package utilitytrace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/butler/butler/internal/acceptance"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/readcontrol"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SourceBinding struct {
	UtilityID string
	Pin       catalog.SourceRef
}

// Freeze before execution. RunID is selected separately through its owning input.
type Binding struct {
	SuiteSHA256, CaseID, Arm, TaskID, SessionKey, OwnerID string
	TaskRevision, ConstraintsRevision                     int64
	OriginalFilesSHA256                                   string
	Sources                                               []SourceBinding
}
type Snapshot struct {
	RunID                            string
	Tables                           map[string]json.RawMessage
	AuthorityCurrent, SourcesCurrent bool
	RecipientFiles                   map[string]string
}

// Capture queries every owning call/intent/dispatch, including failed ones, in a
// single repeatable-read transaction. No secret/settings/auth tables are read.
func Capture(ctx context.Context, pool *pgxpool.Pool, cat *catalog.Store, b Binding, run string, recipientFiles map[string]string) (Snapshot, error) {
	s := Snapshot{RunID: run, Tables: map[string]json.RawMessage{}, RecipientFiles: recipientFiles}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return s, err
	}
	defer tx.Rollback(ctx)
	queries := map[string]string{
		"tasks":          `SELECT to_jsonb(t) v FROM tasks t WHERE task_id=$2`,
		"runs":           `SELECT to_jsonb(t) v FROM runs t WHERE run_id=$1`,
		"inputs":         `SELECT to_jsonb(t) v FROM accepted_inputs t WHERE task_id=$2`,
		"calls":          `SELECT to_jsonb(t) v FROM tool_calls t WHERE run_id=$1`,
		"intents":        `SELECT to_jsonb(t) v FROM action_intents t WHERE run_id=$1`,
		"effects":        `SELECT to_jsonb(t) v FROM action_effects t WHERE action_id IN(SELECT effect_id FROM action_intents WHERE run_id=$1)`,
		"evidence":       `SELECT to_jsonb(t) v FROM action_effect_evidence t WHERE source_action_id IN(SELECT action_id FROM action_intents WHERE run_id=$1)`,
		"dispatches":     `SELECT to_jsonb(t) v FROM action_effect_dispatches t WHERE execution_action_id IN(SELECT action_id FROM action_intents WHERE run_id=$1)`,
		"finals":         `SELECT to_jsonb(t) v FROM run_final_results t WHERE run_id=$1`,
		"pins":           `SELECT to_jsonb(t) v FROM memory_run_sources t WHERE run_id=$1`,
		"source_heads":   `SELECT jsonb_build_object('source_id',s.source_id,'revision',s.head_revision,'access_revision',s.access_revision,'content_hash',r.content_hash,'body',r.body,'status',s.status) v FROM memory_sources s JOIN memory_source_revisions r ON r.source_id=s.source_id AND r.revision=s.head_revision WHERE s.source_id IN(SELECT source_id FROM memory_run_sources WHERE run_id=$1)`,
		"model_attempts": `SELECT to_jsonb(t) v FROM execution_model_attempts t WHERE run_id=$1`,
	}
	for name, q := range queries {
		// Bind both selector arguments even in queries that use only one.
		q = `WITH selectors AS (SELECT $1::text AS run,$2::text AS task) SELECT COALESCE(jsonb_agg(v),'[]'::jsonb) FROM (` + q + `) rows`
		var raw json.RawMessage
		if err = tx.QueryRow(ctx, q, run, b.TaskID).Scan(&raw); err != nil {
			return s, fmt.Errorf("capture %s: %w", name, err)
		}
		if len(raw) > 32<<20 {
			return s, errors.New("bounded run evidence exceeded")
		}
		s.Tables[name] = raw
	}
	_, err = actionauth.CheckTaskAuthorityTx(ctx, tx, b.TaskID, b.OwnerID, nil, time.Now().UTC())
	s.AuthorityCurrent = err == nil
	if err == nil {
		err = cat.CheckRunSourcesTx(ctx, tx, run)
	}
	// A closed-run observer must not forge/reuse a live lease token. Verify
	// current captured grants read-only, alongside Catalog's source graph fence.
	grantsCurrent := false
	if err == nil {
		err = tx.QueryRow(ctx, `SELECT NOT EXISTS(
 SELECT 1 FROM memory_run_sources p JOIN memory_sources s USING(source_id)
 WHERE p.run_id=$1 AND NOT (s.owner_id=$2 AND (s.scope_type='task' AND s.scope_id=$3 OR s.scope_type='session' AND s.scope_id=$4 OR EXISTS(
 SELECT 1 FROM memory_scope_grants g WHERE g.task_id=$3 AND g.source_id=s.source_id AND g.owner_id=$2 AND g.task_revision=$5 AND g.access_revision=s.access_revision AND g.purpose='memory_read' AND COALESCE((to_jsonb(g)->>'expires_at')::timestamptz,'infinity'::timestamptz)>clock_timestamp()))))`, run, b.OwnerID, b.TaskID, b.SessionKey, b.TaskRevision).Scan(&grantsCurrent)
	}
	s.SourcesCurrent = err == nil && grantsCurrent
	// Guard rejection is retained; it must never become an eligible observation.
	return s, nil
}

type taskRow struct {
	ID          string `json:"task_id"`
	Session     string `json:"session_key"`
	Owner       string `json:"owner_id"`
	Revision    int64  `json:"revision"`
	Constraints int64  `json:"constraints_revision"`
	LastRun     string `json:"last_run_id"`
}
type runRow struct {
	ID       string     `json:"run_id"`
	Session  string     `json:"session_key"`
	State    string     `json:"current_state"`
	Status   string     `json:"status"`
	Finished *time.Time `json:"finished_at"`
}
type inputRow struct {
	ID       string             `json:"input_id"`
	Run      string             `json:"run_id"`
	Task     string             `json:"task_id"`
	Session  string             `json:"session_key"`
	Owner    string             `json:"owner_id"`
	Status   string             `json:"status"`
	Attempts int                `json:"attempts"`
	Event    ingress.InputEvent `json:"event_json"`
}
type callRow struct {
	ID       string          `json:"tool_call_id"`
	Run      string          `json:"run_id"`
	Name     string          `json:"tool_name"`
	Status   string          `json:"status"`
	Args     json.RawMessage `json:"args"`
	Result   json.RawMessage `json:"result"`
	Finished *time.Time      `json:"finished_at"`
}
type intentRow struct {
	ID      string                   `json:"action_id"`
	Run     string                   `json:"run_id"`
	Call    string                   `json:"tool_call_id"`
	Effect  string                   `json:"effect_id"`
	Binding actionauth.ActionBinding `json:"binding"`
}
type effectRow struct {
	ID    string `json:"action_id"`
	State string `json:"state"`
}
type dispatchRow struct {
	Action   string     `json:"execution_action_id"`
	Effect   string     `json:"action_id"`
	Outcome  string     `json:"outcome"`
	Finished *time.Time `json:"finished_at"`
}
type evidenceRow struct {
	Action   string                    `json:"source_action_id"`
	Effect   string                    `json:"action_id"`
	Evidence actionauth.EffectEvidence `json:"evidence"`
}
type finalRow struct {
	Run         string `json:"run_id"`
	Task        string `json:"task_id"`
	Session     string `json:"session_key"`
	Revision    int64  `json:"task_revision"`
	Constraints int64  `json:"constraints_revision"`
	Content     string `json:"content"`
	Hash        string `json:"content_hash"`
	State       string `json:"answer_state"`
}
type pinRow struct {
	ID       string `json:"source_id"`
	Revision int64  `json:"source_revision"`
	Access   int64  `json:"access_revision"`
	Hash     string `json:"content_hash"`
}
type headRow struct {
	ID       string `json:"source_id"`
	Revision int64  `json:"revision"`
	Access   int64  `json:"access_revision"`
	Hash     string `json:"content_hash"`
	Body     string `json:"body"`
	Status   string `json:"status"`
}

func rows[T any](s Snapshot, key string) ([]T, error) {
	var v []T
	raw, ok := s.Tables[key]
	if !ok || json.Unmarshal(raw, &v) != nil || v == nil {
		return nil, fmt.Errorf("missing or invalid %s", key)
	}
	return v, nil
}
func sha(v string) string { h := sha256.Sum256([]byte(v)); return hex.EncodeToString(h[:]) }

// Project verifies a sealed original snapshot before deriving scorer fields.
// The seal must come from the collector's original artifact, not an answer.
func Project(suite acceptance.UtilitySuite, b Binding, s Snapshot, seal string) (acceptance.UtilityObservation, error) {
	o := acceptance.UtilityObservation{CaseID: b.CaseID, Arm: b.Arm, SuiteSHA256: b.SuiteSHA256, Reads: []acceptance.UtilityRead{}, Final: json.RawMessage(`null`), Receipt: acceptance.UtilityReceipt{Effect: "none", Check: "not_checked"}}
	deny := func(v string) (acceptance.UtilityObservation, error) { return o, errors.New(v) }
	if suite.Validate() != nil || acceptance.MemoryDigest(suite) != b.SuiteSHA256 || acceptance.MemoryDigest(s) != seal {
		return deny("suite or original trace seal mismatch")
	}
	var c *acceptance.UtilityCase
	for n := range suite.Cases {
		if suite.Cases[n].ID == b.CaseID {
			c = &suite.Cases[n]
		}
	}
	if c == nil {
		return deny("case missing")
	}
	nativeIDs, utilityIDs := map[string]bool{}, map[string]bool{}
	for _, mapping := range b.Sources {
		if mapping.Pin.ID == "" || mapping.UtilityID == "" || nativeIDs[mapping.Pin.ID] || utilityIDs[mapping.UtilityID] {
			return deny("source mapping is not one-to-one")
		}
		nativeIDs[mapping.Pin.ID], utilityIDs[mapping.UtilityID] = true, true
	}
	o.InputSHA256 = acceptance.MemoryDigest(c.Input())
	tasks, e := rows[taskRow](s, "tasks")
	if e != nil || len(tasks) != 1 {
		return deny("one current task required")
	}
	t := tasks[0]
	if t.ID != b.TaskID || t.Session != b.SessionKey || t.Owner != b.OwnerID || t.Revision != b.TaskRevision || t.Constraints != b.ConstraintsRevision || t.LastRun != s.RunID {
		return deny("task/run revisions changed")
	}
	runs, e := rows[runRow](s, "runs")
	if e != nil || len(runs) != 1 || runs[0].ID != s.RunID || runs[0].Session != b.SessionKey || runs[0].Finished == nil {
		return deny("closed owning run required")
	}
	inputs, e := rows[inputRow](s, "inputs")
	if e != nil || len(inputs) != 1 {
		return deny("ambiguous owning input")
	}
	in := inputs[0]
	if in.Run != s.RunID || in.Task != b.TaskID || in.Owner != b.OwnerID || in.Session != b.SessionKey || in.Attempts != 1 {
		return deny("foreign or retried input")
	}
	var payload struct {
		Text   string `json:"text"`
		Task   string `json:"task_id"`
		Owner  string `json:"user_id"`
		Intent string `json:"intent"`
	}
	if acceptance.DecodeStrict([]byte(in.Event.PayloadJSON), &payload, "text", "task_id", "user_id", "intent") != nil || payload.Task != b.TaskID || payload.Owner != b.OwnerID || payload.Intent != "conversation" {
		return deny("owner payload missing")
	}
	var projected acceptance.UtilityInput
	if acceptance.DecodeStrict([]byte(payload.Text), &projected, "case_id", "instruction", "question", "current", "current_document_ids") != nil || acceptance.MemoryDigest(projected) != o.InputSHA256 {
		return deny("actual input differs from frozen projection")
	}
	calls, e := rows[callRow](s, "calls")
	if e != nil {
		return deny(e.Error())
	}
	intents, e := rows[intentRow](s, "intents")
	if e != nil || len(intents) != len(calls) {
		return deny("call/intent population mismatch")
	}
	effects, e := rows[effectRow](s, "effects")
	if e != nil || len(effects) != len(calls) {
		return deny("effect population mismatch")
	}
	dispatches, e := rows[dispatchRow](s, "dispatches")
	if e != nil || len(dispatches) != len(calls) {
		return deny("dispatch population mismatch")
	}
	evidence, e := rows[evidenceRow](s, "evidence")
	if e != nil || len(evidence) != len(calls) {
		return deny("evidence population mismatch")
	}
	pins, e := rows[pinRow](s, "pins")
	if e != nil {
		return deny(e.Error())
	}
	heads, e := rows[headRow](s, "source_heads")
	if e != nil {
		return deny(e.Error())
	}
	if _, e = rows[json.RawMessage](s, "model_attempts"); e != nil {
		return deny(e.Error())
	}
	seen := map[string]bool{}
	prepared, checked := "", ""
	inspectSeen := false
	allCompleted := true
	for _, call := range calls {
		if call.Run != s.RunID || call.ID == "" || seen[call.ID] || call.Finished == nil {
			return deny("foreign/duplicate/unfinished tool call")
		}
		seen[call.ID] = true
		var intent *intentRow
		for n := range intents {
			if intents[n].Call == call.ID {
				if intent != nil {
					return deny("ambiguous intent")
				}
				intent = &intents[n]
			}
		}
		if intent == nil {
			return deny("intent missing")
		}
		a := intent.Binding
		ah, err := actionauth.ArgumentsHash(string(call.Args))
		if err != nil || a.ArgsHash != ah || a.RunID != s.RunID || intent.Run != s.RunID || a.ToolCallID != call.ID || a.ToolName != call.Name || a.TaskID != b.TaskID || a.TaskRevision != b.TaskRevision || a.ConstraintsRevision != b.ConstraintsRevision || a.OwnerID != b.OwnerID || a.ActionID != intent.ID {
			return deny("permanent action binding mismatch")
		}
		state := ""
		for _, v := range effects {
			if v.ID == intent.Effect {
				if state != "" {
					return deny("ambiguous effect")
				}
				state = v.State
			}
		}
		if state == "" {
			return deny("effect missing")
		}
		nd, err := budget.NormalizeResult(string(call.Result))
		if err != nil {
			return deny("invalid tool result")
		}
		found := 0
		for _, v := range evidence {
			ev := v.Evidence
			if v.Action == intent.ID && v.Effect == intent.Effect && ev.SourceActionID == intent.ID && ev.SourceRunID == s.RunID && ev.SourceToolCallID == call.ID && ev.NormalizedResultDigest == nd && ev.Source == "runtime_response" && string(ev.Verdict) == state {
				found++
			}
		}
		if found != 1 {
			return deny("result not corroborated by Broker evidence")
		}
		found = 0
		for _, v := range dispatches {
			wantOutcome := "unknown"
			if state == "effect_confirmed" && call.Status == "completed" {
				wantOutcome = "dispatch_completed"
			}
			if v.Action == intent.ID && v.Effect == intent.Effect && v.Finished != nil && v.Outcome == wantOutcome {
				found++
			}
		}
		if found != 1 {
			return deny("permanent dispatch missing")
		}
		if call.Name == "extension.project.prepare-patch" {
			o.Receipt.PrepareCalls++
		}
		if call.Status != "completed" || state != "effect_confirmed" {
			allCompleted = false
			if call.Name == "extension.project.prepare-patch" {
				o.Receipt.Effect = "unknown"
			}
			continue
		}
		switch call.Name {
		case readcontrol.Search, readcontrol.Open:
			data, err := readcontrol.DecodeResult(call.Result)
			if err != nil {
				return deny("invalid actual memory result")
			}
			if data.Status != "ok" && data.Status != "empty" {
				return deny("memory did not produce usable evidence")
			}
			for _, item := range data.Items {
				var sb *SourceBinding
				for n := range b.Sources {
					if b.Sources[n].Pin.ID == item.Source.ID {
						if sb != nil {
							return deny("duplicate source mapping")
						}
						sb = &b.Sources[n]
					}
				}
				if sb == nil {
					return deny("unmapped exposed source")
				}
				var src *acceptance.UtilitySource
				for n := range c.Sources {
					if c.Sources[n].Source.ID == sb.UtilityID {
						src = &c.Sources[n]
					}
				}
				if src == nil || !c.SourceAllowed(*src, b.Arm) || item.Source.Revision != sb.Pin.Revision || item.Source.AccessRevision != sb.Pin.AccessRevision || item.Source.Hash != sb.Pin.Hash {
					return deny("source pin or access mismatch")
				}
				headOK := false
				for _, h := range heads {
					headOK = headOK || (h.ID == item.Source.ID && h.Revision == sb.Pin.Revision && h.Access == sb.Pin.AccessRevision && h.Hash == sb.Pin.Hash && h.Status == "active" && h.Body == src.Source.Body)
				}
				if !headOK {
					return deny("current source content mismatch")
				}
				stage := "candidate"
				if call.Name == readcontrol.Open {
					pinOK := false
					for _, p := range pins {
						pinOK = pinOK || (p.ID == item.Source.ID && p.Revision == sb.Pin.Revision && p.Access == sb.Pin.AccessRevision && p.Hash == sb.Pin.Hash)
					}
					if !pinOK || item.Truncated || item.Redacted || item.Text != src.Source.Body {
						return deny("partial or unpinned source read")
					}
					stage = "opened"
				}
				o.Reads = append(o.Reads, acceptance.UtilityRead{Ref: src.Ref(), Stage: stage})
			}
		case "extension.project.inspect", "extension.project.prepare-patch", "extension.project.check":
			var env struct {
				Status int    `json:"status_code"`
				Body   string `json:"body"`
			}
			wantStatus := 200
			if call.Name == "extension.project.prepare-patch" {
				wantStatus = 201
			}
			if json.Unmarshal(call.Result, &env) != nil || env.Status != wantStatus {
				return deny("recipient response invalid")
			}
			var body struct {
				Artifact string `json:"artifact_id"`
				Passed   *bool  `json:"checks_passed"`
				Applied  bool   `json:"applied"`
				Project  string `json:"project_ref"`
			}
			if json.Unmarshal([]byte(env.Body), &body) != nil {
				return deny("recipient body invalid")
			}
			if body.Applied {
				return deny("unexpected application")
			}
			switch call.Name {
			case "extension.project.inspect":
				if body.Passed == nil || *body.Passed || body.Project != c.Resource {
					return deny("current inspect missing")
				}
				inspectSeen = true
			case "extension.project.prepare-patch":
				if body.Artifact == "" || prepared != "" {
					return deny("one current prepared artifact required")
				}
				prepared = body.Artifact
			case "extension.project.check":
				if body.Artifact == "" || body.Passed == nil || !*body.Passed {
					return deny("current check failed")
				}
				checked = body.Artifact
			}
		default:
			return deny("unqualified tool in run")
		}
	}
	if prepared != "" {
		if prepared != checked || !inspectSeen {
			return deny("prepare/check correlation missing")
		}
		o.Receipt.Effect = "confirmed"
		o.Receipt.Artifact = prepared
		o.Receipt.Check = "passed"
	} else if c.Condition == "inspect_only" {
		if !inspectSeen {
			return deny("inspection absent")
		}
		o.Receipt.Check = "failed"
	}
	finals, e := rows[finalRow](s, "finals")
	if e != nil {
		return deny(e.Error())
	}
	if len(finals) == 1 {
		f := finals[0]
		if f.Run != s.RunID || f.Task != b.TaskID || f.Session != b.SessionKey || f.Revision != b.TaskRevision || f.Constraints != b.ConstraintsRevision || sha(f.Content) != f.Hash || f.State != "final" {
			return deny("final identity/content mismatch")
		}
		o.Final = json.RawMessage(f.Content)
	} else if len(finals) != 0 {
		return deny("ambiguous final")
	}
	o.Completed = allCompleted && len(finals) == 1 && in.Status == "completed" && (runs[0].State == "completed" || runs[0].State == "RUN_STATE_COMPLETED")
	o.AuthorityCurrent = s.AuthorityCurrent && s.SourcesCurrent
	o.OriginalUnchanged = acceptance.MemoryDigest(s.RecipientFiles) == b.OriginalFilesSHA256
	o.EvidenceComplete = true
	// Translate only a previously frozen source identity. The original final
	// remains in the sealed snapshot; values/effects/prose are never repaired.
	var typed acceptance.UtilityFinal
	if acceptance.DecodeStrict(o.Final, &typed, "outcome", "value", "sources", "effect", "artifact", "check") == nil {
		for n, ref := range typed.Sources {
			mapped := false
			for _, sb := range b.Sources {
				if ref.ID == sb.Pin.ID && ref.Revision == sb.Pin.Revision && ref.SHA256 == sb.Pin.Hash {
					for _, src := range c.Sources {
						if src.Source.ID == sb.UtilityID {
							typed.Sources[n] = src.Ref()
							mapped = true
						}
					}
				}
			}
			if !mapped {
				return deny("final cites an unmapped actual source")
			}
		}
		o.Final, _ = json.Marshal(typed)
	}
	// Incompatible legacy prose is retained as raw bytes and rejected by the new
	// typed scorer; it is never translated into a convenient successful answer.
	if !json.Valid(o.Final) {
		o.Final = json.RawMessage(strconvQuote(string(o.Final)))
	}
	return o, nil
}
func strconvQuote(s string) string { b, _ := json.Marshal(s); return string(b) }
