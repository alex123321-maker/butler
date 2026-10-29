package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/butler/butler/internal/acceptance"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5/pgxpool"
)

const experienceNativeMaximum = 640

var experienceNativeArms = []string{"A_no_experience", "B_same_frozen_notes_no_skills", "C_same_frozen_notes_and_reviewed_skills"}

// Mechanical input only. requested_outcome, class and oracle deliberately have
// no field in the provider-facing input. They remain independent evaluator data.
type experienceNativeTask struct {
	ID                 string `json:"id"`
	Resource           string `json:"resource"`
	EnvironmentProfile string `json:"environment_profile"`
	Reset              string `json:"reset"`
	Instruction        string `json:"instruction"`
	PermissionProfile  string `json:"permission_profile"`
	CurrentState       *struct {
		Effect            string `json:"effect"`
		EffectReference   string `json:"effect_reference"`
		NewPrepareAllowed bool   `json:"new_prepare_allowed"`
	} `json:"current_state,omitempty"`
}

type experienceNativeCorpus struct {
	Originals struct {
		Seed                     int                    `json:"seed"`
		CommonCurrentInstruction string                 `json:"common_current_instruction"`
		CommonProfile            json.RawMessage        `json:"common_profile"`
		RecipientSeeds           json.RawMessage        `json:"recipient_seed_contract"`
		Tasks                    []experienceNativeTask `json:"tasks"`
	}
	Controls struct {
		Seed        int                    `json:"seed"`
		Repetitions int                    `json:"repetitions"`
		Arms        []string               `json:"arms"`
		Cases       []experienceNativeTask `json:"cases"`
	}
	OriginalsSHA256 string
	ControlsSHA256  string
}

func readExperienceNativeCorpus(originals, controls []byte) (experienceNativeCorpus, error) {
	var c experienceNativeCorpus
	_, originalErr := actionauth.ArgumentsHash(string(originals))
	_, controlErr := actionauth.ArgumentsHash(string(controls))
	if len(originals) > 128*1024 || len(controls) > 256*1024 || originalErr != nil || controlErr != nil || json.Unmarshal(originals, &c.Originals) != nil || json.Unmarshal(controls, &c.Controls) != nil || c.Originals.Seed != 90 || c.Controls.Seed != 90 || len(c.Originals.Tasks) != 4 || len(c.Controls.Cases) != 8 || c.Controls.Repetitions != 2 || strings.Join(c.Controls.Arms, ",") != strings.Join(experienceNativeArms, ",") || strings.TrimSpace(c.Originals.CommonCurrentInstruction) == "" || !json.Valid(c.Originals.CommonProfile) || !json.Valid(c.Originals.RecipientSeeds) {
		return c, errors.New("frozen experience cohort rejected")
	}
	for _, tasks := range [][]experienceNativeTask{c.Originals.Tasks, c.Controls.Cases} {
		prior := ""
		for _, task := range tasks {
			if task.ID == "" || task.ID <= prior || len(task.ID) > 80 || task.Resource != "alpha" && task.Resource != "beta" || task.EnvironmentProfile == "" || task.Reset == "" || strings.TrimSpace(task.Instruction) == "" || len(task.Instruction) > 16*1024 || task.PermissionProfile == "" {
				return c, errors.New("bounded current task input rejected")
			}
			prior = task.ID
		}
	}
	c.OriginalsSHA256 = acceptance.MemoryBytesSHA256(originals)
	c.ControlsSHA256 = acceptance.MemoryBytesSHA256(controls)
	return c, nil
}

type experienceNativeSlot struct {
	Sequence   int        `json:"sequence"`
	ID         string     `json:"id"`
	Kind       string     `json:"kind"`
	CaseID     string     `json:"case_id"`
	Repetition int        `json:"repetition"`
	Arm        string     `json:"arm"`
	Maximum    int        `json:"maximum_provider_entries"`
	Status     string     `json:"status"`
	Blocker    string     `json:"blocker"`
	Started    *time.Time `json:"started_at"`
	Finished   *time.Time `json:"finished_at"`
}

func experienceNativeSchedule(c experienceNativeCorpus) []experienceNativeSlot {
	slots := []experienceNativeSlot{}
	add := func(kind, id, arm string, repetition, maximum int) {
		sequence := len(slots) + 1
		// Opaque identities avoid presenting outcome/arm labels as task identity.
		identity := experienceDigest(fmt.Sprintf("experience90-input-v1\x00%d\x00%s\x00%s\x00%d", sequence, id, arm, repetition))
		slots = append(slots, experienceNativeSlot{Sequence: sequence, ID: identity, Kind: kind, CaseID: id, Repetition: repetition, Arm: arm, Maximum: maximum, Status: "not_executed", Blocker: "preparation_not_completed"})
	}
	for _, task := range c.Originals.Tasks {
		add("original", task.ID, "shared", 0, 12)
	}
	for _, task := range c.Originals.Tasks {
		add("note", task.ID, "shared", 0, 1)
	}
	add("skill_preparation", "shared", "shared", 0, 12)
	for repetition := 1; repetition <= c.Controls.Repetitions; repetition++ {
		for index, task := range c.Controls.Cases {
			arms := c.Controls.Arms
			if repetition == 2 {
				arms = []string{c.Controls.Arms[2], c.Controls.Arms[1], c.Controls.Arms[0]}
			}
			for offset := 0; offset < 3; offset++ {
				add("control", task.ID, arms[(index+offset)%3], repetition, 12)
			}
		}
	}
	return slots
}

func experienceNativeBudgetPolicy() budget.Policy {
	p := budget.DefaultPolicy()
	p.Models["antigravity:gemini-3.8-flash-high"] = budget.ModelPrice{Basis: "subscription_no_overage"}
	p.ModelMillis, p.OutputTokens = 300000, 32768
	p.ToolMillis = 120000
	// Both foreground and preparation reserve the full declared profile. This
	// is the experiment's total partition ceiling, not a shared production-day
	// authority proof across disposable clones. The independent global journal
	// carries every earlier entry across clone and UTC-day boundaries.
	p.Task = budget.Units{ModelTurns: 12, Tokens: 12 * 1081344, ActiveMillis: 12*300000 + 11*p.ToolMillis, ProviderInputBytes: 12 * p.ModelInputBytes, ExternalCalls: 11, EgressBytes: 11 * p.ToolEgressBytes}
	p.Mandate, p.Conversation = p.Task, p.Task
	p.Day = budget.Units{ModelTurns: experienceNativeMaximum, Tokens: experienceNativeMaximum * 1081344, ActiveMillis: experienceNativeMaximum*300000 + 53*11*p.ToolMillis, ProviderInputBytes: experienceNativeMaximum * p.ModelInputBytes, ExternalCalls: 53 * 11, EgressBytes: 53 * 11 * p.ToolEgressBytes}
	return p
}

func experienceNativeSourcePolicy() budget.Policy {
	p := experienceNativeBudgetPolicy()
	// The source task and its root mandate/conversation also own their one-use
	// note job. Declare 12 foreground + 1 note before the first reservation;
	// strict existing-scope limits are never enlarged after source execution.
	u := p.Task
	u.ModelTurns++
	u.Tokens += 1081344
	u.ActiveMillis += 300000
	u.ProviderInputBytes += p.ModelInputBytes
	p.Task, p.Mandate, p.Conversation = u, u, u
	return p
}

type experienceNativeEntry struct {
	Sequence    int                       `json:"sequence"`
	Slot        string                    `json:"slot"`
	RunID       string                    `json:"run_id"`
	Operation   string                    `json:"operation_id"`
	Reservation string                    `json:"reservation_id"`
	Kind        string                    `json:"kind"`
	Request     json.RawMessage           `json:"request"`
	Events      []json.RawMessage         `json:"events"`
	Usage       *transport.Usage          `json:"usage"`
	Millis      *int64                    `json:"milliseconds"`
	Failure     string                    `json:"failure"`
	Canceled    string                    `json:"context_end"`
	Finished    bool                      `json:"finished"`
	Stages      []transport.ProviderStage `json:"provider_stages"`
}

// This is a capture/admission wrapper, never answering logic. A durable private
// entry exists before crossing into the real provider. It does not claim that a
// reserved method entry reached HTTP. Missing provider usage remains null.
type experienceNativeProvider struct {
	transport.ModelProvider
	mu                  sync.Mutex
	proof               string
	slots               []experienceNativeSlot
	active              int
	maximum             int
	entries             []*experienceNativeEntry
	failed              error
	ongoing             bool
	ended               chan struct{}
	global              *pgxpool.Pool
	window              string
	casePool            *pgxpool.Pool
	controlled          bool
	owner               string
	started             time.Time
	executionSlots      int
	executionConfigured bool
}

func (p *experienceNativeProvider) configureExecutionSlots(selected int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, err := experienceNativeExecutionLimit(selected); err != nil {
		return err
	}
	if p.executionConfigured || p.active != -1 || p.ongoing || len(p.entries) != 0 || p.global != nil || p.ModelProvider != nil {
		return errors.New("execution scope must precede storage, provider and source work")
	}
	p.executionSlots = selected
	p.executionConfigured = true
	if selected != 0 {
		return p.writeNew("execution-scope.json", map[string]any{"execution_slots": selected, "full_denominator": len(p.slots), "intentional_omissions": len(p.slots) - selected})
	}
	return nil
}

func newExperienceNativeProvider(proof string, slots []experienceNativeSlot, owners ...string) (*experienceNativeProvider, error) {
	maximum := 0
	for _, slot := range slots {
		maximum += slot.Maximum
	}
	owner := experienceNativeOwner
	if len(owners) > 1 {
		return nil, errors.New("one frozen window owner required")
	}
	if len(owners) == 1 {
		owner = owners[0]
	}
	if !filepath.IsAbs(proof) || len(slots) != 57 || maximum != experienceNativeMaximum && maximum != guided115Maximum || owner == "" || len(owner) > 160 {
		return nil, errors.New("full finite experience schedule required")
	}
	p := &experienceNativeProvider{proof: proof, slots: append([]experienceNativeSlot(nil), slots...), active: -1, maximum: maximum, owner: owner, started: time.Now().UTC()}
	// This must precede PG migrations, credentials, provider construction and
	// original-source work. The exact denominator survives a setup abort.
	if err := p.writeNew("schedule.json", map[string]any{"format": "experience90-window-v1", "owner_id": owner, "maximum_entries": p.maximum, "slots": slots}); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *experienceNativeProvider) writeNew(name string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(p.proof, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err = f.Write(append(raw, '\n')); err != nil {
		return err
	}
	return f.Sync()
}

// The owning global journal is outside every disposable task-data clone and
// survives recipient/template resets. Allocations are disjoint experiment
// partitions, not a claim of global production authority across cloned stores.
func (p *experienceNativeProvider) installGlobal(ctx context.Context, pool *pgxpool.Pool, window string) error {
	if pool == nil || len(window) != 64 || p.global != nil {
		return errors.New("one persistent independent window journal required")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('experience90-independent-global-window',0))`); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS experience_native_windows(window_id TEXT PRIMARY KEY,owner_id TEXT NOT NULL,maximum INTEGER NOT NULL CHECK(maximum IN(640,1276)),used INTEGER NOT NULL DEFAULT 0,closed BOOLEAN NOT NULL DEFAULT FALSE);
CREATE TABLE IF NOT EXISTS experience_native_allocations(window_id TEXT NOT NULL REFERENCES experience_native_windows,slot_id TEXT NOT NULL,maximum INTEGER NOT NULL CHECK(maximum IN(1,12,24)),used INTEGER NOT NULL DEFAULT 0,PRIMARY KEY(window_id,slot_id));
CREATE TABLE IF NOT EXISTS experience_native_entries(window_id TEXT NOT NULL REFERENCES experience_native_windows,sequence INTEGER NOT NULL,slot_id TEXT NOT NULL,run_id TEXT NOT NULL,kind TEXT NOT NULL,operation_id TEXT NOT NULL,reservation_id TEXT NOT NULL,request_json JSONB NOT NULL,usage_json JSONB,PRIMARY KEY(window_id,sequence))`); err != nil {
		return err
	}
	var previous json.RawMessage
	if err = tx.QueryRow(ctx, `SELECT jsonb_build_object('windows',COALESCE((SELECT jsonb_agg(to_jsonb(w) ORDER BY window_id) FROM experience_native_windows w),'[]'::jsonb),'entries',COALESCE((SELECT jsonb_agg(jsonb_build_object('window_id',e.window_id,'sequence',e.sequence,'slot',e.slot_id,'run_id',e.run_id,'kind',e.kind,'operation_id',e.operation_id,'reservation_id',e.reservation_id,'request_sha256',encode(sha256(convert_to(e.request_json::text,'UTF8')),'hex'),'usage',e.usage_json) ORDER BY e.window_id,e.sequence) FROM experience_native_entries e),'[]'::jsonb))`).Scan(&previous); err != nil {
		return err
	}
	var open int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM experience_native_windows WHERE NOT closed`).Scan(&open); err != nil || open != 0 {
		return errors.New("an earlier independent window is still open; retain and reconcile it first")
	}
	if err = p.writeNew("previous-windows.json", previous); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO experience_native_windows(window_id,owner_id,maximum)VALUES($1,$2,$3)`, window, p.owner, p.maximum); err != nil {
		return errors.New("one-use persistent window was used or unavailable")
	}
	for _, slot := range p.slots {
		if _, err = tx.Exec(ctx, `INSERT INTO experience_native_allocations(window_id,slot_id,maximum)VALUES($1,$2,$3)`, window, slot.ID, slot.Maximum); err != nil {
			return err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	p.global, p.window = pool, window
	return nil
}

func (p *experienceNativeProvider) reserveGlobal(ctx context.Context, e *experienceNativeEntry) error {
	if p.global == nil {
		return nil // synthetic capture unit tests only; native entry requires it
	}
	tx, err := p.global.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var sequence int
	if err = tx.QueryRow(ctx, `UPDATE experience_native_windows SET used=used+1 WHERE window_id=$1 AND owner_id=$2 AND NOT closed AND used<maximum RETURNING used`, p.window, p.owner).Scan(&sequence); err != nil || sequence != e.Sequence {
		return errors.New("persistent window sequence/bound denied")
	}
	tag, err := tx.Exec(ctx, `UPDATE experience_native_allocations SET used=used+1 WHERE window_id=$1 AND slot_id=$2 AND used<maximum`, p.window, e.Slot)
	if err != nil || tag.RowsAffected() != 1 {
		return errors.New("persistent original allocation exhausted")
	}
	if _, err = tx.Exec(ctx, `INSERT INTO experience_native_entries(window_id,sequence,slot_id,run_id,kind,operation_id,reservation_id,request_json)VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, p.window, e.Sequence, e.Slot, e.RunID, e.Kind, e.Operation, e.Reservation, e.Request); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (p *experienceNativeProvider) begin(index int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.failed != nil || p.active != -1 || p.ongoing || index < 0 || index >= len(p.slots) || p.executionSlots != 0 && index >= p.executionSlots || p.slots[index].Status != "not_executed" {
		return errors.New("one-use single foreground slot required")
	}
	p.active = index
	p.slots[index].Status, p.slots[index].Blocker = "running", ""
	now := time.Now().UTC()
	p.slots[index].Started = &now
	return p.writeNew(fmt.Sprintf("slot-%03d-start.json", index+1), p.slots[index])
}

func (p *experienceNativeProvider) end(status, blocker string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.active < 0 || p.ongoing || status != "completed" && status != "failed" {
		return errors.New("active complete provider stream required")
	}
	i := p.active
	p.slots[i].Status, p.slots[i].Blocker = status, blocker
	now := time.Now().UTC()
	p.slots[i].Finished = &now
	p.active = -1
	return p.writeNew(fmt.Sprintf("slot-%03d-end.json", i+1), p.slots[i])
}

func (p *experienceNativeProvider) reserve(ctx context.Context, run, kind string, request any) (*experienceNativeEntry, error) {
	// Flow may submit after a terminal tool proposal before the previous
	// capture has observed transport EOF. Join bookkeeping before another entry;
	// never race its usage/final capture or replace the previous request.
	p.mu.Lock()
	prior := p.ended
	p.mu.Unlock()
	if prior != nil {
		select {
		case <-prior:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.failed != nil || p.active < 0 || p.executionSlots != 0 && p.active >= p.executionSlots || p.ongoing || p.ModelProvider == nil || len(p.entries) >= p.maximum {
		return nil, errors.New("prospective provider window closed")
	}
	slot, used := p.slots[p.active], 0
	op, admitted := budget.ModelOperationFromContext(ctx)
	reservation := ""
	if !p.controlled {
		if p.global == nil || p.casePool == nil || !admitted || op == "" {
			return nil, errors.New("native entry lacks independent journal or owning admitted operation")
		}
		var err error
		if slot.Kind == "note" {
			err = p.casePool.QueryRow(ctx, `SELECT a.reservation_id FROM memory_model_attempts a JOIN execution_budget_reservations r USING(reservation_id) WHERE a.operation_id=$1 AND a.operation_id=$2 AND a.state='reserved' AND a.provider='antigravity' AND a.model='gemini-3.8-flash-high' AND r.owner_id=$3`, op, run, p.owner).Scan(&reservation)
		} else {
			err = p.casePool.QueryRow(ctx, `SELECT a.reservation_id FROM execution_model_attempts a JOIN execution_budget_reservations r USING(reservation_id) WHERE a.operation_id=$1 AND a.run_id=$2 AND a.provider='antigravity' AND a.model='gemini-3.8-flash-high' AND r.owner_id=$3`, op, run, p.owner).Scan(&reservation)
		}
		if err != nil || reservation == "" {
			return nil, errors.New("current owning model reservation not corroborated")
		}
	}
	for _, entry := range p.entries {
		if entry.Slot == slot.ID {
			used++
		}
	}
	if used >= slot.Maximum {
		return nil, errors.New("original slot finite provider entries exhausted")
	}
	raw, err := json.Marshal(request)
	if err != nil || len(raw) > 8388608 {
		return nil, errors.New("bounded original request capture rejected")
	}
	e := &experienceNativeEntry{Sequence: len(p.entries) + 1, Slot: slot.ID, RunID: run, Operation: op, Reservation: reservation, Kind: kind, Request: raw, Events: []json.RawMessage{}}
	if err = p.reserveGlobal(ctx, e); err != nil {
		p.failed = err
		return nil, err
	}
	if err = p.writeNew(fmt.Sprintf("entry-%03d-start.json", e.Sequence), e); err != nil {
		p.failed = err
		return nil, err
	}
	p.entries, p.ongoing = append(p.entries, e), true
	p.ended = make(chan struct{})
	return e, nil
}

type experienceNativeEvent struct {
	EventType      transport.EventType        `json:"event_type"`
	RunID          string                     `json:"run_id"`
	AssistantDelta *transport.AssistantDelta  `json:"assistant_delta,omitempty"`
	AssistantFinal *transport.AssistantFinal  `json:"assistant_final,omitempty"`
	ToolCall       *transport.ToolCallRequest `json:"tool_call,omitempty"`
	ToolCallBatch  *transport.ToolCallBatch   `json:"tool_call_batch,omitempty"`
	Usage          *transport.Usage           `json:"usage"`
	ModelVersion   *string                    `json:"model_version,omitempty"`
	ErrorCode      string                     `json:"error_code,omitempty"`
	ErrorReason    string                     `json:"error_reason,omitempty"`
	HTTPStatus     int                        `json:"http_status,omitempty"`
}

func experienceNativeSafeEvent(e transport.TransportEvent) experienceNativeEvent {
	r := experienceNativeEvent{EventType: e.EventType, RunID: e.RunID}
	// Only declared model output, proposals and measured usage are evidence.
	// Provider error details, arbitrary PayloadJSON and session/auth metadata
	// are never copied, even when attached to an otherwise ordinary event.
	switch e.EventType {
	case transport.EventTypeAssistantDelta:
		r.AssistantDelta = e.AssistantDelta
	case transport.EventTypeAssistantFinal:
		r.AssistantFinal = e.AssistantFinal
	case transport.EventTypeToolCallRequested:
		r.ToolCall = e.ToolCall
	case transport.EventTypeToolCallBatchRequested:
		r.ToolCallBatch = e.ToolCallBatch
	}
	if e.Usage != nil && e.Usage.Validate() == nil {
		r.Usage = e.Usage
	}
	if r.Usage == nil && r.AssistantFinal != nil && r.AssistantFinal.Usage != nil && r.AssistantFinal.Usage.Validate() == nil {
		r.Usage = r.AssistantFinal.Usage
	}
	r.ModelVersion = e.ProviderModelVersion
	if e.TransportError != nil {
		r.ErrorCode = transport.DiagnosticCode(e.TransportError)
		r.ErrorReason = transport.DiagnosticReason(e.TransportError)
		r.HTTPStatus = transport.DiagnosticHTTPStatus(e.TransportError)
	}
	return r
}

func (p *experienceNativeProvider) observe(ctx context.Context, e *experienceNativeEntry, started time.Time, in transport.EventStream, providerErr error) (transport.EventStream, error) {
	finish := func(failure string) {
		p.mu.Lock()
		defer p.mu.Unlock()
		elapsed := time.Since(started).Milliseconds()
		e.Millis, e.Finished, e.Failure = &elapsed, true, failure
		if ctx.Err() != nil {
			e.Canceled = "context_canceled_or_expired"
		}
		if err := p.writeNew(fmt.Sprintf("entry-%03d-end.json", e.Sequence), e); err != nil {
			p.failed = err
		}
		if p.global != nil && e.Usage != nil {
			retained, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			raw, _ := json.Marshal(e.Usage)
			if _, err := p.global.Exec(retained, `UPDATE experience_native_entries SET usage_json=$3 WHERE window_id=$1 AND sequence=$2 AND usage_json IS NULL`, p.window, e.Sequence, raw); err != nil {
				p.failed = err
			}
			cancel()
		}
		p.ongoing = false
		close(p.ended)
	}
	if providerErr != nil {
		reason := transport.DiagnosticCode(providerErr)
		if reason == "" {
			reason = "provider_method_error_unknown"
		}
		finish(reason)
		return nil, providerErr
	}
	if in == nil {
		finish("nil_provider_stream")
		return nil, errors.New("provider stream absent")
	}
	out := make(chan transport.TransportEvent, 64)
	go func() {
		failure := ""
		captured := 0
		terminal := false
		defer close(out)
		defer func() { finish(failure) }()
		for {
			select {
			case <-ctx.Done():
				p.mu.Lock()
				e.Canceled = "context_canceled_or_expired"
				p.mu.Unlock()
				if !terminal {
					failure = "context_canceled_or_expired"
				}
				return
			case event, ok := <-in:
				if !ok {
					return
				}
				safe := experienceNativeSafeEvent(event)
				raw, err := json.Marshal(safe)
				captured += len(raw)
				if err != nil || len(raw) > 8*1024*1024 || captured > 32*1024*1024 {
					failure = "provider_event_capture_rejected"
					return
				}
				p.mu.Lock()
				e.Events = append(e.Events, raw)
				if safe.Usage != nil {
					u := *safe.Usage
					e.Usage = &u
				}
				if event.TransportError != nil {
					failure = transport.DiagnosticCode(event.TransportError)
				}
				terminal = terminal || event.EventType == transport.EventTypeAssistantFinal || event.EventType == transport.EventTypeToolCallRequested || event.EventType == transport.EventTypeToolCallBatchRequested
				p.mu.Unlock()
				select {
				case out <- event:
				case <-ctx.Done():
					failure = "context_canceled_or_expired"
					return
				}
			}
		}
	}()
	return out, nil
}

func (p *experienceNativeProvider) StartRun(ctx context.Context, request transport.StartRunRequest) (transport.EventStream, error) {
	e, err := p.reserve(ctx, request.Context.RunID, "start", request)
	if err != nil {
		return nil, err
	}
	start := time.Now()
	ctx = p.stageContext(ctx, e)
	stream, err := p.ModelProvider.StartRun(ctx, request)
	return p.observe(ctx, e, start, stream, err)
}
func (p *experienceNativeProvider) SubmitToolResult(ctx context.Context, request transport.SubmitToolResultRequest) (transport.EventStream, error) {
	e, err := p.reserve(ctx, request.RunID, "tool_result", request)
	if err != nil {
		return nil, err
	}
	start := time.Now()
	ctx = p.stageContext(ctx, e)
	stream, err := p.ModelProvider.SubmitToolResult(ctx, request)
	return p.observe(ctx, e, start, stream, err)
}
func (p *experienceNativeProvider) ContinueRun(ctx context.Context, request transport.ContinueRunRequest) (transport.EventStream, error) {
	e, err := p.reserve(ctx, request.RunID, "continuation", request)
	if err != nil {
		return nil, err
	}
	start := time.Now()
	ctx = p.stageContext(ctx, e)
	stream, err := p.ModelProvider.ContinueRun(ctx, request)
	return p.observe(ctx, e, start, stream, err)
}

func (p *experienceNativeProvider) stageContext(ctx context.Context, e *experienceNativeEntry) context.Context {
	original := ctx
	return transport.WithProviderStageObserver(ctx, func(stage transport.ProviderStage) {
		transport.ObserveProviderStage(original, stage)
		p.mu.Lock()
		defer p.mu.Unlock()
		if len(e.Stages) < 32 {
			e.Stages = append(e.Stages, stage)
		}
	})
}
func (p *experienceNativeProvider) InferenceLimits() (int64, int64) {
	return p.ModelProvider.(interface{ InferenceLimits() (int64, int64) }).InferenceLimits()
}
func (p *experienceNativeProvider) InferenceTimeout() time.Duration {
	return p.ModelProvider.(interface{ InferenceTimeout() time.Duration }).InferenceTimeout()
}

func (p *experienceNativeProvider) export() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	// Snapshot JSON owns every byte; a later transport EOF cannot mutate the
	// recorded original request or result. A failed slot is never replaced.
	for i := range p.slots {
		if p.slots[i].Status == "running" {
			p.slots[i].Status, p.slots[i].Blocker = "failed", "collector_aborted_before_slot_sealed"
			now := time.Now().UTC()
			p.slots[i].Finished = &now
		}
		if p.slots[i].Status == "not_executed" {
			p.slots[i].Blocker = "window_closed_before_execution"
			if p.executionSlots != 0 && i >= p.executionSlots {
				p.slots[i].Blocker = "predeclared_short_smoke_omission"
			}
		}
	}
	window := map[string]any{"slots": p.slots, "entries": p.entries, "maximum_entries": p.maximum, "capture_error": p.failed != nil, "active_slot": p.active, "started_at": p.started, "finished_at": time.Now().UTC(), "elapsed_millis": time.Since(p.started).Milliseconds()}
	if p.executionSlots != 0 {
		window["execution_slots"] = p.executionSlots
	}
	err := p.writeNew("window-final.json", window)
	if p.global != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, closeErr := p.global.Exec(ctx, `UPDATE experience_native_windows SET closed=TRUE WHERE window_id=$1`, p.window)
		if err == nil {
			err = closeErr
		}
		if exportErr := p.exportGlobal(ctx); err == nil {
			err = exportErr
		}
	}
	return err
}

func (p *experienceNativeProvider) exportGlobal(ctx context.Context) error {
	out := map[string]json.RawMessage{}
	for _, table := range []string{"experience_native_windows", "experience_native_allocations", "experience_native_entries", "experience_native_templates", "experience_native_case_databases", "experience_native_history"} {
		var exists bool
		if err := p.global.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, table).Scan(&exists); err != nil {
			return errors.New("global journal export availability failed")
		}
		raw := json.RawMessage(`[]`)
		if exists {
			if err := p.global.QueryRow(ctx, `SELECT COALESCE(jsonb_agg(to_jsonb(t)),'[]'::jsonb) FROM `+table+` t WHERE window_id=$1`, p.window).Scan(&raw); err != nil {
				return errors.New("complete private global journal export failed")
			}
		}
		out[table] = raw
	}
	return p.writeNew("global-journal-final.json", out)
}

// A local synthetic cohort proves mechanics without importing a holdout rubric
// or pretending controlled output is native task/lesson quality.
func experienceWindowFixture() experienceNativeCorpus {
	var c experienceNativeCorpus
	c.Originals.Seed, c.Controls.Seed, c.Controls.Repetitions = 90, 90, 2
	c.Controls.Arms = append([]string(nil), experienceNativeArms...)
	for i := 0; i < 8; i++ {
		v := experienceNativeTask{ID: fmt.Sprintf("current-%02d", i+1), Resource: "alpha", EnvironmentProfile: "fixture-v1", Reset: "fresh", Instruction: "Observe current data", PermissionProfile: "bounded-current"}
		c.Controls.Cases = append(c.Controls.Cases, v)
		if i < 4 {
			c.Originals.Tasks = append(c.Originals.Tasks, v)
		}
	}
	return c
}

func TestExperienceNativeScheduleKeepsDenominatorAndFiniteQuotes(t *testing.T) {
	c := experienceWindowFixture()
	slots := experienceNativeSchedule(c)
	p, err := newExperienceNativeProvider(t.TempDir(), slots)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = newExperienceNativeProvider(p.proof, slots); err == nil {
		t.Fatal("used window reopened")
	}
	counts, total := map[string]int{}, 0
	for _, slot := range slots {
		counts[slot.Kind]++
		total += slot.Maximum
	}
	if total != 640 || counts["original"] != 4 || counts["note"] != 4 || counts["skill_preparation"] != 1 || counts["control"] != 48 || slots[9].Arm != experienceNativeArms[0] || slots[10].Arm != experienceNativeArms[1] || slots[11].Arm != experienceNativeArms[2] || slots[33].Arm != experienceNativeArms[2] || slots[34].Arm != experienceNativeArms[1] || slots[35].Arm != experienceNativeArms[0] {
		t.Fatal("prospective population/order changed", counts, total)
	}
	if err = p.export(); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(p.proof, "window-final.json"))
	if bytes.Count(raw, []byte(`"status": "not_executed"`)) != 57 || bytes.Count(raw, []byte(`"window_closed_before_execution"`)) != 57 {
		t.Fatal("setup abort hid unexecuted denominator")
	}
	policy := experienceNativeBudgetPolicy()
	if policy.Validate() != nil || policy.Day.ModelTurns != int64(total) || policy.Task.ModelTurns != 12 || policy.OutputTokens != 32768 || policy.ModelMillis != 300000 || policy.Task.ActiveMillis < 12*300000+11*policy.ToolMillis {
		t.Fatal("prospective admission cannot cover declared entries")
	}
	source := experienceNativeSourcePolicy()
	if source.Validate() != nil || source.Task.ModelTurns != 13 || source.Mandate.ModelTurns != 13 || source.Conversation.ModelTurns != 13 || source.Day.ModelTurns != 640 {
		t.Fatal("source/note parent quotes do not fit their prospective policy")
	}
}

func TestExperienceNativeShortSmokeGuardsAndRetainsOmissions(t *testing.T) {
	slots := experienceNativeSchedule(experienceWindowFixture())
	legacy, err := newExperienceNativeProvider(t.TempDir(), slots)
	if err != nil || legacy.begin(33) != nil || legacy.end("failed", "controlled metadata transition; no provider") != nil {
		t.Fatal("legacy full57 no longer permits repetition2", err)
	}
	p, err := newExperienceNativeProvider(t.TempDir(), slots)
	if err != nil || p.configureExecutionSlots(33) != nil {
		t.Fatal("prospective short scope rejected", err)
	}
	if p.configureExecutionSlots(0) == nil {
		t.Fatal("sealed short scope expanded")
	}
	seen := map[string]bool{}
	for _, slot := range slots[9:33] {
		key := slot.CaseID + "/" + slot.Arm
		if slot.Kind != "control" || slot.Repetition != 1 || seen[key] {
			t.Fatal("short prefix lost an independent case/arm")
		}
		seen[key] = true
	}
	if len(seen) != 24 || p.begin(32) != nil || p.end("failed", "controlled boundary transition; no provider") != nil || p.begin(33) == nil || p.slots[33].Started != nil {
		t.Fatal("short boundary admitted repetition2 or omitted a first-repetition arm")
	}
	if p.export() != nil {
		t.Fatal("short whole-denominator export failed")
	}
	raw, _ := os.ReadFile(filepath.Join(p.proof, "window-final.json"))
	var window struct {
		Slots          []experienceNativeSlot
		ExecutionSlots int `json:"execution_slots"`
	}
	if json.Unmarshal(raw, &window) != nil || len(window.Slots) != 57 || window.ExecutionSlots != 33 || bytes.Count(raw, []byte(`"predeclared_short_smoke_omission"`)) != 24 {
		t.Fatal("intentional omissions disappeared from full57 history")
	}
	for _, slot := range window.Slots[33:] {
		if slot.Repetition != 2 || slot.Status != "not_executed" || slot.Started != nil || slot.Finished != nil {
			t.Fatal("omitted repetition was relabeled as executed")
		}
	}
}

type experienceWindowFailureProvider struct{ transport.ModelProvider }

func (*experienceWindowFailureProvider) Name() string { return "controlled-no-network" }
func (*experienceWindowFailureProvider) StartRun(context.Context, transport.StartRunRequest) (transport.EventStream, error) {
	return nil, errors.New("controlled admission/provider failure; no inference")
}

func TestExperienceNativeCaptureRetainsUnknownAndExhaustion(t *testing.T) {
	p, err := newExperienceNativeProvider(t.TempDir(), experienceNativeSchedule(experienceWindowFixture()))
	if err != nil {
		t.Fatal(err)
	}
	p.ModelProvider = &experienceWindowFailureProvider{}
	p.controlled = true
	if p.begin(4) != nil { // note slot: one actual entry, even if usage unknown
		t.Fatal("note slot rejected")
	}
	request := transport.StartRunRequest{}
	_, err = p.StartRun(context.Background(), request)
	if err == nil || len(p.entries) != 1 || p.entries[0].Usage != nil || !p.entries[0].Finished {
		t.Fatal("failed original disappeared or unknown usage became zero")
	}
	if _, err = p.StartRun(context.Background(), request); err == nil || len(p.entries) != 1 {
		t.Fatal("failed note received hidden replacement attempt")
	}
	if p.end("failed", "controlled_failure") != nil || p.begin(4) == nil || p.export() != nil {
		t.Fatal("failed one-use slot was replaceable")
	}
	f, err := os.Open(filepath.Join(p.proof, "entry-001-end.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	raw, _ := io.ReadAll(f)
	if !bytes.Contains(raw, []byte(`"usage": null`)) {
		t.Fatal("unobserved spend was normalized")
	}
}

type experienceWindowStageProvider struct{ transport.ModelProvider }

func (*experienceWindowStageProvider) Name() string { return "controlled-no-network" }
func (*experienceWindowStageProvider) StartRun(ctx context.Context, _ transport.StartRunRequest) (transport.EventStream, error) {
	transport.ObserveProviderStage(ctx, transport.ProviderStageAuthentication)
	transport.ObserveProviderStage(ctx, transport.ProviderStage("private-unsupported-stage"))
	transport.ObserveProviderStage(ctx, transport.ProviderStageHTTP)
	return nil, ctx.Err()
}
func (p *experienceWindowStageProvider) SubmitToolResult(ctx context.Context, _ transport.SubmitToolResultRequest) (transport.EventStream, error) {
	return p.StartRun(ctx, transport.StartRunRequest{})
}
func (p *experienceWindowStageProvider) ContinueRun(ctx context.Context, _ transport.ContinueRunRequest) (transport.EventStream, error) {
	return p.StartRun(ctx, transport.StartRunRequest{})
}

func TestExperienceNativeStageCaptureKeepsCancellationAndUnknownUsage(t *testing.T) {
	for _, method := range []string{"start", "tool_result", "continuation"} {
		t.Run(method, func(t *testing.T) {
			p, err := newExperienceNativeProvider(t.TempDir(), experienceNativeSchedule(experienceWindowFixture()))
			if err != nil {
				t.Fatal(err)
			}
			p.ModelProvider, p.controlled = &experienceWindowStageProvider{}, true
			if p.begin(0) != nil {
				t.Fatal("controlled stage slot unavailable")
			}
			var forwarded []transport.ProviderStage
			ctx, cancel := context.WithCancel(transport.WithProviderStageObserver(context.Background(), func(s transport.ProviderStage) { forwarded = append(forwarded, s) }))
			cancel()
			switch method {
			case "start":
				_, err = p.StartRun(ctx, transport.StartRunRequest{})
			case "tool_result":
				_, err = p.SubmitToolResult(ctx, transport.SubmitToolResultRequest{})
			case "continuation":
				_, err = p.ContinueRun(ctx, transport.ContinueRunRequest{})
			}
			if !errors.Is(err, context.Canceled) || len(p.entries) != 1 {
				t.Fatal("original canceled entry disappeared")
			}
			e := p.entries[0]
			if !e.Finished || e.Usage != nil || e.Canceled != "context_canceled_or_expired" || e.Failure != "provider_method_error_unknown" || len(e.Stages) != 2 || len(forwarded) != 2 || e.Stages[0] != transport.ProviderStageAuthentication || e.Stages[1] != transport.ProviderStageHTTP {
				t.Fatal("stages invented usage/provider failure or hid cancellation", e)
			}
			if p.end("failed", "controlled_cancel") != nil || p.export() != nil {
				t.Fatal("canceled original was not retained")
			}
			raw, err := os.ReadFile(filepath.Join(p.proof, "window-final.json"))
			if err != nil || bytes.Contains(raw, []byte("private-unsupported-stage")) || !bytes.Contains(raw, []byte(`"usage": null`)) || bytes.Count(raw, []byte(`"status": "not_executed"`)) != 56 {
				t.Fatal("canceled original changed the full denominator or leaked diagnostics")
			}
		})
	}
}

func TestExperienceNativeAbortRetainsGlobalPartitionIntegration(t *testing.T) {
	p, err := newExperienceNativeProvider(t.TempDir(), experienceNativeSchedule(experienceWindowFixture()))
	if err != nil {
		t.Fatal(err)
	}
	ctx, stop := context.WithTimeout(context.Background(), 15*time.Second)
	defer stop()
	global := memorytest.Database(t)
	if err = p.installGlobal(ctx, global, experienceDigest(t.Name()+time.Now().String())); err != nil {
		t.Fatal(err)
	}
	p.ModelProvider, p.controlled = &experienceWindowFailureProvider{}, true
	if p.begin(4) != nil {
		t.Fatal("controlled original note unavailable")
	}
	if _, err = p.StartRun(ctx, transport.StartRunRequest{}); err == nil || p.end("failed", "controlled_abort") != nil || p.export() != nil {
		t.Fatal("failed original or journal closure was lost")
	}
	var used, allocations, maximum int
	var closed bool
	if global.QueryRow(ctx, `SELECT used,closed,(SELECT count(*) FROM experience_native_allocations WHERE window_id=$1),(SELECT sum(maximum) FROM experience_native_allocations WHERE window_id=$1) FROM experience_native_windows WHERE window_id=$1`, p.window).Scan(&used, &closed, &allocations, &maximum) != nil || used != 1 || !closed || allocations != 57 || maximum != 640 {
		t.Fatal("abort refunded spend or changed the prospective partition")
	}
	raw, err := os.ReadFile(filepath.Join(p.proof, "global-journal-final.json"))
	var exported map[string][]json.RawMessage
	if err != nil || json.Unmarshal(raw, &exported) != nil || len(exported["experience_native_allocations"]) != 57 || len(exported["experience_native_entries"]) != 1 || !bytes.Contains(exported["experience_native_entries"][0], []byte(`"usage_json": null`)) {
		t.Fatal("abort export omitted actual unknown spend")
	}
}

func TestExperienceNativePersistentPartitionsDoNotResetIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx := context.Background()
	p, err := newExperienceNativeProvider(t.TempDir(), experienceNativeSchedule(experienceWindowFixture()))
	if err != nil {
		t.Fatal(err)
	}
	p.ModelProvider = &experienceWindowFailureProvider{}
	p.controlled = true
	window := strings.Repeat("a", 64)
	if p.installGlobal(ctx, pool, window) != nil || p.begin(4) != nil {
		t.Fatal("persistent one-use preparation denied")
	}
	if _, err = p.StartRun(ctx, transport.StartRunRequest{}); err == nil {
		t.Fatal("controlled failure disappeared")
	}
	var used, count int
	var usage json.RawMessage
	if pool.QueryRow(ctx, `SELECT used FROM experience_native_windows WHERE window_id=$1`, window).Scan(&used) != nil || used != 1 || pool.QueryRow(ctx, `SELECT count(*) FROM experience_native_allocations WHERE window_id=$1`, window).Scan(&count) != nil || count != 57 || pool.QueryRow(ctx, `SELECT usage_json FROM experience_native_entries WHERE window_id=$1 AND sequence=1`, window).Scan(&usage) != nil || usage != nil {
		t.Fatal("global/partition history or NULL usage lost")
	}
	other, err := newExperienceNativeProvider(t.TempDir(), experienceNativeSchedule(experienceWindowFixture()))
	if err != nil || other.installGlobal(ctx, pool, window) == nil {
		t.Fatal("fresh clone/proof reset persistent original window")
	}
	if p.end("failed", "controlled_original_failure") != nil || p.export() != nil {
		t.Fatal("persistent failed window export denied")
	}
	if pool.QueryRow(ctx, `SELECT count(*) FROM experience_native_entries WHERE window_id=$1 AND usage_json IS NULL`, window).Scan(&count) != nil || count != 1 {
		t.Fatal("closed window erased unknown original")
	}
}

type experienceWindowEventProvider struct {
	transport.ModelProvider
	events []transport.TransportEvent
}

func (*experienceWindowEventProvider) Name() string { return "controlled-no-network" }
func (p *experienceWindowEventProvider) StartRun(context.Context, transport.StartRunRequest) (transport.EventStream, error) {
	ch := make(chan transport.TransportEvent, len(p.events))
	for _, event := range p.events {
		ch <- event
	}
	close(ch)
	return ch, nil
}

func TestExperienceNativeErrorCaptureOmitsProviderSecretsAndRetainsFinalUsage(t *testing.T) {
	p, err := newExperienceNativeProvider(t.TempDir(), experienceNativeSchedule(experienceWindowFixture()))
	if err != nil {
		t.Fatal(err)
	}
	const canary = "fake-provider-private-credential-canary-never-export"
	p.controlled = true
	failure := transport.NewTransportErrorEvent("run", "controlled", &transport.Error{Type: transport.ErrorTypeProviderUnavailable, Message: canary, ProviderCode: canary, ProviderDetails: map[string]any{"access_token": canary}, HTTPStatus: 503})
	final := transport.NewAssistantFinalEvent("run", "controlled", transport.AssistantFinal{Content: "Observed current data", Usage: &transport.Usage{InputTokens: 8, OutputTokens: 3, TotalTokens: 19}})
	final.Usage = nil // valid legacy final-only usage must still be captured
	p.ModelProvider = &experienceWindowEventProvider{events: []transport.TransportEvent{failure, final}}
	if p.begin(0) != nil {
		t.Fatal("slot rejected")
	}
	stream, err := p.StartRun(context.Background(), transport.StartRunRequest{})
	if err != nil {
		t.Fatal(err)
	}
	for range stream {
	}
	if p.end("failed", "controlled_provider_error") != nil || p.export() != nil {
		t.Fatal("capture export denied")
	}
	files, _ := filepath.Glob(filepath.Join(p.proof, "*.json"))
	for _, path := range files {
		raw, _ := os.ReadFile(path)
		if bytes.Contains(raw, []byte(canary)) || bytes.Contains(raw, []byte("ProviderDetails")) || bytes.Contains(raw, []byte("PayloadJSON")) {
			t.Fatal("raw provider credential/error exported", filepath.Base(path))
		}
	}
	if p.entries[0].Usage == nil || p.entries[0].Usage.TotalTokens != 19 || p.entries[0].Failure != string(transport.ErrorTypeProviderUnavailable) {
		t.Fatal("complete observed usage/error vocabulary lost")
	}
}

func TestExperienceNativeRequiresOwningReservationBeforeGlobalEntryIntegration(t *testing.T) {
	f := newDispatchAdmissionFixture(t)
	global := memorytest.Database(t)
	p, err := newExperienceNativeProvider(t.TempDir(), experienceNativeSchedule(experienceWindowFixture()), f.f.parent.OwnerID)
	if err != nil {
		t.Fatal(err)
	}
	p.ModelProvider = &experienceWindowFailureProvider{}
	p.casePool = f.f.pool
	if p.installGlobal(f.f.ctx, global, strings.Repeat("b", 64)) != nil || p.begin(0) != nil {
		t.Fatal("window rejected")
	}
	if _, err = p.StartRun(f.owned, transport.StartRunRequest{Context: transport.TransportRunContext{RunID: f.call.RunId}}); err == nil || len(p.entries) != 0 {
		t.Fatal("unadmitted model method entered global window")
	}
	f.f.budgets.Policy.Models["antigravity:gemini-3.8-flash-high"] = budget.ModelPrice{Basis: "subscription_no_overage"}
	f.f.budgets.Policy.OutputTokens = 32768
	admitted, err := f.f.budgets.AdmitModel(f.owned, f.call.RunId, "", "antigravity", "gemini-3.8-flash-high", 1048576)
	if err != nil {
		t.Fatal(err)
	}
	ctx := budget.WithModelOperation(f.owned, admitted.OperationID)
	if _, err = p.StartRun(ctx, transport.StartRunRequest{Context: transport.TransportRunContext{RunID: "different-run"}}); err == nil || len(p.entries) != 0 {
		t.Fatal("foreign reservation entered global window")
	}
	if _, err = p.StartRun(ctx, transport.StartRunRequest{Context: transport.TransportRunContext{RunID: f.call.RunId}}); err == nil || len(p.entries) != 1 || p.entries[0].Operation != admitted.OperationID || p.entries[0].Reservation != admitted.ReservationID {
		t.Fatal("actual admitted operation not captured")
	}
	var operation, reservation string
	if global.QueryRow(ctx, `SELECT operation_id,reservation_id FROM experience_native_entries WHERE window_id=$1 AND sequence=1`, p.window).Scan(&operation, &reservation) != nil || operation != admitted.OperationID || reservation != admitted.ReservationID {
		t.Fatal("exact owning identities missing from durable global journal")
	}
	if p.export() != nil || p.slots[0].Status != "failed" || p.slots[1].Status != "not_executed" {
		t.Fatal("fatal omitted unfinished original")
	}
}
