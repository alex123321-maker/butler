package adaptive

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/readcontrol"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrDenied = errors.New("adaptive retrieval proof denied")
var ErrBound = errors.New("adaptive retrieval bound reached")

type Store struct {
	pool    *pgxpool.Pool
	sources *catalog.Store
}

func NewStore(pool *pgxpool.Pool, sources *catalog.Store) (*Store, error) {
	if pool == nil || sources == nil || sources.Pool() != pool {
		return nil, ErrDenied
	}
	return &Store{pool: pool, sources: sources}, nil
}

// Init binds a deterministic current-input plan after task attachment. It never
// silently rebases an existing run onto amended input, constraints or policy.
func (s *Store) Init(ctx context.Context, plan Plan) error {
	if s == nil || !plan.Valid() {
		return ErrDenied
	}
	token, ok := ownership.TokenFromContext(ctx)
	if !ok {
		return ErrDenied
	}
	ctx = owneridentity.WithActor(ctx, owneridentity.Actor{})
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = s.sources.AccessTx(ctx, tx, nil, "memory_read"); err != nil {
		return err
	}
	var owner, key, task string
	var revision, constraints int64
	if err = tx.QueryRow(ctx, `SELECT s.user_id,r.session_key,coalesce(r.task_id,''),coalesce(r.task_revision,0),coalesce(t.constraints_revision,0) FROM runs r JOIN sessions s USING(session_key) LEFT JOIN tasks t ON t.task_id=r.task_id WHERE r.run_id=$1`, token.RunID).Scan(&owner, &key, &task, &revision, &constraints); err != nil {
		return ErrDenied
	}
	raw, err := json.Marshal(plan)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO memory_retrieval_plans(run_id,owner_id,session_key,task_id,task_revision,constraints_revision,policy_version,plan_hash,plan_json) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT(run_id) DO NOTHING`, token.RunID, owner, key, task, revision, constraints, Version, Digest(raw), raw)
	if err != nil {
		return err
	}
	var hash string
	if err = tx.QueryRow(ctx, `SELECT plan_hash FROM memory_retrieval_plans WHERE run_id=$1 AND owner_id=$2 AND session_key=$3 AND task_id=$4 AND task_revision=$5 AND constraints_revision=$6`, token.RunID, owner, key, task, revision, constraints).Scan(&hash); err != nil || hash != Digest(raw) {
		return ErrDenied
	}
	return tx.Commit(ctx)
}

func (s *Store) planTx(ctx context.Context, tx pgx.Tx, runID string, lock bool) (Plan, time.Time, error) {
	var plan Plan
	clause := ""
	if lock {
		clause = " FOR UPDATE OF p"
	}
	var raw []byte
	var hash, version, state string
	var deadline *time.Time
	err := tx.QueryRow(ctx, `SELECT p.plan_json,p.plan_hash,p.policy_version,p.deadline,p.state FROM memory_retrieval_plans p JOIN runs r USING(run_id) JOIN sessions s ON s.session_key=r.session_key LEFT JOIN tasks t ON t.task_id=r.task_id WHERE p.run_id=$1 AND p.owner_id=s.user_id AND p.session_key=r.session_key AND p.task_id=coalesce(r.task_id,'') AND p.task_revision=coalesce(r.task_revision,0) AND p.constraints_revision=coalesce(t.constraints_revision,0)`+clause, runID).Scan(&raw, &hash, &version, &deadline, &state)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return plan, time.Time{}, ErrDenied
		}
		return plan, time.Time{}, err
	}
	// PostgreSQL canonical JSON spelling differs from Go; compare its decoded
	// typed canonical representation, rejecting corrupt/unknown stored fields.
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&plan) != nil || !plan.Valid() || version != Version || state != "active" {
		return plan, time.Time{}, ErrDenied
	}
	canonical, _ := json.Marshal(plan)
	if Digest(canonical) != hash {
		return plan, time.Time{}, ErrDenied
	}
	if deadline != nil {
		return plan, *deadline, nil
	}
	return plan, time.Time{}, nil
}

// Plan reloads the immutable durable plan on continuation/restart. A newly
// composed process cannot reset counters or silently reclassify the question.
func (s *Store) Plan(ctx context.Context, runID string) (Plan, error) {
	if s == nil {
		return Plan{}, ErrDenied
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Plan{}, err
	}
	defer tx.Rollback(ctx)
	p, _, err := s.planTx(ctx, tx, runID, false)
	return p, err
}

func (s *Store) CheckProposal(ctx context.Context, runID, tool, raw string) error {
	if !readcontrol.IsTool(tool) {
		return nil
	}
	a, err := readcontrol.DecodeArgs(tool, raw)
	if err != nil {
		return ErrDenied
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	p, deadline, err := s.planTx(ctx, tx, runID, false)
	if err != nil {
		return err
	}
	var calls, searches, returned int
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT calls,searches,returned_bytes,clock_timestamp() FROM memory_retrieval_plans WHERE run_id=$1`, runID).Scan(&calls, &searches, &returned, &now); err != nil {
		return err
	}
	if !p.SearchEnabled() || calls >= p.MaxCalls || tool == readcontrol.Search && searches >= p.MaxSearches || returned >= p.MaxResponseBytes || !deadline.IsZero() && !now.Before(deadline) {
		return ErrBound
	}
	if tool == readcontrol.Search && !p.accepts(a) {
		return ErrDenied
	}
	return tx.Rollback(ctx)
}

func (p Plan) accepts(a readcontrol.Args) bool {
	return (a.Strategy == p.Strategy || p.Intent == "multi_hop") && a.EventFrom == p.EventFrom && a.EventUntil == p.EventUntil
}

// Reserve is separate from provider IPC and uses the caller's already proven
// current dispatch. Counters are conservative: a failed/unknown read is spent.
func (s *Store) Reserve(ctx context.Context, e readcontrol.Envelope, a readcontrol.Args) (time.Time, error) {
	token, ok := ownership.TokenFromContext(ctx)
	if !ok || token.RunID != e.RunID {
		return time.Time{}, ErrDenied
	}
	ctx = owneridentity.WithActor(ctx, owneridentity.Actor{})
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return time.Time{}, err
	}
	defer tx.Rollback(ctx)
	if _, err = s.sources.AccessTx(ctx, tx, nil, "memory_read"); err != nil {
		return time.Time{}, err
	}
	p, deadline, err := s.planTx(ctx, tx, e.RunID, true)
	if err != nil {
		return deadline, err
	}
	if !p.SearchEnabled() || e.Tool == readcontrol.Search && !p.accepts(a) {
		return deadline, ErrDenied
	}
	var calls, searches, bytes int
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT calls,searches,returned_bytes,clock_timestamp() FROM memory_retrieval_plans WHERE run_id=$1`, e.RunID).Scan(&calls, &searches, &bytes, &now); err != nil {
		return deadline, err
	}
	if calls >= p.MaxCalls || e.Tool == readcontrol.Search && searches >= p.MaxSearches || bytes >= p.MaxResponseBytes || !deadline.IsZero() && !now.Before(deadline) {
		return deadline, ErrBound
	}
	if deadline.IsZero() {
		deadline = now.Add(time.Duration(p.MaxMillis) * time.Millisecond)
	}
	strategy := a.Strategy
	if strategy == "" {
		strategy = "catalog_exact"
	}
	_, err = tx.Exec(ctx, `INSERT INTO memory_retrieval_operations(effect_id,run_id,tool_call_id,tool_name,args_hash,strategy) VALUES($1,$2,$3,$4,$5,$6)`, e.EffectID, e.RunID, e.ToolCallID, e.Tool, Digest([]byte(e.ArgsJSON)), strategy)
	if err != nil {
		return deadline, err
	}
	n := 0
	if e.Tool == readcontrol.Search {
		n = 1
	}
	_, err = tx.Exec(ctx, `UPDATE memory_retrieval_plans SET calls=calls+1,searches=searches+$2,deadline=$3 WHERE run_id=$1`, e.RunID, n, deadline)
	if err != nil {
		return deadline, err
	}
	return deadline, tx.Commit(ctx)
}

// ObserveTx commits source-opening receipts with the owning read publication.
// Search results never receive read_ref and cannot satisfy a final citation.
func (s *Store) ObserveTx(ctx context.Context, tx pgx.Tx, e readcontrol.Envelope, a readcontrol.Args, out *readcontrol.Result) error {
	token, ok := ownership.TokenFromContext(ctx)
	if !ok || token.RunID != e.RunID || out == nil {
		return ErrDenied
	}
	p, deadline, err := s.planTx(ctx, tx, e.RunID, true)
	if err != nil {
		return err
	}
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return err
	}
	if deadline.IsZero() || !now.Before(deadline) {
		return ErrBound
	}
	if e.Tool != readcontrol.Search && out.Status == "ok" {
		for i := range out.Items {
			item := &out.Items[i]
			n := utf8.RuneCountInString(item.Text)
			if n == 0 {
				continue
			}
			offset := 0
			if e.Tool == readcontrol.Open {
				offset = a.Offset
			}
			key := fmt.Sprintf("%s\x00%s\x00%s\x00%d", e.EffectID, item.Source.ID, item.FragmentKey, offset)
			item.ReadRef = "read_" + Digest([]byte(key))
			_, err = tx.Exec(ctx, `INSERT INTO memory_retrieval_opened(read_ref,effect_id,source_id,source_revision,access_revision,content_hash,fragment_key,read_offset,char_count,text_hash) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, item.ReadRef, e.EffectID, item.Source.ID, item.Source.Revision, item.Source.AccessRevision, item.Source.Hash, item.FragmentKey, offset, n, Digest([]byte(item.Text)))
			if err != nil {
				return err
			}
		}
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return err
	}
	// The permanent Broker's credential boundary decodes/re-encodes JSON maps
	// even for secret-free memory reads. Bind the exact representation it will
	// confirm, rather than the owning struct's declaration-key order. No field
	// is removed and numbers retain their spelling.
	confirmedHash, err := brokerResultDigest(raw)
	if err != nil {
		return err
	}
	var used int
	if err = tx.QueryRow(ctx, `SELECT returned_bytes FROM memory_retrieval_plans WHERE run_id=$1`, e.RunID).Scan(&used); err != nil {
		return err
	}
	if used+len(raw) > p.MaxResponseBytes {
		return ErrBound
	}
	tag, err := tx.Exec(ctx, `UPDATE memory_retrieval_operations SET state='observed',result_status=$2,returned_bytes=$3,result_hash=$4 WHERE effect_id=$1 AND run_id=$5 AND tool_call_id=$6 AND args_hash=$7 AND state='reserved'`, e.EffectID, out.Status, len(raw), confirmedHash, e.RunID, e.ToolCallID, Digest([]byte(e.ArgsJSON)))
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrDenied
	}
	_, err = tx.Exec(ctx, `UPDATE memory_retrieval_plans SET returned_bytes=returned_bytes+$2 WHERE run_id=$1`, e.RunID, len(raw))
	return err
}

func brokerResultDigest(raw []byte) (string, error) {
	var value any
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if err := d.Decode(&value); err != nil {
		return "", err
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return Digest(canonical), nil
}
