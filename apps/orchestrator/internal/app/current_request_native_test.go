package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	approvals "github.com/butler/butler/apps/orchestrator/internal/approval"
	"github.com/butler/butler/apps/orchestrator/internal/communication"
	"github.com/butler/butler/apps/orchestrator/internal/completion"
	"github.com/butler/butler/apps/orchestrator/internal/input"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/communicationcontrol"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5"
)

type requestScopeCapture struct {
	transport.ModelProvider
	Starts []transport.StartRunRequest `json:"starts"`
	mu     sync.Mutex
	Calls  []requestScopeCall `json:"calls"`
}

type requestScopeCall struct {
	Method, RunRef string
	Usage          *transport.Usage
	Version        *string
	Proposal       *transport.ToolCallRequest
}

func (p *requestScopeCapture) observe(ctx context.Context, method, run string, stream transport.EventStream, err error) (transport.EventStream, error) {
	p.mu.Lock()
	index := len(p.Calls)
	p.Calls = append(p.Calls, requestScopeCall{Method: method, RunRef: run})
	p.mu.Unlock()
	if err != nil {
		return nil, err
	}
	out := make(chan transport.TransportEvent)
	go func() {
		defer close(out)
		for event := range stream {
			p.mu.Lock()
			if event.Usage != nil {
				u := *event.Usage
				p.Calls[index].Usage = &u
			}
			if event.ProviderModelVersion != nil {
				v := *event.ProviderModelVersion
				p.Calls[index].Version = &v
			}
			if event.ToolCall != nil {
				v := *event.ToolCall
				p.Calls[index].Proposal = &v
			}
			p.mu.Unlock()
			select {
			case out <- event:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

func (p *requestScopeCapture) InferenceLimits() (int64, int64) {
	return p.ModelProvider.(interface{ InferenceLimits() (int64, int64) }).InferenceLimits()
}
func (p *requestScopeCapture) StartRun(ctx context.Context, q transport.StartRunRequest) (transport.EventStream, error) {
	widget := false
	for _, tool := range q.ToolDefinitions {
		widget = widget || tool.Name == "communication.widget"
	}
	if !widget {
		return nil, errors.New("native preflight requires actual widget contract before inference")
	}
	p.Starts = append(p.Starts, q)
	stream, err := p.ModelProvider.StartRun(ctx, q)
	return p.observe(ctx, "start", q.Context.RunID, stream, err)
}

func (p *requestScopeCapture) SubmitToolResult(ctx context.Context, q transport.SubmitToolResultRequest) (transport.EventStream, error) {
	stream, err := p.ModelProvider.SubmitToolResult(ctx, q)
	return p.observe(ctx, "tool_result", q.RunID, stream, err)
}
func (p *requestScopeCapture) ContinueRun(ctx context.Context, q transport.ContinueRunRequest) (transport.EventStream, error) {
	stream, err := p.ModelProvider.ContinueRun(ctx, q)
	return p.observe(ctx, "continue", q.RunID, stream, err)
}

// Explicit, bounded, synthetic owner scenarios through the real Core, separate
// Broker process, PostgreSQL, native provider, canonical answer and fresh Core
// recovery. No working-owner input, live Telegram send or external action.
func TestNativeCurrentRequestScopeLab(t *testing.T) {
	if os.Getenv("BUTLER_CURRENT_REQUEST_NATIVE_LAB") != "1" {
		t.Skip("explicit isolated native window required")
	}
	if os.Getenv("BUTLER_COMMUNICATION_AUTH_POSTGRES_URL") == "" || os.Getenv("BUTLER_GUARDED_REQUEST_BROKER_BINARY") == "" {
		t.Fatal("private native references required")
	}
	currentRequestScopeLab(t, true)
}

func TestCurrentRequestScopeCoreBrokerIntegration(t *testing.T) {
	if os.Getenv("BUTLER_GUARDED_REQUEST_BROKER_BINARY") == "" {
		t.Skip("isolated compiled Broker required")
	}
	currentRequestScopeLab(t, false)
}

func currentRequestScopeLab(t *testing.T, native bool) {
	t.Setenv("BUTLER_CURRENT_REQUEST_NATIVE_LAB", "1")
	t.Setenv("BUTLER_COMMUNICATION_NATIVE_HARNESS", "1")
	root := os.Getenv("BUTLER_CURRENT_REQUEST_REPORT_DIR")
	if !native && root == "" {
		root = t.TempDir()
	}
	if root == "" {
		t.Fatal("owned report directory required")
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	// O_EXCL prohibits accidentally rerunning a closed window over its evidence.
	lock, err := os.OpenFile(filepath.Join(root, "started"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal("window already started or unavailable")
	}
	lock.Close()
	manifest := filepath.Join(root, "manifest")
	f := newDelegationGuardFixtureWithManifest(t, manifest)
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	f.ctx = ctx
	f.budgets.Policy.Models["antigravity:gemini-3.8-flash-high"] = budget.ModelPrice{Basis: "subscription_no_overage"}
	f.budgets.Policy.Models["fixture:fixture"] = budget.ModelPrice{Basis: "local_no_billing"}
	// All four requests share one conversation allowance; never reset it after
	// a wait, recovery, or new request. Reservations include full provider limits.
	f.budgets.Policy.Day.ModelTurns = 16
	f.budgets.Policy.Day.Tokens = 16 * (1048576 + 8192)
	f.budgets.Policy.Conversation.ModelTurns = 16
	f.budgets.Policy.Conversation.Tokens = 16 * (1048576 + 8192)
	f.budgets.Policy.ModelMillis = 90000
	f.budgets.Policy.Conversation.ActiveMillis = 16 * (90000 + 120000)
	inputs := input.NewStore(f.pool)
	inputs.CommunicationEpoch = 1
	sessions := session.NewPostgresRepository(f.pool)
	runs := run.NewService(run.NewPostgresRepository(f.pool), nil)
	repo := approvals.NewPostgresRepository(f.pool)
	repo.SetCheckpointHooks(inputs)
	approve := approvals.NewService(repo, nil)
	reader := &actionContextReader{runs: runs, sessions: sessions, leases: f.leases, tasks: f.store, approvals: approve, pool: f.pool, communicationReady: true}
	store := &communication.Store{Pool: f.pool, Inputs: inputs, Sources: f.sources, WebAvailable: true, SourceGuard: f.sources.CheckRunSourcesTx, ChannelGuard: func(context.Context, pgx.Tx, string, communication.Binding, communicationcontrol.Args) error {
		return nil
	}}
	actor := owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: f.parent.OwnerID, Channel: "web", SessionKey: "web:verified", DeliveryID: "bind"})
	if err = store.Bind(actor, communication.Binding{ID: "web", Channel: "web", Recipient: "web:verified", Enabled: true, Capabilities: []string{"message", "widget"}}, 0); err != nil {
		t.Fatal(err)
	}
	h, err := communicationcontrol.NewHandler("guarded-request-private-process-fixture-token", &CommunicationControl{Store: store})
	if err != nil {
		t.Fatal(err)
	}
	stub := &dispatchAdmissionFixture{f: f, reader: reader, manifestPath: manifest, call: &toolbrokerv1.ToolCall{ToolName: "communication.message"}}
	broker := controlBrokerProcess(t, stub, h, communicationcontrol.Path, "BUTLER_COMMUNICATION_HARNESS=1")
	writer := completion.New(f.pool)
	writer.Communication = store
	writer.DataGuard = f.sources.CheckRunSourcesTx
	trace := &communicationScript{f: f}
	scripted := &scopeFixtureProvider{f: f}
	var model transport.ModelProvider = scripted
	providerName, modelName := "fixture", "fixture"
	if native {
		model = communicationNativeProvider(t, trace)
		providerName, modelName = "antigravity", "gemini-3.8-flash-high"
	}
	provider := &requestScopeCapture{ModelProvider: model}
	makeCore := func() *flow.Service {
		return flow.NewService(sessions, f.leases, runs, transcript.NewStore(f.pool), provider, flow.Config{
			// Direct Core execution has no input-worker heartbeat. Keep a finite
			// lease covering the bounded provider call and publication transaction.
			OwnerID: "issue147-isolated", ProviderName: providerName, ModelName: modelName, LeaseTTL: 300, AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_2,
			Communication: store, CommunicationAdmission: func(ctx context.Context, id string) error {
				return store.AdmitRun(ctx, id, false, []string{"communication.message", "communication.widget"})
			},
			Tools: broker, ToolCatalog: broker, ActionAuthorizer: broker, ApprovalChecker: broker, ApprovalService: approve, Checkpoints: inputs, Completion: writer, MemorySources: f.sources, Budgets: f.budgets,
		}, nil)
	}
	type outcome struct {
		Name, RunID, InputID, Status, Question, Result, Error string
		Asked                                                 bool
	}
	outcomes := []outcome{}
	defer func() {
		c, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		var ledger json.RawMessage
		if e := f.pool.QueryRow(c, `SELECT jsonb_build_object('attempts',count(*),'observed',count(actual_input_tokens),'unknown',count(*)-count(actual_input_tokens),'known_input_tokens',sum(actual_input_tokens),'known_output_tokens',sum(actual_output_tokens)) FROM execution_model_attempts`).Scan(&ledger); e != nil {
			t.Error("ledger capture failed")
		}
		provider.mu.Lock()
		var toolStates json.RawMessage
		if e := f.pool.QueryRow(c, `SELECT COALESCE(jsonb_agg(jsonb_build_object('tool',tool_name,'status',status,'args',args,'error',error,'result',result)),'[]'::jsonb) FROM tool_calls`).Scan(&toolStates); e != nil {
			t.Error("tool state capture", e)
		}
		data, _ := json.MarshalIndent(map[string]any{"outcomes": outcomes, "provider_starts": provider.Starts, "calls": provider.Calls, "ledger": ledger, "tool_states": toolStates}, "", "  ")
		provider.mu.Unlock()
		if e := os.WriteFile(filepath.Join(root, "evidence.json"), data, 0600); e != nil {
			t.Error(e)
		}
		t.Logf("finite native ledger: %s", ledger)
	}()
	const fruits = "Спроси меня новым вопросом с выбором нескольких вариантов, какие фрукты взять на пикник: яблоки, сливы или груши. Дождись моего ответа, затем сообщи только выбранные фрукты."
	cases := []struct {
		name, request, text string
		labels              []string
		question            bool
	}{
		{"first_question", fruits, "", []string{"яблок", "слив"}, true},
		{"identical_new_question", fruits, "", []string{"груш"}, true},
		{"new_topic", "Спроси меня текстовым вопросом, какую скатерть взять. Дождись моего ответа и затем сообщи только выбранную скатерть.", "Белую льняную скатерть с зелёной каймой.", nil, true},
		{"explicit_followup", "Повтори только мой последний выбор скатерти, без нового вопроса.", "", nil, false},
	}
	for _, tc := range cases {
		scripted.name = tc.name
		scripted.question = tc.question
		scripted.text = tc.text
		o := outcome{Name: tc.name}
		payload, _ := json.Marshal(map[string]string{"text": tc.request, "user_id": f.parent.OwnerID})
		event := ingress.InputEvent{SessionKey: f.parent.SessionKey, Source: "web", EventID: "issue147:" + tc.name, IdempotencyKey: "issue147:" + tc.name, EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, PayloadJSON: string(payload), CreatedAt: time.Now()}
		accepted, _, e := inputs.Accept(ctx, event, f.parent.OwnerID, "")
		if e != nil {
			t.Fatal(e)
		}
		o.InputID = accepted.InputID
		claim, e := inputs.Claim(ctx, 5*time.Minute)
		if e != nil {
			t.Fatal(e)
		}
		r, e := makeCore().Execute(input.WithClaim(ctx, claim), event)
		if e != nil || r == nil {
			o.Error = "execute failed"
			outcomes = append(outcomes, o)
			t.Fatalf("native execute: %v", e)
		}
		o.RunID = r.RunID
		var id string
		var qraw []byte
		qerr := f.pool.QueryRow(ctx, `SELECT interaction_id,question_json FROM communication_interactions WHERE run_id=$1 AND state='open'`, r.RunID).Scan(&id, &qraw)
		if qerr == nil {
			o.Asked = true
			var q communicationcontrol.Args
			if e = json.Unmarshal(qraw, &q); e != nil {
				t.Fatal(e)
			}
			o.Question = q.Question
			if !tc.question {
				o.Error = "unexpected extra question"
				outcomes = append(outcomes, o)
				t.Fatal(o.Error)
			}
			answer := communicationcontrol.Answer{Text: tc.text}
			for _, label := range tc.labels {
				for _, option := range q.Input.Options {
					if strings.Contains(strings.ToLower(option.Label), label) {
						answer.OptionIDs = append(answer.OptionIDs, option.ID)
					}
				}
			}
			if len(answer.OptionIDs) != len(tc.labels) {
				t.Fatal("native question omitted requested choices")
			}
			owner := owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: f.parent.OwnerID, Channel: "web", SessionKey: "web:verified", DeliveryID: "answer:" + tc.name})
			if _, e = store.Respond(owner, id, "web", 1, 1, answer); e != nil {
				t.Fatal(e)
			}
			if e = inputs.Finish(ctx, claim, "waiting_for_user", "", "", 0); e != nil {
				t.Fatal(e)
			}
			claim, e = inputs.Claim(ctx, 5*time.Minute)
			if e != nil {
				t.Fatal(e)
			}
			// A fresh Service exercises durable recovery; no provider session is reused.
			r, e = makeCore().Resume(input.WithClaim(ctx, claim), r.RunID, event)
		}
		if e != nil || r == nil {
			o.Error = "resume failed"
			outcomes = append(outcomes, o)
			t.Fatalf("native resume: %v", e)
		}
		o.Status = r.ReceiptStatus
		if r.CurrentState == commonv1.RunState_RUN_STATE_COMPLETED {
			o.Status = "completed"
		}
		var result []byte
		if e = f.pool.QueryRow(ctx, `SELECT payload FROM communication_publications WHERE run_id=$1 AND purpose='result' ORDER BY created_at DESC LIMIT 1`, r.RunID).Scan(&result); e != nil {
			o.Error = "no result publication"
		} else {
			o.Result = string(result)
			if native {
				var publication struct {
					Text string `json:"text"`
				}
				if json.Unmarshal(result, &publication) != nil || !requestScopeResultMatches(tc.name, publication.Text) {
					o.Error = "published result does not match this request's accepted answer"
				}
			}
		}
		if tc.question && !o.Asked {
			o.Error = "new question stage missing"
		}
		outcomes = append(outcomes, o)
		if e = inputs.Finish(ctx, claim, o.Status, r.ErrorCode, "", 0); e != nil {
			t.Fatal(e)
		}
		if o.Error != "" {
			t.Error(tc.name, o.Error)
		}
		if r.CurrentState != commonv1.RunState_RUN_STATE_COMPLETED {
			t.Fatalf("native run did not complete: %s %s", r.ReceiptStatus, r.ErrorCode)
		}
	}
	if !native {
		ownerStages, answeredStages := 0, 0
		for _, start := range provider.Starts {
			markers := 0
			for _, item := range start.InputItems {
				var envelope struct {
					Data struct {
						Kind    string `json:"kind"`
						Current struct {
							Run         string `json:"run_ref"`
							Phase       string `json:"phase"`
							Interaction string `json:"interaction_ref"`
						} `json:"current_request"`
					} `json:"data"`
				}
				_ = json.Unmarshal([]byte(item.Content), &envelope)
				if envelope.Data.Kind != "current_request" {
					continue
				}
				markers++
				m := envelope.Data.Current
				if m.Run != start.Context.RunID {
					t.Fatal("scope escaped the owning run")
				}
				switch m.Phase {
				case "owner_request":
					ownerStages++
					if m.Interaction != "" {
						t.Fatal("new request inherited an answer")
					}
				case "answered_question":
					answeredStages++
					var canonical bool
					if e := f.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM communication_interactions WHERE interaction_id=$1 AND run_id=$2 AND state='resolved')`, m.Interaction, m.Run).Scan(&canonical); e != nil || !canonical {
						t.Fatal("marker does not identify the accepted interaction", e)
					}
				default:
					t.Fatal("unknown request phase")
				}
			}
			if markers != 1 {
				t.Fatal("current request marker missing or duplicated", markers)
			}
		}
		if ownerStages != 4 || answeredStages != 3 {
			t.Fatal("incorrect request/answer boundaries", ownerStages, answeredStages)
		}
	}
}

// This finite gate grades the frozen synthetic answers, not arbitrary prose.
func requestScopeResultMatches(name, text string) bool {
	text = strings.ReplaceAll(strings.ToLower(text), "ё", "е")
	has := func(parts ...string) bool {
		for _, part := range parts {
			if !strings.Contains(text, part) {
				return false
			}
		}
		return true
	}
	switch name {
	case "first_question":
		return has("яблок", "слив") && !has("груш")
	case "identical_new_question":
		return has("груш") && !has("яблок") && !has("слив")
	default:
		return has("бел", "льнян", "зелен", "кайм") && !has("яблок") && !has("слив") && !has("груш")
	}
}

// Scripted transport only exercises storage/Broker/answer/recovery plumbing;
// its responses are never counted as native semantic acceptance.
type scopeFixtureProvider struct {
	communicationScript
	f          *delegationGuardFixture
	name, text string
	question   bool
	serial     int
}

func (p *scopeFixtureProvider) StartRun(ctx context.Context, q transport.StartRunRequest) (transport.EventStream, error) {
	id := q.Context.RunID
	var questions, results int
	if e := p.f.pool.QueryRow(ctx, `SELECT count(*) FILTER(WHERE purpose='question'),count(*) FILTER(WHERE purpose='result') FROM communication_publications WHERE run_id=$1`, id).Scan(&questions, &results); e != nil {
		return nil, e
	}
	if results > 0 {
		return communicationEvents(transport.NewAssistantFinalEvent(id, "fixture", transport.AssistantFinal{Content: "done", FinishReason: "stop"}), transport.NewTerminalEvent(id, transport.EventTypeRunCompleted, "fixture")), nil
	}
	tool, raw := "communication.message", `{"schema_version":1,"purpose":"result","text":"Груши"}`
	if questions > 0 {
		var interaction, publication string
		if e := p.f.pool.QueryRow(ctx, `SELECT interaction_id,publication_id FROM communication_interactions WHERE run_id=$1 AND state='resolved'`, id).Scan(&interaction, &publication); e != nil {
			return nil, e
		}
		encoded, _ := json.Marshal(map[string]any{"schema_version": 1, "purpose": "result", "text": "Груши", "source_refs": []string{"message:response:" + interaction}, "reply_to_publication_ref": publication})
		raw = string(encoded)
	}
	if p.question && questions == 0 {
		tool = "communication.widget"
		raw = `{"schema_version":1,"question":"Какие фрукты взять?","input":{"type":"multi_choice","options":[{"id":"a","label":"Яблоки"},{"id":"b","label":"Сливы"},{"id":"c","label":"Груши"}]}}`
		if p.text != "" {
			raw = `{"schema_version":1,"question":"Какую скатерть взять?","input":{"type":"text"}}`
		}
	}
	p.serial++
	return communicationEvents(transport.NewToolCallRequestedEvent(id, "fixture", transport.ToolCallRequest{ToolCallRef: fmt.Sprintf("scope-%d", p.serial), ToolName: tool, ArgsJSON: raw})), nil
}
func (p *scopeFixtureProvider) SubmitToolResult(ctx context.Context, q transport.SubmitToolResultRequest) (transport.EventStream, error) {
	return p.StartRun(ctx, transport.StartRunRequest{Context: transport.TransportRunContext{RunID: q.RunID}})
}
