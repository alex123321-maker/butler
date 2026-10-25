# Butler — Run Lifecycle Specification

The opt-in [issue97 research driver](../../cmd/effort-eval/README.md) owns bounded
synthetic solve/review phases outside production construction. It commits its
fixture working state before another model pass, versions owner amendments and
keeps candidates/hypotheses separate from primary observations. Review snapshots
are labelled harness data, not new owner commands. This does not change Input,
task/run/session leases, production completion, approval or delivery semantics.

Communication diagnostics observe durable metadata without acquiring an
execution lease, resuming an Input or reading model/content/secret state. The
operator's bounded open-question ceiling is enforced atomically across owner
sessions at publication; quota denial creates no partial publication or wait.
Exact receipt replay and existing no-expiry waits retain their identities and
spent budgets. Readiness and active/drain operation are documented in
[communication diagnostics](../operations/communication-diagnostics.md).

Credential vault requests use the existing durable interaction checkpoint with
kind `credential_interaction`, distinct from preference questions and action
approvals. Core joins task wait, Input suspension and coarse run wait under the
live session lease. Protected owner response atomically marks that same Input
ready; recovery reads canonical request status and safe field references only.
Early responses, expiry and revocation preserve the checkpoint. No secret value
is stored in prepared/model context, and permission is rechecked at Broker use.
Explicit failed/stopped Broker results do not suspend a credential interaction.
A budget stop retains its reason and the existing execution-stopped checkpoint.
Only a completed request or uncertain dispatch acknowledgement enters the private
request suspension path, including Broker's `effect_unknown` receipt after a
durable request commit. The exact run/call's stored request remains the proof;
an ambiguous receipt alone cannot create a wait or authorize replay.

Core startup reserves one connection from `BUTLER_POSTGRES_MAX_CONNS` for live
lease, Task authority and token-only source-currentness observations. The remaining business pool has at least
two connections: Core therefore requires a total maximum of at least three.
Converter-participating source proofs use business capacity: returned-package
access can bind selected source pins and repeat continuation/lifecycle guards.
Unrelated token-only proofs retain the reserved reader under business saturation.
A current PG classification selects capacity only and is repeated after access
locks; both routes execute all authority/source guards with unchanged deadlines.
The total maximum and execution budgets do not increase. Channel polls, Web
feeds and business writes cannot acquire the reserved connection. Both readers
still use current PostgreSQL state and their existing authority checks; an
unavailable observer fails closed. The foreground local-model admission watchdog
uses this same reserve, with its unchanged250ms interval and200ms proof deadline.
It rolls back source observations; admission, allowance reservation and returned
result acceptance remain business transactions with their existing commit/pin
semantics. Local-model cancellation retains only a fixed phase/class diagnostic,
and independent parent cancellation/deadlines remain authoritative.
Observer cancellation logs only fixed phase
and classification plus elapsed time, never raw storage/source errors.

Single-transaction owning writers (interaction recovery, preparation checkpoint,
run lease refresh, private communication publication and final persistence) lock the live fencing row inside the
business transaction and recheck database-clock expiry before commit. They use
one pooled connection, preserving capacity for the ownership watchdog even with
two connections. Lease mutations still wait outside the pool for guarded writers;
multi-repository writers retain their existing shared guard. The local guarded
context must never escape its transaction or authorize a separate write.

Interaction recovery reads canonical preference DATA with shared Session/run
proofs so a retained Web feed cannot hold it behind an unnecessary exclusive
lock. Session ownership, exact Input claim, task authority and source guards
remain mandatory; claim/lease expiry is rechecked after source waits. Recovery
retains the owning watchdog cancellation cause and fixed interaction proof
phase, not raw storage errors. Input heartbeat cancellation separately retains
claim loss/unavailable renewal without overwriting an independent provider error.
An authenticated owner may acknowledge a stopped taskless explicit Input as
failed through the communication control described in the user-communication
spec. It requires no live claim/lease or executing write. UNKNOWN denies by
default; an owner may explicitly acknowledge the exact retained unknown IDs of
local communication preparation only. This abandons the Input, never reconciles
or replays the effect; external/other uncertain writes still deny. The Input/run failure,
closed obligation and audit commit together. Historical errors, answers and
spent/unknown reservations remain intact. No retry or successful result is made.

Kernel-admitted mobile batch tasks use primary history from that task's runs,
while retaining Session ownership/order and transcript revision/privacy fences.
Current typed goals, constraints and mandates remain mandatory. See
[notification task context](mobile-memory-search.md); ordinary session replay
and verified summary coverage are unchanged.

Optional [tool discovery](tool-discovery.md) starts with two catalog schemas and reconstructs selected exact method refs from successful current-run tool receipts. Every continuation and recovery revalidates current scope. Loading cannot replay actions or replace approval/authority, and stale refs never silently upgrade.


Optional [agent schedules](agent-schedules.md) persist future instructions through
exact Broker control and admit one ordinary Task/Input per due occurrence.
They hold no run/lease during the delay. Schedule cancellation/revision and current
source/authority checks fence queued/running occurrences through ordinary task
authority, separately from task completion and final delivery. Reminders use the
existing channel worker with their own durable outbox and no fake model run.

The current kernel accepts one tool proposal per model response. Trusted
orchestration instructions declare that limit independently of the operator's
base prompt and ask the model to wait for its correlated tool result. The Broker
and transport still reject unsupported batches before any member dispatch; the
instruction does not grant permission, guarantee model compliance or retry a
failed/unknown effect. See the [transport contract](model-transport-contract.md).

Verified summary generation is optional independent background work, specified in
[task compaction](../../internal/memory/compaction/README.md). It does not keep or
revive a Session Service lease, complete a task, accept profile facts or infer tool
success. Current typed task/constraints/assignment versions gate publication.
Compaction starts before the foreground hard allowance, keeps the newest raw tail
and preserves every owner statement. Failed/stale generation leaves history and
task state intact; if mandatory context still cannot fit, preparation explains
the budget failure instead of silently removing constraints or unknown effects.

Optional [background model jobs](background-memory-jobs.md) have their own durable
owner policy and claim after user-run completion. Their scanner/transport failures
cannot delay or undo user finalization. Completed task identity/revision remain
intact; no historical lease grants new dispatch permission. Attempt spending,
source freshness, cancellation and atomic derivative publication are independent
of user-run success and are observable through the existing memory job controls.

Memory preparation separates local query permission from external data release.
It re-reads the task/assignment/privacy stamp and current source proof after
retrieval; amendment/revocation blocks stale publication. `memory-context-v3`
keeps mandatory task/history/working context, bounds optional evidence by size
and class quotas, and reports selection counts/reason codes without source text.
Complete-context admission trims optional items individually and reserves
`min(4096, configured allowance/8)` conservative units for answer/tool
continuation; mandatory overflow remains an explicit preparation failure.
An oversized hybrid query skips optional lookup with `query_input_limit`; the
complete owner message and ordinary authority/source checks remain in preparation.

The prospective V4 lab acceptance keeps separate completed-model, original-task
verification, action-effect and notice-delivery evidence. Its native chain cannot
open from an assumed baseline: exact independently reviewed full quality and
paired-cost PostgreSQL evidence must establish the actual520-call prerequisite.
Restart, tree Stop, explicit delivery absence and lost ACK retain owning product
rows in immutable content-free witnesses before fixture cleanup. See
[V4 chain protocol](../ai/scenario-release-chain-v4-high-plan-20261013.md).

Complete model-turn usage is independent of run/task completion, tool success
and notification delivery. The budget wrapper saves one optional complete
observation per admitted inference request before releasing its tool/final event;
identical duplicate events cannot charge another request. Missing usage stays
NULL. Current source/task guards still deny release/admission, and a later tool
failure or cancellation neither erases an observed request nor refunds allowance.
See [execution-budget contract](execution-budget-contract.md) for validation and
forward-only history rules.

Private Broker claim source guards derive a read-proof worker token from the
already locked exact current PostgreSQL lease/run/grant. They do not trust an RPC
caller token or owner Actor. The same-transaction derivative has no general write
authority and does not escape; accepted converter continuation input, current
task/source/055 identity and post-wait expiry still require independent checks.
See [action authorization](action-authorization-contract.md).

The optional guarded tab request is an ordinary exact Broker local control.
Its owning publication atomically suspends the actual tool-running input/run/task
with finite owner choices and an immutable dispatch mapping. Core detects that
durable waiting state before generic result/checkpoint/model continuation,
including a lost private ACK. Recovery needs fresh Session/input/source/task
authority, exact owner selection and confirmed request journal outcome. A choice
cannot reconcile an uncertain dispatch or authorize a browser effect. See
[guarded composition](single-tab-production-composition.md).

Tool preparation and dispatch follow
[exact action authorization](action-authorization-contract.md). The approved
intent retains stable IDs across durable suspension, while a resumed worker
must obtain a fresh current lease/fence grant. Dispatch/result checkpoints and
non-reusable unknown outcomes prevent recovery from repeating external effects.

Guarded tab choice has a distinct version1 `guarded_tab_selection` continuation,
not an exact-action approval. Choice publication, run/task waiting state and
current input suspension commit together. Owner select/revoke readies the same
input under its original finite request; the previous worker may finish after
that decision without losing the wakeup. Core recovery acquires a new Session
lease, validates the actual input claim, current Catalog/task/selection and
rebuilds current tool definitions. It starts a fresh model context with opaque
selection DATA and grants no browser execution authority. Later effects still
require Tool Broker. Missing/advanced checkpoints deny this selection recovery.

[Execution budgets](execution-budget-contract.md) admit each actual model turn,
effect claim and child creation against shared durable scopes. Exhaustion/loops
save a concrete `execution_stopped` checkpoint and prior continuation, block
input/task and cancel transport. A frozen finalizing checkpoint never invokes
another model/effect; a budget stop cannot become verified completion.

Failed provider attempts preserve a content-free transport subtype alongside
the existing general error class. Source-check expiry is
`memory_source_check_timeout`, separate from provider expiry, stale sources and
rate limiting. The same normalization applies before logs, persistent failure
state and returning to the input worker. Unknown provider types become
`internal_transport_error`; private provider error text is never a diagnostic.

An actual tool proposal invalidates any earlier candidate final and its answer
classification. Only a fresh `assistant_final` after the last tool continuation
can supply the frozen final body; an empty or nil continuation cannot fall back
to an answer from before that action. A fresh final's own finish reason determines
`final` versus `partial`. Confirmed tool observations and spent reservations stay
durable when a later model turn fails to provide a final. Streaming commentary
does not become final content or a task-completion verdict.

## Планируемый explicit communication profile

Backend foundation теперь содержит optional hooks для explicit final и blocking
interaction checkpoint. Они не подключены Bootstrap. Подробная граница и
проверки — [communication acceptance](../ai/communication-core-acceptance-20261021.md).
Coarse Run state переиспользует `awaiting_approval`, но Input checkpoint
`awaiting_interaction` и marker `communication_interaction` отличают preference
от Approval: approval/grant из ответа не создаётся. Pause сохраняет DATA;
resume владельца создаёт новый Input, старый отменённый Input не оживает.
Ошибка чтения durable question state не означает отсутствия вопроса:
`communication_state_pending` сохраняет checkpoint, запрещает generic
tool-result/model continuation и даёт только bounded повторную проверку.
Input Finish удерживает существующий wait либо ранний ready marker, не выдавая
дополнительного разрешения на исполнение и не расходуя ещё один model turn.
Private communication после approved action может сама вернуть durable widget
wait. Recovery распознаёт suspension до обработки ошибки transport/authorization,
не блокирует уже сохранённое ожидание и не продолжает старый provider context.
Optional trusted admission вызывается после Task/source preparation перед
каталогом; тот же Input восстанавливается только с тем же pinned profile.
Обычный reply поступает через проверенные binding/ACK refs в исходную canonical
Input очередь. Он не получает lease, не меняет task constraints и не создаёт grant.

[User Communication](user-communication-spec.md) задаёт будущий admission profile:
публикации выполняются через Broker-tool; model final сохраняется как внутренний
результат попытки без implicit send. Ожидание widget сохраняется без живого worker,
а недостающий ответ получает bounded continuation. Описанный ниже текущий
final-result/outbox путь остаётся baseline для legacy admissions до миграции.
Task completion, execution, reply obligation и delivery не объединяются.

Foundation C02 разделяет publication/attempt_final в transcript и memory readers
и вводит минимальный explicit message lifecycle; C03 доказывает его сквозным
сценарием до reports/widgets. Production enable ждёт полного профиля. Widget V1
только blocking: owning publication, Input checkpoint и Run/Task wait intent
коммитятся вместе до доставки. Ready marker раннего ответа не затирается поздним
Finish старого worker; unique resume требует fresh ownership. Обычный ответ на
публикацию из другого канала разрешается в canonical session/task через проверенный
delivery mapping, без автоматического слияния сессий владельца. Это требования
целевого профиля, не свойства уже включённого main.

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

### 3.0 Поручение и попытка исполнения

Целевой контракт [Persistent Assistant Spec](persistent-assistant-spec.md)
разделяет постоянное поручение и run. Run — ограниченная попытка; task ref может
отсутствовать для разговора. Final answer и `run.completed` не подтверждают
завершение поручения. Его статус, план, constraints revision, checkpoints и
evidence сохраняются отдельно и переживают рестарт и несколько runs.

Целевая последовательность: durable ingress receipt → session ordering/lease →
task revision и context → model proposal → durable action intent → exact-action
authorization/approval → Broker execution → observation/effect reconciliation →
result/verdict → delivery outbox. HTTP disconnect не отменяет принятое событие;
поздний worker ограничен fencing epoch. Ожидание человека освобождает активное
исполнение и возобновляется durable event, а не только in-memory channel.

Typed [initiative admission](initiative-contract.md) creates the task and its
ordered input atomically after current goal/mandate and prospective budget
checks. Source observations never grant authority. Actual run execution retains
the same approval, budgeting, stop and verified-completion boundaries.

До реализации соответствующих миграций существующие API представляют run как
task. Это переходный read model, а не подтверждение долговечности поручения.

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

> **Implementation note**: Session Service and Orchestrator are described here as separate logical subsystems with distinct responsibilities. In the current deployment they share a single binary (`apps/orchestrator`), but are isolated at the Go package level (`internal/session/`, `internal/run/`, `internal/orchestrator/`). This preserves the logical boundary without the operational cost of a separate service. If scaling or isolation requirements change, the packages can be promoted to their own service without altering contracts.

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
- доставку approval request, включая typed selection approvals;
- приём user approval / reject action или selection decision;
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
Orchestrator собирает execution context: transcript state, tool availability, запрашивает у memory service session summary + structured memory bundle и на этой стадии собирает финальный transport-facing system prompt из operator base prompt и runtime context sections.

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
Run атомарно сохраняет transcript, обязательный final artifact, unverified результат попытки, outbox и post-run jobs.

Transient persistence error сохраняет `finalizing` checkpoint для ограниченного повторного сохранения ответа. Delivery и auxiliary failures не меняют сохранённый результат или `completed`; отсутствие artifact не допускает `completed`.

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
- `finalizing -> failed` только по явной terminal policy; transient persistence recovery повторяет сохранение ответа без model/tools
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
10. `effective system prompt`, assembled by orchestrator from the base prompt plus runtime sections

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
- approval может быть не только бинарным approve/reject, но и typed selection payload;
- для bind-like flows (например, выбор вкладки браузера) approval response может создать durable capability state до следующего tool execution шага;
- при approve run переходит в `tool_running`;
- при reject run переходит в `cancelled`;
- при timeout run переходит в `timed_out`.

### 12.3 Исполнение

Missing approval checker, policy lookup failure, or a missing approval gate for
an action requiring approval stops execution before contacting the Tool Broker.
The same rule applies to every member of a batch. A technical policy failure
never grants permission.

Если approval не требуется или уже получен:
- Tool Broker валидирует schema;
- применяет policy;
- разрешает credential refs только для тех tool families, которые их поддерживают по контракту;
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

В `finalizing` orchestrator сохраняет результат и durable outbox одной транзакцией. После commit отдельный worker доставляет только сохранённое уведомление. Telegram unknown send не повторяется автоматически; Web `result_ready` означает доступность PostgreSQL результата, а не прочтение клиентом. См. [atomic result and delivery](atomic-result-delivery.md).

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
- successful completion создаёт durable auxiliary cleanup job, ограниченный текущим run_id;
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

Operator Doctor uses a real authenticated owner task, accepted input, run and
Session Service lease. Its fixed local command drives the existing execution
states without a model request. Broker prepare/authorize/execute and durable
approval/recovery remain unchanged. POST `/api/v1/doctor/check` returns a pending
receipt (202); polling its `input_id` reads that exact operation. Runtime and
memory reporting failures remain independent results and cannot become a
healthy report because HTTP delivery succeeded. Historical JSON remains readable.
The command's tool reference is deterministic within its actual run and distinct
between runs. Transcript tool IDs are globally unique; a later Doctor operation
must append its own record without overwriting a prior check or colliding with
its ID. Existing suspended continuations retain their recorded exact reference.

Текущий baseline дополнительно предусматривает read-only operator visibility для memory state вне критического run path:

- orchestrator exposes `GET /api/v1/memory` для scope-based просмотра durable memory records;
- Web UI `/memory` использует этот endpoint для просмотра Working / Profile / Episodic / Chunk memory и provenance-safe links;
- этот просмотр не изменяет run lifecycle и не является частью execution-time memory mutation.

---

## 18. Cancel и timeout semantics

An expired Redis session lease is reported as `ErrLeaseNotFound`, including
Redis Lua's nil response. The executor must stop renewal and execution after
this signal; expiry is not a successful renewal or a generic backend outage.

## 18.1 Cancel

The owning lease supervisor retains a private, fixed cancellation cause when a
current authority read fails, an exact lease/run/session/fence comparison changes,
or lease renewal fails. If stream consumption observes the resulting
`context.Canceled`, Core preserves that cause for failure reporting without
storage/provider details. The error still classifies as cancelled and retains
the existing ownership/renewal sentinel. Ordinary parent cancellation and an
independent transport error keep their own cause. This adds no new cleanup
authority, lease retry, guard interval or permission to dispatch/publish. Older
generic cancellation records cannot be retroactively classified from this change.

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

При завершении run Orchestrator сначала отменяет и дожидается остановки
renewal worker, включая незавершённый `RenewLease`, затем вызывает
`ReleaseLease` с ограниченным таймаутом. Renewal использует контекст run;
его штатная отмена при cleanup не превращает успешный run в failed.

Если lease истёк или был потерян:
- run должен быть остановлен или переведён в recovery path;
- Session Service должен предотвратить конкурентное продолжение двумя owners.

---

## 22. Resume и retry

### 22.1 Resume

Для terminal run resume создаёт новый run со ссылкой на исходный. Production
durable ingress также восстанавливает безопасные сохранённые границы ещё
nonterminal run под новым ownership fence; это сохраняет исходный `run_id` и
не меняет terminal history. См. [durable execution](durable-execution-spec.md).

При продолжении после terminal run новый run:
- получает новый `run_id`;
- ссылается на предыдущий run через `resumes_run_id`;
- использует уже сохранённый session/transcript state;
- не меняет terminal state исходного run.

Resume допускается только если:
- есть достаточное состояние для безопасного продолжения;
- lease успешно восстановлен или переиздан;
- terminal run уже не может быть корректно продолжен как тот же execution unit.

### 22.2 Session-scoped external capability state

Некоторые operator-approved capability states могут жить дольше одного run и переиспользоваться следующими run внутри той же Butler session.

Примеры:
- active approval-derived capability;
- session-bound browser access вроде `single_tab_session`.

Для таких состояний действуют правила:
- durable source of truth хранится вне run record;
- resumed run получает только безопасную capability reference, а не низкоуровневый runtime handle;
- потеря capability state не должна нарушать session ownership model;
- run lifecycle остаётся run-centric даже если capability state переживает несколько runs.

### 22.3 Retry

Retry допускается не для всего run, а на уровне отдельных компонентов:
- input event retry;
- transport retry;
- tool retry;
- async memory pipeline retry.

Политики retry должны быть вынесены в отдельные subspec/contract documents.
Production Core допускает один отложенный повтор модели на accepted input после
номинального model-turn timeout. Новый запрос имеет отдельное списание бюджета;
подтверждённые действия не переисполняются. Отмена, истёкшие полномочия и
неизвестный эффект запрещают повтор. См. [model retry policy](model-retry-policy.md).

---

## 23. Минимальные поля run record

Реализован optional `task_id` и pinned `task_revision` в PostgreSQL. Один task
имеет несколько attempts; ordinary conversation run допускает NULL task ref.
`completed` у run означает окончание модельного хода, а не проверенный результат
поручения. Финал сохраняется как unverified task result без очистки рабочей
области. Owner lifecycle и verified completion описаны в
[Durable tasks](durable-tasks-spec.md). Proto run record сохраняет переходную
совместимость; task API возвращает product state отдельно от run state.

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
16. Required final artifact, transcript, unverified attempt result, outbox and auxiliary jobs are committed atomically.
17. Durable auxiliary memory work is scheduled independently of notification delivery.
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

Current fast-response guarantee: Orchestrator registers the approval waiter before
creating the visible approval record or delivering its notification. Duplicate
registration cannot replace it, only one decision is accepted, and cancellation
wins over a buffered response. Where a durable approval exists, its final status
is authoritative over the in-memory response. This fixes the registration race;
durable worker resume after restart and task/action-version binding remain pending
in #5–#7 and must not be inferred from this guarantee.

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

## Durable acceptance and continuation implementation

The implemented acceptance/ownership/approval boundaries are specified in
[Durable Execution](durable-execution-spec.md). PostgreSQL stores ordered input
receipts, monotonic expiring leases and versioned data-only approval continuations.
HTTP acceptance returns 202 independently of model execution; context is built
under ownership. Waiting for the owner releases the worker, while unknown external
outcomes are blocked instead of blindly replayed. The contract explicitly lists
safe recovery boundaries, owner proof and delivery/reconciliation limitations.


Owner amendments, tree cancellation and late-response fencing are specified in [Owner Controls](task-owner-controls-spec.md).

Assigned goal/mandate authority is also checked at model admission, tool dispatch,
task state and atomic final-result commits. Owner authority edits synchronously
fence old assignments; bounded durable jobs then pause/stop their branches. The
lease supervisor cancels an idle provider before branch propagation completes.
Committed results/effect evidence remain historical facts. See
[owner goals and mandates](owner-goals-mandates-spec.md).

Reviewed extensions use the current task/run binding before scoped catalog
loading. Preparation failure retains that truthful attempt association. Fresh
approval recovery reloads current capabilities, replacing stale package data
while retaining known observations without redispatch. See
[extension lifecycle](reviewed-extension-lifecycle.md).
## Optional source-bound ingress recovery

An internally accepted voice/delegation event may carry only an opaque stored
source binding. Its optional trusted `InputSourcePreparation` callback resolves
current content after the new live lease, preserving event/session/type/source,
timestamp and idempotency identity. Missing composition or a replaced checkpoint
binding fails closed before inference. Selected source bodies are not queued.

The delegated worker's durable run key is the accepted `input_id`, separate from
the immutable kernel `delegation:<task_id>` event ID. The input store's private
typed verifier must prove the exact current claim, owner/session, live run fence,
pinned task and original contract source before core validates a local copy of
that envelope. The actual run/checkpoint key stays the input ID. Preparation and
recovery both require this proof for rewritten keys; absent or stale verification
fails before provider entry. Original direct-core keys keep strict equality.

`SourceInputEvent` retains that original opaque event; `SourceDataRefs` identifies
exact DATA items for durable projection only. Checkpoints and approval continuations
remove current source prose, preserve refs/digests and keep instructions and actual
tool/effect observations intact. Recovery resolves current input again; a delegation
reader must independently reload selected roots from the current authoritative
contract, rather than treating stored selected IDs as permission. The saved-final
retry path remains separate and never calls the model/source preparation again.

`AttemptSourceRefs` separately identifies ephemeral failed-child observations;
it does not widen a delegated worker's immutable `SourceDataRefs` selection.
With delegation enabled, ordinary preparation/recovery derives blocked immediate
children from the actual current parent run and reloads their typed owning
attempt/journal/budget observations as private SubagentOutput DATA. Checkpoints
store reload markers, never report bodies. Missing owning composition, stale
lease/parent authority or deleted/changed sources deny release. Current parent
run source pins protect provider/event/final publication. No failed child result,
automatic parent wakeup, executor slot release or successful completion follows
from reading the observation. See `delegation-contract.md` for bounds and scope.


Generated converter observations persist only checked-output SourceRefs, hashes
and metadata. Current source/owner authority checks precede exact active-version
guards at provider release/recovery and atomic result commit. A revoked or changed
used catalog generation requires a fresh task attempt; saved tool results never
redispatch a spent action. Local preparation acknowledgement and a model final
remain unverified semantic task results until the existing explicit verifier
checks completion criteria. See [generated converters](generated-converters.md).

Optional converter developer returns require current immutable053/054 identity,
actual tested child final/report and its full privacy graph on provider/recovery
and atomic final paths. Parent collection grants only the candidate DATA source
through a separate ordinary Broker action. Technical result readiness cannot
accept the child or complete the semantic parent task.

Migration055 optionally schedules two once-only technical continuations of the
same original task after confirmed053/054 and then actual parent activation.
Queue/CAS is atomic; original document/source pins, current owner authority,
budget and private input claim/fence remain mandatory. Opaque readiness is DATA;
no synthetic owner message or semantic acceptance is created. Recovery, Broker
and atomic final repeat the guard. See
[owned converter continuations](converter-continuations.md).

Owner-configured verified-task delivery retains all ordinary attempts/finals and
current execution/lease/source checks, while suppressing outward technical drafts
and progress before channel-delivery observation. Only current independent owner
verification of original criteria and the Task completion transaction create the
one root notice. Source-only historical completion/send checks cannot borrow a
completed run's execution authority. Approval/intervention delivery is preserved.

## Server browser manual-control wait

With the optional owner channel configured, the execution kernel checks the
migration086 owner fence before model budget admission, before a tool proposal
and before consuming each provider event. It waits within the existing run
context; cancellation, authority/budget deadlines and lease renewal continue.
This does not create a completed task, new run, approval or model continuation.
An inference started before takeover cannot be undone. Broker independently
blocks browser dispatch before Claim. An acknowledged explicit owner release
restores agent mode, but a fresh observation is required before another guarded
browser action. Lost release ACK keeps the durable fence closed; no timer other
than the actual finite context expiry grants a resumed browser. See the
[browser contract](browser-session-contract.md#owner-viewing-and-manual-control).
