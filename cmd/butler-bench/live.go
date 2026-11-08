package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/butler/butler/internal/benchtrace"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Candidate struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Effort   string `json:"reasoning_effort,omitempty"`
}
type Turn struct {
	Durations      []durationSample `json:"step_durations,omitempty"`
	Started        time.Time        `json:"started_utc"`
	Ended          time.Time        `json:"observed_utc"`
	HTTPUS         *int64           `json:"ingress_request_us"`
	ObserverUS     int64            `json:"evidence_collection_us"`
	ObservedResult string           `json:"result_before_evidence_error,omitempty"`
	EvidenceSHA    string           `json:"evidence_manifest_sha256"`
	Event          string           `json:"event"`
	Input          string           `json:"input,omitempty"`
	Run            string           `json:"run,omitempty"`
	Status         string           `json:"input_state,omitempty"`
	ErrorCode      string           `json:"input_error_code,omitempty"`
	Reason         string           `json:"result"`
	LatencyMS      int64            `json:"elapsed_ms"`
	AnswerHash     string           `json:"answer_sha256,omitempty"`
	WebReady       bool             `json:"web_result_ready"`
	GuardedWait    bool             `json:"protected_tab_wait,omitempty"`
	Tools          []string         `json:"tool_calls,omitempty"`
	ModelCalls     *int             `json:"model_attempts"`
	InputTokens    *int64           `json:"input_tokens"`
	OutputTokens   *int64           `json:"output_tokens"`
	KnownInput     int64            `json:"known_input_tokens"`
	KnownOutput    int64            `json:"known_output_tokens"`
	MatchedModel   bool             `json:"matched_model"`
}

type liveClient struct {
	endpoint, token string
	http            *http.Client
	db              *pgxpool.Pool
	candidate       Candidate
	traceDir        string
}

func newClient(ctx context.Context, endpoint, tokenFile string, candidate Candidate) (*liveClient, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Path != "" && u.Path != "/" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return nil, errors.New("invalid Butler origin")
	}
	ip := net.ParseIP(u.Hostname())
	if u.Scheme != "https" && !(u.Scheme == "http" && (ip.IsLoopback() || u.Hostname() == "localhost")) {
		return nil, errors.New("use HTTPS or a local forwarded port")
	}
	secret, err := os.ReadFile(tokenFile)
	if err != nil {
		return nil, errors.New("owner token file unavailable")
	}
	token := strings.TrimSpace(string(secret))
	if len(token) < 32 || strings.ContainsAny(token, "\r\n") {
		return nil, errors.New("invalid owner token file")
	}
	dsn := os.Getenv("BUTLER_BENCH_DB")
	if dsn == "" {
		return nil, errors.New("set BUTLER_BENCH_DB to the disposable Butler database")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, errors.New("invalid benchmark database configuration")
	}
	cfg.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	cfg.MaxConns = 2
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, errors.New("database unavailable")
	}
	var ok bool
	// Validate actual SELECT access before the first model request.
	err = pool.QueryRow(ctx, `SELECT current_setting('transaction_read_only')='on'
        FROM (SELECT i.input_id,r.run_id,f.content,t.result,e.state,a.model,d.status
          FROM accepted_inputs i,runs r,run_final_results f,tool_calls t,action_effects e,execution_model_attempts a,delivery_outbox d,action_intents ai LIMIT 0) AS empty
        RIGHT JOIN (VALUES(1)) AS probe(n) ON true`).Scan(&ok)
	if err != nil || !ok {
		pool.Close()
		return nil, errors.New("read-only benchmark evidence tables unavailable")
	}
	conn, err := pool.Acquire(ctx)
	if err == nil {
		_, err = conn.Conn().Prepare(ctx, "", timelineSQL)
		conn.Release()
	}
	if err != nil {
		pool.Close()
		return nil, errors.New("benchmark timeline schema or SELECT permissions unavailable")
	}
	client := &http.Client{Timeout: 15 * time.Second, Transport: &http.Transport{Proxy: nil, MaxResponseHeaderBytes: 16384}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return &liveClient{endpoint: strings.TrimRight(endpoint, "/"), token: token, http: client, db: pool, candidate: candidate}, nil
}

func (c *liveClient) submit(ctx context.Context, session, event string, step Step) (string, error) {
	payload := map[string]string{"text": step.Prompt, "intent": "conversation"}
	if step.Task != "" {
		payload["task_id"] = step.Task
	}
	body, _ := json.Marshal(map[string]any{"event_id": event, "idempotency_key": event, "event_type": "user_message", "source": "web", "session_key": session, "payload": payload})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/api/v1/events", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("X-Butler-Delivery-ID", event)
	resp, err := c.http.Do(req)
	if err != nil {
		return "", errors.New("submission_unknown")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		return "", fmt.Errorf("ingress_http_%d", resp.StatusCode)
	}
	var receipt struct {
		Accepted bool
		ID       string `json:"input_id"`
		Session  string `json:"session_key"`
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 65537))
	if err != nil || len(raw) > 65536 || json.Unmarshal(raw, &receipt) != nil || !receipt.Accepted || receipt.ID == "" || receipt.Session != session {
		return "", errors.New("receipt_mismatch")
	}
	return receipt.ID, nil
}

type snapshot struct {
	guardedWait                                             bool
	status, run, errorCode, answer, answerState, answerHash string
	redacted, webReady                                      bool
	calls                                                   []call
	attempts                                                int
	matched                                                 bool
	input, output                                           *int64
	knownInput, knownOutput                                 int64
}

func (c *liveClient) observe(ctx context.Context, session, event, id, task string) (snapshot, error) {
	var s snapshot
	tx, err := c.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return s, err
	}
	defer tx.Rollback(ctx)
	err = tx.QueryRow(ctx, `SELECT i.status,COALESCE(i.run_id,''),i.error_code,
        COALESCE(f.content,''),COALESCE(f.answer_state,''),COALESCE(f.content_hash,''),COALESCE(f.privacy_redacted,false),
        EXISTS(SELECT 1 FROM delivery_outbox d WHERE d.result_id=f.result_id AND d.run_id=i.run_id AND d.session_key=i.session_key AND d.channel='web' AND d.status='result_ready')
        FROM accepted_inputs i LEFT JOIN runs r ON r.run_id=i.run_id AND r.session_key=i.session_key
        LEFT JOIN run_final_results f ON f.run_id=r.run_id AND f.session_key=r.session_key
        WHERE i.input_id=$1 AND i.event_id=$2 AND i.session_key=$3 AND i.source='web'
          AND ($4='' OR i.task_id=$4 AND (f.result_id IS NULL OR f.task_id=$4))
          AND (i.run_id IS NULL OR r.run_id IS NOT NULL)`, id, event, session, task).Scan(&s.status, &s.run, &s.errorCode, &s.answer, &s.answerState, &s.answerHash, &s.redacted, &s.webReady)
	if err != nil || s.run == "" {
		return s, err
	}
	err = tx.QueryRow(ctx, `SELECT count(*)::integer,COALESCE(bool_and(provider=$2 AND model=$3),false),
        CASE WHEN count(*)>0 AND count(actual_input_tokens)=count(*) THEN sum(actual_input_tokens)::bigint END,
        CASE WHEN count(*)>0 AND count(actual_output_tokens)=count(*) THEN sum(actual_output_tokens)::bigint END,
        COALESCE(sum(actual_input_tokens),0)::bigint,COALESCE(sum(actual_output_tokens),0)::bigint
        FROM execution_model_attempts WHERE run_id=$1`, s.run, c.candidate.Provider, c.candidate.Model).Scan(&s.attempts, &s.matched, &s.input, &s.output, &s.knownInput, &s.knownOutput)
	if err != nil {
		return s, err
	}
	// Include an admitted effect even if its transcript write was lost.
	rows, err := tx.Query(ctx, `WITH t AS (SELECT * FROM tool_calls WHERE run_id=$1),
        i AS (SELECT * FROM action_intents WHERE run_id=$1)
        SELECT COALESCE(t.tool_name,i.binding->>'tool_name',''),COALESCE(t.status,''),COALESCE(e.state,''),
               COALESCE(t.args,'{}'::jsonb),COALESCE(t.result,'{}'::jsonb)
        FROM t FULL JOIN i ON i.tool_call_id=t.tool_call_id AND i.binding->>'tool_name'=t.tool_name
        LEFT JOIN action_effects e ON e.action_id=i.effect_id
        ORDER BY COALESCE(t.started_at,i.created_at),COALESCE(t.tool_call_id,i.tool_call_id)`, s.run)
	if err != nil {
		return s, err
	}
	s.calls, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (call, error) {
		var v call
		err := r.Scan(&v.Name, &v.Status, &v.Effect, &v.Args, &v.Result)
		return v, err
	})
	if err != nil {
		return s, err
	}
	if s.status == "waiting_for_user" && task != "" {
		err = tx.QueryRow(ctx, `SELECT count(*)=1 FROM accepted_inputs i
			JOIN guarded_single_tab_requests g ON g.approval_id=i.checkpoint_json->>'approval_id'
			JOIN guarded_single_tab_dispatches d ON d.request_id=g.request_id AND d.run_id=g.run_id
			WHERE i.input_id=$1 AND i.run_id=g.run_id AND i.session_key=g.session_key AND i.owner_id=g.owner_id AND i.task_id=g.task_id
			AND i.checkpoint='awaiting_approval' AND i.checkpoint_json->'continuation'->>'kind'='guarded_tab_selection'
			AND i.checkpoint_json->'continuation'->>'approval_id'=g.approval_id AND i.checkpoint_json->'continuation'->>'version'='1' AND g.state='pending' AND g.expires_at>clock_timestamp()`, id).Scan(&s.guardedWait)
		if err != nil {
			return s, err
		}
	}
	return s, tx.Commit(ctx)
}

func (s snapshot) fill(t *Turn) {
	t.Run, t.Status, t.WebReady = s.run, s.status, s.webReady
	t.GuardedWait = s.guardedWait
	if safeCodeRE.MatchString(s.errorCode) {
		t.ErrorCode = s.errorCode
	}
	if s.run != "" {
		t.ModelCalls = &s.attempts
		t.MatchedModel, t.InputTokens, t.OutputTokens = s.matched, s.input, s.output
		t.KnownInput, t.KnownOutput = s.knownInput, s.knownOutput
	}
	t.Tools = nil
	for _, v := range s.calls {
		t.Tools = append(t.Tools, v.Name)
	}
	if s.answerHash != "" {
		t.AnswerHash = s.answerHash
	}
}

func verdict(s snapshot, step Step) string {
	if s.attempts > 0 && !s.matched {
		return "model_mismatch"
	}
	if step.Rule == "guarded_wait" && s.status == "waiting_for_user" && s.attempts > 0 && s.guardedWait && s.answerState == "" {
		for _, c := range s.calls {
			if c.Name != "tools.search" && c.Name != "tools.load" && c.Name != "single_tab.bind.guarded" {
				return "unexpected_tool"
			}
			if c.Effect == "executing" || c.Effect == "effect_unknown" {
				return "unresolved_effect"
			}
		}
		return "pass"
	}
	switch s.status {
	case "queued", "running":
		return ""
	case "waiting_for_user", "blocked", "failed", "expired", "cancelled":
		return "input_" + s.status
	case "completed":
		if s.attempts == 0 {
			return "model_unobserved"
		}
		hash := sha256.Sum256([]byte(s.answer))
		if s.answerState != "final" || s.redacted || s.answerHash != hex.EncodeToString(hash[:]) {
			return "final_evidence_missing"
		}
		if !s.webReady {
			return "web_result_missing"
		}
		return grade(step, s.answer, s.calls)
	default:
		return "unexpected_input_state"
	}
}

func (c *liveClient) turn(ctx context.Context, session, event string, step Step, dir string) (t Turn) {
	t.Event = event
	if err := benchtrace.Enroll(c.traceDir, session, event); err != nil {
		t.Reason = "trace_enrollment_failed"
		return
	}
	start := time.Now()
	t.Started = start.UTC()
	var grading *observation
	defer func() {
		end := time.Now()
		t.Ended = end.UTC()
		t.LatencyMS = end.Sub(start).Milliseconds()
		if grading != nil {
			if err := writeJSON(filepath.Join(dir, "grading.json"), grading); err != nil {
				t.ObservedResult = t.Reason
				t.Reason = "grading_write_failed"
			}
		}
		// A timed-out or cancelled run still needs its last available evidence.
		observe, stop := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		defer stop()
		if c.db != nil && t.Input == "" && (t.Reason == "submission_unknown" || t.Reason == "receipt_mismatch") {
			_ = c.db.QueryRow(observe, `SELECT input_id,COALESCE(run_id,'') FROM accepted_inputs WHERE event_id=$1 AND session_key=$2 AND source='web'`, event, session).Scan(&t.Input, &t.Run)
		}
		if err := c.capture(observe, dir, session, step, &t); err != nil {
			if t.ObservedResult == "" {
				t.ObservedResult = t.Reason
			}
			t.Reason = err.Error()
		}
		t.ObserverUS = time.Since(end).Microseconds()
	}()
	id, err := c.submit(ctx, session, event, step)
	requestUS := time.Since(start).Microseconds()
	t.HTTPUS = &requestUS
	if err != nil {
		t.Reason = err.Error()
		return
	}
	t.Input = id
	ticker := time.NewTicker(350 * time.Millisecond)
	defer ticker.Stop()
	for {
		s, err := c.observe(ctx, session, event, id, step.Task)
		if err != nil {
			if ctx.Err() != nil {
				t.Reason = "timeout"
			} else {
				t.Reason = "evidence_unavailable"
			}
			return
		}
		s.fill(&t)
		if reason := verdict(s, step); reason != "" {
			t.Reason = reason
			if !s.redacted {
				proof := observationFor(s)
				grading = &proof
			}
			return
		}
		select {
		case <-ctx.Done():
			t.Reason = "timeout"
			return
		case <-ticker.C:
		}
	}
}
