# Butler — Model Transport Contract

Memory query locality is independent of provider release. A local-only task can
use authorized PostgreSQL evidence and the pinned internal inference service;
the resulting query/snippets remain subject to current task privacy, source
revision/access and complete-request guards before cloud transport. Local query
permission and zero incremental billing do not authorize external generation.

Trusted composition may install request-local `WithInferenceAuthority` checks.
Native Antigravity checks this callback before authentication and immediately
before HTTP dispatch, alongside caller-owned budgets and in-flight/final guards.
It is never serialized or model-selected and grants no authority. Unhooked calls
preserve their contract; cancellation and current-source stale/timeout diagnostics
remain distinct. No retry or fallback follows from the callback.

Provider timeout and owning model-budget deadline are independent bounds; the
earliest task, mandate, lease or caller deadline still wins. The Orchestrator's
legacy BUTLER_OPENAI_TIMEOUT_SECONDS setting supplies the selected provider's
timeout, including Antigravity. A five-minute Gemini setting therefore requires
300 transport seconds and model_millis300000 in the operator budget policy.
New memory-eval trials explicitly use both five-minute bounds; sealed historical
two-minute observations and scoring thresholds are not rewritten.

The lab-only V4 observation collector preserves bounded filtered final bytes
after owning SQL spend persistence and separately records immutable encoding,
retention and lifecycle facts. It forwards the original events unchanged and
never stores reasoning, raw provider frames/errors or credentials. Provisional
versions cannot establish a completed native observation. Real High is frozen
to the requested profile and observed version before the new window; old failures
and unknown measurements remain immutable. See
[V4 evidence/accounting](../ai/scenario-high-v4-history-accounting-20261013.md).

Optional `TransportEvent.ProviderModelVersion` carries an actually observed
provider version on complete native outcomes. It is independent of the requested
model profile and proprietary weight bytes. Missing metadata remains unavailable;
malformed, aliased or conflicting native versions deny a complete release. No
route, authority or model promotion follows from a version observation.

Reviewed skill descriptions and selected procedures are private Document DATA,
never system instructions. Fresh recovery reloads pinned versions through the
current catalog; revocation/dependency failure stops before provider release.
Skill authority and [release conditions](reviewed-skills.md) are independent of
provider-side continuation state.

[Execution budgets](execution-budget-contract.md) reserve declared conservative
input/output/time before each actual inference request. Exact operator cost basis
and current assigned privacy policy are required. Private
`transport.WithInputByteLimit` constrains complete native serialized history and
tool declarations before HTTP write; unset cap is finite. Native Antigravity
declares inference limits; undeclared adapters fail closed in admitted production
execution. No automatic model fallback or overage follows from a budget quote.

Execution failure diagnostics retain the general `transport_error` run class and
a fixed subtype in its error message. `transport.DiagnosticCode` allows only
declared transport codes, including `memory_source_stale` and
`memory_source_check_timeout`. Provider text/codes/details and unknown type strings
are discarded before execution logging, run persistence and input-worker return.
An owning source-check timeout is distinct from a provider timeout; both deny
release. These diagnostics do not grant a retry or relax any source deadline.

Antigravity HTTP rejection errors retain only the numeric HTTP status as typed
metadata. `DiagnosticHTTPStatus` exports the finite300–599 range, otherwise0;
response bodies/headers remain excluded. Memory-eval also projects six exact
transport-owned protocol literals onto fixed reason codes, never substring
matches or arbitrary error text. Earlier lost statuses/reasons remain unknown.
These fields observe a failed admission and grant no retry or continuation.
For400/422, a64KiB private INVALID_ARGUMENT envelope can be projected onto four
fixed diagnostic classes (unknown minItems/maxItems, response-schema rejection,
other invalid argument). `DiagnosticReason` rejects all other strings. Provider
messages, field paths/details and raw bytes remain unexported; a class records an
upstream claim, not proof of the precise cause. Earlier unknowns stay unknown.

Native memory-eval on laba follows the established private antigravity-proxy route
and bypasses loopback/own PostgreSQL/local-models. New operator pins require this
route; actual Docker environment is reduced to proxy/bypass booleans checked
before provider auth. Historical roots retain their original direct-route proof.
Native auth and possible OAuth refresh use only the runtime's strict encrypted
PostgreSQL SecretStore after its durable one-use entry. A disposable metadata
process must never resolve refresh-capable credentials. The rejected historical
metadata probe proves profile availability only; its possible rotation is unknown
and it does not prove generation capability or allow retry.

## Планируемое отделение transport output от публикации

В целевом [explicit communication profile](user-communication-spec.md) события
assistant_delta/assistant_final сохраняют transport-смысл, но не служат командой
отправки пользователю. communication tools используют обычный tool-call контракт.
Transport не выбирает канал, получателя, widget state или продолжение задачи.
Это дополнение не объявляет новый профиль включённым в текущем baseline.

## 1. Статус документа

**Тип документа:** Architecture Subspec / Model Transport Contract
**Версия:** 0.1
**Статус:** Draft
**Связанные документы:** Butler PRD + Architecture, Butler Run Lifecycle Specification, Tooling and Execution Specification, Technology Stack and Engineering Standards

---

## 2. Назначение

Этот документ формально описывает контракт между `Orchestrator` и `Model Transport Layer` в системе Butler.

Model Transport Layer — это абстракция над конкретными model providers (провайдерами моделей), которая обеспечивает единый способ:

* запускать model turns;
* продолжать model loop после tool results;
* получать streaming events;
* обрабатывать provider-side session state;
* отменять активное исполнение;
* нормализовать provider-specific behavior.

Цель документа — зафиксировать:

* интерфейс взаимодействия;
* модель команд и событий;
* модель ошибок;
* поведение при streaming;
* поведение при tool calls;
* различие между Butler run и provider-side session.

---

## 3. Основные принципы

### 3.1 Transport is not memory

Model Transport Layer не является источником истины для памяти, transcript history или session identity.

### 3.2 Transport is not orchestration

Model Transport Layer не принимает решений о том:

* какие tools доступны;
* какую память подтягивать;
* нужно ли approval;
* как интерпретировать бизнес-логику run.

### 3.3 Transport normalizes provider behavior

Model Transport Layer обязан скрывать различия между:

* WebSocket-first providers;
* request/response providers;
* streaming и non-streaming providers;
* stateful и stateless providers.

### 3.4 Orchestrator owns run semantics

`Orchestrator` владеет run lifecycle и использует transport только как execution channel к модели.

### 3.5 Provider-side session is external state

Provider-side session может существовать, но она не заменяет Butler session и не определяет run semantics.

### 3.6 Provider auth is resolved before transport

OAuth/device-flow логика, refresh tokens и хранение provider credentials находятся в системном control-plane слое Butler.
Transport получает только runtime auth material (например, bearer token и provider-specific headers) и не выполняет login flow самостоятельно.

---

## 4. Задачи Model Transport Layer

Model Transport Layer должен обеспечивать:

1. старт model turn;
2. продолжение model turn после tool result;
3. streaming delivery model events;
4. provider capability negotiation;
5. cancellation;
6. нормализацию tool call requests;
7. нормализацию финального model output;
8. привязку к provider-side session references;
9. единый error model.

---

## 5. Границы ответственности

### 5.1 Что делает Orchestrator

Orchestrator:

* собирает context bundle;
* выбирает model provider и model profile;
* определяет доступные tools;
* определяет autonomy mode;
* решает, что делать с tool calls;
* решает, какие события отправлять в Channel Adapter;
* управляет run state machine.

### 5.2 Что делает Model Transport Layer

Model Transport Layer:

* принимает transport command от Orchestrator;
* преобразует её в provider-specific request/session operation;
* получает provider events;
* преобразует их в transport events Butler;
* поддерживает provider session references;
* возвращает нормализованную ошибку при сбое.

### 5.3 Что Transport не делает

Transport не должен:

* читать memory stores напрямую;
* вызывать tools напрямую;
* управлять approval flow;
* обновлять transcript как source of truth;
* принимать решения о retry вне transport-local semantics.

---

## 6. Provider classes

В Butler поддерживаются две категории providers.

### 6.1 Cloud providers

Примеры:

* OpenAI
* другие remote model APIs

Характеристики:

* могут поддерживать WebSocket-first transport;
* могут поддерживать provider-side sessions;
* могут поддерживать tool calling;
* могут поддерживать streaming.

### 6.2 Local providers

Примеры:

* локальные model servers
* self-hosted inference endpoints

Характеристики:

* могут не поддерживать WebSocket transport;
* могут быть stateless;
* могут поддерживать streaming частично или не поддерживать вообще.

### 6.3 Нормативное правило

WebSocket-first transport является обязательной стратегией **для providers, где это поддерживается**.
Для локальных моделей допускается другой transport backend, но внешний logical contract для Orchestrator должен оставаться тем же.

---

## 7. Logical contract overview

Orchestrator взаимодействует с Model Transport Layer через пять базовых операций:

1. `start_run()`
2. `continue_run()`
3. `submit_tool_result()`
4. `stream_events()`
5. `cancel_run()`

Это логические операции. Их конкретная реализация может отличаться у cloud/local providers.

Для текущего Butler baseline принято следующее wire-level решение:
- `StartRun`, `ContinueRun` и `SubmitToolResult` являются server-streaming RPC и возвращают поток `TransportEvent`;
- `StreamEvents` остаётся отдельным наблюдательным streaming RPC для случаев пассивной подписки на уже идущий run;
- `CancelRun` возвращает одиночное подтверждение/terminal event.

Для OpenAI API baseline V1 transport backend должен предпочитать Realtime WebSocket (`wss`) и переходить на HTTP SSE fallback, если WebSocket backend недоступен на раннем этапе запуска, не меняя logical contract, видимый Orchestrator.
Для OpenAI Codex и GitHub Copilot допускается HTTP streaming backend, если provider не поддерживает совместимый WebSocket path.

### 7.1 Native Antigravity subscription adapter

`antigravity` uses Go HTTP/SSE directly. The retained default tiered profile uses production AICode v1
`https://aicode.googleapis.com/v1:streamGenerateContent?alt=sse`. Requests and SSE
responses are flat. Subscription routing sends `aicode.experience` instead of the
mutually exclusive `model` field. No Cloud Code project/request wrapper or
optional CLI session/entitlement fields are sent. Output allowance is 8192 tokens.
This is an experimental opt-in integration; Google's Antigravity FAQ declares
third-party consumer integration unsupported. It is not official product support.
The default model is `gemini-3.8-flash-tiered` (Gemini 3.8 Flash). CLI inference is
removed; optional initial interactive sign-in stays outside Butler context.
Control-plane `providerauth.Manager` imports a private operator stream into the
encrypted PostgreSQL SecretStore and owns refresh. AICode requires no project;
legacy imported project values are retained only for backward compatibility and
never select a route. Transport
receives only typed short-lived auth, never refresh tokens, credential files or CLI
execution. No automatic API billing, credit overage or alternate endpoint exists.

Explicit `gemini-3.8-flash-high` instead uses daily Cloud Code
`/v1internal:streamGenerateContent?alt=sse` with model/project/request,
requestType=agent, product userAgent, per-turn UUID requestId and a Butler-owned
run sessionId. Manager discovers project via fixed authenticated loadCodeAssist
and confirms paidTier g1-pro/ultra; private project/access metadata never enters
model context or durable events. Wrapped `response` SSE is normalized with the
same cancellation, complete-terminal/function/schema/signature and usage guards.

In the implemented direct Go Core path, optional `TransportEvent.Usage` is one
complete inference-request observation, independent of text final versus tool
proposal/batch/complete terminal error. The existing `AssistantFinal.Usage` stays
compatible; normal final constructors mirror it into the event, and legacy
final-only adapters still work. Antigravity emits complete terminal SSE counts
on its tool proposal as well as its final text. A complete invalid function
proposal may retain measured usage on its safe error while granting no execution.
After a clean EOF and all terminal candidate/trailer checks, a `MAX_TOKENS`
function rejection likewise retains only complete validated usage and optional
observed version on its one transport error. Rejected parts, text and signatures
never accompany that error. A body read failure, incomplete frame, conflicting or
aliased metadata still returns no measurements; EOF is not skipped to retain
provisional values. This forward-only rule cannot recover historical unknown cost.
This logical field does not introduce a new RPC or claim an implemented protobuf
mapping where only the historical service declaration exists.

Usage is absent for missing/partial metadata, incomplete SSE and cancellation
before a complete observation; explicit reported zero remains distinct. Native
count keys are exact, counts must be nonnegative finite integers, sums must not
overflow and total cannot be below input/output subtotal. Other provider token
dimensions can explain a larger total; Butler does not invent them. Provisional
nonterminal snapshots cannot become complete-request measurements. Neither
provider usage nor a saved terminal error authorizes effects or another turn.
High keeps8192 output/1048576 conservative input reservation, thinkingBudget=-1
and checks the entire serialized envelope against the private byte limit.
Only the Orchestrator's Butler prompt/context/catalog is forwarded unchanged:
no Desktop/third-party wrapper instructions, identity rewriting, hidden reminders
or extra tools. No route/model retry or fallback exists. See
[the bounded High acceptance](../ai/antigravity-high-acceptance-20261012.md).

Only supplied Butler functions are advertised. Complete native function proposals
must match the exact per-run alias and schema before becoming `tool_call_requested`.
No automatic provider tool execution is supported. Butler executes through Broker
and submits a correlated native functionResponse; transport replays original model
parts and opaque thought signatures without exposing thought text/signatures to
assistant output. Foreign/lost refs, duplicate results, ambiguous calls, unsupported
parts and incomplete streams fail closed. Provider context is transient; Butler
replays its durable context after restart instead of trusting provider references.

Normalized live results retain the Broker envelope `{status,result,error?}`.
For `task.delegate`, the admitted child's ID is at
`functionResponse.response.result.task_id`; `status:"completed"` acknowledges
local admission only. The validated receipt body, including an empty `input_id`
for a waiting child, is preserved in durable tool history. Recovery reconstructs
the status/result/error envelope and presents it as private ToolOutput DATA;
its additional DATA wrapper places that same ID at `data.result.result.task_id`.
Recovery does not reconstruct native function calls or thought signatures.
Tool-phase text/signatures remain native live-context parts; only the last
model turn's terminal answer supplies the stored final, without concatenating
earlier streamed commentary. See
[receipt path verification](../ai/delegation-receipt-path-acceptance.md).

Capabilities: text streaming, one function proposal per turn, cancellation, usage;
no batch calls or provider stateful restart. All HTTP redirects are rejected and
errors are bounded/masked. Subscription model availability is distinct from stored
credentials. A real text/proposal/continuation smoke is required before activation;
operator AICode unary and SSE text probes passed through the configured lab proxy.
These probes do not establish that Go function/result continuation passed; keep
activation gated on that separate smoke. Earlier Cloud Code 429 responses do not
prove subscription quota exhaustion.
Production AICode 429 with bounded, recognized `HIGH_DEMAND` uses the fixed
`model_capacity_unavailable` subtype for temporary model capacity. Other 429s
remain `rate_limited`; neither establishes exhausted subscription quota. No raw
error metadata or automatic retry crosses this boundary. Historical general
`rate_limited` records cannot be reclassified without their original evidence.
See [protocol, operator import and validation status](../../deploy/antigravity/README.md).

An isolated diagnostic command may execute the installed CLI only against a
synthetic loopback HTTPS server, with fake credentials and network denied. It is
outside the provider factory and run lifecycle, reads no real credential store,
and exports only safe wire metadata. The completed CLI 1.3.3 capture proves that
explicit AICode mode serializes HTTP v1 with sessionId; the protobuf namespace
does not determine HTTP version. Fixture routing/model differences do not prove
production server acceptance or justify copying optional fields into the native
adapter. See [the diagnosis](../ai/antigravity-quota-diagnosis.md). No runtime
contract, credential ownership or inference permission changes with this tool.

A separate private operator comparison may use the retained authorization only
after explicit owner inference authorization and tested finite PostgreSQL admission.
It is outside provider factory, Orchestrator and Tool Broker execution. Manager
alone owns audit/refresh/encrypted persistence. CLI receives the same short-lived
token in an isolated profile behind an internal-network, fixed-host HTTPS gate;
main credential/proxy environment/control key are not mounted into CLI. Only
fixed safe metadata leaves the backend. Model/prompt/credit/byte/deadline checks,
charge-before-write, duplicate/restart refusal and sealed carried-forward reports
apply to every dispatch. Function proposals are blocked before CLI execution.
This does not create CLI inference fallback or grant other production authority.

The authorized2026-10-11 comparison completed CLI Flash-medium/CloudCode and Go
Flash-tiered/AICode with512; a separate byte-preserved Go8192 request returned
HIGH_DEMAND. Capacity is proven for that new rejection, not a causal protocol or
output-allocation defect. Keep production8192 and the existing fixed diagnostic;
neither the live text test nor fixture success closes delegation/tool acceptance.
See [the exact diagnosis and accounting](../ai/antigravity-quota-diagnosis.md).

A separate Desktop diagnostic may use the retained Manager-owned authorization
for explicitly allowlisted read-only metadata while denying **every** generation
before upstream. It adds no inference admission, account fallback or production
authority. Raw profiles/headers/responses stay inside the private backend;
only fixed safe request projections are exported. The2026-10-12 installed
Windows2.22 frontend/official same-CL Linux backend capture measured Flash-high,
daily Cloud Code v1internal,65536 output and15 declarations, including additional
LOW writes blocked before Google. The actual Go network-none fixture retained
flat AICode/tiered/8192 and parsed SSE. This is not the original Windows backend
capture or a causal defect; that capture itself changed no production behavior.
The subsequent explicitly requested High profile and admitted22 are described
above. Any additional live pair needs independent finite durable admission
carrying all22 tracked attempts and eight generation reports forward; issue16's
delegation gates remain separate. See
[Desktop capture evidence](../../deploy/antigravity/desktop-interception-20261012.json).

---

## 8. Core transport entities

### Pinned local observation adapter

`internal/transport/ollama` is an explicitly composed experimental text-only
ModelProvider. It uses HTTP/NDJSON against a reserved private Ollama service or
literal loopback, with no environment proxy or redirects. Its exact Qwen3:0.6b
manifest, finite4096/256 context/output profile and immutable isolated runner are
described in its README. There is no production selector or automatic fallback.
Each Start carries fresh Butler-owned input; continuations, tools, foreign refs
and model-supplied options refuse. Instruction/data provenance and secret denial
apply before serialization. The complete body and conservative byte/template
token bound are checked before HTTP, avoiding silent evidence truncation.
The sole installed model digest is checked before each inference. Streams have
bounded body/line/text/time and refuse incomplete finals, duplicated envelope
keys, unexpected thinking/tools and token-limit overruns. Actual usage requires
both explicit counts; missing/null counts remain unknown. Multiple contexts use
one loaded service runner, with serialized admission and a finite queue. This
does not grant mandate privacy authority or verify any delegated task result.

### 8.1 TransportRunContext

Контекст transport-взаимодействия для конкретного run.

Минимальные поля:

* `run_id`
* `session_key`
* `provider_name`
* `model_name`
* `provider_session_ref` при наличии
* `supports_streaming`
* `supports_tool_calls`
* `supports_stateful_resume`

### 8.2 ProviderSessionRef

Ссылка на provider-side session.

Минимальные поля:

* `provider_name`
* `session_ref`
* `response_ref` или аналогичный chain reference при наличии
* `created_at`
* `last_used_at`

### 8.3 TransportCommand

Команда от Orchestrator к Transport.

Минимальные поля:

* `command_id`
* `run_id`
* `command_type`
* `provider_name`
* `payload`
* `created_at`

### 8.4 TransportEvent

Событие от Transport к Orchestrator.

Минимальные поля:

* `event_id`
* `run_id`
* `event_type`
* `provider_name`
* `payload`
* `timestamp`

---

## 9. Команды transport слоя

### 9.1 start_run

#### Назначение

Запускает первый model turn для run.

#### Вход

* provider selection
* model selection
* prepared input items
* available tool contracts
* transport options
* existing provider_session_ref, если run продолжает существующую provider-side session

`prepared input items` may already include the fully assembled Butler system instruction. That instruction is authored and ordered by Orchestrator; Transport carries it through provider normalization only and does not assemble prompt sections itself.

#### Результат

* start acknowledgement;
* zero or more streaming events;
* provider_session_ref creation/update;
* финальный output event или tool request event.

---

### 9.2 continue_run

#### Назначение

Продолжает model interaction после промежуточных событий внутри того же run.

Используется, когда:

* нужно продолжить turn на provider-side session;
* нужно подать дополнительные input items;
* нужно продолжить stateful conversation.

---

### 9.3 submit_tool_result

#### Назначение

Подаёт результат tool execution обратно в transport/model loop.

#### Вход

* tool call correlation reference;
* normalized tool result;
* run_id;
* provider_session_ref.

#### Результат

* transport возобновляет model loop;
* генерирует streaming / final / next tool request events.

---

### 9.4 stream_events

#### Назначение

Поток transport events от provider к Orchestrator.

#### Принцип

Streaming delivery может быть:

* реальным stream transportом;
* внутренней event queue abstraction;
* адаптером над request/response provider.

Orchestrator должен видеть единый event stream contract.

---

### 9.5 cancel_run

#### Назначение

Прерывает активное transport execution для данного run.

#### Результат

* provider request/session получает cancel signal;
* transport прекращает выпуск новых model events;
* terminal cancel/timeout event возвращается Orchestrator.

---

## 10. Формат transport command payloads

### 10.1 StartRunRequest

Минимальные поля:

* `run_id`
* `session_key`
* `provider_name`
* `model_name`
* `input_items`
* `tool_definitions`
* `provider_session_ref` при наличии
* `streaming_enabled`
* `transport_options`

### 10.2 ContinueRunRequest

Минимальные поля:

* `run_id`
* `provider_session_ref`
* `input_items`
* `transport_options`

### 10.3 SubmitToolResultRequest

Минимальные поля:

* `run_id`
* `provider_session_ref`
* `tool_call_ref`
* `tool_result`
* `transport_options`

### 10.4 CancelRunRequest

Минимальные поля:

* `run_id`
* `provider_session_ref`
* `reason`

---

## 11. Формат transport events

Transport обязан выпускать нормализованные события.

### 11.1 Event types

Минимальный набор event types:

* `run_started`
* `provider_session_bound`
* `assistant_delta`
* `assistant_final`
* `tool_call_requested`
* `tool_call_batch_requested`
* `transport_warning`
* `transport_error`
* `run_cancelled`
* `run_timed_out`
* `run_completed`

---

### 11.2 run_started

Означает, что provider-side execution успешно началось.

Payload:

* `provider_session_ref` при наличии
* `capabilities_snapshot`

### 11.3 provider_session_bound

Означает, что transport создал или обновил provider-side session binding.

Payload:

* `provider_session_ref`

### 11.4 assistant_delta

Incremental text/token/event output от модели.

Payload:

* `delta_type`
* `content`
* `sequence_no`

### 11.5 assistant_final

Финальный model output без новых tool requests.

Payload:

* `content`
* `finish_reason`
* `usage` при наличии

### 11.6 tool_call_requested

Запрос одного tool call.

Payload:

* `tool_call_ref`
* `tool_name`
* `args`
* `sequence_no`

### 11.7 tool_call_batch_requested

Запрос нескольких tool calls одним provider event.

Payload:

* `tool_calls[]`
* `batch_ref`
* `sequence_no`

#### V1 правило

Transport может представить complete batch event. Текущий Orchestrator объявляет
`SupportsBatchToolCalls=false` и отклоняет весь batch как
`batch_tool_calls_unsupported` до первого Tool Broker dispatch. Это разрешённое
unsupported поведение Run Lifecycle Spec и действующая execution-budget policy.
Модель должна выбирать один tool call и ждать его коррелированный результат.
Представление batch в transport не предоставляет capability его исполнения;
acceptance wrapper не сериализует его и не исполняет частично. Native Antigravity
также отклоняет неподдерживаемый multi-function response как capability mismatch.

### 11.8 transport_warning

Нефатальное предупреждение transport слоя.

Примеры:

* provider degraded streaming;
* provider ignored stateful continuation hint;
* partial capability mismatch.

### 11.9 transport_error

Фатальная transport error.

Payload:

* `error_type`
* `message`
* `retryable`
* `provider_details`

### 11.10 run_cancelled / run_timed_out / run_completed

Terminal events transport слоя.

---

## 12. Streaming semantics

### 12.1 Общий принцип

Streaming является transport capability, но жизненный цикл run остаётся под управлением Orchestrator.

### 12.2 Что считается stream event

* `assistant_delta`
* provider-side progress/warning events
* tool call request events

### 12.3 Что делает Orchestrator

Orchestrator:

* принимает stream events;
* передаёт user-visible deltas в Channel Adapter;
* решает, какие события писать в transcript;
* останавливает stream при cancel/timeout.

### 12.4 V1 правило

Отдельного run state для streaming не вводится.
Streaming delivery происходит внутри `model_running`.

Если preferred WebSocket backend недоступен до начала meaningful provider output, transport может переключиться на fallback backend и обязан выпустить `transport_warning`, чтобы Orchestrator сохранил observability о degraded path.

---

## 13. Tool call semantics

### 13.1 Provider-side tool request

Если provider возвращает tool call request:

* transport не исполняет tool сам;
* transport формирует `tool_call_requested` или `tool_call_batch_requested` event;
* дальше control возвращается Orchestrator.

### 13.2 Provider-side tool correlation

Каждый tool call request должен иметь transport correlation reference:

* `tool_call_ref`
* optional provider-native id

Этот reference используется в `submit_tool_result()`.

### 13.3 Batch tool calls

Transport должен уметь нормализовать multiple tool calls в одном provider ответе, но не обязан сам решать, выполнять их параллельно или последовательно.

Это решение принимает Orchestrator.

---

## 14. Provider-side session semantics

В [Persistent Assistant Spec](persistent-assistant-spec.md) модельный управляющий
и ограниченный исполнитель используют один logical transport contract.
Task identity, completion verdict, approvals, constraints и budget принадлежат
Butler. WebSocket-first — предпочтение при наличии capability; оно не требует
конкретной модели и не меняет продуктовую семантику. Выбор provider учитывает
качество, инструменты, confidentiality и общий бюджет.

При повторном использовании provider session нельзя повторно добавлять уже
принятые сообщения без проверяемой replay boundary. При потере такой границы
нужна новая модельная сессия с контекстом из Butler. Проверки включают stateless
локальный и cloud adapter, смену provider и свежий owner constraint после summary.

### Reviewed capability catalog

Before inference, the Orchestrator pins the owned current task attempt and asks
Broker for its current scoped catalog. Extension schemas omit package descriptions;
fixed core descriptions identify their methods/effects. Source/version/manifest
documentation is separate provenance-tagged DATA, never provider instructions.
Fresh recovery reloads the catalog rather than restoring stale definitions. Known
tool observations stay context data and do not cause external dispatch. A future
semantic selector may only rank the already authorized catalog and needs its own
admitted inference budget; it cannot add tool IDs or promote package text to policy.

### 14.1 Принцип

Provider-side session является внешней оптимизацией transport слоя.

Она может использоваться для:

* stateful continuation;
* reduced context resend;
* tool-heavy flows;
* realtime sessions.

### 14.2 Ограничение

Provider-side session не является canonical session state Butler.

### 14.3 Binding rules

Transport обязан:

* сообщать Orchestrator о создании provider_session_ref;
* сообщать об обновлении response/session references;
* корректно работать, если provider-side session недоступна или потеряна.

### 14.4 Local providers

Для локальных моделей `provider_session_ref` может отсутствовать.
В этом случае transport работает в stateless mode, но logical contract остаётся тем же.

#### Current baseline

Текущий orchestrator начинает каждый новый run с новой provider session и
собственным контекстом Butler. Это исключает повторную вставку истории в
provider chain, для которого пока нет durable replay boundary. После первого
ответа ссылки этого run сохраняются и используются для его tool-result/resume
циклов. Возврат cross-run reuse допускается только с проверяемой границей
отправленных сообщений и тестом потери provider state. Цена текущего решения —
повторная передача контекста и возможная потеря provider-side caching.

---

## 15. Cloud vs local behavior

### 15.1 Cloud providers

Обычно поддерживают:

* WebSocket-first transport или streaming HTTP;
* provider-side state;
* tool calling;
* usage metadata.

### 15.2 Local providers

Могут поддерживать:

* обычный HTTP request/response;
* ограниченный streaming;
* отсутствие provider session state;
* отсутствие native tool calling.

### 15.3 Нормативное требование

Если локальный provider не поддерживает native tool calls, transport должен либо:

* нормализовать provider output в assistant_final without tool calls;
* либо объявить capability mismatch, чтобы Orchestrator не выбирал этот provider для tool-heavy runs.

---

## 16. Error model

The [bounded model retry policy](model-retry-policy.md) belongs to Core's durable
execution path. It makes a fresh provider session and admission after one trusted
nominal model-turn timeout. Transport performs one actual request per command;
it does not hide retries, reuse a paid attempt ID or fill missing usage.

Transport обязан нормализовать ошибки в следующие классы:

* `provider_unavailable`
* `transport_connection_error`
* `provider_timeout`
* `provider_protocol_error`
* `capability_mismatch`
* `invalid_tool_request`
* `stateful_session_lost`
* `rate_limited`
* `model_capacity_unavailable`
* `internal_transport_error`

### Для каждой ошибки должны быть поля:

* `error_type`
* `message`
* `retryable`
* `provider_name`
* `provider_code` при наличии

---

## 17. Capability model

Transport должен уметь сообщать capability snapshot для выбранного provider/model.

Минимальные capabilities:

* `supports_streaming`
* `supports_tool_calls`
* `supports_batch_tool_calls`
* `supports_stateful_sessions`
* `supports_cancel`
* `supports_usage_metadata`

Orchestrator обязан учитывать этот snapshot при выборе execution plan.

---

## 18. Idempotency и correlation

### 18.1 Correlation rules

Все transport команды и события должны коррелироваться минимум по:

* `run_id`
* `provider_session_ref` при наличии
* `tool_call_ref` для tool-related events

### 18.2 Idempotency

Transport не должен самостоятельно создавать дубликаты terminal events.
При повторных delivery случаях события должны быть либо дедуплицируемы, либо помечены как повторные.

---

## 19. Cancellation semantics

### 19.1 Что происходит при cancel

1. Orchestrator отправляет `cancel_run()`.
2. Transport пытается передать cancel в provider.
3. Transport перестаёт выпускать новые assistant deltas после подтверждённой отмены.
4. Orchestrator переводит run в terminal state согласно Run Lifecycle Spec.

### 19.2 Если provider не поддерживает cancel

Transport обязан вернуть warning/error semantics, а Orchestrator должен считать run best-effort cancelled.

---

## 20. Наблюдаемость

Transport слой должен логировать и метрифицировать минимум:

* число start_run / continue_run / submit_tool_result / cancel_run вызовов;
* provider latency;
* stream duration;
* assistant delta count;
* tool call request count;
* provider_session_ref bindings;
* transport errors by type;
* capability mismatches.

---

## 21. Минимальный V1 scope

Model Transport Contract V1 обязательно включает:

* единые команды start/continue/submit-tool-result/cancel;
* единый event stream contract;
* provider_session_ref model;
* streaming semantics;
* tool call normalization;
* cloud/local provider compatibility model;
* error normalization;
* capability snapshot.

---

## 22. Открытые решения

1. Формат `input_items` для unified provider abstraction.
2. Нужен ли dedicated transport supervisor для long-lived provider sessions.
3. Стратегия восстановления после `stateful_session_lost`.
4. Поддержка provider-native multimodal events в V1 или позже.

---

## 23. Итоговый тезис

Model Transport Layer в Butler — это provider-normalizing execution channel между Orchestrator и внешней или локальной моделью.

Он:

* не владеет памятью;
* не владеет run semantics;
* не исполняет tools;
* не заменяет Butler session;
* но обязан давать Orchestrator единый, предсказуемый и provider-agnostic контракт для model turns, streaming, tool requests, provider-side sessions и ошибок.

Главный принцип transport слоя Butler:

**WebSocket-first там, где это возможно; единый logical contract всегда; внутреннее состояние Butler всегда важнее внешнего session state провайдера.**

## Security and admitted serialization

Trusted Antigravity construction may freeze an explicit Cloud Code High generated
token ceiling up to32768 and an object JSON response schema for tool-free
observations. Defaults retain8192 and ordinary tool semantics. `InferenceLimits`
quotes the actual complete ceiling, including thinking; private serialization
limits include the schema. Model request options cannot change either setting.
Schemas never establish semantic truth or execution authority. An unsupported
upstream request remains failed without retry, MIME fallback or effort downgrade.

The additive, explicitly trusted `Config.GoogleResponseSchema` selects Google's
native OpenAPI-subset `responseSchema` wire field for tool-free High observations.
It is mutually exclusive with `JSONResponseSchema`; existing defaults and legacy
JSON Schema serialization remain unchanged. Constructor validation requires a
duplicate-free uppercase OBJECT root, only OBJECT/ARRAY/STRING/BOOLEAN/INTEGER/
NUMBER types and properties/required/items/nullable/string enums/bounded array
counts. Depth8/nodes256/properties32/enums64/16KiB bounds, unique valid required
names and canonical decimal-string `maxItems`1–128 deny unsupported structure
before auth. References, annotations, defaults, unions and unknown fields deny.
No `additionalProperties` field exists in this subset; caller-owned strict field,
source and semantic validation remains mandatory, with original output preserved.
Model transport options cannot change dialect/model/effort/resources or add tools.
The [public Google contract](https://cloud.google.com/vertex-ai/generative-ai/docs/reference/rest/v1beta1/GenerationConfig)
documents this alternative; the [Gemini reference](https://ai.google.dev/api/generate-content#GenerationConfig)
marks public `responseSchema` deprecated. A separately reviewed fixed four-entry
High diagnostic established private Cloud Code compatibility for two exact native
extraction originals and correlated all four original replies/costs with owning
PostgreSQL. Both legacy originals also passed; this does not establish causal
quality superiority, general model correctness or a cause of earlier failures.
See [actual compatibility evidence](../ai/scenario-schema-dialect-actual-audit-20261013.md).
No fallback/repair/retry is added.

Only typed code-owned instructions carry system role; external/context/derived observations carry provenance as data. Secret-class inputs fail before serialization. Budget037 passes a private input byte cap and the native provider checks its complete actual request, including prior function calls, thought signatures and tool results, before HTTP. Other adapters have serialization guards but need verified capacity/cost declarations before budgeted production inference. HTTP error status is retained without untrusted body/code/details; no raw network error or response content enters durable error events. Production auxiliary model/embed adapters have no durable job-owned admission and remain disabled. See [agent security boundaries](agent-security-boundaries.md).

Trusted callers may install a request-local `WithProviderStageObserver` for
content-free diagnosis. Antigravity reports only authentication, request
serialization, HTTP round trip and response-stream phases. The observer receives
no token, project, endpoint, response, model body or raw error; unknown stage
strings are ignored. It changes no event, timeout, retry, ownership or execution
decision. An HTTP phase is not proof of upstream inference. Older observations
without this telemetry remain unknown and cannot be backfilled.
