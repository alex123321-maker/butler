# Butler — Run Lifecycle Specification

## 1. Статус документа

**Тип документа:** Architecture Subspec / Run Lifecycle  
**Версия:** 0.3  
**Статус:** Baseline  
**Связанные документы:** Butler PRD + Architecture, Memory Model, Tooling and Execution Specification, Technology Stack and Engineering Standards

---

## 2. Назначение

Этот документ формально описывает жизненный цикл `run` в системе Butler.

`Run` — это единица активного исполнения агентной логики в ответ на входящее событие или внутренний триггер. Run связывает между собой:
- входящее сообщение или системное событие;
- Session Service;
- Orchestrator;
- Model Transport Layer;
- Tool Broker и tool runtimes;
- Transcript Store;
- Memory pipeline;
- Channel Adapter.

Цель документа — зафиксировать:
- формальные границы ответственности;
- состояния и переходы run;
- синхронные и асинхронные шаги;
- поведение при tool calls, approval pause, streaming delivery, отмене, ошибках и таймаутах.

---

## 3. Основные определения

### 3.1 Run

`Run` — это одно исполнение агентного цикла внутри конкретной session context (контекста сессии).

Run:
- имеет уникальный `run_id`;
- принадлежит одной session;
- имеет ограниченный жизненный цикл;
- может включать один или несколько model turns (ходов модели);
- может включать несколько tool calls;
- завершается статусом `completed`, `failed`, `cancelled` или `timed_out`.

### 3.2 Session

`Session` — это долгоживущий контейнер контекста, в рамках которого запускаются runs.

Session:
- живёт дольше, чем один run;
- хранит identity, ordering state и ownership state;
- связывает transcript history и memory scope.

### 3.3 Input Event

`Input Event` — событие, инициирующее run.

Примеры:
- пользовательское сообщение из Telegram;
- сообщение из Web UI;
- системный doctor-trigger;
- внутренний retry / resume event.

### 3.4 Provider-side session

`Provider-side session` — внешняя транспортная или модельная сущность на стороне model provider.

Примеры:
- stateful WebSocket session;
- response chain на стороне провайдера;
- realtime conversation session.

Принцип:
- `run_id` — внутренняя execution unit Butler;
- provider-side session — внешняя транспортная сущность;
- один provider-side session может переживать несколько runs;
- один run обычно соответствует одному turn или последовательности turn-resume шагов внутри provider-side session.

---

## 4. Цели lifecycle модели

Система жизненного цикла run должна обеспечивать:

1. **Single active owner per session** — только один активный owner run на session.
2. **Predictable ordering** — понятный порядок обработки событий внутри session.
3. **Clear separation of responsibilities** — жёсткое разделение Session Service и Orchestrator.
4. **Tool-aware execution** — поддержка tool-heavy workflows.
5. **Approval-aware execution** — поддержка обязательного подтверждения пользователя.
6. **Interruptibility** — поддержка cancel / timeout / failure paths.
7. **Durable history** — надёжное сохранение transcript.
8. **Async memory enrichment** — вынесение memory pipeline из критического пользовательского пути.

---

## 5. Границы ответственности

## 5.1 Session Service

Session Service отвечает за:
- session identity;
- session lookup / creation;
- ordering guarantees;
- lock / lease ownership;
- active run state;
- deduplication входящих событий на уровне session;
- статус активного run.

Session Service **не отвечает** за:
- planning;
- context assembly;
- tool decision logic;
- model interaction;
- memory extraction.

## 5.2 Orchestrator

Orchestrator отвечает за:
- создание execution plan для run;
- загрузку session summary и memory bundle;
- выбор model provider, конкретной модели и tools;
- вызов Model Transport Layer;
- обработку tool calls;
- запуск approval flow;
- взаимодействие с Channel Adapter для выдачи partial и final response;
- завершение run;
- постановку post-run задач в memory pipeline.

Выбранные `model_provider` и `model_name` должны быть определены до первого transport вызова и сохранены в metadata / run record как часть Butler run truth.

Orchestrator **не отвечает** за:
- выдачу session ownership;
- глобальный ordering state;
- хранение секретов;
- прямое исполнение инструментов.

## 5.3 Model Transport Layer

Model Transport Layer отвечает за:
- старт model turn;
- продолжение turn;
- приём streaming events;
- передачу tool results обратно в provider loop;
- cancellation на стороне модели.

Полный контракт между Orchestrator и Model Transport Layer описан в `docs/architecture/model-transport-contract.md`.

## 5.4 Tool Broker

Tool Broker отвечает за:
- валидацию tool call;
- policy enforcement;
- credential mediation;
- routing в runtime containers;
- result normalization.

## 5.5 Memory Pipeline Worker

Memory Pipeline Worker отвечает за:
- извлечение memory candidates;
- классификацию;
- запись в memory stores;
- обновление retrieval index;
- обновление session summary по policy.

## 5.6 Channel Adapter

Channel Adapter отвечает за:
- доставку пользователю partial response events;
- доставку final response;
- доставку approval request;
- приём user approval / reject action;
- нормализацию channel-specific UX в единые input/output events.

---

## 6. Input sources

Run может быть запущен следующими типами событий:

1. `user_message`
2. `ui_action`
3. `system_diagnostic_trigger`
4. `scheduled_internal_event`
5. `resume_or_retry_event`
6. `approval_response_event`

Каждый input event должен иметь:
- `event_id`
- `event_type`
- `session_key`
- `source`
- `payload`
- `created_at`
- `idempotency_key`

---

## 7. Состояния run

Run должен иметь явную state machine.

### 7.1 Список состояний

1. `created`
2. `queued`
3. `acquired`
4. `preparing`
5. `model_running`
6. `tool_pending`
7. `awaiting_approval`
8. `tool_running`
9. `awaiting_model_resume`
10. `finalizing`
11. `completed`
12. `failed`
13. `cancelled`
14. `timed_out`

### 7.2 Значение состояний

#### `created`
Run создан как сущность, но ещё не поставлен в очередь исполнения.

#### `queued`
Run ожидает получения session ownership.

Если в текущем synchronous execution path ownership не удаётся получить и run не будет автоматически переисполнен отдельным worker, orchestrator должен завершить такой run в `failed`, а не оставлять его навсегда в `queued`.

#### `acquired`
Session lock / lease получен, run стал активным owner для session.

#### `preparing`
Orchestrator собирает execution context: transcript state, tool availability и запрашивает у memory service session summary + memory bundle.

#### `model_running`
Идёт активное взаимодействие с Model Transport Layer. Streaming delivery пользователю, если она поддерживается каналом, также происходит внутри этого состояния.

#### `tool_pending`
Модель запросила tool call, но он ещё не отправлен в Tool Broker или ещё не принято решение о необходимости approval.

#### `awaiting_approval`
Tool call требует пользовательского подтверждения, approval request уже отправлен, исполнение инструмента ещё не началось.

#### `tool_running`
Инструмент исполняется во внешнем runtime.

#### `awaiting_model_resume`
Tool result уже получен, orchestrator готовит продолжение модельного цикла.

#### `finalizing`
Run завершает transcript persistence, final response delivery guarantee и постановку post-run задач.

Если на этом этапе возникает техническая ошибка persistence или delivery guarantee, run должен переходить в `failed`, а не зависать в `finalizing`.

#### `completed`
Run завершён успешно.

#### `failed`
Run завершён с ошибкой, не допускающей успешного завершения.

#### `cancelled`
Run был явно отменён пользователем или системой, либо пользователь отклонил approval request.

#### `timed_out`
Run остановлен из-за превышения допустимого времени, включая approval timeout.

---

## 8. Допустимые переходы состояний

Базовые переходы:

- `created -> queued`
- `queued -> acquired`
- `acquired -> preparing`
- `preparing -> model_running`
- `model_running -> tool_pending`
- `tool_pending -> awaiting_approval` если `approval_required == true`
- `tool_pending -> tool_running` если approval не нужен
- `awaiting_approval -> tool_running` при approve
- `awaiting_approval -> cancelled` при reject
- `awaiting_approval -> timed_out` при approval timeout
- `tool_running -> awaiting_model_resume`
- `awaiting_model_resume -> model_running`
- `model_running -> finalizing`
- `finalizing -> completed`

Переходы ошибок:
- `created -> failed` если run не удалось поставить в исполняемый путь после создания
- `queued -> failed` если ownership acquisition сорвался и automatic retry path отсутствует
- `acquired -> failed` если run не удалось перевести в preparation path после получения ownership
- `preparing -> failed`
- `model_running -> failed`
- `tool_running -> failed`
- `awaiting_approval -> failed` при технической ошибке approval flow
- `finalizing -> failed` при ошибке transcript persistence или final delivery guarantee
- `any active state -> cancelled`
- `any active state -> timed_out`

---

## 9. High-level flow

### 9.1 Синхронный путь пользовательского запроса

1. Input event поступает в ingress layer.
2. Session Service вычисляет `session_key`.
3. Session Service создаёт или находит session.
4. Session Service создаёт run и переводит его в `queued`.
5. Session Service выдаёт lock / lease для session и переводит run в `acquired`.
6. Orchestrator переводит run в `preparing`.
7. Orchestrator собирает context bundle.
8. Orchestrator переводит run в `model_running`.
9. Model Transport Layer запускает модельный цикл.
10. Если модель возвращает tool call, run проходит через `tool_pending`, а затем либо `awaiting_approval`, либо `tool_running`.
11. После финального ответа run переходит в `finalizing`.
12. Transcript и результат фиксируются.
13. Запускается post-run async processing.
14. Run переходит в `completed`.
15. Session lease освобождается.

---

## 10. Context preparation

На стадии `preparing` orchestrator обязан собрать:

1. `input event payload`
2. `session metadata`
3. `session summary`
4. `working memory snapshot`
5. `relevant episodic memories`
6. `relevant profile memory`
7. `available tool contracts`
8. `credential context`, если он передан пользователем
9. `autonomy_mode`

### 10.1 Что не делается на стадии preparing

- не выполняются tools;
- не обновляется память;
- не выполняется memory extraction;
- не выполняется финальная запись transcript.

Примечание для текущего baseline: на стадии `preparing` orchestrator обязан запросить у memory service bundle, который читает durable Working Memory snapshot из PostgreSQL (`memory_working`) и включает в memory bundle структурированные поля `goal`, `active_entities`, `pending_steps`, `working_status`, если snapshot существует. Это чтение не считается mutation memory pipeline.

---

## 11. Model interaction loop

### 11.1 Общий принцип

Model interaction может быть многошаговым внутри одного run.

Один run может включать цикл:
- `model_running`
- `tool_pending`
- `awaiting_approval` при необходимости
- `tool_running`
- `awaiting_model_resume`
- обратно в `model_running`

до тех пор, пока модель не сформирует финальный ответ или run не завершится ошибкой / отменой / таймаутом.

### 11.2 Обязательные свойства model loop

- каждое tool request событие должно иметь correlation с `run_id`;
- каждый provider event должен быть журналируемым на уровне transcript/event log;
- orchestrator должен иметь возможность остановить loop по timeout или cancel signal;
- streaming response delivery допускается только внутри `model_running`.

### 11.3 V1 policy: sequential tool execution only

В Butler V1 допускается только один активный tool call за раз.

Это означает:
- `tool_running` всегда означает ровно один активный tool execution;
- parallel tool execution не поддерживается;
- если provider возвращает несколько tool calls в одном ответе, orchestrator обязан сериализовать их в deterministic order или отклонить как unsupported provider behavior;
- partial failure semantics для параллельных инструментов в V1 отсутствует.

---

## 12. Tool call lifecycle внутри run

### 12.1 Инициация

Когда модель в состоянии `model_running` запрашивает tool call:
- run переводится в `tool_pending`;
- orchestrator валидирует, что tool вообще допустим в текущем контексте;
- определяется, нужен ли approval.

### 12.2 Approval pause

Если tool call требует approval:
- run переводится в `awaiting_approval`;
- orchestrator формирует approval request;
- Channel Adapter доставляет approval request пользователю;
- при approve run переходит в `tool_running`;
- при reject run переходит в `cancelled`;
- при timeout run переходит в `timed_out`.

### 12.3 Исполнение

Если approval не требуется или уже получен:
- Tool Broker валидирует schema;
- применяет policy;
- разрешает credential refs при необходимости;
- маршрутизирует вызов в runtime container;
- run переводится в `tool_running`.

### 12.4 Возврат результата

После успешного ответа tool runtime:
- результат прикрепляется к run;
- run переводится в `awaiting_model_resume`;
- orchestrator подаёт tool result обратно в Model Transport Layer;
- run возвращается в `model_running`.

### 12.5 Ошибка инструмента

Если tool execution завершается ошибкой:
- ошибка нормализуется Tool Broker;
- orchestrator решает, можно ли вернуть ошибку модели как tool result, либо завершить run как `failed`;
- политика retry определяется отдельным Tool Runtime Contract.

---

## 13. Channel delivery semantics

### 13.1 Partial response delivery

Если канал поддерживает streaming или progressive delivery:
- partial response events могут отправляться пользователю во время `model_running`.

### 13.2 Final response delivery

В `finalizing` orchestrator обязан гарантировать, что итоговый final response доставлен пользователю или зафиксирована ошибка доставки.

### 13.3 Approval delivery

Approval request отправляется пользователю только в состоянии `awaiting_approval`.

### 13.4 Channel abstraction

Channel Adapter не влияет на state machine напрямую, но получает lifecycle events от Orchestrator и преобразует их в channel-specific UX.

---

## 14. Transcript persistence

### 14.1 Что должно фиксироваться

Transcript Store должен получить:
- input event;
- assistant outputs;
- tool calls;
- tool results;
- policy denials;
- approval request / approval response events;
- final response;
- terminal run state.

### 14.2 Когда фиксировать

Есть два уровня записи:

1. **incremental event logging**  
   во время run фиксируются ключевые события жизненного цикла.

2. **final transcript persistence**  
   в `finalizing` записывается финальное представление run.

---

## 15. Working Memory and execution state

Рабочая память и transient execution state определяются в `Memory Model Specification`.

Для целей lifecycle действует правило:
- durable logical working state snapshots хранятся в PostgreSQL;
- transient execution state, locks и scratch values хранятся в Redis;
- эти слои не конкурируют и не дублируют друг друга.

Текущий baseline orchestrator дополнительно применяет явную lifecycle policy для durable Working Memory:
- при входе в `preparing` snapshot загружается и добавляется в memory bundle;
- после записи user transcript и перед model execution orchestrator сохраняет актуализированный snapshot с goal/status для текущего run;
- во время tool-heavy execution snapshot может обновляться на meaningful checkpoints (например, перед и после tool execution);
- при `completed` snapshot очищается по policy;
- при `failed` snapshot сохраняется с terminal working status и compact final note, чтобы следующий run мог безопасно продолжить задачу.

Для transient Working Memory текущий baseline использует отдельный Redis-backed store:
- transient state привязан к `session_key + run_id`, а не заменяет durable snapshot;
- orchestrator пишет transient checkpoints в `preparing`, `tool_running`, `awaiting_model_resume` и terminal paths;
- successful completion очищает transient state сразу;
- failed/abandoned paths оставляют transient state только на ограниченный TTL, после чего Redis очищает его автоматически.

---

## 16. Memory pipeline

### 16.1 Принцип

Memory Pipeline не должен быть частью критического пользовательского пути, кроме минимально необходимого session bookkeeping.

### 16.2 Синхронно после run

Синхронно в `finalizing` допускается:
- сохранить transcript;
- зафиксировать terminal run state;
- при необходимости сохранить compact session summary pointer или lightweight summary update marker.

### 16.3 Асинхронно после run

Асинхронный worker должен выполнять:
1. extract memory candidates;
2. classify candidates;
3. resolve conflicts;
4. write structured memory;
5. update vector retrieval index;
6. refresh session summary по policy.

### 16.4 Исполнитель

Memory Pipeline выполняется отдельным async worker process / service, а не synchronously inside orchestrator response path.

---

## 17. Session summary

### 17.1 Назначение

Session summary — компактное машинно-пригодное представление актуального контекста session.

### 17.2 Содержимое

Минимально session summary должен включать:
- текущую цель или активный контекст;
- недавние важные события;
- активные открытые задачи;
- критичные пользовательские или системные факты;
- ссылки на релевантную working / episodic memory.

### 17.3 Генерация

Session summary обновляется post-run через memory pipeline worker.  
При необходимости допустим lightweight synchronous update marker, но не полная генерация summary в критическом пути.

### 17.4 Operator visibility baseline

Текущий baseline дополнительно предусматривает read-only operator visibility для memory state вне критического run path:

- orchestrator exposes `GET /api/v1/memory` для scope-based просмотра durable memory records;
- Web UI `/memory` использует этот endpoint для просмотра Working / Profile / Episodic / Chunk memory и provenance-safe links;
- этот просмотр не изменяет run lifecycle и не является частью execution-time memory mutation.

---

## 18. Cancel и timeout semantics

## 18.1 Cancel

Run может быть отменён:
- пользователем;
- системой;
- администратором;
- при shutdown / maintenance;
- при reject approval request.

При отмене:
1. orchestrator устанавливает cancel flag;
2. transport layer получает cancel signal;
3. tool executions по возможности прерываются;
4. run переходит в `cancelled`;
5. transcript фиксирует частично завершённое состояние.

## 18.2 Timeout

Timeout может быть:
- global run timeout;
- model turn timeout;
- tool call timeout;
- approval timeout.

Если превышен timeout:
- run завершает активный шаг;
- переводится в `timed_out`;
- фиксируется причина;
- lease освобождается.

---

## 19. Error model

Run должен поддерживать нормализованные terminal error classes:
- `validation_error`
- `transport_error`
- `tool_error`
- `policy_denied`
- `credential_error`
- `approval_error`
- `timeout`
- `cancelled`
- `internal_error`

### 19.1 Ошибка не всегда равна failed

Некоторые ошибки инструмента могут быть возвращены модели как наблюдаемое tool result событие и не обязаны завершать run состоянием `failed`.

### 19.2 Failed

Состояние `failed` используется только если run не может продолжаться и не может успешно завершиться.

---

## 20. Idempotency и deduplication

### 20.1 Input deduplication

Session Service обязан проверять `idempotency_key` input event, чтобы избежать двойного запуска одного и того же run из-за повторной доставки сообщения.

### 20.2 Tool deduplication

Если retry layer повторно исполняет tool call, это должно быть явно отражено в tool call state и audit log.

---

## 21. Lease model

### 21.1 Назначение

Lease гарантирует, что в рамках одной session только один active orchestrator owner управляет run.

### 21.2 Свойства

Lease должен иметь:
- `lease_id`
- `session_key`
- `run_id`
- `owner_id`
- `expires_at`
- `renewal_policy`

### 21.3 Потеря lease

Если lease истёк или был потерян:
- run должен быть остановлен или переведён в recovery path;
- Session Service должен предотвратить конкурентное продолжение двумя owners.

---

## 22. Resume и retry

### 22.1 Resume

В Butler V1 resume реализуется как создание нового run, а не продолжение старого run с тем же `run_id`.

Resumed run:
- получает новый `run_id`;
- ссылается на предыдущий run через `resumes_run_id`;
- использует уже сохранённый session/transcript state;
- не меняет terminal state исходного run.

Resume допускается только если:
- есть достаточное состояние для безопасного продолжения;
- lease успешно восстановлен или переиздан;
- terminal run уже не может быть корректно продолжен как тот же execution unit.

### 22.2 Retry

Retry допускается не для всего run, а на уровне отдельных компонентов:
- input event retry;
- transport retry;
- tool retry;
- async memory pipeline retry.

Политики retry должны быть вынесены в отдельные subspec/contract documents.

---

## 23. Минимальные поля run record

Run record должен содержать минимум:
- `run_id`
- `session_key`
- `input_event_id`
- `status`
- `autonomy_mode`
- `current_state`
- `model_provider`
- `provider_session_ref` при наличии
- `lease_id`
- `resumes_run_id` при наличии
- `started_at`
- `updated_at`
- `finished_at`
- `error_type`
- `error_message`

---

## 24. Наблюдаемость

Для каждого run должны быть доступны:
- state transition log;
- duration per phase;
- tool call count;
- model turn count;
- final status;
- timeout / cancel markers;
- approval markers;
- post-run pipeline status.

Минимальные phase metrics:
- prepare duration;
- model duration;
- approval wait duration;
- tool duration;
- finalize duration;
- total run duration.

---

## 25. Sequence model

### 25.1 Нормальный успешный сценарий

1. Input event arrives.
2. Session Service resolves session and acquires lease.
3. Run enters `preparing`.
4. Orchestrator assembles context.
5. Run enters `model_running`.
6. Model requests browser tool.
7. Run enters `tool_pending`.
8. If approval required, run enters `awaiting_approval`.
9. Approval is received.
10. Run enters `tool_running`.
11. Tool Broker executes tool and returns result.
12. Run enters `awaiting_model_resume` then `model_running`.
13. Model produces final answer.
14. Partial streaming may be delivered during `model_running`.
15. Run enters `finalizing`.
16. Transcript is persisted and final response delivery is guaranteed.
17. Async memory pipeline is scheduled.
18. Run enters `completed`.
19. Lease is released.

---

## 26. Минимальный V1 scope

Run Lifecycle V1 обязательно включает:
- state machine;
- approval state;
- session lease model;
- orchestrator / session service separation;
- tool-aware model loop;
- sequential-only tool execution;
- transcript persistence;
- channel delivery semantics;
- async memory pipeline trigger;
- cancel / timeout semantics;
- idempotency handling.

---

## 27. Открытые решения

1. Формальный transport contract между Orchestrator и Model Transport Layer.
2. Формальный transport contract между Tool Broker и runtime containers.
3. Retry policy по browser/http/model слоям.
4. Точный формат session summary.
5. Recovery semantics при потере lease во время активного tool call.
6. Partial result policy для cancelled/timed_out runs.
7. Правила сериализации multiple tool calls, если provider вернул их одним batch response.

---

## 28. Итоговый тезис

Run в Butler — это строго управляемая execution unit (единица исполнения), которая:
- принадлежит одной session;
- имеет одного активного owner через lease model;
- проходит через явную state machine;
- поддерживает approval pause;
- поддерживает tool-resume циклы;
- допускает streaming delivery внутри `model_running`;
- завершает transcript synchronously;
- запускает memory enrichment asynchronously.

Главный принцип lifecycle Butler:

**Session Service отвечает за владение и порядок, Orchestrator — за ход исполнения и взаимодействие с transport/tools/channels, approval встроен в lifecycle как отдельная pause-фаза, а memory enrichment и tool execution вынесены за пределы основного пользовательского критического пути там, где это возможно.**
