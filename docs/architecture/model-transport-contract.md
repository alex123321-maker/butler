# Butler — Model Transport Contract

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

---

## 8. Core transport entities

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

Transport может отдать batch event, но Orchestrator в V1 обязан сериализовать его в sequential execution policy согласно Run Lifecycle Spec.

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

Текущий baseline orchestrator повторно использует последний валидный `provider_session_ref` того же `model_provider` для новых run внутри той же session на best-effort основе.

Это нужно, чтобы immediate follow-up turns могли продолжать provider-side state даже до того, как async memory pipeline успеет обновить session summary.

Если ссылка отсутствует, невалидна или provider-side session потеряна, Butler продолжает run без неё и полагается на собственный prompt/memory context.

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

Transport обязан нормализовать ошибки в следующие классы:

* `provider_unavailable`
* `transport_connection_error`
* `provider_timeout`
* `provider_protocol_error`
* `capability_mismatch`
* `invalid_tool_request`
* `stateful_session_lost`
* `rate_limited`
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
