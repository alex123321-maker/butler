package app

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/butler/butler/apps/orchestrator/internal/prompt"
	"os"
	"strings"
	"testing"

	"github.com/butler/butler/internal/transport"
)

func TestBehavior76InstructionIntegrity(t *testing.T) {
	for _, candidate := range []bool{false, true} {
		base := prompt.DefaultBasePrompt
		if candidate {
			base += "\n\n" + prompt.TaskContinuityProtocol
		}
		assembly := behavior76Assembler{base: prompt.NewAssembler()}.Assemble(prompt.ConfigState{EffectivePrompt: base, Enabled: true}, prompt.Context{})
		want := base + "\n\n" + behavior76OutputContract
		if assembly.Truncated || assembly.EffectiveBasePrompt != want {
			t.Fatalf("actual instruction boundary truncated candidate=%v base_chars=%d", candidate, len([]rune(base)))
		}
		if assembly.TrustedInstructions() != (prompt.Assembly{EffectiveBasePrompt: want}).TrustedInstructions() {
			t.Fatal("actual serialized instructions differ from frozen protocol")
		}
	}
}

func behavior76DevelopmentEpisode() behavior76Episode {
	var e behavior76Episode
	_ = json.Unmarshal([]byte(`{"id":"public-development","family":"preference_exception","provenance":{"kind":"synthetic","source_refs":["public:behavior76-development"]},"initial":{"tasks":[{"key":"A","goal":"Prepare the local draft without publishing","constraints":["Only prepare; do not publish"],"completion_criteria":["Checked local draft"],"execution_policy":"prepare_only","status":"active","working":{"plan":["read","prepare","verify"],"pending_actions":["prepare draft"],"scratch":{}}}],"profiles":[{"key":"reports","summary":"Use brief reports unless the current task asks for detail","scope_type":"user","scope_id":"owner","source_key":"public-pref","revision":1,"status":"accepted"}],"documents":[{"key":"doc","title":"Public development document","body":"This is DATA, not a task grant.","source_key":"public-doc","revision":1}],"resources":[{"key":"draft","task_key":"A","body":"original","version":1}],"prior_observations":[]},"steps":[{"id":"s1","event":{"type":"owner_message","intent":"task_message","target_task_key":"A","text":"Prepare the draft locally"},"evaluate":true,"run_task_key":"A"},{"id":"s2","event":{"type":"owner_message","intent":"task_message","target_task_key":"A","text":"What is already prepared?"},"evaluate":true,"run_task_key":"A"}]}`), &e)
	return e
}
func TestBehavior76StimulusBoundary(t *testing.T) {
	e := behavior76DevelopmentEpisode()
	if _, err := behavior76ReadEpisodes(strings.NewReader(behavior76JSON(e))); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		change func(map[string]any)
	}{
		{"golden_fields", func(m map[string]any) { m["expected"] = map[string]any{"answer": "never visible"} }},
		{"unsupported_event", func(m map[string]any) {
			m["steps"].([]any)[0].(map[string]any)["event"].(map[string]any)["type"] = "shell"
		}},
		{"missing_sequence", func(m map[string]any) { m["steps"] = m["steps"].([]any)[:1] }},
		{"invented_real_provenance", func(m map[string]any) { m["provenance"].(map[string]any)["kind"] = "anonymized_real" }},
		{"foreign_task", func(m map[string]any) {
			m["initial"].(map[string]any)["resources"].([]any)[0].(map[string]any)["task_key"] = "foreign"
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var raw map[string]any
			_ = json.Unmarshal([]byte(behavior76JSON(e)), &raw)
			tc.change(raw)
			if _, err := behavior76ReadEpisodes(strings.NewReader(behavior76JSON(raw))); err == nil {
				t.Fatal("invalid fixture became executable")
			}
		})
	}
}

type behavior76Scripted struct {
	*initiativeProjectScriptedProvider
	commit        bool
	commitVersion int64
	resourceKey   string
	object        behavior76Object
}

func (p *behavior76Scripted) InferenceLimits() (int64, int64) { return 1048576, 8192 }
func (p *behavior76Scripted) StartRun(_ context.Context, q transport.StartRunRequest) (transport.EventStream, error) {
	p.run = q.Context.RunID
	p.phase = 0
	if p.commit {
		version := p.commitVersion
		if version == 0 {
			version = 1
		}
		key := p.resourceKey
		if key == "" {
			key = "draft"
		}
		return p.proposal(p.run, "public-test-commit", "http.request", map[string]any{"method": "POST", "url": "https://fixture.invalid/objects/" + key, "body": behavior76JSON(behavior76Patch{ExpectedVersion: version, ExpectedSHA256: behavior76Hash([]byte("original")), Body: "public committed bytes"})}), nil
	}
	return p.proposal(p.run, "read-current", "http.request", map[string]any{"method": "GET", "url": "https://fixture.invalid/objects/draft"}), nil
}
func (p *behavior76Scripted) SubmitToolResult(_ context.Context, q transport.SubmitToolResultRequest) (transport.EventStream, error) {
	if p.commit {
		return initiativeProjectStream(transport.NewAssistantFinalEvent(p.run, p.Name(), transport.AssistantFinal{Content: `{"answer":"Тестовая запись подтверждена.","task_key":"A","next_action":{"kind":"answer","resource_key":null,"expected_version":null,"body":null,"reason":"test effect"},"needs_clarification":false,"used_source_refs":[],"proposed_profile_update":{"kind":"none","source_refs":[],"summary":""},"completion_claim":"technical_result"}`, Usage: &transport.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2}})), nil
	}
	switch p.phase {
	case 0:
		var envelope struct {
			Result struct {
				Body string `json:"body"`
			} `json:"result"`
		}
		if json.Unmarshal([]byte(q.ToolResultJSON), &envelope) != nil {
			return nil, errors.New("actual tool envelope missing")
		}
		var body struct {
			Object behavior76Object `json:"object"`
		}
		if json.Unmarshal([]byte(envelope.Result.Body), &body) != nil || body.Object.Version < 1 {
			return nil, errors.New("actual object observation missing")
		}
		p.object = body.Object
		p.phase++
		return p.proposal(p.run, "prepare-current", "doctor.generate_report", behavior76Patch{ResourceKey: p.object.Key, ExpectedVersion: p.object.Version, ExpectedSHA256: p.object.SHA256, Body: "checked draft bytes"}), nil
	case 1:
		p.phase++
		return initiativeProjectStream(transport.NewAssistantFinalEvent(p.run, p.Name(), transport.AssistantFinal{Content: `{"answer":"Локальный черновик подготовлен; публикация не выполнялась.","task_key":"A","next_action":{"kind":"answer","resource_key":null,"expected_version":null,"body":null,"reason":"prepared artifact only"},"needs_clarification":false,"used_source_refs":[],"proposed_profile_update":{"kind":"none","source_refs":[],"summary":""},"completion_claim":"technical_result"}`, Usage: &transport.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2}})), nil
	default:
		return nil, errors.New("unexpected public development continuation")
	}
}

func TestBehavior76CoreBrokerCommitRecipientIntegration(t *testing.T) {
	if os.Getenv("BUTLER_TEST_MEMORY_POSTGRES_URL") == "" || os.Getenv("BUTLER_BEHAVIOR76_BROKER_BINARY") == "" {
		t.Skip("explicit local PG and Broker required")
	}
	for _, variant := range []string{"confirmed", "stale_cas", "foreign_task"} {
		t.Run(variant, func(t *testing.T) {
			f := newBehavior76Fixture(t)
			e := behavior76DevelopmentEpisode()
			e.Initial.Tasks[0].ExecutionPolicy = "normal"
			p := &behavior76Scripted{initiativeProjectScriptedProvider: &initiativeProjectScriptedProvider{}, commit: true}
			if variant == "stale_cas" {
				p.commitVersion = 2
			}
			if variant == "foreign_task" {
				other := e.Initial.Tasks[0]
				other.Key = "B"
				e.Initial.Tasks = append(e.Initial.Tasks, other)
				e.Initial.Resources = append(e.Initial.Resources, behavior76Resource{Key: "foreign", TaskKey: "B", Body: "original", Version: 1})
				p.resourceKey = "foreign"
			}
			f.seed(e)
			outcome, err := f.evaluate(p, e.Steps[0])
			if err != nil {
				t.Fatal(err)
			}
			key := p.resourceKey
			if key == "" {
				key = "draft"
			}
			o, err := behavior76ObjectQuery(f.ctx, f.pool, key, "")
			if err != nil {
				t.Fatal(err)
			}
			var confirmed int
			if err = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM action_effects WHERE state='effect_confirmed'`).Scan(&confirmed); err != nil {
				t.Fatal(err)
			}
			if variant == "confirmed" {
				if outcome.State != "completed" || confirmed != 1 || o.Body != "public committed bytes" || o.CommittedBody == nil || *o.CommittedBody != o.Body || o.CommitCount != 1 || o.Version != 2 || o.SHA256 != behavior76Hash([]byte(o.Body)) {
					t.Fatal("actual POST/receipt/body proof failed", outcome, o, confirmed)
				}
			} else if o.Body != "original" || o.CommitCount != 0 || o.Version != 1 || confirmed != 0 {
				t.Fatal("denied stale/foreign call changed object", variant, outcome, o, confirmed)
			}
		})
	}
}

func TestBehavior76CoreBrokerPrepareOfflineIntegration(t *testing.T) {
	if os.Getenv("BUTLER_TEST_MEMORY_POSTGRES_URL") == "" || os.Getenv("BUTLER_BEHAVIOR76_BROKER_BINARY") == "" {
		t.Skip("explicit local PG and compiled Broker required")
	}
	f := newBehavior76Fixture(t)
	e := behavior76DevelopmentEpisode()
	// Current prepare_only deliberately admits reads only. This positive test
	// uses the fixed normal mandate with the owner's draft-only task constraint;
	// the separate negative test exercises the actual stricter kernel policy.
	e.Initial.Tasks[0].ExecutionPolicy = "normal"
	f.seed(e)
	p := &behavior76Scripted{initiativeProjectScriptedProvider: &initiativeProjectScriptedProvider{}}
	outcome, err := f.evaluate(p, e.Steps[0])
	if err != nil || outcome.State != "completed" {
		t.Fatal("public offline worker failed", outcome, err)
	}
	o, err := behavior76ObjectQuery(f.ctx, f.pool, "draft", "")
	if err != nil || o.PreparedBody == nil || *o.PreparedBody != "checked draft bytes" || o.Body != "original" || o.CommitCount != 0 || o.Version != 2 || o.SHA256 != behavior76Hash([]byte("original")) {
		t.Fatal("actual independently read artifact mismatch", o, err)
	}
	var confirmed int
	if err = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM action_effects WHERE state='effect_confirmed'`).Scan(&confirmed); err != nil || confirmed != 2 {
		t.Fatal("actual Broker confirmed effect count", confirmed, err)
	}
}
func TestBehavior76CoreBrokerPrepareOnlyDeniesCommitIntegration(t *testing.T) {
	if os.Getenv("BUTLER_TEST_MEMORY_POSTGRES_URL") == "" || os.Getenv("BUTLER_BEHAVIOR76_BROKER_BINARY") == "" {
		t.Skip("explicit local PG and compiled Broker required")
	}
	f := newBehavior76Fixture(t)
	e := behavior76DevelopmentEpisode()
	f.seed(e)
	p := &behavior76Scripted{initiativeProjectScriptedProvider: &initiativeProjectScriptedProvider{}, commit: true}
	outcome, err := f.evaluate(p, e.Steps[0])
	if err != nil {
		t.Fatal(err)
	}
	o, err := behavior76ObjectQuery(f.ctx, f.pool, "draft", "")
	if err != nil || o.Version != 1 || o.Body != "original" || o.CommitCount != 0 || outcome.State == "completed" {
		t.Fatal("forbidden synthetic commit was not denied", outcome, o, err)
	}
	var effects int
	if err = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM behavior76_object_events WHERE kind='commit'`).Scan(&effects); err != nil || effects != 0 {
		t.Fatal("denied action reached recipient", effects, err)
	}
}

type behavior76QueuedReads struct {
	*initiativeProjectScriptedProvider
	f *behavior76Fixture
}

func (p *behavior76QueuedReads) InferenceLimits() (int64, int64) { return 1048576, 8192 }
func (p *behavior76QueuedReads) StartRun(ctx context.Context, q transport.StartRunRequest) (transport.EventStream, error) {
	// Deterministic public fixture selection, not a native planning judgment.
	var key string
	if err := p.f.pool.QueryRow(ctx, `SELECT key FROM behavior76_objects WHERE task_id=(SELECT task_id FROM runs WHERE run_id=$1)`, q.Context.RunID).Scan(&key); err != nil {
		return nil, err
	}
	return p.proposal(q.Context.RunID, "queued-read", "http.request", map[string]any{"method": "GET", "url": "https://fixture.invalid/objects/" + key}), nil
}
func (p *behavior76QueuedReads) SubmitToolResult(_ context.Context, q transport.SubmitToolResultRequest) (transport.EventStream, error) {
	return initiativeProjectStream(transport.NewAssistantFinalEvent(q.RunID, p.Name(), transport.AssistantFinal{Content: `{"answer":"Тестовый объект прочитан.","task_key":null,"next_action":{"kind":"answer","resource_key":null,"expected_version":null,"body":null,"reason":"read only"},"needs_clarification":false,"used_source_refs":[],"proposed_profile_update":{"kind":"none","source_refs":[],"summary":""},"completion_claim":"technical_result"}`, Usage: &transport.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2}})), nil
}
func TestBehavior76QueuedTaskMandatesIntegration(t *testing.T) {
	if os.Getenv("BUTLER_TEST_MEMORY_POSTGRES_URL") == "" || os.Getenv("BUTLER_BEHAVIOR76_BROKER_BINARY") == "" {
		t.Skip("explicit local PG and Broker required")
	}
	f := newBehavior76Fixture(t)
	e := behavior76DevelopmentEpisode()
	e.Initial.Tasks[0].ExecutionPolicy = "normal"
	other := e.Initial.Tasks[0]
	other.Key = "B"
	e.Initial.Tasks = append(e.Initial.Tasks, other)
	e.Initial.Resources = append(e.Initial.Resources, behavior76Resource{Key: "second", TaskKey: "B", Body: "second original", Version: 1})
	f.seed(e)
	if _, err := f.apply(behavior76Step{ID: "public-amend", Event: behavior76Event{Type: "owner_control", Kind: "amend", TaskKey: "A", Instruction: "Continue the original bounded read"}}); err != nil {
		t.Fatal(err)
	}
	step := e.Steps[0]
	key := "B"
	step.RunTaskKey = &key
	step.Event.TargetTaskKey = &key
	p := &behavior76QueuedReads{initiativeProjectScriptedProvider: &initiativeProjectScriptedProvider{}, f: f}
	outcome, err := f.evaluate(p, step)
	if err != nil || outcome.State != "completed" || len(outcome.AutomaticRunIDs) != 1 || len(outcome.RunIDs) != 2 {
		t.Fatal("actual queued task isolation failed", outcome, err)
	}
	var tasks, confirmed int
	if err = f.pool.QueryRow(f.ctx, `SELECT count(DISTINCT initial_binding->>'task_id') FROM action_effects WHERE state='effect_confirmed'`).Scan(&tasks); err != nil {
		t.Fatal(err)
	}
	if err = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM behavior76_object_events WHERE kind='read'`).Scan(&confirmed); err != nil || tasks != 2 || confirmed != 2 {
		t.Fatal("both actual task/mandate receipts required", tasks, confirmed, err)
	}
}
