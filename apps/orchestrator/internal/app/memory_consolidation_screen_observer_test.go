package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5/pgxpool"
)

type consolidationScreenCorpus struct {
	Format                   string `json:"format"`
	CommonCurrentInstruction string `json:"common_current_instruction"`
	Pairs                    int    `json:"pairs"`
	BackgroundJobs           int    `json:"background_jobs"`
	ForegroundInputs         int    `json:"foreground_inputs"`
	MaxActualModelEntries    int    `json:"max_actual_model_entries"`
	SourceSets               []struct {
		ID        string
		Sessions  int
		Documents []struct {
			ID, Body string
			Session  int
		}
		Accepted struct {
			Document, Key, Value string
		} `json:"accepted"`
	} `json:"source_sets"`
	Cases []struct {
		ID             string
		SourceSet      string `json:"source_set"`
		View, Question string
		CurrentTask    struct {
			Goal        string
			Constraints []string
			Scratch     map[string]any
		} `json:"current_task"`
		Before struct {
			Correction struct{ Document, Body string } `json:"ordinary_owner_correction"`
			Supersede  struct{ Value string }          `json:"supersede_accepted_same_key"`
		} `json:"before"`
		Oracle json.RawMessage
	} `json:"cases"`
}

func consolidationScreenOpaque(t *testing.T) string {
	t.Helper()
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(raw[:])
}

type consolidationScreenSlot struct {
	ID, CaseID, Kind, Arm string
	State, Blocker        string
	Session, RunID        string
	Answer                string
	Limit                 int
	StartedAt, FinishedAt time.Time
	CurrentTask           json.RawMessage
}

// The complete nine-slot population is sealed before PostgreSQL/auth/provider
// work. Hidden treatment/rubric identifiers stay only in evaluator artifacts.
func consolidationScreenSchedule(t *testing.T, proof string) (*consolidationScreenCorpus, []*consolidationScreenSlot, []byte) {
	t.Helper()
	raw, err := os.ReadFile("docs/testing/fixtures/issue89/selected-v1.json")
	if os.IsNotExist(err) {
		raw, err = os.ReadFile(filepath.Join("..", "..", "..", "..", "docs", "testing", "fixtures", "issue89", "selected-v1.json"))
	}
	var corpus consolidationScreenCorpus
	if err != nil || json.Unmarshal(raw, &corpus) != nil || len(corpus.Cases) != 3 || len(corpus.SourceSets) != 2 || corpus.ForegroundInputs != 6 || corpus.MaxActualModelEntries != 99 || corpus.Pairs != 3 || corpus.BackgroundJobs != 3 || corpus.CommonCurrentInstruction == "" {
		t.Fatal("frozen three-pair corpus unavailable")
	}
	slots := []*consolidationScreenSlot{}
	for i := range corpus.Cases {
		for _, treatment := range []struct {
			kind, arm string
			limit     int
		}{{"background", "preparation", 1}, {"foreground", "baseline", 16}, {"foreground", "consolidated", 16}} {
			slots = append(slots, &consolidationScreenSlot{ID: consolidationScreenOpaque(t), CaseID: corpus.Cases[i].ID, Kind: treatment.kind, Arm: treatment.arm, State: "not_executed", Blocker: "earlier_stage_not_completed", Limit: treatment.limit})
		}
	}
	if err = os.MkdirAll(proof, 0700); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("BUTLER_CONSOLIDATION_SCREEN_USE_SEALED_SCHEDULE") == "1" {
		var sealed struct {
			Format  string                     `json:"format"`
			Hash    string                     `json:"corpus_sha256"`
			Slots   []*consolidationScreenSlot `json:"slots"`
			Maximum int                        `json:"maximum_actual_entries"`
		}
		data, e := os.ReadFile(filepath.Join(proof, "initial-schedule.json"))
		if e != nil || json.Unmarshal(data, &sealed) != nil || sealed.Format != "consolidation-nine-logical-slots-v1" || sealed.Hash != experienceDigest(string(raw)) || sealed.Maximum != 99 || len(sealed.Slots) != len(slots) {
			t.Fatal("complete pre-auth sealed schedule differs")
		}
		ids := map[string]bool{}
		for i, slot := range sealed.Slots {
			id, e := hex.DecodeString(slot.ID)
			if e != nil || len(id) != 16 || ids[slot.ID] || slot.CaseID != slots[i].CaseID || slot.Kind != slots[i].Kind || slot.Arm != slots[i].Arm || slot.Limit != slots[i].Limit || slot.State != "not_executed" || slot.Blocker != "earlier_stage_not_completed" || slot.RunID != "" || slot.Answer != "" || slot.Session != "" || !slot.StartedAt.IsZero() || !slot.FinishedAt.IsZero() || len(slot.CurrentTask) > 0 && string(slot.CurrentTask) != "null" {
				t.Fatal("pre-auth slot identity/state differs")
			}
			ids[slot.ID] = true
		}
		if e = screenWriteExclusive(filepath.Join(proof, "window-begun.json"), []byte(`{"one_use_execution":true}`)); e != nil {
			t.Fatal("sealed native window already consumed")
		}
		return &corpus, sealed.Slots, raw
	}
	sealed, _ := json.MarshalIndent(map[string]any{"format": "consolidation-nine-logical-slots-v1", "corpus_sha256": experienceDigest(string(raw)), "slots": slots, "maximum_actual_entries": 99}, "", "  ")
	if err = screenWriteExclusive(filepath.Join(proof, "initial-schedule.json"), sealed); err != nil {
		t.Fatal("one-use complete schedule already consumed or unavailable")
	}
	return &corpus, slots, raw
}

func screenWriteExclusive(path string, raw []byte) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(raw)
	if writeErr == nil {
		writeErr = file.Sync()
	}
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		return errors.New("complete schedule could not be sealed")
	}
	return nil
}

type consolidationScreenCall struct {
	EntryID, OperationID, SlotID, RunID, Kind string
	ReservationID                             string
	Request                                   json.RawMessage
	Usage                                     *transport.Usage
	Final                                     *transport.AssistantFinal
	ModelVersion                              string
	Failure, Reason                           string
	HTTPStatus                                int
	Millis                                    int64
	Stages                                    []transport.ProviderStage
	ToolCalls                                 []string
	Proposals                                 []transport.ToolCallRequest
	FinishedAt                                time.Time
}

type consolidationScreenProvider struct {
	transport.ModelProvider
	pool          *pgxpool.Pool
	mu            sync.Mutex
	active        *consolidationScreenSlot
	forbiddenIDs  []string
	forbiddenText []string
	calls         []*consolidationScreenCall
	profileValue  string
}

func (p *consolidationScreenProvider) InferenceLimits() (int64, int64) {
	return p.ModelProvider.(interface{ InferenceLimits() (int64, int64) }).InferenceLimits()
}
func (p *consolidationScreenProvider) InferenceTimeout() time.Duration {
	return p.ModelProvider.(interface{ InferenceTimeout() time.Duration }).InferenceTimeout()
}

func (p *consolidationScreenProvider) selectSlot(slot *consolidationScreenSlot, forbiddenIDs, forbiddenText []string, current string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.active, p.forbiddenIDs, p.forbiddenText, p.profileValue = slot, forbiddenIDs, forbiddenText, current
}

// All nested serialized DATA is audited, not just the first user message. This
// includes real tool results on every continuation. No oracle/treatment label or
// other source-set reference/body may cross this observation-only wrapper.
func screenContainsJSON(raw []byte, needle string) bool {
	if strings.Contains(string(raw), needle) {
		return true
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return false
	}
	var walk func(any, int) bool
	walk = func(v any, depth int) bool {
		if depth > 12 {
			return false
		}
		switch x := v.(type) {
		case string:
			if strings.Contains(x, needle) {
				return true
			}
			var nested any
			if json.Unmarshal([]byte(x), &nested) == nil {
				return walk(nested, depth+1)
			}
		case []any:
			for _, child := range x {
				if walk(child, depth+1) {
					return true
				}
			}
		case map[string]any:
			for key, child := range x {
				if strings.Contains(key, needle) || walk(child, depth+1) {
					return true
				}
			}
		}
		return false
	}
	return walk(value, 0)
}

func (p *consolidationScreenProvider) reserve(ctx context.Context, runID, kind string, request any) (*consolidationScreenCall, error) {
	raw, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	slot := p.active
	if slot == nil || slot.State != "begun" {
		return nil, errors.New("prospective logical slot not begun")
	}
	for _, needle := range append([]string{"oracle", "requested_outcome", "pair-01", "pair-02", "pair-03", "A_no_experience", "B_same_frozen", "source_sets", "source_set", "max_actual_model_entries"}, p.forbiddenIDs...) {
		if screenContainsJSON(raw, needle) {
			return nil, errors.New("provider input isolation rejected")
		}
	}
	for _, text := range p.forbiddenText {
		if screenContainsJSON(raw, text) {
			return nil, errors.New("foreign case text in provider input")
		}
	}
	if kind == "start" && slot.Kind == "foreground" && (!screenContainsJSON(raw, p.profileValue) || !screenContainsJSON(raw, "tool_effect") || !screenContainsJSON(raw, "unknown")) {
		return nil, errors.New("current scoped fact or typed task missing")
	}
	op, ok := budget.ModelOperationFromContext(ctx)
	if !ok || op == "" {
		return nil, errors.New("durable actual model operation missing")
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var reservationID string
	if slot.Kind == "background" {
		err = tx.QueryRow(ctx, `SELECT a.reservation_id FROM memory_model_attempts a JOIN execution_budget_reservations r USING(reservation_id) WHERE a.operation_id=$1 AND a.operation_id=$2 AND r.owner_id='owner' AND a.state='reserved'`, op, runID).Scan(&reservationID)
	} else {
		err = tx.QueryRow(ctx, `SELECT a.reservation_id FROM execution_model_attempts a JOIN execution_budget_reservations r USING(reservation_id) WHERE a.operation_id=$1 AND a.run_id=$2 AND r.owner_id='owner' AND a.observed_at IS NULL`, op, runID).Scan(&reservationID)
	}
	if err != nil {
		return nil, errors.New("owning durable model attempt not corroborated")
	}
	var entries int
	if err = tx.QueryRow(ctx, `UPDATE consolidation_screen_window SET entries=entries+1 WHERE singleton AND NOT closed AND entries<99 RETURNING entries`).Scan(&entries); err != nil {
		return nil, err
	}
	var count int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM consolidation_screen_calls WHERE slot_id=$1`, slot.ID).Scan(&count); err != nil {
		return nil, err
	}
	if count >= slot.Limit {
		return nil, errors.New("finite logical slot entry bound exhausted")
	}
	entryID := experienceDigest(slot.ID + "\x00" + op)
	if _, err = tx.Exec(ctx, `INSERT INTO consolidation_screen_calls(entry_id,slot_id,operation_id,run_id,kind,request) VALUES($1,$2,$3,$4,$5,$6)`, entryID, slot.ID, op, runID, kind, raw); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	c := &consolidationScreenCall{EntryID: entryID, SlotID: slot.ID, OperationID: op, RunID: runID, Kind: kind, Request: raw, ReservationID: reservationID}
	p.calls = append(p.calls, c)
	return c, nil
}

func (p *consolidationScreenProvider) observe(ctx context.Context, c *consolidationScreenCall, begin time.Time, stream transport.EventStream, err error) (transport.EventStream, error) {
	finish := func() {
		p.mu.Lock()
		c.Millis = time.Since(begin).Milliseconds()
		c.FinishedAt = time.Now().UTC()
		raw, _ := json.Marshal(c)
		p.mu.Unlock()
		bounded, stop := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer stop()
		_, _ = p.pool.Exec(bounded, `UPDATE consolidation_screen_calls SET observation=$2,finished_at=clock_timestamp() WHERE entry_id=$1`, c.EntryID, raw)
	}
	if err != nil {
		p.mu.Lock()
		c.Failure, c.Reason, c.HTTPStatus = transport.DiagnosticCode(err), transport.DiagnosticReason(err), transport.DiagnosticHTTPStatus(err)
		p.mu.Unlock()
		finish()
		return nil, err
	}
	out := make(chan transport.TransportEvent, 32)
	go func() {
		defer close(out)
		defer finish()
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-stream:
				if !ok {
					return
				}
				p.mu.Lock()
				if event.Usage != nil && event.Usage.Validate() == nil {
					value := *event.Usage
					c.Usage = &value
				}
				if event.AssistantFinal != nil {
					value := *event.AssistantFinal
					c.Final = &value
					if value.Usage != nil && value.Usage.Validate() == nil {
						usage := *value.Usage
						c.Usage = &usage
					}
				}
				if event.ToolCall != nil {
					c.ToolCalls = append(c.ToolCalls, event.ToolCall.ToolName)
					c.Proposals = append(c.Proposals, *event.ToolCall)
				}
				if event.ToolCallBatch != nil {
					for _, call := range event.ToolCallBatch.ToolCalls {
						c.ToolCalls = append(c.ToolCalls, call.ToolName)
						c.Proposals = append(c.Proposals, call)
					}
				}
				if event.ProviderModelVersion != nil {
					c.ModelVersion = *event.ProviderModelVersion
				}
				if event.TransportError != nil {
					c.Failure, c.Reason, c.HTTPStatus = transport.DiagnosticCode(event.TransportError), transport.DiagnosticReason(event.TransportError), transport.DiagnosticHTTPStatus(event.TransportError)
				}
				p.mu.Unlock()
				select {
				case out <- event:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out, nil
}

func (p *consolidationScreenProvider) StartRun(ctx context.Context, r transport.StartRunRequest) (transport.EventStream, error) {
	c, err := p.reserve(ctx, r.Context.RunID, "start", r)
	if err != nil {
		return nil, err
	}
	begin := time.Now()
	original := ctx
	ctx = transport.WithProviderStageObserver(ctx, func(stage transport.ProviderStage) {
		transport.ObserveProviderStage(original, stage)
		p.mu.Lock()
		if len(c.Stages) < 16 {
			c.Stages = append(c.Stages, stage)
		}
		p.mu.Unlock()
	})
	stream, err := p.ModelProvider.StartRun(ctx, r)
	return p.observe(ctx, c, begin, stream, err)
}
func (p *consolidationScreenProvider) SubmitToolResult(ctx context.Context, r transport.SubmitToolResultRequest) (transport.EventStream, error) {
	c, err := p.reserve(ctx, r.RunID, "tool_result", r)
	if err != nil {
		return nil, err
	}
	begin := time.Now()
	original := ctx
	ctx = transport.WithProviderStageObserver(ctx, func(stage transport.ProviderStage) {
		transport.ObserveProviderStage(original, stage)
		p.mu.Lock()
		if len(c.Stages) < 16 {
			c.Stages = append(c.Stages, stage)
		}
		p.mu.Unlock()
	})
	stream, err := p.ModelProvider.SubmitToolResult(ctx, r)
	return p.observe(ctx, c, begin, stream, err)
}

// Totals retain every admitted attempt, including failures and unknown usage.
// They do not infer invoice prices from a subscription transport. NonInput is
// the observed total minus input, so includes reasoning when the provider counts
// it. Input/output subtotal is only a lower bound when total is unreported.
type consolidationScreenCost struct {
	Entries, KnownUsage, UnknownUsage, ToolCalls int
	Input, Output, ReportedTotal, NonInput       int64
	UnreportedTotal                              int
	RequestBytes, OutputJSONBytes                int64
	TransportMillis, WallMillis                  int64
}

func consolidationScreenCosts(slots []*consolidationScreenSlot, calls []*consolidationScreenCall) map[string]consolidationScreenCost {
	result := map[string]consolidationScreenCost{}
	byID := map[string]*consolidationScreenSlot{}
	for _, slot := range slots {
		byID[slot.ID] = slot
		if !slot.StartedAt.IsZero() && !slot.FinishedAt.IsZero() {
			keys := []string{slot.Kind + ":" + slot.Arm, "pair:" + slot.CaseID + ":" + slot.Arm, "full_unique_experiment"}
			if slot.Arm != "baseline" {
				keys = append(keys, "after_including_all_preparation", "pair:"+slot.CaseID+":after_including_preparation")
			}
			for _, key := range keys {
				cost := result[key]
				cost.WallMillis += slot.FinishedAt.Sub(slot.StartedAt).Milliseconds()
				result[key] = cost
			}
		}
	}
	for _, call := range calls {
		slot := byID[call.SlotID]
		if slot == nil {
			continue
		}
		keys := []string{slot.Kind + ":" + slot.Arm, "pair:" + slot.CaseID + ":" + slot.Arm, "full_unique_experiment"}
		if slot.Arm != "baseline" {
			keys = append(keys, "after_including_all_preparation", "pair:"+slot.CaseID+":after_including_preparation")
		}
		for _, key := range keys {
			cost := result[key]
			cost.Entries++
			cost.RequestBytes += int64(len(call.Request))
			outputs, _ := json.Marshal(struct {
				Final     *transport.AssistantFinal
				Proposals []transport.ToolCallRequest
			}{call.Final, call.Proposals})
			cost.OutputJSONBytes += int64(len(outputs))
			cost.TransportMillis += call.Millis
			cost.ToolCalls += len(call.ToolCalls)
			if call.Usage == nil {
				cost.UnknownUsage++
			} else {
				cost.KnownUsage++
				cost.Input += int64(call.Usage.InputTokens)
				cost.Output += int64(call.Usage.OutputTokens)
				if call.Usage.TotalTokens == 0 {
					cost.UnreportedTotal++
				} else {
					cost.ReportedTotal += int64(call.Usage.TotalTokens)
					cost.NonInput += int64(call.Usage.TotalTokens - call.Usage.InputTokens)
				}
			}
			result[key] = cost
		}
	}
	return result
}
func (*consolidationScreenProvider) ContinueRun(context.Context, transport.ContinueRunRequest) (transport.EventStream, error) {
	return nil, errors.New("screen cannot replay a provider session")
}
