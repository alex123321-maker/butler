package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/butler/butler/internal/actionauth"
	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// This recipient is a declared synthetic effect backend. It sends no HTTP and
// touches no arbitrary host file. Production Broker authorization precedes it;
// independent SQL reads, rather than its RPC acknowledgment, establish bytes.
type behavior76Runtime struct {
	runtimev1.UnimplementedToolRuntimeServiceServer
	pool             *pgxpool.Pool
	scope            *behavior76FixtureScope
	loseNextWriteAck atomic.Bool                         // Trusted public fault test only; no corpus/model selector.
	beforeCommit     func(context.Context, string) error // Fixed public race-test hook, configured before serving work.
}
type behavior76Object struct {
	Key           string  `json:"key"`
	TaskKey       string  `json:"task_key"`
	TaskID        string  `json:"task_id"`
	Body          string  `json:"body"`
	Version       int64   `json:"version"`
	SHA256        string  `json:"sha256"`
	PreparedBody  *string `json:"prepared_body"`
	CommittedBody *string `json:"committed_body"`
	CommitCount   int64   `json:"commit_count"`
}
type behavior76Patch struct {
	ResourceKey     string `json:"resource_key"`
	ExpectedVersion int64  `json:"expected_version"`
	ExpectedSHA256  string `json:"expected_sha256"`
	Body            string `json:"body"`
}

func behavior76Hash(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }
func behavior76JSON(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(raw)
}

const behavior76ObjectsSQL = `CREATE TABLE behavior76_objects(
 key text PRIMARY KEY,task_key text NOT NULL,task_id text NOT NULL REFERENCES tasks(task_id),
 body text NOT NULL,version bigint NOT NULL CHECK(version>0),sha256 text NOT NULL,
 prepared_body text,committed_body text,commit_count bigint NOT NULL DEFAULT 0);
 CREATE TABLE behavior76_object_events(n bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 resource_key text NOT NULL,kind text NOT NULL,action_id text NOT NULL DEFAULT '',
 before_state jsonb NOT NULL,after_state jsonb NOT NULL,at timestamptz NOT NULL DEFAULT clock_timestamp());
 CREATE TABLE behavior76_recipient_receipts(action_id text PRIMARY KEY,receipt jsonb NOT NULL);
 CREATE TABLE behavior76_task_mandates(task_id text NOT NULL,task_revision bigint NOT NULL,
 mandate_id text NOT NULL,PRIMARY KEY(task_id,task_revision));`

func behavior76ObjectQuery(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, key, suffix string) (behavior76Object, error) {
	var o behavior76Object
	err := q.QueryRow(ctx, `SELECT key,task_key,task_id,body,version,sha256,prepared_body,committed_body,commit_count FROM behavior76_objects WHERE key=$1 `+suffix, key).Scan(&o.Key, &o.TaskKey, &o.TaskID, &o.Body, &o.Version, &o.SHA256, &o.PreparedBody, &o.CommittedBody, &o.CommitCount)
	return o, err
}

func (r *behavior76Runtime) Execute(ctx context.Context, req *runtimev1.ExecuteRequest) (response *runtimev1.ExecuteResponse, resultErr error) {
	if r.scope != nil {
		done, err := r.scope.begin(ctx, nil)
		if err != nil {
			return nil, err
		}
		defer done()
		defer func() {
			if resultErr == nil {
				return
			}
			category := "unclassified"
			switch resultErr.Error() {
			case "synthetic resource belongs to another task":
				category = "foreign_task"
			case "synthetic resource CAS denied":
				category = "cas_rejected"
			case "synthetic ACK deliberately lost":
				category = "lost_ack"
			}
			if category == "unclassified" {
				r.scope.global.Store(true)
			}
			cleanup, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if req.GetContext().GetActionId() == "" || req.GetToolCall() == nil {
				r.scope.global.Store(true)
				return
			}
			_, err := r.pool.Exec(cleanup, `INSERT INTO behavior76_recipient_diagnostics(action_id,run_id,tool_call_id,category) VALUES($1,$2,$3,$4)`, req.Context.ActionId, req.ToolCall.RunId, req.ToolCall.ToolCallId, category)
			if err != nil {
				r.scope.global.Store(true)
			}
		}()
	}
	if req.GetToolCall() == nil || req.GetContext().GetActionId() == "" || len(req.GetResolvedCredentials()) != 0 {
		return nil, errors.New("synthetic recipient requires exact authorized action")
	}
	call := req.ToolCall
	if req.Context.RunId != call.RunId || req.Context.ToolCallId != call.ToolCallId || req.Context.ToolName != call.ToolName {
		return nil, errors.New("synthetic dispatch identity mismatch")
	}
	var args struct {
		Method string `json:"method"`
		URL    string `json:"url"`
		Body   string `json:"body"`
		behavior76Patch
	}
	if json.Unmarshal([]byte(call.ArgsJson), &args) != nil {
		return nil, errors.New("invalid recipient arguments")
	}
	key, kind := args.ResourceKey, "prepare"
	if call.ToolName == "http.request" {
		if !strings.HasPrefix(args.URL, "https://fixture.invalid/objects/") {
			return nil, errors.New("synthetic object origin required")
		}
		key = strings.TrimPrefix(args.URL, "https://fixture.invalid/objects/")
		switch strings.ToUpper(args.Method) {
		case "GET":
			kind = "read"
		case "POST":
			kind = "commit"
		default:
			return nil, errors.New("synthetic recipient permits GET/POST only")
		}
		if kind == "commit" && json.Unmarshal([]byte(args.Body), &args.behavior76Patch) != nil {
			return nil, errors.New("invalid commit body")
		}
	} else if call.ToolName != "doctor.generate_report" {
		return nil, errors.New("unexpected synthetic tool")
	} else if json.Unmarshal([]byte(call.ArgsJson), &args.behavior76Patch) != nil {
		return nil, errors.New("invalid prepare body")
	}
	if key == "" || strings.ContainsAny(key, "/\\?#") {
		return nil, errors.New("invalid synthetic resource key")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	// The recipient independently checks the actual admitted action and owning
	// current task. It cannot manufacture an authorization from model arguments.
	var taskID, ownerID, keyID, fingerprint string
	err = tx.QueryRow(ctx, `SELECT a.binding->>'task_id',a.binding->>'owner_id',e.idempotency_key,e.fingerprint FROM action_intents a JOIN action_effects e ON e.action_id=a.effect_id WHERE a.binding->>'run_id'=$1 AND a.binding->>'tool_call_id'=$2 AND a.effect_id=$3`, call.RunId, call.ToolCallId, req.Context.ActionId).Scan(&taskID, &ownerID, &keyID, &fingerprint)
	if err != nil {
		return nil, fmt.Errorf("actual effect binding unavailable: %w", err)
	}
	if fingerprint != req.Context.ActionFingerprint || keyID != req.Context.IdempotencyKey {
		return nil, errors.New("synthetic dispatch fingerprint mismatch")
	}
	var prior []byte
	if err = tx.QueryRow(ctx, `SELECT receipt FROM behavior76_recipient_receipts WHERE action_id=$1`, req.Context.ActionId).Scan(&prior); err == nil {
		return &runtimev1.ExecuteResponse{Result: &toolbrokerv1.ToolResult{RunId: call.RunId, ToolCallId: call.ToolCallId, ToolName: call.ToolName, Status: "completed", ResultJson: string(prior)}}, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	// Repeat current ownership/task checks in the recipient transaction before
	// resource locks. These are additional checks on an actual Broker dispatch,
	// never a caller-created grant or a substitute for Broker admission.
	var live bool
	if err = tx.QueryRow(ctx, `SELECT o.lease_id=r.lease_id AND o.run_id=r.run_id AND o.expires_at>clock_timestamp() FROM runs r JOIN session_ownership o USING(session_key) WHERE r.run_id=$1 FOR SHARE OF o`, call.RunId).Scan(&live); err != nil || !live {
		return nil, errors.New("synthetic dispatch ownership unavailable")
	}
	if err = tx.QueryRow(ctx, `SELECT t.status='active' AND t.revision=(a.binding->>'task_revision')::bigint AND t.constraints_revision=(a.binding->>'constraints_revision')::bigint AND t.owner_id=$3 FROM tasks t JOIN action_intents a ON a.binding->>'task_id'=t.task_id WHERE t.task_id=$1 AND a.binding->>'run_id'=$2 AND a.effect_id=$4 FOR SHARE OF t`, taskID, call.RunId, ownerID, req.Context.ActionId).Scan(&live); err != nil || !live {
		return nil, errors.New("synthetic dispatch task authority unavailable")
	}
	o, err := behavior76ObjectQuery(ctx, tx, key, "FOR UPDATE")
	if err != nil {
		return nil, err
	}
	if o.TaskID != taskID || ownerID == "" {
		return nil, errors.New("synthetic resource belongs to another task")
	}
	if err = tx.QueryRow(ctx, `SELECT expires_at>clock_timestamp() FROM session_ownership WHERE run_id=$1`, call.RunId).Scan(&live); err != nil || !live {
		return nil, errors.New("synthetic dispatch expired after resource wait")
	}
	before := o
	if kind != "read" {
		if o.Version != args.ExpectedVersion || o.SHA256 != args.ExpectedSHA256 {
			return nil, errors.New("synthetic resource CAS denied")
		}
		if len(args.behavior76Patch.Body) > 32768 {
			return nil, errors.New("synthetic body limit")
		}
		o.Version++
		if kind == "prepare" {
			v := args.behavior76Patch.Body
			o.PreparedBody = &v
		} else {
			o.Body = args.behavior76Patch.Body
			o.SHA256 = behavior76Hash([]byte(o.Body))
			v := o.Body
			o.CommittedBody = &v
			o.CommitCount++
		}
		if _, err = tx.Exec(ctx, `UPDATE behavior76_objects SET body=$2,version=$3,sha256=$4,prepared_body=$5,committed_body=$6,commit_count=$7 WHERE key=$1`, key, o.Body, o.Version, o.SHA256, o.PreparedBody, o.CommittedBody, o.CommitCount); err != nil {
			return nil, err
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO behavior76_object_events(resource_key,kind,action_id,before_state,after_state) VALUES($1,$2,$3,$4,$5)`, key, kind, req.Context.ActionId, behavior76JSON(before), behavior76JSON(o)); err != nil {
		return nil, err
	}
	result := map[string]any{"object": o, "synthetic_test_effect": true}
	if kind == "commit" {
		result["butler_effect_receipt"] = map[string]any{"idempotency_key": keyID, "action_fingerprint": fingerprint, "effect_state": string(actionauth.StateConfirmed), "artifact_id": fmt.Sprintf("synthetic:%s:v%d:%s", key, o.Version, o.SHA256)}
	}
	raw := behavior76JSON(result)
	if call.ToolName == "http.request" {
		raw = behavior76JSON(map[string]any{"status_code": 200, "truncated": false, "body": raw})
	}
	if _, err = tx.Exec(ctx, `INSERT INTO behavior76_recipient_receipts(action_id,receipt) VALUES($1,$2)`, req.Context.ActionId, raw); err != nil {
		return nil, err
	}
	if r.beforeCommit != nil {
		if err = r.beforeCommit(ctx, kind); err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	if r.scope != nil && kind != "read" && r.loseNextWriteAck.CompareAndSwap(true, false) {
		return nil, errors.New("synthetic ACK deliberately lost")
	}
	return &runtimev1.ExecuteResponse{Result: &toolbrokerv1.ToolResult{RunId: call.RunId, ToolCallId: call.ToolCallId, ToolName: call.ToolName, Status: "completed", ResultJson: raw}}, nil
}

type behavior76ContextReader struct{ *actionContextReader }

func (r behavior76ContextReader) GetActionContext(ctx context.Context, run string) (actionauth.ContextSnapshot, error) {
	c, err := r.actionContextReader.GetActionContext(ctx, run)
	if err != nil {
		return c, err
	}
	if c.TaskID != "" {
		err = r.pool.QueryRow(ctx, `SELECT mandate_id FROM behavior76_task_mandates WHERE task_id=$1 AND task_revision=$2`, c.TaskID, c.TaskRevision).Scan(&c.MandateID)
	}
	return c, err
}
