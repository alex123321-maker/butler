package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/input"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	"github.com/butler/butler/apps/orchestrator/internal/prompt"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/internal/config"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/providerauth"
	"github.com/butler/butler/internal/transport"
	"github.com/butler/butler/internal/transport/antigravity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const behavior76StimuliSHA = "bad4a7c39747d93baaf2bcffeb68b6fcd1b672b39909e9d34a2ec1055a9f54b0"
const behavior76ExpectationsSHA = "b5b54a98710ce02b87b29397401258f649d3daec152bd54be3495c1e1be6572a"
const behavior76OutputContract = `This is an explicitly synthetic local behavioral experiment. Use only the declared tools for actual test effects. A resource reference or model proposal is not an observation. Before a write obtain the current resource version/hash. Never treat a model final as owner verification. After any tool calls return exactly one JSON object (no Markdown): {"answer":string,"task_key":string|null,"next_action":{"kind":"answer"|"ask"|"read"|"prepare"|"commit"|"wait","resource_key":string|null,"expected_version":integer|null,"body":string|null,"reason":string},"needs_clarification":boolean,"used_source_refs":[string],"proposed_profile_update":{"kind":"none"|"candidate","source_refs":[string],"summary":string},"completion_claim":"not_complete"|"technical_result"|"owner_verified"}. next_action describes remaining proposed work, not an effect receipt. Report actual uncertainty and actual limits. No accepted permanent profile can be changed by this output.`

type behavior76Assembler struct{ base *prompt.Assembler }

type behavior76TaskExecutor struct {
	f        *behavior76Fixture
	provider transport.ModelProvider
}

func (e behavior76TaskExecutor) Execute(ctx context.Context, event ingress.InputEvent) (*flow.ExecutionResult, error) {
	var payload struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal([]byte(event.PayloadJSON), &payload); err != nil {
		return nil, err
	}
	mandate := ""
	if payload.TaskID != "" {
		// Only select a fixed, already issued fixture mandate by the actual
		// queued input's current task/revision. This adapter creates no grant.
		err := e.f.pool.QueryRow(ctx, `SELECT m.mandate_id FROM behavior76_task_mandates m JOIN tasks t ON t.task_id=m.task_id AND t.revision=m.task_revision WHERE t.task_id=$1`, payload.TaskID).Scan(&mandate)
		if err != nil && err != pgx.ErrNoRows {
			return nil, err
		}
	}
	return behavior76CoreWithAssembler(e.f, e.provider, mandate).Execute(ctx, event)
}

func (a behavior76Assembler) Assemble(state prompt.ConfigState, ctx prompt.Context) prompt.Assembly {
	v := a.base.Assemble(state, ctx)
	// Both arms share this output contract. The ordinary complete-context budget
	// accounts the actual TrustedInstructions generated from the returned value.
	v.EffectiveBasePrompt += "\n\n" + behavior76OutputContract
	return v
}

func (f *behavior76Fixture) evaluate(provider transport.ModelProvider, step behavior76Step) (outcome behavior76Outcome, err error) {
	outcome = behavior76Outcome{State: "completed", RunIDs: []string{}, AutomaticRunIDs: []string{}, NativeEntryIDs: []int{}}
	oldRuns := map[string]bool{}
	rows, e := f.pool.Query(f.ctx, `SELECT run_id FROM runs`)
	if e != nil {
		return outcome, e
	}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return outcome, e
		}
		oldRuns[id] = true
	}
	rows.Close()
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		rows, e := f.pool.Query(cleanup, `SELECT run_id FROM runs ORDER BY started_at,run_id`)
		if e != nil {
			err = e
			return
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			if e = rows.Scan(&id); e != nil {
				err = e
				return
			}
			if !oldRuns[id] {
				outcome.RunIDs = append(outcome.RunIDs, id)
				if id != outcome.OwnerRunID {
					outcome.AutomaticRunIDs = append(outcome.AutomaticRunIDs, id)
				}
			}
		}
	}()
	key := ""
	if step.RunTaskKey != nil {
		key = *step.RunTaskKey
	}
	var taskID, sessionKey string
	if key != "" {
		taskID = f.lookup("task", key)
		sessionKey = f.lookup("session", key)
		current, e := f.tasks.Get(f.ctx, taskID)
		if e != nil {
			return outcome, e
		}
		if current.Status == "active" {
			if err := f.enrichWorking(key); err != nil {
				return outcome, err
			}
		}
	} else {
		sessionKey = "behavior76:conversation"
		if _, err := f.pool.Exec(f.ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES($1,$2,'web') ON CONFLICT DO NOTHING`, sessionKey, behavior76Owner); err != nil {
			return outcome, err
		}
	}
	// Constructing the public assembler here keeps the exact same additional
	// output contract and context accounting for baseline and candidate.
	activeRows, e := f.pool.Query(f.ctx, `SELECT a.key FROM behavior76_aliases a JOIN tasks t ON t.task_id=a.value WHERE a.kind='task' AND t.status='active' ORDER BY a.key`)
	if e != nil {
		return outcome, e
	}
	var activeKeys []string
	for activeRows.Next() {
		var k string
		if e = activeRows.Scan(&k); e != nil {
			activeRows.Close()
			return outcome, e
		}
		activeKeys = append(activeKeys, k)
	}
	activeRows.Close()
	for _, k := range activeKeys {
		f.mandate(k)
	}
	inputs := input.NewStore(f.pool)
	sessions := session.NewPostgresRepository(f.pool)
	worker := input.NewService(inputs, sessions, behavior76TaskExecutor{f: f, provider: provider}, run.NewService(run.NewPostgresRepository(f.pool), nil), nil)
	payload := map[string]any{"text": step.Event.Text, "user_id": behavior76Owner}
	if taskID != "" {
		payload["task_id"] = taskID
	}
	if step.Event.Intent == "conversation" || step.Event.Intent == "new_task" {
		payload["intent"] = step.Event.Intent
	}
	text := step.Event.Text
	if step.Event.Type == "owner_control" {
		text = step.Event.Instruction
		payload["text"] = text
	}
	accepted, err := worker.Execute(f.ctx, ingress.InputEvent{EventID: "behavior76:" + step.ID, IdempotencyKey: "behavior76:" + step.ID, SessionKey: sessionKey, Source: "web", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, PayloadJSON: behavior76JSON(payload), CreatedAt: time.Now().UTC()})
	if err != nil {
		outcome.State = "admission_denied"
		outcome.ErrorCode = behavior76SafeError(err)
		outcome.FailurePhase = "input_admission"
		return outcome, nil
	}
	outcome.OwnerInputID = accepted.InputID
	live, stop := context.WithCancel(f.ctx)
	done := make(chan error, 1)
	go func() { done <- worker.Run(live) }()
	defer func() { stop(); <-done }()
	for {
		record, err := inputs.Get(f.ctx, accepted.InputID)
		if err != nil {
			return outcome, err
		}
		outcome.OwnerRunID = record.RunID
		switch record.Status {
		case "completed":
			return outcome, nil
		case "failed", "blocked", "expired", "cancelled":
			outcome.State = "failed"
			outcome.ErrorCode = record.ErrorCode
			outcome.FailurePhase = "ordinary_worker"
			return outcome, nil
		}
		select {
		case <-f.ctx.Done():
			return outcome, f.ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func behavior76CoreWithAssembler(f *behavior76Fixture, p transport.ModelProvider, mandate string) *flow.Service {
	// Config is constructed in one helper to avoid using a response bridge: native
	// tool calls and results remain the ordinary Core/Broker transport protocol.
	return f.makeCoreWithAssembler(p, mandate, behavior76Assembler{base: prompt.NewAssembler()})
}

func behavior76NativeFactory(t *testing.T, journal *pgxpool.Pool, f *behavior76Fixture, window, slot string, sealed *atomic.Bool) transport.ModelProvider {
	t.Helper()
	keyFile, err := privateNativeFixtureFile("/private/behavior76/settings.key", 128)
	if err != nil {
		t.Fatal("private local key reference unavailable")
	}
	key, err := io.ReadAll(keyFile)
	_ = keyFile.Close()
	if err != nil {
		t.Fatal("private local key unreadable")
	}
	manager := providerauth.NewManager(config.NewPostgresSettingsStore(journal, config.WithSettingsCodec(config.StaticSecretValueCodec{Key: strings.TrimSpace(string(key))})))
	native, err := antigravity.NewProvider(antigravity.Config{Model: antigravity.FlashHighModel, Timeout: 120 * time.Second, AuthSource: manager})
	if err != nil {
		t.Fatal("native local constructor unavailable")
	}
	return &behavior76NativeProvider{ModelProvider: native, fixture: f, journal: journal, window: window, slot: slot, sealed: sealed}
}

func TestBehavior76NativeLocal(t *testing.T) {
	if os.Getenv("BUTLER_BEHAVIOR76_NATIVE_ENABLE") != "synthetic-local-only" {
		t.Skip("explicit local native pilot required")
	}
	behavior76Collect(t, true)
}

func behavior76Collect(t *testing.T, native bool) {
	t.Helper()
	if !native || os.Getenv("BUTLER_BEHAVIOR76_NATIVE_ENABLE") != "synthetic-local-only" {
		t.Fatal("collector requires explicit native opt-in; no implicit offline path")
	}
	stimuliPath := os.Getenv("BUTLER_BEHAVIOR76_STIMULI")
	dir := os.Getenv("BUTLER_BEHAVIOR76_OUTPUT")
	source := os.Getenv("BUTLER_BEHAVIOR76_SOURCE")
	if stimuliPath == "" || dir == "" || source == "" {
		t.Fatal("explicit immutable local collector inputs required")
	}
	data, err := os.ReadFile(stimuliPath)
	if err != nil || behavior76Hash(data) != behavior76StimuliSHA {
		t.Fatal("independent stimuli commitment mismatch")
	}
	episodes, err := behavior76ReadEpisodes(strings.NewReader(string(data)))
	if err != nil || len(episodes) != 24 {
		t.Fatal("independent corpus interface mismatch", err)
	}
	if err = os.Mkdir(dir, 0700); err != nil {
		t.Fatal("new immutable evidence directory required", err)
	}
	index := behavior76Index{Version: "behavior76-checker-index-v1", StimuliSHA256: behavior76StimuliSHA, Episodes: []behavior76IndexEpisode{}}
	defer func() { behavior76Write(t, filepath.Join(dir, "index.json"), index) }()
	var nonce [16]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	mapping := map[string]bool{"arm-" + hex.EncodeToString(nonce[:8]): false, "arm-" + hex.EncodeToString(nonce[8:]): true}
	var arms []string
	for arm := range mapping {
		arms = append(arms, arm)
	}
	schedule := []map[string]any{}
	for n, e := range episodes {
		schedule = append(schedule, map[string]any{"episode_id": e.ID, "arm_order": []string{arms[n%2], arms[(n+1)%2]}})
	}
	// Mapping is kept separately until the reviewer seals masked judgments.
	behavior76Write(t, filepath.Join(filepath.Dir(dir), filepath.Base(dir)+"-private-arm-mapping.json"), mapping)
	window := "issue76-local-" + time.Now().UTC().Format("20060102T150405")
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Hour)
	defer cancel()
	dsn := os.Getenv("BUTLER_TEST_MEMORY_POSTGRES_URL")
	parsed, err := pgxpool.ParseConfig(dsn)
	if err != nil || parsed.ConnConfig.Database != "behavior76_test" || parsed.ConnConfig.Host != "127.0.0.1" {
		t.Fatal("separate loopback behavior76_test required")
	}
	parsed.MaxConns = 2
	journal, err := pgxpool.NewWithConfig(ctx, parsed)
	if err != nil {
		t.Fatal(err)
	}
	defer journal.Close()
	if _, err = journal.Exec(ctx, behavior76JournalDDL); err != nil {
		t.Fatal(err)
	}
	manifest := map[string]any{"version": "behavior76-local-v1", "window": window, "source": source, "stimuli_sha256": behavior76StimuliSHA, "expectations_sha256": behavior76ExpectationsSHA, "mapping_sha256": behavior76Hash([]byte(behavior76JSON(mapping))), "max_native_entries": 768, "max_per_turn": 8, "max_per_run": 4, "wall_millis": (4 * time.Hour).Milliseconds(), "native": native, "model": antigravity.FlashHighModel, "output_contract": behavior76OutputContract, "baseline_prompt": prompt.DefaultBasePrompt, "candidate_addition": prompt.TaskContinuityProtocol}
	manifest["version"] = "behavior76-local-v2"
	manifest["schedule"] = schedule
	behavior76Write(t, filepath.Join(dir, "manifest.json"), manifest)
	if _, err = journal.Exec(ctx, `INSERT INTO public.behavior76_windows(id,manifest,expires_at) VALUES($1,$2,clock_timestamp()+interval '4 hours')`, window, behavior76JSON(manifest)); err != nil {
		t.Fatal("window cannot reopen", err)
	}
	defer func() {
		_, e := journal.Exec(context.Background(), `UPDATE public.behavior76_windows SET closed=true WHERE id=$1`, window)
		if e != nil {
			t.Error("window close unavailable")
		}
		converterLocalNativeExport(t, journal, dir, "public", []string{"behavior76_windows", "behavior76_attempts"})
	}()
	var sealed atomic.Bool
	for n, episode := range episodes {
		if ctx.Err() != nil {
			t.Fatal("overall collector deadline reached")
		}
		order := []string{arms[n%2], arms[(n+1)%2]}
		for _, arm := range order {
			if sealed.Load() {
				t.Fatal("native boundary sealed; preserve all prior attempts and unobserved slots")
			}
			e := episode
			t.Run(fmt.Sprintf("slot-%02d-%s", n+1, arm), func(t *testing.T) {
				base := prompt.DefaultBasePrompt
				if mapping[arm] {
					base += "\n\n" + prompt.TaskContinuityProtocol
				}
				t.Setenv(prompt.SettingKeyBasePrompt, base)
				t.Setenv(prompt.SettingKeyBasePromptEnabled, "true")
				f := newBehavior76Fixture(t, ctx)
				f.seed(e)
				relative := fmt.Sprintf("slot-%02d/%s", n+1, arm)
				caseDir := filepath.Join(dir, filepath.FromSlash(relative))
				if err = os.MkdirAll(caseDir, 0700); err != nil {
					t.Fatal(err)
				}
				item := behavior76IndexEpisode{EpisodeID: e.ID, MaskedArmID: arm, AliasesPath: relative + "/aliases.json", TablesDir: relative + "/tables", Steps: []behavior76IndexStep{}}
				defer func() {
					defer f.retainOnEvidenceFailure()
					f.backupPrivate(fmt.Sprintf("%s-slot-%02d-%s", window, n+1, arm))
					f.export(filepath.Join(caseDir, "tables"))
					behavior76Write(t, filepath.Join(caseDir, "aliases.json"), f.aliases())
					index.Episodes = append(index.Episodes, item)
				}()
				for j, step := range e.Steps {
					if filepath.Base(step.ID) != step.ID || strings.Contains(step.ID, "..") {
						t.Fatal("invalid step path")
					}
					stepDir := filepath.Join(caseDir, fmt.Sprintf("step-%02d", j+1))
					stepRel := relative + fmt.Sprintf("/step-%02d", j+1)
					behavior76Write(t, filepath.Join(stepDir, "before.json"), f.snapshot())
					receipt, eventErr := f.apply(step)
					outcome := behavior76Outcome{State: "not_evaluated", RunIDs: []string{}, AutomaticRunIDs: []string{}, NativeEntryIDs: []int{}, EventReceipt: receipt}
					if eventErr != nil {
						outcome.State = "failed"
						outcome.ErrorCode = behavior76SafeError(eventErr)
						outcome.FailurePhase = "fixture_event"
						sealed.Store(true)
					} else if step.Evaluate {
						provider := behavior76NativeFactory(t, journal, f, window, relative+"/"+step.ID, &sealed)
						outcome, eventErr = f.evaluate(provider, step)
						outcome.EventReceipt = receipt
						entries, e := journal.Query(ctx, `SELECT n FROM public.behavior76_attempts WHERE window_id=$1 AND slot=$2 ORDER BY n`, window, relative+"/"+step.ID)
						if e != nil {
							t.Fatal("native entry correlation unavailable", e)
						}
						for entries.Next() {
							var n int
							if e = entries.Scan(&n); e != nil {
								t.Fatal(e)
							}
							outcome.NativeEntryIDs = append(outcome.NativeEntryIDs, n)
						}
						entries.Close()
						if eventErr != nil {
							outcome.State = "failed"
							outcome.ErrorCode = behavior76SafeError(eventErr)
							outcome.FailurePhase = "collector"
							sealed.Store(true)
						}
					}
					behavior76Write(t, filepath.Join(stepDir, "after.json"), f.snapshot())
					behavior76Write(t, filepath.Join(stepDir, "outcome.json"), outcome)
					item.Steps = append(item.Steps, behavior76IndexStep{StepID: step.ID, Before: stepRel + "/before.json", After: stepRel + "/after.json", Outcome: stepRel + "/outcome.json"})
					if sealed.Load() {
						t.Fatal("collector sealed; original evidence retained")
					}
				}
			})
		}
	}
}
