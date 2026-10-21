package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	approvals "github.com/butler/butler/apps/orchestrator/internal/approval"
	"github.com/butler/butler/apps/orchestrator/internal/channel/telegramrich"
	"github.com/butler/butler/apps/orchestrator/internal/communication"
	"github.com/butler/butler/apps/orchestrator/internal/completion"
	"github.com/butler/butler/apps/orchestrator/internal/deliveryoutbox"
	"github.com/butler/butler/apps/orchestrator/internal/input"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
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

type communicationScript struct {
	f            *delegationGuardFixture
	starts       int
	runID, final string
	question     bool
	scenario     string
}

func (*communicationScript) Name() string                    { return "fixture" }
func (*communicationScript) InferenceLimits() (int64, int64) { return 16384, 2048 }
func (*communicationScript) Capabilities(context.Context, transport.TransportRunContext) (transport.CapabilitySnapshot, error) {
	return transport.CapabilitySnapshot{}, nil
}
func (p *communicationScript) StartRun(ctx context.Context, q transport.StartRunRequest) (transport.EventStream, error) {
	p.starts++
	p.runID = q.Context.RunID
	if q.Context.ProviderSessionRef != nil {
		return nil, errors.New("stale provider context")
	}
	if p.starts == 1 && os.Getenv("BUTLER_COMMUNICATION_REQUIRE_REPORT") == "1" {
		ready := false
		for _, d := range q.ToolDefinitions {
			if d.Name == "communication.report" {
				ready = true
			}
		}
		if !ready {
			return nil, errors.New("ready report missing from initial model catalog")
		}
	}
	var n int
	if e := p.f.pool.QueryRow(ctx, `SELECT count(*) FROM communication_publications WHERE run_id=$1`, p.runID).Scan(&n); e != nil {
		return nil, e
	}
	needed := 2
	if strings.HasPrefix(p.scenario, "missing_response") {
		needed = 1
	}
	if n >= needed || p.scenario == "quiet" || strings.HasPrefix(p.scenario, "missing_response") && p.starts == 1 {
		return communicationEvents(transport.NewAssistantFinalEvent(p.runID, "fixture", transport.AssistantFinal{Content: p.final, FinishReason: "stop"}), transport.NewTerminalEvent(p.runID, transport.EventTypeRunCompleted, "fixture")), nil
	}
	purpose, text, ref := "progress", "Working on the request", "message-1"
	if n == 1 {
		purpose, text, ref = "result", "Confirmed bounded result", "message-2"
	}
	raw, _ := json.Marshal(map[string]any{"schema_version": 1, "purpose": purpose, "text": text})
	tool := "communication.message"
	if p.scenario == "report_result" && n == 1 {
		tool = "communication.report"
		raw = []byte(`{"schema_version":1,"purpose":"result","title":"Synthetic report","summary":"Synthetic summary","blocks":[{"type":"table","rows":[["Component","Result"],["Core","Connected"],["Broker","Checked"]]}]}`)
	}
	if p.question && n == 0 {
		tool = "communication.widget"
		raw = []byte(`{"schema_version":1,"question":"Choose","input":{"type":"single_choice","options":[{"id":"a","label":"A"}]}}`)
	}
	return communicationEvents(transport.NewAssistantDeltaEvent(p.runID, "fixture", transport.AssistantDelta{Content: "private commentary"}), transport.NewToolCallRequestedEvent(p.runID, "fixture", transport.ToolCallRequest{ToolCallRef: ref, ToolName: tool, ArgsJSON: string(raw)})), nil
}
func (p *communicationScript) SubmitToolResult(ctx context.Context, q transport.SubmitToolResultRequest) (transport.EventStream, error) {
	return p.StartRun(ctx, transport.StartRunRequest{Context: transport.TransportRunContext{RunID: q.RunID}})
}
func (p *communicationScript) ContinueRun(_ context.Context, q transport.ContinueRunRequest) (transport.EventStream, error) {
	if !strings.HasPrefix(p.scenario, "missing_response") {
		return nil, errors.New("unexpected missing-response correction")
	}
	p.starts++
	if p.scenario == "missing_response_stream_failure" {
		return nil, errors.New("private provider failure")
	}
	if p.scenario == "missing_response_exhausted" {
		return communicationEvents(transport.NewAssistantFinalEvent(q.RunID, "fixture", transport.AssistantFinal{Content: "private unfulfilled final", FinishReason: "stop"}), transport.NewTerminalEvent(q.RunID, transport.EventTypeRunCompleted, "fixture")), nil
	}
	return communicationEvents(transport.NewToolCallRequestedEvent(q.RunID, "fixture", transport.ToolCallRequest{ToolCallRef: "correction-result", ToolName: "communication.message", ArgsJSON: `{"schema_version":1,"purpose":"result","text":"Corrected bounded result"}`})), nil
}
func (*communicationScript) CancelRun(context.Context, transport.CancelRunRequest) (*transport.TransportEvent, error) {
	return nil, nil
}
func communicationEvents(events ...transport.TransportEvent) transport.EventStream {
	ch := make(chan transport.TransportEvent, len(events))
	for _, e := range events {
		ch <- e
	}
	close(ch)
	return ch
}

type failCommunicationFinal struct {
	real completion.Writer
	fail bool
}

func (w *failCommunicationFinal) Save(ctx context.Context, q completion.Request) (completion.FinalRecord, error) {
	if w.fail {
		w.fail = false
		return completion.FinalRecord{}, errors.New("fixture final storage unavailable")
	}
	return w.real.Save(ctx, q)
}

func TestCommunicationActualCoreBrokerTwoMessagesAndFrozenFinalIntegration(t *testing.T) {
	communicationCoreScenarios(t, false)
}

func TestCommunicationTelegramComposedCoreBrokerIntegration(t *testing.T) {
	communicationCoreScenarios(t, true)
}

func TestCommunicationTelegramReadyCatalogIntegration(t *testing.T) {
	t.Setenv("BUTLER_COMMUNICATION_NATIVE_HARNESS", "1")
	t.Setenv("BUTLER_COMMUNICATION_REQUIRE_REPORT", "1")
	communicationCoreScenarios(t, true)
}

func communicationCoreScenarios(t *testing.T, telegramMode bool, native ...bool) {
	isNative := len(native) > 0 && native[0]
	if os.Getenv("BUTLER_GUARDED_REQUEST_BROKER_BINARY") == "" {
		t.Skip("actual private Broker test binary required")
	}
	for _, scenario := range []string{"empty_terminal", "different_terminal", "lost_private_ack", "private_outcome_unknown", "frozen_save_recovery", "widget_handoff_after_25_hours", "quiet", "missing_response_corrected", "missing_response_exhausted", "missing_response_stream_failure", "report_result"} {
		if scenario == "report_result" && (!telegramMode || os.Getenv("BUTLER_COMMUNICATION_REQUIRE_REPORT") != "1") {
			continue
		}
		if isNative && scenario != "different_terminal" {
			continue
		}
		if telegramMode && scenario == "widget_handoff_after_25_hours" {
			continue
		}
		t.Run(scenario, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "communication-deletions.log")
			f := newDelegationGuardFixtureWithManifest(t, path)
			if isNative || os.Getenv("BUTLER_COMMUNICATION_REQUIRE_REPORT") == "1" {
				ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
				t.Cleanup(cancel)
				f.ctx = ctx
				f.budgets.Policy.Models["antigravity:gemini-3.8-flash-high"] = budget.ModelPrice{Basis: "subscription_no_overage"}
				f.budgets.Policy.Conversation.ModelTurns = 6
				// The actual provider reserves its full 1 Mi-token context for each
				// attempt. Keep six finite turns without understating that contract.
				f.budgets.Policy.Conversation.Tokens = 6 * (1048576 + 8192)
				f.budgets.Policy.Day.ModelTurns = 6
				f.budgets.Policy.Day.Tokens = 6 * (1048576 + 8192)
				t.Cleanup(func() {
					var report []byte
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					if err := f.pool.QueryRow(ctx, `SELECT jsonb_build_object('model_attempts',count(*),'observed_attempts',count(actual_input_tokens),'unknown_attempts',count(*)-count(actual_input_tokens),'input_tokens',CASE WHEN count(*)=count(actual_input_tokens) THEN sum(actual_input_tokens) END,'output_tokens',CASE WHEN count(*)=count(actual_output_tokens) THEN sum(actual_output_tokens) END) FROM execution_model_attempts`).Scan(&report); err == nil {
						label := "synthetic"
						if isNative {
							label = "native"
						}
						t.Logf("%s finite ledger: %s", label, report)
					}
				})
			}
			f.budgets.Policy.Models["fixture:fixture"] = budget.ModelPrice{Basis: "local_no_billing"}
			inputs := input.NewStore(f.pool)
			inputs.CommunicationEpoch = 1
			sessions := session.NewPostgresRepository(f.pool)
			runs := run.NewService(run.NewPostgresRepository(f.pool), nil)
			repo := approvals.NewPostgresRepository(f.pool)
			repo.SetCheckpointHooks(inputs)
			approve := approvals.NewService(repo, nil)
			reader := &actionContextReader{runs: runs, sessions: sessions, leases: f.leases, tasks: f.store, approvals: approve, pool: f.pool, communicationReady: true}
			store := &communication.Store{Pool: f.pool, Inputs: inputs, SourceGuard: f.sources.CheckRunSourcesTx, ChannelGuard: func(context.Context, pgx.Tx, string, communication.Binding, communicationcontrol.Args) error {
				return nil
			}}
			var telegramFlow *telegramCommunication
			if telegramMode {
				b := communication.Binding{ID: "telegram", Version: 1, Channel: "telegram", Recipient: "telegram:42"}
				policy := telegramExportPolicy{owner: f.parent.OwnerID, binding: b, allowPrivate: true, catalog: f.sources, sourceGuard: f.sources.CheckRunSourcesTx}
				store.ChannelGuard = policy.check
				store.PlanDelivery = (telegramrich.Planner{Profile: telegramrich.Capabilities{Message: true, Photo: true, Rich: true, Document: true}, Resolve: policy.media}).Plan
				telegramFlow = &telegramCommunication{store: store, binding: telegramrich.OwnerBinding{OwnerID: f.parent.OwnerID, BindingID: b.ID, Version: 1, BotID: 77, ChatID: 42, UserID: 42}, policy: policy}
				telegramFlow.installInputHooks()
				if isNative {
					chat, err := strconv.ParseInt(os.Getenv("BUTLER_TEST_TELEGRAM_RICH_CHAT_ID"), 10, 64)
					if err != nil {
						t.Fatal("private test chat required")
					}
					telegramFlow, err = newTelegramCommunication(f.ctx, f.pool, f.sources, inputs, f.sources.CheckRunSourcesTx, TelegramCommunicationOptions{OwnerID: f.parent.OwnerID, BindingID: b.ID, ChatID: chat, ExpectedBot: os.Getenv("BUTLER_TEST_TELEGRAM_RICH_BOT"), TokenFile: os.Getenv("BUTLER_TEST_TELEGRAM_RICH_TOKEN_FILE"), BotAPIProxyFile: os.Getenv("BUTLER_TEST_TELEGRAM_RICH_PROXY_FILE"), AllowPrivate: true})
					if err != nil {
						t.Fatal("native bot composition unavailable")
					}
					store = telegramFlow.store
				}
			}
			for _, ch := range []string{"web", "telegram"} {
				if isNative {
					continue
				} // The verified constructor already bound it.
				if telegramMode && ch == "web" {
					continue
				}
				recipient := ch + ":verified"
				capabilities := []string{"message", "widget"}
				if telegramMode {
					recipient = "telegram:42"
					capabilities = []string{"message", "photo", "report"}
				}
				actor := owneridentity.WithActor(f.ctx, owneridentity.Actor{OwnerID: f.parent.OwnerID, Channel: ch, SessionKey: recipient, DeliveryID: "bind"})
				if e := store.Bind(actor, communication.Binding{ID: ch, Channel: ch, Recipient: recipient, Enabled: true, Capabilities: capabilities}, 0); e != nil {
					t.Fatal(e)
				}
			}
			const token = "guarded-request-private-process-fixture-token"
			h, e := communicationcontrol.NewHandler(token, &CommunicationControl{Store: store})
			if e != nil {
				t.Fatal(e)
			}
			lost := false
			wrapped := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				raw, _ := io.ReadAll(r.Body)
				r.Body = io.NopCloser(bytes.NewReader(raw))
				var env communicationcontrol.Envelope
				_ = json.Unmarshal(raw, &env)
				if scenario == "private_outcome_unknown" && env.Lookup {
					http.Error(w, "fixture receipt unavailable", 503)
					return
				}
				if (scenario == "lost_private_ack" || scenario == "private_outcome_unknown") && !env.Lookup && !lost {
					record := httptest.NewRecorder()
					h.ServeHTTP(record, r)
					if record.Code == 200 {
						lost = true
						http.Error(w, "fixture lost ACK", 503)
						return
					}
					for key, values := range record.Header() {
						w.Header()[key] = values
					}
					w.WriteHeader(record.Code)
					_, _ = w.Write(record.Body.Bytes())
					return
				}
				h.ServeHTTP(w, r)
			})
			stub := &dispatchAdmissionFixture{f: f, reader: reader, manifestPath: path, call: &toolbrokerv1.ToolCall{ToolName: "communication.message"}}
			broker := controlBrokerProcess(t, stub, wrapped, communicationcontrol.Path, "BUTLER_COMMUNICATION_HARNESS=1")
			writer := completion.New(f.pool)
			writer.Communication = store
			writer.DataGuard = f.sources.CheckRunSourcesTx
			finalWriter := &failCommunicationFinal{real: writer, fail: scenario == "frozen_save_recovery"}
			provider := &communicationScript{f: f, question: scenario == "widget_handoff_after_25_hours", scenario: scenario}
			var model transport.ModelProvider = provider
			providerName, modelName := "fixture", "fixture"
			if isNative {
				model = communicationNativeProvider(t, provider)
				providerName, modelName = "antigravity", "gemini-3.8-flash-high"
			}
			if scenario == "different_terminal" {
				provider.final = "Different private terminal text"
			}
			makeCore := func() *flow.Service {
				mode := commonv1.AutonomyMode_AUTONOMY_MODE_1
				if isNative || os.Getenv("BUTLER_COMMUNICATION_REQUIRE_REPORT") == "1" {
					mode = commonv1.AutonomyMode_AUTONOMY_MODE_2
				}
				return flow.NewService(sessions, f.leases, runs, transcript.NewStore(f.pool), model, flow.Config{OwnerID: "communication-fixture-worker", ProviderName: providerName, ModelName: modelName, LeaseTTL: 30, AutonomyMode: mode, Communication: store, CommunicationAdmission: func(ctx context.Context, id string) error {
					if telegramFlow != nil {
						if scenario == "quiet" {
							return store.AdmitRun(ctx, id, true, []string{"communication.message", "communication.photo", "communication.report"})
						}
						return telegramFlow.admit(ctx, id)
					}
					return store.AdmitRun(ctx, id, scenario == "quiet", []string{"communication.message", "communication.widget"})
				}, Tools: broker, ToolCatalog: broker, ActionAuthorizer: broker, ApprovalChecker: broker, ApprovalService: approve, Checkpoints: inputs, Completion: finalWriter, MemorySources: f.sources, Budgets: f.budgets}, nil)
			}
			event := ingress.InputEvent{SessionKey: f.parent.SessionKey, Source: "web", EventID: "communication-input", IdempotencyKey: "communication-input", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, PayloadJSON: `{"text":"Return the bounded result","user_id":"guard-owner"}`, CreatedAt: time.Now()}
			var accepted input.Record
			if telegramFlow != nil {
				text := "Return the bounded result"
				if isNative {
					text = "Это изолированная проверка общения Butler на синтетических данных. Сначала отправь одно короткое сообщение communication.message с purpose=progress о начале проверки. После получения результата инструмента отправь один communication.report с purpose=result, заголовком «Проверка живого агента», кратким summary и таблицей: компонент/результат, Core/подключён, Broker/проверен. Вложения не нужны. Выполняй по одному вызову инструмента за ход. После успешной публикации заверши выполнение коротким внутренним итогом, не повторяй уже опубликованное. Не создавай других сообщений, покупок или внешних действий."
				}
				u := telegramrich.Update{ID: 1, Message: &telegramrich.Message{ID: 1, Text: text, Chat: telegramrich.Chat{ID: telegramFlow.binding.ChatID, Type: "private"}, From: telegramrich.User{ID: telegramFlow.binding.UserID}}}
				i := &telegramrich.Ingress{Store: store, Binding: telegramFlow.binding, Conversation: telegramFlow.conversation}
				if e = i.Reply(f.ctx, u); e != nil {
					t.Fatal(e)
				}
				var inputID string
				if e = f.pool.QueryRow(f.ctx, `SELECT input_id FROM accepted_inputs WHERE event_id=$1`, fmt.Sprintf("telegram-rich:%d:1", telegramFlow.binding.BotID)).Scan(&inputID); e != nil {
					t.Fatal(e)
				}
				accepted, e = inputs.Get(f.ctx, inputID)
				event = accepted.Event
			} else {
				accepted, _, e = inputs.Accept(f.ctx, event, f.parent.OwnerID, "")
			}
			if e != nil {
				t.Fatal(e)
			}
			claimTTL := time.Minute
			if isNative {
				claimTTL = 5 * time.Minute
			}
			claim, e := inputs.Claim(f.ctx, claimTTL)
			if e != nil {
				t.Fatal(e)
			}
			result, e := makeCore().Execute(input.WithClaim(f.ctx, claim), event)
			for decisions := 0; decisions < 5 && e == nil && result != nil && result.ReceiptStatus == "waiting_for_user"; decisions++ {
				var id string
				e = f.pool.QueryRow(f.ctx, `SELECT approval_id FROM approvals WHERE run_id=$1 AND status='pending'`, result.RunID).Scan(&id)
				if errors.Is(e, pgx.ErrNoRows) && provider.question {
					if e = f.pool.QueryRow(f.ctx, `SELECT interaction_id FROM communication_interactions WHERE run_id=$1 AND state='open'`, result.RunID).Scan(&id); e != nil {
						t.Fatal(e)
					}
					if _, e = f.pool.Exec(f.ctx, `UPDATE accepted_inputs SET expires_at=clock_timestamp()-INTERVAL '25 hours' WHERE input_id=$1`, accepted.InputID); e != nil {
						t.Fatal(e)
					}
					actor := owneridentity.WithActor(f.ctx, owneridentity.Actor{OwnerID: f.parent.OwnerID, Channel: "telegram", SessionKey: "telegram:verified", DeliveryID: "preference"})
					if _, e = store.Respond(actor, id, "telegram", 1, 1, communicationcontrol.Answer{OptionIDs: []string{"a"}}); e != nil {
						t.Fatal(e)
					}
				} else {
					if e != nil {
						t.Fatal(e)
					}
					actor := owneridentity.WithActor(f.ctx, owneridentity.Actor{OwnerID: f.parent.OwnerID, Channel: "web", SessionKey: f.parent.SessionKey, DeliveryID: id})
					if _, _, e = approve.ResolveByApprovalID(actor, approvals.ResolveByApprovalIDParams{ApprovalID: id, Approved: true, ResolvedVia: "web", ActorType: "web", ActorID: f.parent.OwnerID, ResolvedBy: f.parent.OwnerID}); e != nil {
						t.Fatal(e)
					}
				}
				if e = inputs.Finish(f.ctx, claim, "waiting_for_user", "", "", 0); e != nil {
					t.Fatal(e)
				}
				claim, e = inputs.Claim(f.ctx, time.Minute)
				if e != nil {
					t.Fatal(e)
				}
				result, e = makeCore().Resume(input.WithClaim(f.ctx, claim), result.RunID, event)
			}
			if scenario == "frozen_save_recovery" {
				if e == nil {
					t.Fatal("final fault not reached")
				}
				starts := provider.starts
				if e = inputs.Finish(f.ctx, claim, "queued", "fixture_final_failure", "", 0); e != nil {
					t.Fatal(e)
				}
				claim, e = inputs.Claim(f.ctx, time.Minute)
				if e != nil {
					t.Fatal(e)
				}
				result, e = makeCore().Resume(input.WithClaim(f.ctx, claim), provider.runID, event)
				if provider.starts != starts {
					t.Fatal("frozen save invoked model")
				}
			}
			failedCorrection := scenario == "missing_response_exhausted" || scenario == "missing_response_stream_failure"
			blocked := failedCorrection || scenario == "private_outcome_unknown"
			if e != nil || result == nil || !blocked && result.CurrentState != commonv1.RunState_RUN_STATE_COMPLETED || blocked && result.ReceiptStatus != "blocked" {
				t.Fatal("real Core/Broker flow failed", result, e)
			}
			if scenario == "lost_private_ack" && !lost {
				t.Fatal("lost ACK not reached")
			}
			pubs, deliveries, finals, wantEffects, wantTurns := 2, 4, 1, 2, 3
			effectState := "effect_confirmed"
			if scenario == "private_outcome_unknown" {
				pubs, deliveries, finals, wantEffects, wantTurns = 1, 2, 0, 1, 1
				effectState = "effect_unknown"
				if !lost || result.ErrorCode != "effect_unknown" {
					t.Fatal("uncertain local effect silently continued", result)
				}
			}
			if scenario == "quiet" {
				pubs, deliveries, finals, wantEffects, wantTurns = 0, 0, 1, 0, 1
			}
			if scenario == "missing_response_corrected" {
				pubs, deliveries, finals, wantEffects, wantTurns = 1, 2, 1, 1, 3
			}
			if failedCorrection {
				pubs, deliveries, finals, wantEffects, wantTurns = 1, 2, 0, 0, 2
			}
			if telegramMode {
				deliveries = pubs
			}
			if isNative {
				wantTurns = provider.starts
				if wantTurns < 3 || wantTurns > 6 {
					t.Fatal("native model exceeded finite turn acceptance", wantTurns)
				}
			}
			for table, want := range map[string]int{"communication_publications": pubs, "delivery_outbox": deliveries, "run_final_results": finals} {
				var n int
				if e = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM `+table+` WHERE run_id=$1`, provider.runID).Scan(&n); e != nil || n != want {
					t.Fatal(table, n, want, e)
				}
			}
			var effects, turns int
			if e = f.pool.QueryRow(f.ctx, `SELECT count(DISTINCT e.action_id) FROM action_effects e JOIN action_intents i ON i.effect_id=e.action_id WHERE i.run_id=$1 AND e.state=$2`, provider.runID, effectState).Scan(&effects); e != nil || effects != wantEffects {
				t.Fatal("effects repeated or unconfirmed", effects, e)
			}
			if e = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM execution_model_attempts WHERE run_id=$1`, provider.runID).Scan(&turns); e != nil || turns != provider.starts || turns != wantTurns {
				t.Fatal("model allowance reset or unaccounted", turns, provider.starts, e)
			}
			var finalKind, finalRole string
			if finals > 0 {
				if e = f.pool.QueryRow(f.ctx, `SELECT metadata->>'kind',role FROM messages WHERE message_id=$1`, "final:"+provider.runID).Scan(&finalKind, &finalRole); e != nil || finalKind != "attempt_final" || finalRole != "system" {
					t.Fatal("final became public", finalKind, finalRole, e)
				}
			}
			if failedCorrection {
				var count int
				var state, body string
				if e = f.pool.QueryRow(f.ctx, `SELECT correction_count,obligation_state FROM communication_admissions WHERE input_id=$1`, accepted.InputID).Scan(&count, &state); e != nil || count != 1 || state != "closed_with_failure" {
					t.Fatal("unbounded correction", count, state, e)
				}
				if e = f.pool.QueryRow(f.ctx, `SELECT payload->>'text' FROM communication_publications WHERE run_id=$1`, provider.runID).Scan(&body); e != nil || strings.Contains(body, "private") {
					t.Fatal("unsafe final/error fallback", body, e)
				}
			}
			saved, e := inputs.Get(f.ctx, accepted.InputID)
			if e != nil || saved.RunID != provider.runID {
				t.Fatal("new input/run replay", saved, e)
			}
			if telegramMode && scenario != "private_outcome_unknown" {
				d := (&communication.DeliveryStore{Core: store}).ForBinding(telegramFlow.policy.binding)
				var nativeSender deliveryoutbox.Sender
				if isNative {
					controls := task.NewController(f.store, f.leases.Authority(), f.leases, run.NewPostgresRepository(f.pool), repo, inputs, transcript.NewStore(f.pool))
					adapter, err := telegramFlow.adapter(controls)
					if err != nil {
						t.Fatal(err)
					}
					nativeSender = adapter
				}
				for n := 0; n < pubs; n++ {
					m, err := d.Claim(f.ctx, 30*time.Second)
					if err != nil || m == nil {
						t.Fatal("missing planned Telegram part", err)
					}
					if err = d.AuthorizeSend(f.ctx, *m); err != nil {
						t.Fatal(err)
					}
					var part telegramrich.Part
					if json.Unmarshal([]byte(m.Content), &part) != nil || !isNative && part.Method != "sendMessage" && !(scenario == "report_result" && part.Method == "sendRichMessage") || strings.Contains(part.Text, "private") {
						t.Fatal("unplanned/implicit Telegram fallback")
					}
					outcome := deliveryoutbox.Outcome{Status: "sent", ACK: fmt.Sprint(n + 1)}
					if nativeSender != nil {
						outcome = nativeSender.SendStored(f.ctx, *m)
						if outcome.Status != "sent" {
							t.Fatal("native channel outcome", outcome.Status, outcome.ErrorCode)
						}
						t.Logf("native Telegram part=%d method=%s ACK=%s", n, part.Method, outcome.ACK)
					}
					if err = d.Finish(f.ctx, *m, outcome); err != nil {
						t.Fatal(err)
					}
				}
				if m, err := d.Claim(f.ctx, 30*time.Second); err != nil || m != nil {
					t.Fatal("unexpected final/status duplicate", m, err)
				}
			}
		})
	}
}
