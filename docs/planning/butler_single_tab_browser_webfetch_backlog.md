# Butler Single-Tab Browser Control + WebFetch v1 - Implementation Backlog

## 1. Назначение

Этот бэклог описывает пошаговую реализацию двух связанных направлений:

- `single_tab` browser control для локального браузера пользователя;
- `web.fetch` / `web.extract` подсистемы с self-hosted primary path.

Документ фиксирует не только очередность задач, но и обязательные продуктовые и архитектурные решения, чтобы реализация не разошлась с Butler execution model, approval flow и Tool Broker boundaries.

---

## 2. Зафиксированные решения

- Используется tab-bound модель доступа: доступ привязывается к одной вкладке через `tab_id`, а не к домену.
- После bind нет дополнительных approval на действия внутри этой вкладки.
- Смена домена в той же вкладке не разрывает `single_tab_session`.
- У агента нет public tools для list/switch/open/close/focus tab или window operations.
- `single_tab.*` оформляется как отдельная tool family, а не как расширение `browser.*`.
- `credential_ref` в `single_tab.*` не поддерживается вообще.
- `single_tab_session` является durable session-scoped сущностью в PostgreSQL.
- Redis может использоваться только для transient lease/heartbeat/host liveness state.
- Approval выбора вкладки должен приходить в Web UI и Telegram через единый durable approval flow.
- `browser-bridge` / native host считается host companion component, а не обычным compose-only runtime.
- `web.fetch*` должно иметь self-hosted primary provider; внешние SaaS допустимы только как fallback.

---

## 3. Обязательные инварианты

- Session Service сохраняет ownership и ordering; single-tab bind не должен переносить эту ответственность в runtime или extension.
- Orchestrator управляет lifecycle bind/approval/resume, но не исполняет browser actions напрямую.
- Tool Broker остаётся обязательной точкой validation, policy enforcement и routing для всех `single_tab.*` и `web.fetch*`.
- Local host и extension исполняют команды, но не принимают orchestration decisions.
- Сырые `tab_id` не попадают в model-visible context, Telegram payloads или public tool args.
- `single_tab.*` не поддерживает секретные инъекции, cookies, storage-state restore и любые credential-bearing args.
- WebFetch cache не является source of truth; это только performance layer.

---

## 4. Формат задач

- **ID** - уникальный идентификатор.
- **Приоритет** - P0 (критично), P1 (важно), P2 (улучшение).
- **Зависимости** - задачи, которые должны быть выполнены раньше.
- **Результат** - конкретный артефакт задачи.
- **Критерии приемки** - проверяемые условия готовности.

---

## 5. Эпик A - Contract and spec preparation

### A-01: Обновить архитектурные спеки под `single_tab` и `web.fetch`
- **Приоритет:** P0
- **Зависимости:** нет
- **Результат:** согласованный набор spec updates
- **Критерии приемки:**
  - Обновлены `docs/architecture/run-lifecycle-spec.md`, `docs/architecture/tool-runtime-adr.md`, `docs/architecture/credential-management.md`.
  - В спеке явно зафиксировано, что `single_tab.*` не поддерживает `credential_ref`.
  - В спеке явно зафиксирован approval type `BROWSER_TAB_SELECTION`.
  - В спеке явно зафиксировано, что `single_tab_session` session-scoped и durable.

### A-02: Определить protobuf-контракты для browser tab selection и WebFetch
- **Приоритет:** P0
- **Зависимости:** A-01
- **Результат:** новые или расширенные proto definitions
- **Критерии приемки:**
  - Добавлен контракт для typed approval payload `BrowserTabSelection`.
  - Добавлен контракт для `SingleTabService` или эквивалентного runtime-facing API.
  - Добавлен контракт для `WebFetchService` или новых `toolbroker` tool schemas.
  - Generated Go bindings собираются без ручных правок.

### A-03: Спроектировать durable storage для tab selection и tab sessions
- **Приоритет:** P0
- **Зависимости:** A-01
- **Результат:** миграции и storage model design
- **Критерии приемки:**
  - Спроектированы таблицы `single_tab_sessions`, `approval_tab_candidates`, `web_fetch_cache`.
  - Для `single_tab_sessions` определены статусы минимум: `PENDING_APPROVAL`, `ACTIVE`, `TAB_CLOSED`, `REVOKED_BY_USER`, `EXPIRED`, `HOST_DISCONNECTED`.
  - Для `approval_tab_candidates` предусмотрен short-lived one-time `tab_token`.
  - У таблиц есть индексы по `session_key`, `status`, `approval_id`, `created_at`.

### A-04: Обновить planning notes и service ownership boundaries
- **Приоритет:** P1
- **Зависимости:** A-01
- **Результат:** зафиксированное ownership map по новым подсистемам
- **Критерии приемки:**
  - Документировано, что `browser-bridge` не принимает orchestration decisions.
  - Документировано, что Tool Broker выполняет prereq check для `single_tab_session`.
  - Документировано, что WebFetch primary path отделен от текущего `tool-http` runtime.

---

## 6. Эпик B - WebFetch v1 foundation

### B-01: Ввести public tool contracts `web.fetch`, `web.fetch_batch`, `web.extract`
- **Приоритет:** P0
- **Зависимости:** A-01, A-02
- **Результат:** tool registry contracts и schemas
- **Критерии приемки:**
  - В `configs/tools.json` или его successor registry добавлены новые `web.fetch*` инструменты.
  - Контракты не смешиваются с текущими `http.*`.
  - Ответы нормализованы и содержат `provider`, `final_url`, `status`, `content_type`, `text`, `metadata`.

### B-02: Реализовать provider abstraction для WebFetch
- **Приоритет:** P0
- **Зависимости:** B-01
- **Результат:** provider chain с fallback policy
- **Критерии приемки:**
  - Поддержан порядок `self_hosted_primary -> jina_reader_fallback -> plain_http_fallback`.
  - Fallback policy прозрачна и логируется.
  - Silent behavior-changing fallback без observability не допускается.

### B-03: Добавить отдельный runtime `apps/tool-webfetch/` или эквивалентную границу
- **Приоритет:** P0
- **Зависимости:** B-02
- **Результат:** отдельный runtime/service boundary для WebFetch
- **Критерии приемки:**
  - Реализация не размывает ответственность текущего `apps/tool-http/`.
  - README нового сервиса описывает provider model, env surface и health checks.
  - Docker Compose wiring добавлено отдельно от `tool-http`.

### B-04: Реализовать self-hosted primary provider adapter
- **Приоритет:** P0
- **Зависимости:** B-03
- **Результат:** рабочий primary provider path
- **Критерии приемки:**
  - Butler может извлекать HTML и текст без внешней SaaS-зависимости.
  - Конфигурация primary provider self-hosted и документирована.
  - Есть bounded timeout, retry policy и нормализованные error classes.

### B-05: Реализовать fallback providers и cache layer
- **Приоритет:** P1
- **Зависимости:** B-04
- **Результат:** fallback adapters и durable cache
- **Критерии приемки:**
  - `jina_reader_fallback` и `plain_http_fallback` работают за единым контрактом.
  - `web_fetch_cache` снижает повторные запросы, но не ломает correctness.
  - Есть cache TTL и explicit cache-hit metadata.

### B-06: Добавить тесты и acceptance checks для WebFetch
- **Приоритет:** P0
- **Зависимости:** B-05
- **Результат:** unit/integration coverage для WebFetch path
- **Критерии приемки:**
  - Есть unit tests для provider selection и fallback.
  - Есть integration tests для cache behavior и normalized responses.
  - README и `.env.example` обновлены.

---

## 7. Эпик C - Browser extension and host companion

### C-01: Создать каркас `web/extensions/chromium-butler/`
- **Приоритет:** P0
- **Зависимости:** A-01, A-02
- **Результат:** baseline extension skeleton
- **Критерии приемки:**
  - Есть manifest с `tabs`, `scripting`, `storage`, `nativeMessaging`.
  - Документирована dev/install flow для Chromium/Chrome/Edge.
  - Extension не публикует multi-tab operations наружу в Butler tool layer.

### C-02: Создать `apps/browser-bridge/` как host companion process
- **Приоритет:** P0
- **Зависимости:** A-01, A-02
- **Результат:** native messaging host baseline
- **Критерии приемки:**
  - Процесс реализован на Go.
  - Есть stdin/stdout native messaging contract с extension.
  - Есть README с install/registration steps для Windows baseline.

### C-03: Реализовать list-tabs flow только для approval path
- **Приоритет:** P0
- **Зависимости:** C-01, C-02, A-03
- **Результат:** restricted tab discovery pipeline
- **Критерии приемки:**
  - Browser bridge умеет получать текущий список вкладок только для подготовки approval.
  - Агент не видит список вкладок.
  - `tab_id` не выходит в модельный контекст.

### C-04: Реализовать bind/release/liveness команды host-side
- **Приоритет:** P0
- **Зависимости:** C-03
- **Результат:** host-side session registry и tab guard
- **Критерии приемки:**
  - Поддержаны внутренние команды минимум: `list_tabs_for_approval`, `bind_to_tab`, `check_bound_tab_alive`, `dispatch_single_tab_action`, `release_single_tab_session`.
  - Host-side registry жёстко привязывает action к bound tab.
  - При закрытии вкладки host отправляет signal о `TAB_CLOSED`.

### C-05: Реализовать content script injection и visible capture
- **Приоритет:** P1
- **Зависимости:** C-04
- **Результат:** browser-side action substrate
- **Критерии приемки:**
  - Поддержаны click/fill/type/scroll/extract/capture within bound tab.
  - Действия выполняются только в bound tab.
  - Ошибки нормализуются для broker/runtime layer.

---

## 7.1 Эпик C2 - Remote Extension Transport (non-localhost mode)

### C2-01: Ввести dual transport model для extension (`native` + `remote`)
- **Приоритет:** P0
- **Зависимости:** C-01, C-02
- **Результат:** единый extension transport abstraction
- **Критерии приемки:**
  - Текущий native messaging путь сохраняется как `native` режим.
  - Добавлен `remote` режим с подключением extension к удаленному Butler API по HTTPS.
  - Переключение режима реализовано как явная конфигурация extension, без неявных fallback.

### C2-02: Добавить extension-auth слой для remote API
- **Приоритет:** P0
- **Зависимости:** C2-01
- **Результат:** отдельная auth boundary для browser extension канала
- **Критерии приемки:**
  - Добавлен отдельный набор API endpoint-ов для extension remote transport.
  - Endpoint-ы требуют machine-to-machine auth (например, bearer token / device token), а не UI cookie-сессию.
  - Auth material хранится в extension storage и никогда не попадает в model-visible tool args.
  - В логах и activity не раскрывается токен.

### C2-03: Реализовать server relay для single-tab action dispatch
- **Приоритет:** P0
- **Зависимости:** C2-02, D-03
- **Результат:** серверный relay между `tool-browser-local` и extension
- **Критерии приемки:**
  - `tool-browser-local` отправляет action dispatch в orchestrator relay endpoint вместо localhost-only bridge.
  - Orchestrator передает action в активное extension-соединение и ждет ответ с bounded timeout.
  - Ошибки relay нормализуются в `HOST_UNAVAILABLE`, `TAB_CLOSED`, `ACTION_NOT_ALLOWED`.
  - Session guard сохраняется: dispatch разрешен только для `ACTIVE` single-tab session.

### C2-04: Реализовать extension connection lifecycle для remote режима
- **Приоритет:** P1
- **Зависимости:** C2-03
- **Результат:** управляемый lifecycle подключения extension к удаленному серверу
- **Критерии приемки:**
  - Есть handshake/registration шага для extension instance и browser identity metadata.
  - Есть heartbeats/liveness и корректный переход в `HOST_DISCONNECTED` при потере канала.
  - Одна browser instance не может выполнять actions для чужого `single_tab_session`.

### C2-05: Ввести rollout-режимы и migration strategy
- **Приоритет:** P1
- **Зависимости:** C2-04
- **Результат:** безопасный переход от localhost-only к dual transport
- **Критерии приемки:**
  - Есть feature flags на orchestrator/tool-browser-local/extension.
  - Есть режимы rollout: `native_only`, `dual`, `remote_preferred`.
  - Документирован rollback к `native_only` без миграции данных.

---

## 8. Эпик D - `single_tab.*` runtime and broker/orchestrator integration

### D-01: Ввести public tool family `single_tab.*`
- **Приоритет:** P0
- **Зависимости:** A-01, A-02
- **Результат:** public contracts для `single_tab.*`
- **Критерии приемки:**
  - Добавлены инструменты минимум: `single_tab.status`, `single_tab.navigate`, `single_tab.reload`, `single_tab.go_back`, `single_tab.go_forward`, `single_tab.click`, `single_tab.fill`, `single_tab.type`, `single_tab.press_keys`, `single_tab.scroll`, `single_tab.wait_for`, `single_tab.extract_text`, `single_tab.capture_visible`, `single_tab.release`.
  - Во всех input schemas присутствует `session_id`.
  - Ни один input schema не принимает `tab_id`, cookies, storage state или `credential_ref`.

### D-02: Реализовать broker-side prereq checks для `single_tab_session`
- **Приоритет:** P0
- **Зависимости:** A-03, D-01
- **Результат:** session-aware Tool Broker policy
- **Критерии приемки:**
  - Broker отклоняет `single_tab.*` без валидного `ACTIVE` session.
  - Нормализованы ошибки минимум: `APPROVAL_REQUIRED`, `SESSION_NOT_FOUND`, `SESSION_NOT_ACTIVE`, `TAB_CLOSED`, `HOST_UNAVAILABLE`, `ACTION_NOT_ALLOWED`.
  - Проверка выполняется до runtime routing.

### D-03: Добавить runtime target для local browser control
- **Приоритет:** P0
- **Зависимости:** C-04, D-01, D-02
- **Результат:** new runtime route для `single_tab.*`
- **Критерии приемки:**
  - Tool Broker маршрутизирует `single_tab.*` в dedicated runtime target.
  - Runtime не содержит multi-tab public surface.
  - Runtime не хранит durable tab session state как source of truth.

### D-04: Реализовать orchestrator bind flow
- **Приоритет:** P0
- **Зависимости:** A-03, D-01, D-02
- **Результат:** orchestrator-managed bind lifecycle
- **Критерии приемки:**
  - `single_tab.bind` инициирует approval flow вместо прямого доступа к browser runtime.
  - После выбора вкладки orchestrator создаёт `single_tab_session`.
  - Approval resume path соответствует текущему run lifecycle.

### D-05: Подмешать `session_id` в tool context без раскрытия `tab_id`
- **Приоритет:** P0
- **Зависимости:** D-04
- **Результат:** safe model-visible tool usage
- **Критерии приемки:**
  - Агент видит только `session_id`.
  - `tab_id` остаётся host/runtime-only internal reference.
  - Tool summary и prompt instructions обновлены под `single_tab.*`.

### D-06: Явно исключить secret-bearing browser operations из single-tab scope
- **Приоритет:** P0
- **Зависимости:** D-01
- **Результат:** закреплённая security boundary
- **Критерии приемки:**
  - В registry нет `single_tab.set_cookie`, `single_tab.restore_storage_state` и аналогов.
  - `single_tab.fill/type` принимают только обычные строки, без typed secret references.
  - Спеки и prompt guidance явно это отражают.

### D-07: Добавить unit/integration tests для broker/runtime/orchestrator path
- **Приоритет:** P0
- **Зависимости:** D-06
- **Результат:** test coverage для single-tab execution path
- **Критерии приемки:**
  - Есть tests на отсутствие bind -> `APPROVAL_REQUIRED`.
  - Есть tests на `TAB_CLOSED`, `REVOKED_BY_USER`, `HOST_DISCONNECTED`.
  - Есть tests, что `tab_id` не попадает в public responses и transcript artifacts.

---

## 9. Эпик E - Web UI and Telegram approval UX

### E-01: Расширить durable approval model для tab selection
- **Приоритет:** P0
- **Зависимости:** A-02, A-03
- **Результат:** typed approval payload support
- **Критерии приемки:**
  - Approval model поддерживает `approval_type = BROWSER_TAB_SELECTION`.
  - Durable payload хранит список кандидатов через `approval_tab_candidates`.
  - Web и Telegram работают с единым approval record.

### E-02: Реализовать Web UI выбора вкладки
- **Приоритет:** P0
- **Зависимости:** E-01, C-03
- **Результат:** web approval card для tab selection
- **Критерии приемки:**
  - Пользователь видит title, domain и дополнительную метаинформацию по вкладке.
  - Выбор одной вкладки переводит approval в terminal state и создаёт `single_tab_session`.
  - Есть `Deny` и `Cancel`.

### E-03: Реализовать Telegram approval для выбора вкладки
- **Приоритет:** P0
- **Зависимости:** E-01, C-03
- **Результат:** Telegram inline keyboard для tab selection
- **Критерии приемки:**
  - Telegram message использует short-lived candidate token, а не `tab_id`.
  - Пользователь может выбрать ровно одну вкладку, отклонить или отменить.
  - После выбора приходит подтверждение со статусом session activation.

### E-04: Привести channel adapters к общей approval payload модели
- **Приоритет:** P1
- **Зависимости:** E-02, E-03
- **Результат:** shared approval rendering and resolution logic
- **Критерии приемки:**
  - Telegram и Web не используют разрозненные ad hoc payloads.
  - Callback/resolve path идемпотентен и устойчив к race conditions.
  - Activity/audit отражают источник подтверждения.

---

## 10. Эпик F - Audit, observability, failure handling

### F-01: Добавить аудит bind lifecycle и single-tab actions
- **Приоритет:** P0
- **Зависимости:** D-04, E-04
- **Результат:** operator-visible audit trail
- **Критерии приемки:**
  - Логируется кто подтвердил bind, какая вкладка была выбрана, когда создана и завершена session.
  - Логируются user-visible `single_tab.*` actions без утечки чувствительных данных.
  - Activity feed и debug refs отражают bind lifecycle.

### F-02: Добавить health and reconnect model для browser bridge
- **Приоритет:** P1
- **Зависимости:** C-02, C-04
- **Результат:** устойчивость host companion path
- **Критерии приемки:**
  - `HOST_DISCONNECTED` корректно детектируется и surfaced вверх.
  - Есть heartbeat/liveness model между runtime и host.
  - Есть ручка или статус для operator diagnostics.

### F-03: Обновить deployment docs, env templates и README
- **Приоритет:** P0
- **Зависимости:** B-06, F-02
- **Результат:** полная документация по запуску и эксплуатации
- **Критерии приемки:**
  - Обновлены `.env.example` и `.env.codex-windows.example`.
  - Обновлены `deploy/docker-compose*.yml`.
  - У новых сервисов есть README.
  - Отдельно описана host-side установка extension/native host.

### F-04: End-to-end acceptance suite
- **Приоритет:** P0
- **Зависимости:** F-03
- **Результат:** e2e verification для ключевых сценариев
- **Критерии приемки:**
  - Проверяется запрет browser action без bind approval.
  - Проверяется bind через Web UI и Telegram.
  - Проверяется работа в одной вкладке после cross-domain navigation.
  - Проверяется корректная ошибка после закрытия вкладки.
  - Проверяется, что `web.fetch` работает при доступном self-hosted provider без SaaS dependency.

---

## 11. Рекомендуемый порядок реализации

1. `A-*` - сначала контракты, storage model и spec updates.
2. `B-*` - затем `web.fetch` как независимый vertical slice.
3. `C-*` - потом extension/native host/browser bridge substrate.
4. `D-*` - затем `single_tab.*` runtime, broker и orchestrator integration.
5. `E-*` - после этого Web UI и Telegram approval UX.
6. `F-*` - в конце audit, observability, docs и end-to-end hardening.

---

## 12. Явно вне scope этой итерации

- Любые multi-tab tools для агента.
- Любые `credential_ref` или secret injection flows в `single_tab.*`.
- Browser window management.
- Автоматический bind без user approval.
- SaaS-only WebFetch SLA baseline.
