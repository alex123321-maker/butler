package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Fixture-only diagnostic projection. Queries never select text, args, grant
// bodies, checkpoint payloads, event bodies, DSNs or provider responses. Even
// identifiers are hashed: a mistakenly body-shaped identifier cannot escape.
const nativeAuditMaxRows = 16
const nativeAuditMaxBytes = 65536

var errNativeAudit = errors.New("owned_state_audit_unavailable")
var nativeAuditHash = regexp.MustCompile(`^[a-f0-9]{64}$`)

type nativeOwnedUsage struct {
	Input  *int64 `json:"input_tokens"`
	Output *int64 `json:"output_tokens"`
}

type nativeOwnedRow struct {
	Kind                string            `json:"kind"`
	Ref                 string            `json:"ref_sha256"`
	Run                 string            `json:"run_ref_sha256,omitempty"`
	Task                string            `json:"task_ref_sha256,omitempty"`
	Parent              string            `json:"parent_ref_sha256,omitempty"`
	Claim               string            `json:"claim_ref_sha256,omitempty"`
	Input               string            `json:"input_ref_sha256,omitempty"`
	Lease               string            `json:"lease_ref_sha256,omitempty"`
	Object              string            `json:"object_ref_sha256,omitempty"`
	State               string            `json:"state,omitempty"`
	Status              string            `json:"status,omitempty"`
	ErrorCode           string            `json:"error_code,omitempty"`
	Checkpoint          string            `json:"checkpoint_kind,omitempty"`
	Outcome             string            `json:"outcome,omitempty"`
	Tool                string            `json:"tool,omitempty"`
	ContentHash         string            `json:"content_hash,omitempty"`
	Revision            int64             `json:"revision,omitempty"`
	ConstraintsRevision int64             `json:"constraints_revision,omitempty"`
	AccessRevision      int64             `json:"access_revision,omitempty"`
	GoalRevision        int64             `json:"goal_revision,omitempty"`
	MandateRevision     int64             `json:"mandate_revision,omitempty"`
	Epoch               int64             `json:"fence_epoch,omitempty"`
	Attempts            int64             `json:"attempts,omitempty"`
	InputAllowance      int64             `json:"input_allowance,omitempty"`
	OutputAllowance     int64             `json:"output_allowance,omitempty"`
	Usage               *nativeOwnedUsage `json:"actual_usage,omitempty"`
	Expired             bool              `json:"expired"`
	Finished            bool              `json:"finished"`
}

type nativeOwnedAudit struct {
	Status         string           `json:"status"`
	WorkersStopped bool             `json:"workers_stopped"`
	Available      bool             `json:"owned_state_available"`
	SnapshotAt     time.Time        `json:"snapshot_at"`
	Rows           []nativeOwnedRow `json:"rows"`
}

func nativeAuditRef(value string) string {
	if value == "" {
		return ""
	}
	h := sha256.Sum256([]byte(value))
	return hex.EncodeToString(h[:])
}

func nativeAuditEnum(value string, allowed ...string) string {
	if value == "" {
		return ""
	}
	for _, candidate := range allowed {
		if value == candidate {
			return value
		}
	}
	return "unknown"
}

func nativeAuditState(v string) string {
	return nativeAuditEnum(v, "queued", "running", "waiting_for_user", "blocked", "completed", "failed", "expired", "cancelled", "active", "paused", "created", "acquired", "preparing", "model_running", "tool_pending", "awaiting_approval", "tool_running", "awaiting_model_resume", "finalizing", "timed_out", "waiting", "executing", "result_ready", "accepted", "rejected", "authorized", "inflight", "dispatch_completed", "revoked", "prepared", "allowed", "effect_confirmed", "effect_absent", "effect_unknown", "unknown", "final", "partial", "unverified")
}

func nativeAuditError(v string) string {
	return nativeAuditEnum(v, "input_expired", "retry_exhausted", "recovery_unavailable", "prior_attempt_terminal", "session_busy", "result_persistence_pending", "execution_stopped", "execution_deadline_exceeded", "effect_unknown", "execution_error", "provider_unavailable", "transport_connection_error", "provider_timeout", "provider_protocol_error", "capability_mismatch", "invalid_tool_request", "stateful_session_lost", "rate_limited", "internal_transport_error", "provider_input_bytes_exceeded", "execution_budget_deadline", "execution_budget_tokens", "validation_error", "transport_error", "tool_error", "policy_denied", "credential_error", "approval_error", "internal_error", "cancelled", "timeout", "unknown")
}

func nativeAuditCheckpoint(v string) string {
	return nativeAuditEnum(v, "accepted", "created", "run_created", "preparing", "prepared", "context_prepared", "user_recorded", "model_running", "model_started", "model_budget_reserved", "awaiting_approval", "approval_ready", "tool_dispatching", "tool_result_recorded", "finalizing", "completed", "execution_stopped", "unknown")
}

// A read-only repeatable snapshot starts only after all owning worker goroutines
// or processes have stopped. Missing/oversize evidence is unavailable, never PASS.
func collectNativeOwnedAudit(ctx context.Context, pool *pgxpool.Pool, stopped bool) (nativeOwnedAudit, error) {
	a := nativeOwnedAudit{Status: "unavailable", WorkersStopped: stopped, Rows: []nativeOwnedRow{}}
	if !stopped {
		return a, errNativeAudit
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return a, errNativeAudit
	}
	defer tx.Rollback(ctx)
	if tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&a.SnapshotAt) != nil {
		return a, errNativeAudit
	}
	read := func(query string, scan func(pgx.Rows) (nativeOwnedRow, error)) error {
		rows, e := tx.Query(ctx, query)
		if e != nil {
			return errNativeAudit
		}
		defer rows.Close()
		count := 0
		for rows.Next() {
			count++
			if count > nativeAuditMaxRows {
				return errNativeAudit
			}
			r, e := scan(rows)
			if e != nil {
				return errNativeAudit
			}
			a.Rows = append(a.Rows, r)
		}
		if rows.Err() != nil {
			return errNativeAudit
		}
		return nil
	}
	queries := []struct {
		sql  string
		scan func(pgx.Rows) (nativeOwnedRow, error)
	}{
		{`SELECT input_id,COALESCE(run_id,''),COALESCE(task_id,''),claim_id,status,error_code,checkpoint,attempts,COALESCE(claim_expires_at<=clock_timestamp(),false) FROM accepted_inputs ORDER BY session_sequence LIMIT 17`, func(rows pgx.Rows) (r nativeOwnedRow, e error) {
			r.Kind = "input"
			e = rows.Scan(&r.Ref, &r.Run, &r.Task, &r.Claim, &r.Status, &r.ErrorCode, &r.Checkpoint, &r.Attempts, &r.Expired)
			r.Status, r.ErrorCode, r.Checkpoint = nativeAuditState(r.Status), nativeAuditError(r.ErrorCode), nativeAuditCheckpoint(r.Checkpoint)
			return r, e
		}},
		{`SELECT run_id,COALESCE(task_id,''),COALESCE(lease_id,''),COALESCE(task_revision,0),status,current_state,COALESCE(error_type,''),finished_at IS NOT NULL FROM runs ORDER BY started_at,run_id LIMIT 17`, func(rows pgx.Rows) (r nativeOwnedRow, e error) {
			r.Kind = "run"
			e = rows.Scan(&r.Ref, &r.Task, &r.Lease, &r.Revision, &r.Status, &r.State, &r.ErrorCode, &r.Finished)
			r.Status, r.State, r.ErrorCode = nativeAuditState(r.Status), nativeAuditState(r.State), nativeAuditError(r.ErrorCode)
			return r, e
		}},
		{`SELECT session_key,run_id,lease_id,fence_epoch,expires_at<=clock_timestamp() FROM session_ownership ORDER BY session_key LIMIT 17`, func(rows pgx.Rows) (r nativeOwnedRow, e error) {
			r.Kind = "ownership"
			e = rows.Scan(&r.Ref, &r.Run, &r.Lease, &r.Epoch, &r.Expired)
			return r, e
		}},
		{`SELECT task_id,COALESCE(parent_task_id,''),status,revision,constraints_revision FROM tasks ORDER BY created_at,task_id LIMIT 17`, func(rows pgx.Rows) (r nativeOwnedRow, e error) {
			r.Kind = "task"
			e = rows.Scan(&r.Ref, &r.Parent, &r.Status, &r.Revision, &r.ConstraintsRevision)
			r.Status = nativeAuditState(r.Status)
			return r, e
		}},
		{`SELECT task_id,parent_task_id,state,input_id,result_id,parent_revision,parent_constraints_revision,deadline<=clock_timestamp() FROM task_delegations ORDER BY created_at,task_id LIMIT 17`, func(rows pgx.Rows) (r nativeOwnedRow, e error) {
			r.Kind = "delegation"
			e = rows.Scan(&r.Ref, &r.Parent, &r.State, &r.Input, &r.Object, &r.Revision, &r.ConstraintsRevision, &r.Expired)
			r.State = nativeAuditState(r.State)
			return r, e
		}},
		{`SELECT task_id,goal_id,mandate_id,goal_revision,mandate_revision FROM task_authority_bindings ORDER BY task_id LIMIT 17`, func(rows pgx.Rows) (r nativeOwnedRow, e error) {
			r.Kind = "authority"
			e = rows.Scan(&r.Ref, &r.Parent, &r.Object, &r.GoalRevision, &r.MandateRevision)
			return r, e
		}},
		{`SELECT i.action_id,i.run_id,COALESCE(a.authorization_id,''),COALESCE(a.state,''),COALESCE(d.outcome,''),COALESCE(e.state,''),i.binding->>'tool_name',i.action_hash,i.expires_at<=clock_timestamp() FROM action_intents i LEFT JOIN action_authorizations a USING(action_id) LEFT JOIN action_effect_dispatches d USING(authorization_id) LEFT JOIN action_effects e ON e.action_id=i.effect_id ORDER BY i.created_at,i.action_id LIMIT 17`, func(rows pgx.Rows) (r nativeOwnedRow, e error) {
			r.Kind = "action"
			e = rows.Scan(&r.Ref, &r.Run, &r.Object, &r.Status, &r.Outcome, &r.State, &r.Tool, &r.ContentHash, &r.Expired)
			r.Status, r.Outcome, r.State, r.Tool = nativeAuditState(r.Status), nativeAuditState(r.Outcome), nativeAuditState(r.State), nativeAuditEnum(r.Tool, "task.delegate")
			return r, e
		}},
		{`SELECT tool_call_id,run_id,status,tool_name,finished_at IS NOT NULL FROM tool_calls ORDER BY started_at,tool_call_id LIMIT 17`, func(rows pgx.Rows) (r nativeOwnedRow, e error) {
			r.Kind = "tool"
			e = rows.Scan(&r.Ref, &r.Run, &r.Status, &r.Tool, &r.Finished)
			r.Status, r.Tool = nativeAuditState(r.Status), nativeAuditEnum(r.Tool, "task.delegate")
			return r, e
		}},
		{`SELECT operation_id,run_id,reservation_id,input_allowance,output_allowance,actual_input_tokens,actual_output_tokens FROM execution_model_attempts ORDER BY created_at,operation_id LIMIT 17`, func(rows pgx.Rows) (r nativeOwnedRow, e error) {
			r.Kind, r.Usage = "model", &nativeOwnedUsage{}
			e = rows.Scan(&r.Ref, &r.Run, &r.Object, &r.InputAllowance, &r.OutputAllowance, &r.Usage.Input, &r.Usage.Output)
			return r, e
		}},
		{`SELECT result_id,run_id,COALESCE(task_id,''),answer_state,verdict,content_hash,task_revision,constraints_revision FROM run_final_results ORDER BY created_at,result_id LIMIT 17`, func(rows pgx.Rows) (r nativeOwnedRow, e error) {
			r.Kind = "final"
			e = rows.Scan(&r.Ref, &r.Run, &r.Task, &r.State, &r.Status, &r.ContentHash, &r.Revision, &r.ConstraintsRevision)
			r.State, r.Status = nativeAuditState(r.State), nativeAuditState(r.Status)
			return r, e
		}},
		{`SELECT run_id,source_id,source_revision,access_revision,content_hash FROM memory_run_sources ORDER BY run_id,source_id LIMIT 17`, func(rows pgx.Rows) (r nativeOwnedRow, e error) {
			r.Kind = "source"
			e = rows.Scan(&r.Run, &r.Ref, &r.Revision, &r.AccessRevision, &r.ContentHash)
			return r, e
		}},
	}
	for _, q := range queries {
		if read(q.sql, q.scan) != nil {
			return nativeOwnedAudit{Status: "unavailable", WorkersStopped: stopped}, errNativeAudit
		}
	}
	for i := range a.Rows {
		r := &a.Rows[i]
		for _, id := range []*string{&r.Ref, &r.Run, &r.Task, &r.Parent, &r.Claim, &r.Input, &r.Lease, &r.Object} {
			*id = nativeAuditRef(*id)
		}
		if r.ContentHash != "" && !nativeAuditHash.MatchString(r.ContentHash) {
			return nativeOwnedAudit{Status: "unavailable", WorkersStopped: stopped}, errNativeAudit
		}
	}
	if tx.Commit(ctx) != nil {
		return nativeOwnedAudit{Status: "unavailable", WorkersStopped: stopped}, errNativeAudit
	}
	a.Status, a.Available = "captured_before_cleanup", true
	if _, err := nativeAuditBytes(a); err != nil {
		return nativeOwnedAudit{Status: "unavailable", WorkersStopped: stopped}, errNativeAudit
	}
	return a, nil
}

func nativeAuditBytes(a nativeOwnedAudit) ([]byte, error) {
	if (a.Status != "unavailable" && a.Status != "captured_before_cleanup") || !a.WorkersStopped && (a.Available || len(a.Rows) > 0) {
		return nil, errNativeAudit
	}
	counts := map[string]int{}
	for _, r := range a.Rows {
		if r.Kind == "unknown" || nativeAuditEnum(r.Kind, "input", "run", "ownership", "task", "delegation", "authority", "action", "tool", "model", "final", "source") != r.Kind || !nativeAuditHash.MatchString(r.Ref) {
			return nil, errNativeAudit
		}
		counts[r.Kind]++
		if counts[r.Kind] > nativeAuditMaxRows || nativeAuditState(r.State) != r.State || nativeAuditState(r.Status) != r.Status || nativeAuditError(r.ErrorCode) != r.ErrorCode || nativeAuditCheckpoint(r.Checkpoint) != r.Checkpoint || nativeAuditState(r.Outcome) != r.Outcome || nativeAuditEnum(r.Tool, "task.delegate", "unknown") != r.Tool {
			return nil, errNativeAudit
		}
		for _, h := range []string{r.Ref, r.Run, r.Task, r.Parent, r.Claim, r.Input, r.Lease, r.Object, r.ContentHash} {
			if h != "" && !nativeAuditHash.MatchString(h) {
				return nil, errNativeAudit
			}
		}
		for _, n := range []int64{r.Revision, r.ConstraintsRevision, r.AccessRevision, r.GoalRevision, r.MandateRevision, r.Epoch, r.Attempts, r.InputAllowance, r.OutputAllowance} {
			if n < 0 || n > 1<<53 {
				return nil, errNativeAudit
			}
		}
		if r.Usage != nil {
			for _, n := range []*int64{r.Usage.Input, r.Usage.Output} {
				if n != nil && (*n < 0 || *n > 1<<53) {
					return nil, errNativeAudit
				}
			}
		}
	}
	b, err := json.Marshal(a)
	if err != nil || len(b) > nativeAuditMaxBytes {
		return nil, errNativeAudit
	}
	return b, nil
}

func writeNativeOwnedAudit(path string, a nativeOwnedAudit) error {
	if path == "" {
		return errNativeAudit
	}
	b, err := nativeAuditBytes(a)
	if err != nil {
		return err
	}
	// The path is an operator-selected private report path, never a model arg.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return errNativeAudit
	}
	defer f.Close()
	if _, err = f.Write(b); err != nil || f.Sync() != nil {
		return errNativeAudit
	}
	return nil
}

func TestNativeDelegationOwnedAuditValidation(t *testing.T) {
	base := nativeOwnedAudit{Status: "captured_before_cleanup", Available: true, WorkersStopped: true, Rows: []nativeOwnedRow{{Kind: "input", Ref: nativeAuditRef("fixture-input"), Status: "blocked", ErrorCode: "execution_stopped"}}}
	for _, name := range []string{"raw-error", "raw-state", "raw-tool", "raw-checkpoint", "raw-id", "raw-kind", "raw-status", "negative", "too-many", "too-large"} {
		t.Run(name, func(t *testing.T) {
			a := base
			a.Rows = append([]nativeOwnedRow{}, base.Rows...)
			switch name {
			case "raw-error":
				a.Rows[0].ErrorCode = "password=SECRET_DIAGNOSTIC_MARKER"
			case "raw-state":
				a.Rows[0].State = "SECRET_DIAGNOSTIC_MARKER"
			case "raw-tool":
				a.Rows[0].Tool = "SECRET_DIAGNOSTIC_MARKER"
			case "raw-checkpoint":
				a.Rows[0].Checkpoint = "SECRET_DIAGNOSTIC_MARKER"
			case "raw-id":
				a.Rows[0].Ref = "SECRET_DIAGNOSTIC_MARKER"
			case "raw-kind":
				a.Rows[0].Kind = "unknown"
			case "raw-status":
				a.Status = "unknown"
			case "negative":
				a.Rows[0].Epoch = -1
			case "too-many":
				for len(a.Rows) <= nativeAuditMaxRows {
					a.Rows = append(a.Rows, base.Rows[0])
				}
			case "too-large":
				a.Rows = nil
				for _, kind := range []string{"input", "run", "ownership", "task", "delegation", "authority", "action", "tool", "model", "final", "source"} {
					for i := 0; i < nativeAuditMaxRows; i++ {
						h := base.Rows[0].Ref
						a.Rows = append(a.Rows, nativeOwnedRow{Kind: kind, Ref: h, Run: h, Task: h, Parent: h, Claim: h, Input: h, Lease: h, Object: h, ContentHash: h})
					}
				}
			}
			if b, err := nativeAuditBytes(a); err == nil || len(b) != 0 {
				t.Fatal("unsafe export accepted")
			}
		})
	}
	if nativeAuditError("arbitrary_private_material") != "unknown" {
		t.Fatal("error code allowlist widened")
	}
	if _, err := nativeAuditBytes(base); err != nil {
		t.Fatal("typed export denied")
	}
}

func TestNativeDelegationOwnedAuditPrivacyIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	f := newNativeScenarioFixture(t, ctx)
	const marker = "SECRET_DIAGNOSTIC_MARKER"
	if _, err := f.pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel,created_at,updated_at) VALUES($1,$1,'web',clock_timestamp(),clock_timestamp());`, marker); err != nil {
		t.Fatal("private marker fixture session denied")
	}
	for i, query := range []string{
		`INSERT INTO runs(run_id,session_key,input_event_id,status,current_state,autonomy_mode,model_provider,started_at,updated_at,error_type,error_message) VALUES($1,$1,$1,'failed','failed','mode_2','antigravity',clock_timestamp(),clock_timestamp(),$1,$1)`,
		`INSERT INTO accepted_inputs(input_id,session_key,event_id,dedup_key,source,owner_id,event_json,status,error_code,error_message,checkpoint,checkpoint_json) VALUES($1,$1,$1,$1,'web',$1,jsonb_build_object('text',$1::text),'blocked',$1,$1,$1,jsonb_build_object('private',$1::text))`,
		`INSERT INTO tool_calls(tool_call_id,run_id,tool_name,args,status,runtime_target,started_at,result,error) VALUES($1,$1,$1,jsonb_build_object('password',$1::text),'failed',$1,clock_timestamp(),jsonb_build_object('private',$1::text),jsonb_build_object('message',$1::text))`,
	} {
		if _, err := f.pool.Exec(ctx, query, marker); err != nil {
			code := "unknown"
			var postgres *pgconn.PgError
			if errors.As(err, &postgres) {
				code = nativeAuditEnum(postgres.Code, "42P18", "42P08", "42804", "22023", "42703", "23502", "23503", "23514", "P0001")
			}
			t.Fatalf("private marker state fixture denied statement=%d sqlstate=%s", i+1, code)
		}
	}
	if _, err := collectNativeOwnedAudit(ctx, f.pool, false); err == nil {
		t.Fatal("running workers accepted as final snapshot")
	}
	a, err := collectNativeOwnedAudit(ctx, f.pool, true)
	if err != nil || !a.Available {
		t.Fatal("typed private-state export unavailable")
	}
	b, err := nativeAuditBytes(a)
	if err != nil || strings.Contains(string(b), marker) || strings.Contains(string(b), "password") || strings.Contains(string(b), "checkpoint_json") || strings.Contains(string(b), "error_message") {
		t.Fatal("private material escaped diagnostic projection")
	}
	seen := 0
	for _, r := range a.Rows {
		if r.ErrorCode == "unknown" {
			seen++
		}
	}
	if seen != 2 {
		t.Fatal("unknown error codes were not independently clamped")
	}
	path := nativeAuditTestPath(t, "private-marker")
	if writeNativeOwnedAudit(path, a) != nil || writeNativeOwnedAudit(path, a) == nil {
		t.Fatal("private immutable report file contract failed")
	}
	stat, err := os.Stat(path)
	if err != nil || stat.Mode().Perm() != 0600 {
		t.Fatal("diagnostic report not private")
	}
}

func TestNativeDelegationOwnedAuditRowLimitIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	f := newNativeScenarioFixture(t, ctx)
	if _, err := f.pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel,created_at,updated_at) VALUES('audit-limit','fixture','web',clock_timestamp(),clock_timestamp());
 INSERT INTO runs(run_id,session_key,input_event_id,status,current_state,autonomy_mode,model_provider,started_at,updated_at) SELECT 'audit-'||g,'audit-limit','fixture','failed','failed','mode_2','antigravity',clock_timestamp(),clock_timestamp() FROM generate_series(1,17) g;`); err != nil {
		t.Fatal("bounded state fixture denied")
	}
	if a, err := collectNativeOwnedAudit(ctx, f.pool, true); err == nil || a.Available || len(a.Rows) != 0 {
		t.Fatal("truncated state claimed complete")
	}
}
