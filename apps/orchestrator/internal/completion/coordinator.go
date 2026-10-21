// Package completion commits a run result independently of channel delivery.
package completion

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/artifacts"
	"github.com/butler/butler/apps/orchestrator/internal/deliveryoutbox"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Effect struct {
	ActionID     string   `json:"action_id"`
	EffectState  string   `json:"effect_state"`
	EvidenceRefs []string `json:"evidence_refs"`
}
type EvidenceReader interface {
	ReadRunEffects(context.Context, string) ([]Effect, error)
}
type Request struct {
	RunID, SessionKey, Content, AnswerState string
	TerminalObserved                        bool
	PipelineEnabled                         bool
	TaskID                                  string
	TaskRevision, ConstraintsRevision       int64
}
type FinalRecord struct {
	ResultID            string    `json:"result_id"`
	RunID               string    `json:"run_id"`
	SessionKey          string    `json:"session_key"`
	TaskID              string    `json:"task_id,omitempty"`
	TaskRevision        int64     `json:"task_revision"`
	ConstraintsRevision int64     `json:"constraints_revision"`
	ArtifactID          string    `json:"artifact_id"`
	MessageID           string    `json:"message_id"`
	Content             string    `json:"content"`
	ContentHash         string    `json:"content_hash"`
	AnswerState         string    `json:"answer_state"`
	Verdict             string    `json:"verdict"`
	Effects             []Effect  `json:"effects"`
	Limitations         []string  `json:"limitations"`
	CreatedAt           time.Time `json:"created_at"`
	PrivacyRedacted     bool      `json:"privacy_redacted"`
	RedactionID         string    `json:"redaction_id,omitempty"`
}
type Writer interface {
	Save(context.Context, Request) (FinalRecord, error)
}

var ErrNotFound = errors.New("final result not found")

type Coordinator struct {
	pool     *pgxpool.Pool
	Sessions *session.PostgresRepository
	Runs     interface {
		LockCompletionTx(context.Context, pgx.Tx, string, string) (run.CompletionState, error)
		CompleteTx(context.Context, pgx.Tx, string, time.Time) error
	}
	Tasks     *task.Store
	Artifacts interface {
		SaveFinalTx(context.Context, pgx.Tx, artifacts.CreateParams) error
	}
	Transcript interface {
		AppendFinalTx(context.Context, pgx.Tx, transcript.Message) (transcript.Message, error)
	}
	Communication interface {
		FinalizeTx(context.Context, pgx.Tx, string) (bool, error)
	}
	Outbox    *deliveryoutbox.Store
	Evidence  EvidenceReader
	DataGuard func(context.Context, pgx.Tx, string) error
}

func New(pool *pgxpool.Pool) *Coordinator {
	return &Coordinator{pool: pool, Sessions: session.NewPostgresRepository(pool), Runs: run.NewPostgresRepository(pool), Tasks: task.NewStore(pool), Artifacts: artifacts.NewPostgresRepository(pool), Transcript: transcript.NewStore(pool), Outbox: deliveryoutbox.NewStore(pool)}
}

func (c *Coordinator) Save(ctx context.Context, q Request) (FinalRecord, error) {
	if (strings.TrimSpace(q.Content) == "" && c.Communication == nil) || q.RunID == "" || q.SessionKey == "" {
		return FinalRecord{}, fmt.Errorf("final result requires identity and nonempty content")
	}
	token, ok := ownership.TokenFromContext(ctx)
	if !ok || token.RunID != q.RunID || token.SessionKey != q.SessionKey {
		return FinalRecord{}, ownership.ErrStale
	}
	guarded, release, err := ownership.Begin(ctx)
	if err != nil {
		return FinalRecord{}, err
	}
	defer release()
	ctx = guarded
	effects := []Effect{}
	evidenceUnavailable := true
	if c.Evidence != nil {
		items, e := c.Evidence.ReadRunEffects(ctx, q.RunID)
		if e == nil {
			effects = items
			evidenceUnavailable = false
		}
	}
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return FinalRecord{}, err
	}
	defer tx.Rollback(context.Background())
	channel, err := c.Sessions.LockCompletionTx(ctx, tx, q.SessionKey)
	if err != nil {
		return FinalRecord{}, err
	}
	explicit := false
	if c.Communication != nil {
		explicit, err = c.Communication.FinalizeTx(ctx, tx, q.RunID)
		if err != nil {
			return FinalRecord{}, err
		}
	}
	if explicit && !q.TerminalObserved || strings.TrimSpace(q.Content) == "" && !explicit {
		return FinalRecord{}, fmt.Errorf("typed terminal final required")
	}
	// A repeated persistence attempt returns the immutable record, never tools.
	old, err := get(ctx, tx, q.RunID)
	if err == nil {
		if old.Content != q.Content {
			return FinalRecord{}, fmt.Errorf("final checkpoint conflicts with immutable result")
		}
		return old, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return FinalRecord{}, err
	}
	state, err := c.Runs.LockCompletionTx(ctx, tx, q.RunID, q.SessionKey)
	if err != nil {
		return FinalRecord{}, err
	}
	if state.TaskID != q.TaskID || state.TaskRevision != q.TaskRevision {
		return FinalRecord{}, task.ErrConflict
	}
	now := time.Now().UTC()
	hash := sha256.Sum256([]byte(q.Content))
	r := FinalRecord{ResultID: "result:" + q.RunID, RunID: q.RunID, SessionKey: q.SessionKey, TaskID: state.TaskID, TaskRevision: state.TaskRevision, ArtifactID: "final:" + q.RunID, MessageID: "final:" + q.RunID, Content: q.Content, ContentHash: hex.EncodeToString(hash[:]), AnswerState: q.AnswerState, Verdict: "unverified", Effects: []Effect{}, Limitations: []string{"task_completion_not_verified"}, CreatedAt: now}
	if r.AnswerState == "" {
		r.AnswerState = "final"
	}
	if r.AnswerState != "final" && r.AnswerState != "partial" {
		return r, fmt.Errorf("invalid final answer state")
	}
	if r.AnswerState == "partial" {
		r.Limitations = append(r.Limitations, "model_output_incomplete")
	}
	r.Effects = effects
	if !evidenceUnavailable && len(effects) == 0 {
		if reader, ok := c.Transcript.(interface {
			HasRunToolCallsTx(context.Context, pgx.Tx, string) (bool, error)
		}); ok {
			hasTools, e := reader.HasRunToolCallsTx(ctx, tx, q.RunID)
			if e != nil {
				return r, e
			}
			evidenceUnavailable = hasTools
		}
	}
	if evidenceUnavailable {
		r.Limitations = append(r.Limitations, "effect_evidence_unavailable")
	}
	refs := []string{"artifact:" + r.ArtifactID}
	for _, effect := range r.Effects {
		refs = append(refs, effect.EvidenceRefs...)
		if effect.EffectState != "effect_confirmed" && effect.EffectState != "effect_absent" {
			r.Limitations = append(r.Limitations, "unresolved_action:"+effect.ActionID)
		}
	}
	r.ConstraintsRevision, err = c.Tasks.SaveResultTx(ctx, tx, task.Result{ResultID: r.ResultID, TaskID: r.TaskID, TaskRevision: r.TaskRevision, RunID: r.RunID, Summary: r.Content, EvidenceRefs: refs, CreatedAt: now}, q.ConstraintsRevision)
	if err != nil {
		return r, err
	}
	if c.DataGuard != nil {
		if err = c.DataGuard(ctx, tx, q.RunID); err != nil {
			return r, err
		}
	}
	metadata := `{"source":"model","authoritative_result":true}`
	if explicit {
		metadata = `{"source":"model","kind":"attempt_final","visibility":"internal","published":false}`
	}
	message, err := c.Transcript.AppendFinalTx(ctx, tx, transcript.Message{MessageID: r.MessageID, RunID: r.RunID, SessionKey: r.SessionKey, Content: r.Content, MetadataJSON: metadata, CreatedAt: now})
	if err != nil {
		return r, err
	}
	r.MessageID = message.MessageID
	if err = c.Artifacts.SaveFinalTx(ctx, tx, artifacts.CreateParams{ArtifactID: r.ArtifactID, RunID: r.RunID, SessionKey: r.SessionKey, ContentText: r.Content, SourceRef: r.MessageID, CreatedAt: now}); err != nil {
		return r, err
	}
	effectsJSON, _ := json.Marshal(r.Effects)
	limits, _ := json.Marshal(r.Limitations)
	_, err = tx.Exec(ctx, `INSERT INTO run_final_results(result_id,run_id,session_key,task_id,task_revision,constraints_revision,artifact_id,message_id,content,content_hash,answer_state,effects_json,limitations_json,created_at) VALUES($1,$2,$3,NULLIF($4,''),$5,$6,$7,$8,$9,$10,$11,$12::jsonb,$13::jsonb,$14)`, r.ResultID, r.RunID, r.SessionKey, r.TaskID, r.TaskRevision, r.ConstraintsRevision, r.ArtifactID, r.MessageID, r.Content, r.ContentHash, r.AnswerState, string(effectsJSON), string(limits), now)
	if err != nil {
		return r, err
	}
	notify, err := c.Tasks.NotifyRunResultTx(ctx, tx, r.TaskID)
	if err != nil {
		return r, err
	}
	if notify && !explicit {
		if err = c.Outbox.EnqueueTx(ctx, tx, r.ResultID, r.RunID, r.SessionKey, channel, r.NotificationContent(), now); err != nil {
			return r, err
		}
	}
	kinds := []string{"working_cleanup", "transient_cleanup"}
	if q.PipelineEnabled {
		kinds = append(kinds, "memory_pipeline")
	}
	for _, kind := range kinds {
		_, err = tx.Exec(ctx, `INSERT INTO completion_aux_jobs(job_id,result_id,run_id,session_key,kind)VALUES($1,$2,$3,$4,$5)`, r.ResultID+":"+kind, r.ResultID, r.RunID, r.SessionKey, kind)
		if err != nil {
			return r, err
		}
	}
	if err = c.Runs.CompleteTx(ctx, tx, q.RunID, now); err != nil {
		return r, err
	}
	return r, tx.Commit(ctx)
}

func (r FinalRecord) NotificationContent() string {
	prefix := "Butler: результат попытки сохранён; завершение поручения не проверено."
	if r.AnswerState == "partial" {
		prefix += " Ответ модели неполный."
	}
	for _, limit := range r.Limitations {
		if strings.HasPrefix(limit, "unresolved_action:") || limit == "effect_evidence_unavailable" {
			prefix += " Есть ограничения подтверждения внешних действий; подробности в сохранённом результате."
			break
		}
	}
	return prefix + "\n\n" + r.Content
}

type queryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func get(ctx context.Context, q queryer, runID string) (FinalRecord, error) {
	var r FinalRecord
	var effects, limits []byte
	err := q.QueryRow(ctx, `SELECT result_id,run_id,session_key,COALESCE(task_id,''),task_revision,constraints_revision,artifact_id,message_id,content,content_hash,answer_state,verdict,effects_json,limitations_json,created_at,COALESCE((to_jsonb(r)->>'privacy_redacted')::BOOLEAN,FALSE),COALESCE(to_jsonb(r)->>'redaction_id','') FROM run_final_results r WHERE run_id=$1`, runID).Scan(&r.ResultID, &r.RunID, &r.SessionKey, &r.TaskID, &r.TaskRevision, &r.ConstraintsRevision, &r.ArtifactID, &r.MessageID, &r.Content, &r.ContentHash, &r.AnswerState, &r.Verdict, &effects, &limits, &r.CreatedAt, &r.PrivacyRedacted, &r.RedactionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ErrNotFound
	}
	if err != nil {
		return r, err
	}
	if err = json.Unmarshal(effects, &r.Effects); err != nil {
		return r, err
	}
	err = json.Unmarshal(limits, &r.Limitations)
	return r, err
}
func (c *Coordinator) GetByRun(ctx context.Context, runID string) (FinalRecord, error) {
	return get(ctx, c.pool, runID)
}
