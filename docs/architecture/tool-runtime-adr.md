# Butler — Tooling and Execution Specification

`browser.login` revision1 is a distinct unsafe commit tool, never an alias for
click. Its private two-phase helper admits one exact native form POST and bounded
same-origin GET redirects. Go performs independent, public-egress-guarded access
verification and removes all private nonce/cookie fields from tool output. Only
the reviewed binary/helper pair may produce `browser_login_v1` proof. See the
[login protocol](browser-login-protocol.md) for supported forms and failure rules.

Local preparation revision4 preserves revision2/3 compatibility and adds only a
Go-guarded lost-context rejection before the runner starts. Its fixed proof
does not reconcile prior writes or grant retries. A fresh context/input is a new
ordinary proposal with current authority and accounting. The optional resource
wrapper retains the proof only after durable lane completion; lost Finish stays
unknown. See the browser session and external-effect contracts.

Communication `source_refs` are optional exact references returned by selected
Catalog observations in the current run. Event/turn IDs are not automatically
Catalog source refs; browser/tool-call IDs and page URLs do not belong there.
Browser-only replies can leave the array empty and put the URL in text. The tool
schema now explains this existing boundary; Catalog selection, export policy,
authority and publication validation remain unchanged.

Local browser preparation revision3 adds a narrow pre-input rejection protocol.
Only Go/helper guards before any input dispatch can produce its closed fixed
proof. Broker and PostgreSQL bind it to the original action and pinned local
preparation policy before recording local absence. Revision2 and ambiguous or
post-input failures retain uncertainty. No runtime orchestration, new permission
or retry is introduced; current page reassessment is a new ordinary proposal.

`browser.freeze_snapshot` revision1 observes an existing document only after
freezing proxy connections and helper routes. Broker/Go require the exact
credential-free `read_only/read_result` contract; only the browser session ID
is accepted. Core can use one bounded probe before blocking an uncertain
interaction, through ordinary Broker authorization/accounting. This adds no
runtime orchestration, recipient receipt, secret resolution path or replay.

`web.search` revision1 uses the existing WebFetch runtime and a fixed
credential-free DuckDuckGo Lite GET endpoint. It returns bounded title/URL/snippet
observations and UTC time. Snippets are discovery evidence; current claims need
primary-page verification. Challenges/non-200/unknown markup/oversize fail
explicitly; recognized no-results returns []. Strict decoding runs before Broker
dispatch and in runtime. Public DNS/redirect/egress guards stay active. The fixed
provider URL participates in web caps and query release, including operator
public-web standing consent. No key/service/dependency was added. Plain HTTP
reads send an identifying User-Agent. The registry maps runtime class webfetch
to the existing operator network class web for caps on all its readers.


Broker's optional `BUTLER_PUBLIC_WEB_READS_ENABLED=true` permits autonomous
public page URLs/queries without per-page release approval for fixed web
readers, HTTP GET/HEAD and revision3 browser navigation. It defaults false;
mode1/contract approval and credential/custom payload/write decisions remain
separate. Runtime public-network checks and operator destination caps still
apply. See [standing URL consent](action-authorization-contract.md).
The opt-in [issue97 research command](../../cmd/effort-eval/README.md) can reveal
only frozen, synthetic fixture observations. These are simulated reads, not
runtime calls or Broker acceptance. It has no external-effect executor and cannot
grant tool, credential or delivery authority. Production Core/Broker/tool-runtime
contracts and native function-call/signature handling are unchanged.

The optional credential vault installs canonical Broker-owned `credentials.*`
controls only when its mounted operator key is available. Registry JSON cannot
replace them. Core owns durable owner interaction waits; Broker owns search,
request admission and scoped field release. Browser runtime compares a resolved
field privately and returns `credential_verified`, never values/hash/length.
This does not prove remote login or weaken submission/unknown-effect policy.

Fixed memory read/process controls apply their strict typed argument decoder
during Broker validation, before effect preparation and dispatch. Numeric bounds
and cross-field rules therefore cannot first fail after a dispatch is journalled.
For example, messages.search accepts limit1..10; limit50 fails validation without
an effect_unknown record. Actual transport/dispatched uncertainty retains its
ordinary reconciliation contract. memory.open may return guarded retained mobile
evidence, as described in [the memory contract](mobile-memory-search.md#archive-expiry-and-retained-evidence).

Optional [deferred tool schemas](tool-discovery.md) add Broker-owned `tools.search`/`tools.load` under matching default-false operator flags. Search ranks the complete permitted catalog; loading only selects model schemas and grants no execution authority. Existing dispatch/input/approval/budget checks apply.


Optional fixed `schedule.*` contracts are owning kernel controls, registered by
Broker and routed to Orchestrator `/internal/schedules` after permanent exact
dispatch. They accept timing/instruction DATA without owner IDs or credentials.
No new runtime/container is added. Future effects still use ordinary Tool Broker.
See [agent schedules](agent-schedules.md) for authority, occurrence and delivery.

## Optional operator destination cap

Broker startup may read `BUTLER_NETWORK_ALLOWLISTS_JSON` for `http`, `browser`
and `web` classes. Omitted classes preserve existing policy; an explicit empty
array denies every supplied destination. URL/expected-URL/batch URL arguments
must satisfy both the configured class cap and the original nonempty contract
domain declaration. Exact domains and `*.domain` patterns use the shared egress
policy. Independent checks retain deny-all intersections without translating
them into an unrestricted empty slice. Invalid configuration prevents startup.
The cap is not mutable through model/UI/database settings and does not replace
the contract's hashed release declaration or any current action, credential,
resource, public DNS, redirect or runtime guard. Catalog and runtime execution
retain their existing authority boundaries.

Tool metadata reads use GetToolContract, without validating synthetic arguments.
Missing metadata is an error and cannot become execution permission.

## Web result correctness

Provider-chain construction skips absent optional providers even when their
constructor's nil pointer is held in a Provider interface. An unconfigured
self-hosted primary must not panic the runtime before the configured fallback
can read a page. No-provider configuration returns the ordinary bounded failure.

Implemented reader fixes retain the current runtime/Broker/credential boundaries.
HTML text extraction skips script/style subtrees. Network readers accept UTF-8
text and JSON/XML media types; missing MIME is sniffed. Binary media, invalid UTF-8
and NUL data return a bounded validation failure without publishing their bytes.
Direct HTTP/Jina responses read at most 2 MiB plus a completeness byte, publish
at most 2 MiB on a valid rune boundary and expose `metadata.truncated`. Oversized
self-hosted JSON envelopes fail instead of parsing an incomplete object.
Serialized results exceeding 3 MiB fail explicitly before private gRPC delivery.

`web.extract` revision 2 URL mode requires HTTP 2xx. HTTP errors and redirects
produce validation failures with `reason:http_status` and numeric `status_code`,
without response bodies. Successful output also retains `status_code`, `provider`,
`final_url`, `mime_type` and `metadata`; an empty HTTP2xx document is a valid empty
observation. Inline HTML/text remains supported. The expanded output schema and
revision participate in normal action/approval hashes; no old grant is rewritten.

`http.parse_html` rejects selectors outside tag/.class/#id/tag.class/tag#id before
network execution. Valid no-match responses contain `matches:[]`, never null.

After durable outcome recording, Broker preserves an HTTP/WebFetch failed read's
validation error only when the response is correlated to the actual grant, has
no failed-result body, and there was no transport/schema error. Messages are at
most 512 bytes and details at most 1024 bytes. Its status remains failed, retryable is false,
and the existing effect_unknown journal state/evidence is retained. This neither
confirms a read/document nor grants a retry. Writes, lost replies, malformed
success outputs and mismatched identities retain the generic unknown response.
The effect journal, current authority and automatic-retry guards are unchanged.

Installed declarative capability admission is specified in
[reviewed extension lifecycle](reviewed-extension-lifecycle.md). Immutable
version/review state and current task permission are checked before credential
inspection and within irreversible dispatch; package code is never installed.

The additive [isolated code workspace store](isolated-code-workspaces.md) prepares
bounded owner/task/version-bound code artifacts with file CAS and private temporary
directories. Canonical workspace DATA contracts now use default-false private
Broker/runtime application composition with exact installed readiness, current
input/source authority, shared budgets and finite resource claims. It exposes
bounded artifacts and snapshots, with no host path or shell execution. Production use requires
current resource leases, budgets, provenance and Broker admission; an uncertain
filesystem/PG boundary blocks instead of replaying a patch.

[Bounded generated converters](generated-converters.md) define the first pure
self-development IR and independent runtime checks. Its package/test/permission
foundation is implemented; catalog/task admission remains a separate pending gate.
No generated converter currently becomes a production capability from a test alone.

Current execution policy is defined by
[exact action authorization](action-authorization-contract.md): the complete
action, current contract/resource/task versions and credential metadata are
prepared and verified before secret release and runtime dispatch. Private
Broker RPCs require service authentication; direct/batch calls cannot bypass
the same policy. Browser navigation/interactions have conservative unknown
effects. Uncertain external outcomes stop automatic continuation.

[Execution budgets](execution-budget-contract.md) reserve call/time/egress in the
same transaction as permanent dispatch. Unknown non-read effects block tool
switching within the task tree. Runtime output schemas gate evidence; malformed
completed responses stay uncertain. Batch execution is rejected before the first
dispatch, and no partial result set is supplied as completed model input.

## Owning tool family communication

После #128/#132 message/photo/report квалифицированы для явной ограниченной
композиции. Operator-only `-communication-profile` на Core/Broker регистрирует
их на точно закреплённых Database/OwnerID и authenticated control URL.
Без аргумента сохраняется legacy; отдельный lab constructor требует `_test`.
Новый профиль не меняет exact dispatch, grants, source/privacy, budgets или
approval checks и не делает widget готовым. См. актуальный scoped rollout
в [User Communication](user-communication-spec.md).

Добавлены contracts, strict decoder, fixed authenticated HTTP control и
Broker executor. Startup без operator profile не регистрирует family или route. Private token не
заменяет actual dispatched journal/grant/Input/lease proof. Lookup после lost ACK
только читает сохранённый receipt; публикация повторно не исполняется.
Widget readiness требует отдельной композиции; scoped profile рекламирует
только принятые message/photo/report.
См. [backend scope](../ai/communication-core-acceptance-20261021.md).

[User Communication](user-communication-spec.md) добавляет целевой контракт
communication.message/photo/report/widget. Все операции проходят штатные
Broker schema/policy/grant/effect/budget проверки и фиксированный приватный
endpoint owning Communication module оркестратора. Это явно описанный owning
control-tool путь по существующему паттерну, без нового runtime-контейнера.
Только channel dispatcher исполняет сохранённые разрешённые доставки; он не
принимает orchestration решений. Каталог пока не содержит эту реализацию.

## 1. Статус документа

**Тип документа:** Architecture Subspec / Tooling Specification
**Версия:** 0.1
**Статус:** Draft
**Связанные документы:** Butler PRD + Architecture, Memory Model, Secret Management

---

## 2. Назначение

Данный документ формально описывает подсистему инструментов Butler.

Подсистема инструментов отвечает за:

* выполнение внешних действий от имени Butler;
* изоляцию исполнения от агентного слоя;
* безопасную работу с авторизацией и учётными данными;
* контроль политик, режимов автономности и границ доступа;
* наблюдаемость, аудит и диагностируемость.

---

## 3. Цели

Подсистема инструментов должна обеспечивать:

1. Единый контракт вызова инструментов для агента.
2. Изолированное исполнение инструментов.
3. Централизованный контроль всех tool calls.
4. Поддержку безопасной авторизации через `credential_ref`.
5. Переносимость между разными runtime, включая Playwright, Chrome DevTools Protocol и HTTP clients.
6. Docker-oriented модель развёртывания.
7. Полную трассируемость и аудит выполнения.
8. Поддержку self-inspection и doctor-сценариев.
9. Независимость инструментария от конкретного model transport.

---

## 4. Не-цели

Подсистема инструментов в текущем объёме не включает:

* self-extending tools без независимой проверки и catalog admission;
* произвольную загрузку стороннего кода в runtime;
* прямой доступ LLM к секретам;
* исполнение инструментов без централизованного broker layer;
* смешение execution layer и memory layer.

Целевой контракт [Persistent Assistant Spec](persistent-assistant-spec.md)
допускает staged/validated/versioned расширения и навыки, автоматически
подключаемые в пределах installer mandate. Tool Broker остаётся единственным
путём effect authorization, schema validation, routing и credential mediation.
Модель и runtime не расширяют capabilities. Policy повторно проверяет task,
mandate и tool/schema versions непосредственно перед эффектом; action journal
отличает intent, confirmed effect и unknown outcome. Параллельные исполнители
используют ресурсные аренды и общий бюджет поручения. Новые возможности требуют
отдельных миграций/тестов и не считаются реализованными этим уточнением ADR.

---

## 5. Архитектурные принципы

### 5.1 Tool execution отделено от reasoning

Агент принимает решение о вызове инструмента, но не исполняет инструмент самостоятельно.

### 5.2 Каждый вызов проходит через Tool Broker

Ни один runtime не вызывается агентом напрямую.

### 5.3 Инструменты исполняются в изолированных runtime containers

Исполнение должно быть отделено по классам инструментов.

### 5.4 Tool contracts стабильнее runtime implementation

Контракт инструмента не должен зависеть от того, используется ли внутри Playwright, CDP или другой движок.

### 5.5 Секреты не попадают в модельный контекст

Если инструмент использует авторизацию, агент передаёт только `credential_ref`, а не реальное значение.

### 5.6 Tool output нормализуется

Результаты выполнения приводятся к безопасному и стабильному формату.

### 5.7 Tool policy применяется централизованно

Права, домены, режимы автономности и credential usage проверяются до исполнения.

---

## 6. Логическая модель подсистемы

Подсистема инструментов состоит из следующих компонентов:

1. **Tool Registry**
2. **Tool Broker**
3. **Tool Runtime Containers**
4. **Credential Broker**
5. **Tool Audit Log**
6. **Doctor Runtime**
7. **Configuration Introspection Interface**

---

## 7. Основные сущности

## 7.1 Tool

Логическая операция, доступная агенту.

Примеры:

* `browser.navigate`
* `browser.click`
* `browser.fill`
* `browser.snapshot`
* `http.request`
* `doctor.check_system`

Tool — это публичная операция с формальным контрактом.

---

## 7.2 Tool Contract

Описание доступного инструмента.

Каждый contract должен содержать:

* `tool_name`
* `description`
* `tool_class`
* `input_schema`
* `output_schema`
* `runtime_target`
* `risk_level`
* `supports_credential_refs`
* `requires_approval`
* `supports_streaming`
* `status`

---

## 7.3 Tool Call

Конкретный вызов инструмента в рамках run.

Минимальные поля:

* `tool_call_id`
* `run_id`
* `tool_name`
* `args`
* `status`
* `runtime_target`
* `started_at`
* `finished_at`
* `result`
* `error`

---

## 7.4 Tool Registry

Каталог доступных инструментов.

Должен хранить:

* список инструментов;
* их контракты;
* статус доступности;
* bindings к runtime;
* политики безопасности;
* поддержку credential-aware execution.

---

## 7.5 Tool Broker

Центральный управляющий слой для всех вызовов инструментов.

Отвечает за:

* schema validation;
* policy enforcement;
* autonomy mode enforcement;
* credential mediation;
* routing;
* timeout handling;
* result normalization;
* audit.

---

## 7.6 Tool Runtime

Исполнительный слой, в котором происходит фактическое выполнение инструмента.

Runtime:

* не принимает архитектурных решений;
* не выдает секреты агенту;
* не управляет режимами автономности;
* исполняет только уже разрешённые операции.

---

## 7.7 Credential Reference

Типизированная ссылка на учётные данные, передаваемая в tool args вместо секрета.

Пример:

```json
{
  "type": "credential_ref",
  "alias": "Пиццерия",
  "field": "password"
}
```

---

## 8. Классы инструментов Butler V1

## 8.1 Browser Tools

### Назначение

Взаимодействие с веб-интерфейсами через браузерный runtime.

### Минимальный набор

* `browser.navigate`
* `browser.click`
* `browser.fill`
* `browser.type`
* `browser.wait_for`
* `browser.snapshot`
* `browser.extract_text`
* `browser.set_cookie`
* `browser.restore_storage_state`

### Особенности

* должны поддерживать `credential_ref` там, где вводятся чувствительные данные;
* должны работать поверх абстрактного browser contract;
* должны быть переносимы между Playwright и CDP-based implementations.

### Дополнительный режим: session-bound single-tab control

Butler может поддерживать отдельную tool family `single_tab.*` для управления реальной пользовательской вкладкой, выбранной через explicit approval flow.

Для этого режима действуют отдельные правила:

* доступ привязывается к durable `single_tab_session`, а не к raw `tab_id` в model-visible contract;
* у агента нет public tools для list/switch/open/close/focus tab и window operations;
* follow-up `single_tab.*` action contracts могут разрешать активную durable session из текущего Butler session context, чтобы агенту не приходилось повторно передавать raw tab/session identifiers после успешного bind-flow;
* legacy single-tab runtime содержит recovery через `single_tab.bind`, но не является защищённой ресурсной границей; [guarded user-tab runtime](single-tab-resource-boundary.md) запрещает recovery/повтор и сохраняет uncertain outcome при потере ответа;
* `single_tab.*` не поддерживает `credential_ref`, cookie injection, `storage_state` restore и другие secret-bearing browser args;
* policy boundary строится вокруг session-bound capability и broker-side prereq checks, а не вокруг domain allowlist.
* `single_tab.bind` использует typed `browser_tab_selection` approval: runtime запрашивает tab candidates у extension через orchestrator relay и создаёт durable approval request для Web UI + Telegram.
* чтобы избежать двойного подтверждения, `single_tab.bind` не должен требовать generic pre-tool approval через `requires_approval=true`; подтверждение выполняется внутри bind-flow как typed selection approval.

---

## 8.2 HTTP / Web Tools

### Назначение

Выполнение HTTP-запросов и обработка веб-ответов.

### Минимальный набор

* `http.request`
* `http.download`
* `http.parse_html`

### Особенности

* должны поддерживать auth через `credential_ref`;
* должны валидировать допустимые домены и endpoint classes;
* должны возвращать нормализованный, безопасный output.

---

## 8.3 WebFetch Tools

### Назначение

Нормализованное извлечение HTML, текста и page-derived metadata через provider abstraction с self-hosted primary path.

### Минимальный набор

* `web.fetch`
* `web.fetch_batch`
* `web.extract`

### Особенности

* primary provider должен быть self-hosted;
* внешние SaaS providers допустимы только как fallback;
* output должен явно указывать выбранный provider, cache metadata и final URL;
* реализация должна быть отделена от thin HTTP transport helpers, если retrieval semantics становятся существенно богаче `http.request`.

---

## 8.4 Doctor / Self-Inspection Tools

The fixed operator `doctor.check_system` contract declares read-only retry safety
and `read_result` evidence. This describes its config/health observation, not
assistant quality or an external mutation. The authenticated Doctor UI admits it
through the normal durable input and exact-action Broker path, including owner
confirmation required by autonomy policy. ToolResult status/error and missing
or stale reports remain distinct from RPC/HTTP success; no legacy bypass exists.

### Назначение

Диагностика самого Butler, его конфигурации и зависимостей.

### Минимальный набор

* `doctor.check_system`
* `doctor.check_container`
* `doctor.check_database`
* `doctor.check_provider`
* `doctor.generate_report`

### Особенности

* doctor должен иметь полную информацию о текущей конфигурации Butler;
* doctor должен видеть effective configuration, sources, validation status и component bindings;
* doctor не должен видеть секреты в открытом виде;
* doctor должен работать через configuration introspection layer, а не через ручной разбор env/file sources.
* `doctor.check_container` должен ограничиваться диагностикой через настроенные health/status endpoints сервисов, а не прямым Docker control.
* runtime `tool-doctor` не должен получать Docker socket или выполнять restart/stop/container lifecycle operations; такие операции относятся к отдельному control-plane helper.

---

## 9. Docker-oriented execution model

## 9.1 Базовая модель

Butler использует стратегию `container per tool class`.

### Рекомендуемые runtime containers V1

* `butler-tool-broker`
* `butler-tool-browser`
* `butler-tool-http`
* `butler-tool-webfetch` при выделенном retrieval runtime
* `butler-tool-doctor`

Host companion processes вроде browser bridge / native messaging host могут существовать вне Compose runtime set, если они нужны для локальной интеграции с пользовательским браузером, но они не отменяют broker-routed execution model.

## 9.2 Причины выбора

* изоляция зависимостей;
* отдельные security boundaries;
* разные resource limits;
* независимое обновление runtime classes;
* снижение связности между инструментами.

## 9.3 Что не используется в V1

* контейнер на каждый отдельный tool call;
* один общий runtime для всех tools без разделения по классам.

---

## 10. Формат вызова инструмента

Каждый tool call должен иметь:

* имя инструмента;
* типизированные аргументы;
* execution context;
* optional credential references;
* optional policy metadata.

Пример:

```json
{
  "tool": "browser.fill",
  "args": {
    "selector": "input[type='password']",
    "value": {
      "type": "credential_ref",
      "alias": "Пиццерия",
      "field": "password"
    }
  }
}
```

---

## 11. Типы аргументов

## 11.1 Literal value

```json
{
  "type": "literal",
  "value": "Маргарита"
}
```

## 11.2 Credential reference

```json
{
  "type": "credential_ref",
  "alias": "Пиццерия",
  "field": "username"
}
```

## 11.3 Требование

Все инструменты, работающие с чувствительными входами, обязаны поддерживать не только literal values, но и typed references.

---

## 12. Credential-aware tool execution

## 12.1 Назначение

Подсистема должна позволять инструментам использовать секреты без раскрытия их агенту.

## 12.2 Правило

Агент никогда не передаёт пароль, токен или cookie value напрямую.
Агент передаёт только `credential_ref`.

## 12.3 Процесс

1. Агент формирует tool call с `credential_ref`.
2. Tool Broker валидирует alias, field, домен и режим автономности.
3. Credential Broker разрешает ссылку в runtime-only значение.
4. Runtime использует секрет в момент исполнения.
5. Результат возвращается без раскрытия секрета.

## 12.4 Где обязательно

* `browser.fill`
* `browser.type`
* `browser.set_cookie`
* `browser.restore_storage_state`
* `http.request`

---

## 13. Жизненный цикл tool call

### 13.1 Планирование

Агент выбирает инструмент для следующего шага.

### 13.2 Формирование вызова

Агент создаёт typed tool call.

### 13.3 Валидация

Tool Broker проверяет:

* известен ли инструмент;
* соответствует ли schema;
* разрешён ли вызов;
* нужен ли approval;
* выполнены ли session-scoped prerequisites вроде активного capability session;
* допустимы ли credential references.

### 13.4 Разрешение ссылок

Если есть `credential_ref`, они передаются в Credential Broker.

### 13.5 Маршрутизация

Tool Broker выбирает runtime target.

### 13.6 Исполнение

Runtime выполняет операцию.

### 13.7 Нормализация результата

Результат очищается от чувствительных данных и приводится к output schema.

### 13.8 Возврат агенту

Агент получает только безопасный итог.

### 13.9 Аудит

Вызов фиксируется в transcript store и tool audit log.

---

## 14. Browser tool model

## 14.1 Принцип

Реализованный контракт обычных `browser.*` операций описан в
[Explicit Browser Session Contract](browser-session-contract.md). `browser.read`
читает независимый URL; `browser.open` создаёт явно именованный контекст одного
run с фиксированным origin и TTL. Остальные действия требуют этот handle и
проверяют owner, exclusive control, текущее наблюдение/URL и scope. Snapshot и
extract_text не выполняют навигацию. Живой контекст не восстанавливается скрыто
после падения процесса. `single_tab.*` остаётся отдельным одобренным режимом.

Browser tools должны быть максимально универсальными и не зависеть от жёстко встроенных login сценариев.

## 14.2 Основной подход

Агент управляет UI flow сам:

* навигация;
* клики;
* ожидания;
* поиск элементов;
* заполнение полей.

Если нужно заполнить чувствительное поле, агент передаёт `credential_ref`, а не значение.

## 14.3 Причина

Это обеспечивает:

* переносимость между Playwright и CDP;
* гибкость для нестандартных сайтов;
* отсутствие привязки к одному browser-specific auth flow;
* безопасную авторизацию без доступа модели к секрету.

---

## 15. HTTP tool model

## 15.1 Принцип

HTTP tools должны быть thin execution layer над нормализованным контрактом.

## 15.2 Возможности

* GET / POST / PUT / DELETE по policy;
* headers, query params, body;
* auth через `credential_ref`;
* response normalization.

## 15.3 Ограничения

* domain allowlist;
* method policy;
* payload size limits;
* secret-safe logging.

---

## 16. Doctor and configuration introspection

## 16.1 Требование

Doctor tools должны иметь полный доступ к конфигурационной модели Butler.

## 16.2 Doctor должен видеть

* effective configuration;
* configuration sources;
* validation status;
* defaults / overrides;
* component bindings;
* restart requirements;
* masked secret presence.

## 16.3 Doctor не должен видеть

* реальные секретные значения;
* API keys в полном виде;
* пароли;
* токены;
* cookies.
* Docker socket и прямые container lifecycle controls.

## 16.4 Configuration Introspection Layer

Doctor должен читать конфигурацию через отдельный introspection interface.

### Для каждого config key требуется:

* `key`
* `component`
* `type`
* `required`
* `default_value`
* `effective_value`
* `source`
* `is_secret`
* `requires_restart`
* `validation_status`
* `validation_error`

---

## 17. Роль Tool Broker

Tool Broker — обязательный компонент подсистемы.

### 17.1 Функции

1. registry lookup
2. schema validation
3. policy enforcement
4. autonomy mode enforcement
5. credential mediation
6. runtime routing
7. timeout / retry control
8. result normalization
9. audit logging

### 17.2 Ограничения

Tool Broker:

* не является execution runtime;
* не возвращает секреты агенту;
* не хранит reasoning state;
* не смешивается с model transport.

---

## 18. Tool Registry

## 18.1 Назначение

Хранить формальное описание всех доступных инструментов.

## 18.2 Минимальные поля

* `tool_name`
* `tool_class`
* `description`
* `runtime_target`
* `input_schema`
* `output_schema`
* `risk_level`
* `supports_credential_refs`
* `requires_approval`
* `status`

## 18.3 Статусы

* `active`
* `disabled`
* `experimental`

---

## 19. Политики безопасности

## 19.1 Tool-level policy

Определяет:

* доступность инструмента;
* risk level;
* допустимые режимы автономности;
* необходимость approval.

## 19.2 Domain-level policy

Определяет:

* разрешённые домены;
* допустимые endpoint classes;
* допустимые browser navigation targets.

Для session-bound browser tools вроде `single_tab.*` domain-level policy может быть не основной границей доступа.
В таком режиме policy опирается на active capability session и explicit product rules, а не на domain allowlist.

## 19.3 Credential policy

Определяет:

* какие alias можно использовать;
* в каких tool classes;
* для каких доменов;
* с каким approval policy.

Если tool family по контракту не поддерживает `credential_ref` (например, `single_tab.*`), credential policy к ней не применяется и broker обязан отклонять secret-bearing args на этапе валидации.

---

## 20. Режимы автономности

Подсистема инструментов обязана учитывать глобальный autonomy mode Butler.

### Mode 0 — Read / Assist

* только безопасные read-only инструменты;
* credential usage запрещено.

### Mode 1 — Propose

* агент может предлагать tool calls;
* выполнение mutating calls только после подтверждения.

### Mode 2 — Guarded Execute

* разрешены ограниченные mutating operations;
* credential usage допускается по policy.

### Mode 3 — Extended Autonomy

* доступен широкий спектр действий;
* всё определяется policy layer.

---

## 21. Формат результата инструмента

Результат должен быть:

* безопасным;
* компактным;
* типизированным;
* пригодным для дальнейшей агентной логики.

Пример:

```json
{
  "status": "success",
  "tool": "browser.fill",
  "message": "Поле заполнено",
  "metadata": {
    "selector": "input[type='password']",
    "credential_used": true
  }
}
```

---

## 22. Формат ошибок

Каждый инструмент должен возвращать нормализованные ошибки.

Минимальные поля:

* `error_type`
* `message`
* `retryable`
* `tool_name`
* `details`

Примеры:

* `validation_error`
* `policy_denied`
* `runtime_error`
* `timeout`
* `credential_resolution_failed`
* `domain_not_allowed`

---

## 23. Наблюдаемость и аудит

Для каждого tool call должны фиксироваться:

* `run_id`
* `tool_call_id`
* `tool_name`
* `runtime_target`
* `status`
* `duration_ms`
* `used_credential_alias`
* `target_domain`
* `result_size`
* `error_type`

Для doctor tools дополнительно:

* `checked_component`
* `config_snapshot_id`
* `validation_summary`

---

## 24. Минимальный V1 scope

Подсистема инструментов Butler V1 обязана включать:

* Tool Broker
* Tool Registry
* Browser Runtime
* HTTP Runtime
* Doctor Runtime
* typed tool contracts
* credential-aware execution
* configuration introspection support for doctor
* audit logging
* timeout handling
* safe result normalization

---

## 25. Открытые решения

1. Решено в Sprint 4-5: `Tool Broker` и runtime containers используют gRPC-контракт `proto/runtime/v1/runtime.proto` с unary `Execute`, где broker передаёт уже валидированный `ToolCall`, `ToolContract`, `ExecutionContext` и при необходимости `resolved_credentials`, а runtime возвращает нормализованный `ToolResult`.
2. Конкретная реализация Browser Runtime: Playwright, CDP или hybrid adapter.
3. Политика retries для browser/http failures.
4. Формат streaming outputs в будущем.
5. Уровень детализации doctor reports в V1.
6. Нужен ли отдельный policy engine или достаточно встроенного policy layer в Tool Broker.

---

## 26. Итоговый тезис

Подсистема инструментов Butler должна быть:

* централизованно управляемой;
* контейнерно изолированной;
* credential-aware;
* переносимой между разными runtime;
* безопасной по умолчанию;
* пригодной для полной диагностики состояния самой системы.

Итоговая модель работы выглядит так:

1. Агент работает только с формальными tool contracts.
2. Каждый вызов проходит через Tool Broker.
3. Tool Broker валидирует, проверяет политику и маршрутизирует вызов.
4. Исполнение происходит в runtime container соответствующего класса.
5. Sensitive inputs, когда они вообще поддерживаются данным tool contract, передаются только через `credential_ref`.
6. Секреты разрешаются только в системном слое и только в момент исполнения.
7. Doctor tools получают полный доступ к effective configuration Butler, кроме открытых секретов.
8. Агент получает только безопасный и нормализованный результат.

## 27. Журнал внешних эффектов

ToolContract содержит версию и effect_contract. Broker сохраняет постоянный ID
намерения, отпечаток точных аргументов/контракта и разрешение до вызова runtime.
ExecutionContext передаёт системные action_id, idempotency_key и fingerprint
вне model-visible args. Только явно объявленный HTTP recipient protocol может
получить такой ключ; общий HTTP/browser/Telegram контракт безопасный повтор не
обещает. Runtime не повторяет запрос и не принимает решение об orchestration.
ToolResult возвращает action_id, effect_state и ссылки на доказательства;
технический completed без доказательства внешней записи становится
effect_unknown и останавливает продолжение модели. Подробный контракт и
независимая сверка: [external-effect-journal.md](external-effect-journal.md).

## 28. Недоверенные данные и среда выполнения

Наблюдения инструмента не становятся системными инструкциями, включая их
сводки и сохранённую память. HTTP/Chromium проверяют публичный адрес и все DNS
ответы до отправки, не следуют перенаправлениям и ограничивают исходящие байты.
Chromium использует отдельный public-network proxy с operator domain caps, запрещает WebSocket и
service workers; helper исполняется только из встроенных проверенных байтов.
Doctor получает фиксированный read-only ответ ядра без учётных данных.
Подробные гарантии и ограничения:
[agent-security-boundaries.md](agent-security-boundaries.md).

## 29. Локальная подготовка браузерной формы

`browser.prepare_fill` и `browser.prepare_type` — отдельные revision 2 contracts.
Runtime замораживает все HTTP/CONNECT соединения и page routes до ввода; последующие
наблюдения сохраняют запрет, включая delayed GET/POST autosave. Только отдельно
авторизованный click получает ограниченное окно после live DOM/URL guards, затем
сеть снова замораживается. `local_browser_preparation_v1` подтверждает только
локальное состояние с точным session/URL/новой observation version; не отправку
формы. Legacy live fill/type и click сохраняют неизвестный внешний эффект без
recipient proof. Read/open/navigate revision 3 имеют enforced GET/HEAD contract;
старые navigation contracts не наследуют это право. Подробности и evidence:
[browser-session-contract.md](browser-session-contract.md).

## 30. Ограниченные создаваемые конвертеры

Первый класс `delimited_json_v1` — чистая конечная IR без shell, произвольного
компилятора, process/network/filesystem операций. Broker сохраняет discovery,
candidate, независимые проверки, расходуемые trials, active digest/generation и
usage в052. Отдельный Go runtime работает без сети через фиксированный Unix socket;
ему не передаются основные учётные данные или PostgreSQL. Every call remains an
ordinary exact grant/journal action. Source/owner guards precede lifecycle/effect
guards; activation requires current ConverterPermission and independent passed
checks plus trial. Local acknowledgement proves preparation only. See
[generated-converters.md](generated-converters.md).

The optional reviewed developer-child route is distinct from context-only
delegation. Exact053 capabilities are only discover/develop/test. Immutable054
return proof and ordinary parent `converters.collect` yield references only;
parent original trials and activation retain separate current action grants.

Optional055 owning continuations schedule only those previously permitted parent
phases and fresh-catalog original work, after actual confirmed receipts. They
cannot grant runtime rights or owner authority. Exact source/claim/fence guards
remain before Broker dispatch and final. See
[converter-continuations.md](converter-continuations.md).

## 31. Необязательная ресурсная граница управляемого браузера

`internal/resourceexec` связывает существующие grant/journal/budget с конечной
ресурсной арендой в одной транзакции. ID и режим определяет сервер из точного
контракта и подтверждённого runtime session; модель не выбирает lock/path/fence.
Runtime wrapper проверяет приватный одноразовый ticket через внутренний
авторизованный endpoint после локального lane lock и до реального backend.
Транзакции завершаются до HTTP/gRPC исполнения. Runtime сохраняет текущие
origin/credential/page guards и не принимает решений об orchestration.
Receipt подтверждает завершение локального lane, но не внешний результат;
неопределённость остаётся в journal и не разрешает replay. Конструкторы не
включены в production bootstrap/Compose; legacy `single_tab` не покрыт.
Полный контракт: [resource-lease-contract.md](resource-lease-contract.md).

## 32. Чтение источников памяти через Broker

Опциональные фиксированные `memory.search`, `memory.open`, `memory.neighbors`
регистрируются через `RegisterOwned` при operator-only флаге
`BUTLER_MEMORY_READ_ENABLED` в обоих сервисах. Их runtime target —
`broker://memory-reader-v1`; owning endpoint Orchestrator —
`/internal/memory-read`, с существующей приватной service authentication.
Это read-only DATA-контракты без credential refs и без изменения accepted memory.

Каждый вызов проходит обычные current task/goal/mandate checks, exact
Prepare/Authorize, permanent dispatch, shared budgets и tool trace. Owning service
восстанавливает актуальные input claim и lease/fence из durable binding; модель
не выбирает owner/scope/job authority. Каталог проверяет полный граф источников
после ожиданий и перед возвратом; subsequent provider/final guards сохраняются.

Поиск возвращает краткие ссылки; открытие требует exact revision/access/hash и
stable fragment key. Neighbors читает только соседние catalog ordinals. Offset и
лимиты выражены в Unicode characters; max8192 на вызов, search10/page, pages0–9,
response128KiB. Общие run/tool/context budgets ограничивают повторные чтения.
Version1 — bounded PostgreSQL exact/full-text catalog search без внешних запросов;
адаптивный hybrid retrieval и нагрузочная квалификация остаются отдельными gates.
Полный контракт и диагностика: [memory read control](../../internal/memory/readcontrol/README.md).

## Server browser owner channel

Server Chromium supports public multi-origin navigation (revision3) and a
private owner-only Web viewport. Core authenticates the human, Broker validates
current ownership and routes frames/input with durable control fences, and the
runtime serializes the actual page. This protocol is excluded from model tool
schemas and transcripts. Legacy `single_tab.*` contracts are disabled in the
standard catalog. See [browser owner control](browser-session-contract.md#owner-viewing-and-manual-control).

## Retrieval observations (WebFetch revision 3)

`web.fetch` and batch items return bounded readable anchor links and the runtime
observation timestamp. Links are untrusted discovery DATA and still require a
separate ordinary Broker read. They are not primary-source certification, an
event confirmation or permission to navigate. See
[research and follow-ups](research-followups.md).

Navigation revision4 uses a Go-only lost/frozen-context pre-navigation proof.
The resource wrapper may forward a correlated verified rejection only after
current authority and durable lane Finish; it omits the success ACK and session
mapping. Finish completion describes the lane, not external success. Legacy
navigation and in-flight errors keep conservative UNKNOWN semantics.


Browser observations can include bounded public-form constraints and frozen local
validity flags, without values/messages. This optional observation DATA changes
no input schema, contract revision/hash, permission or effect verifier. Secret
contexts/forms suppress feedback; see the browser and credential contracts.
