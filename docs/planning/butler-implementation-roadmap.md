# Butler Implementation Roadmap

## Planning assumptions

**Текущее состояние:** Sprint 0, Sprint 1 и Sprint 2 фактически реализованы: в репозитории уже есть Go-код, proto-контракты, SQL-миграции, Docker Compose baseline для PostgreSQL/Redis, Session Service, Redis lease model, run persistence, transcript store, ingress normalization, transport/provider loop для OpenAI, synchronous orchestrator ingestion APIs и документированный smoke flow. Sprint 3 остаётся следующим крупным шагом: Telegram adapter, full MVP compose stack, first end-to-end acceptance, Tool Broker skeleton и Working Memory foundation.

**Модель спринтов:**
- 1 спринт = ~2 недели
- Каждый спринт рассчитан на работу одного-двух build-агентов или одного разработчика
- Задачи атомарны и могут выполняться build-агентом с доступом к спецификациям

**Принятые архитектурные решения (из спеков):**
- Go — основной язык, Python — только по исключению
- REST — внешний API, gRPC — межсервисное взаимодействие
- PostgreSQL + pgvector + Redis
- Docker Compose — единственная deployment модель
- Container-per-tool-class (broker, browser, http, doctor)
- WebSocket-first model transport
- Sequential-only tool execution в V1
- Deferred credential resolution через `credential_ref`
- Redis-based queue для async jobs

**Monorepo layout (из technology-stack spec):**
```
apps/           — сервисы (Go)
internal/       — общие Go-пакеты
web/            — Nuxt.js frontend
proto/          — gRPC proto definitions
migrations/     — SQL migrations
docker/         — Dockerfiles
docs/           — архитектурная документация
deploy/         — docker-compose и env templates
```

**Определение MVP cut line:**
Первый рабочий end-to-end сценарий: пользователь отправляет сообщение в Telegram → Butler обрабатывает через Session Service → Orchestrator формирует контекст → Model Transport отправляет в OpenAI → ответ возвращается пользователю → transcript сохраняется в PostgreSQL. Без tool calls, без credential flow, но с работающей инфраструктурой.

**Критический путь:**
```
repo skeleton → shared libs (logging, config, DB) → postgres schema → session service → run lifecycle state machine → model transport (OpenAI) → orchestrator (basic) → telegram adapter → docker-compose → [MVP vertical slice]
```

---

## Sprint 0 — Foundation: Repo skeleton, shared libs, infrastructure contracts

Цель: создать рабочий monorepo с Go modules, shared-пакетами, базовыми proto-определениями и docker-compose baseline. После Sprint 0 можно параллельно разрабатывать сервисы.

---

### S0-01: Repository skeleton and Go module init

- **ID:** S0-01
- **Title:** Repository skeleton and Go module init
- **Subsystem:** repo / infrastructure
- **Why now:** без каркаса monorepo невозможно начать ни один сервис; все последующие задачи зависят от этого
- **Dependencies:** нет
- **Acceptance criteria:**
  - `go.mod` в корне с module path `github.com/<owner>/butler`
  - Директории `apps/`, `internal/`, `web/`, `proto/`, `migrations/`, `docker/`, `deploy/`, `docs/`, `.opencode/` существуют
  - Stub `apps/orchestrator/`, `apps/tool-broker/`, `apps/tool-browser/`, `apps/tool-http/`, `apps/tool-doctor/` с пустыми `main.go`
  - `.gitignore` обновлён для Go, Node, Docker artifacts
  - Базовый `Makefile` или `Taskfile` с target `build`, `test`, `lint`, `proto`
  - `README.md` обновлён, отражает структуру

---

### S0-02: Structured logging package

- **ID:** S0-02
- **Title:** Structured logging package
- **Subsystem:** internal / observability
- **Why now:** каждый сервис с первой строки кода должен использовать единый structured logging; иначе начнётся drift
- **Dependencies:** S0-01
- **Acceptance criteria:**
  - Пакет `internal/log` или `internal/logger`
  - Использует `log/slog` (Go 1.21+ standard library) — **без внешних зависимостей**
  - JSON output по умолчанию
  - Поддержка полей: `service`, `component`, `run_id`, `tool_call_id`, `level`, `message`, `timestamp`
  - Helper для создания child logger с pre-bound fields
  - Secret masking helper: `MaskSecret(value string) string`
  - Unit tests для formatting, field injection и masking

---

### S0-03: Configuration loading package

- **ID:** S0-03
- **Title:** Configuration loading package
- **Subsystem:** internal / config
- **Why now:** все сервисы нуждаются в config loading; doctor spec требует configuration introspection layer с самого начала
- **Dependencies:** S0-01
- **Acceptance criteria:**
  - Пакет `internal/config`
  - Загрузка из environment variables (Go `os.Getenv`, без внешних библиотек)
  - Typed config structs per service (base struct + service-specific extensions)
  - Validation функция: required fields, type checking, допустимые значения
  - Поддержка `is_secret` маркировки для config keys
  - Introspection API (Go interface): `ListKeys() []ConfigKeyInfo` — возвращает key, component, type, required, default, effective_value, source, is_secret, requires_restart, validation_status, validation_error (per doctor spec section 16.4)
  - Unit tests для loading, validation и introspection

---

### S0-04: PostgreSQL connection package

- **ID:** S0-04
- **Title:** PostgreSQL connection package
- **Subsystem:** internal / storage
- **Why now:** Session Service, Transcript Store, Memory — всё зависит от PostgreSQL; нужен единый connection pool package
- **Dependencies:** S0-01, S0-02, S0-03
- **Acceptance criteria:**
  - Пакет `internal/storage/postgres`
  - Использует `pgx/v5` (единственная допустимая PostgreSQL-зависимость, per tech spec)
  - Connection pool creation из config
  - Health check функция
  - Migration runner integration (accept migration directory path)
  - Structured logging при connect/disconnect/error
  - Integration test: connect to PostgreSQL, run health check, close

---

### S0-05: Redis connection package

- **ID:** S0-05
- **Title:** Redis connection package
- **Subsystem:** internal / storage
- **Why now:** Session leases, transient working memory, async job queue — всё зависит от Redis
- **Dependencies:** S0-01, S0-02, S0-03
- **Acceptance criteria:**
  - Пакет `internal/storage/redis`
  - Минимальный зрелый Redis client (один, per tech spec)
  - Connection pool creation из config
  - Health check функция
  - Structured logging
  - Integration test: connect, SET/GET, health check, close

---

### S0-06: Core domain types package

- **ID:** S0-06
- **Title:** Core domain types package
- **Subsystem:** internal / domain
- **Why now:** типы `RunID`, `SessionKey`, `ToolCallID`, `EventID`, run states, error classes используются повсюду; определить их нужно один раз до начала разработки сервисов
- **Dependencies:** S0-01
- **Acceptance criteria:**
  - Пакет `internal/domain`
  - Value types: `RunID`, `SessionKey`, `ToolCallID`, `EventID`, `LeaseID`, `CredentialAlias`
  - Run states enum: `created`, `queued`, `acquired`, `preparing`, `model_running`, `tool_pending`, `awaiting_approval`, `tool_running`, `awaiting_model_resume`, `finalizing`, `completed`, `failed`, `cancelled`, `timed_out` (per run-lifecycle-spec section 7.1)
  - Run state transition validation function (per run-lifecycle-spec section 8)
  - Terminal error classes enum: `validation_error`, `transport_error`, `tool_error`, `policy_denied`, `credential_error`, `approval_error`, `timeout`, `cancelled`, `internal_error` (per run-lifecycle-spec section 19)
  - Input event types enum (per run-lifecycle-spec section 6)
  - Autonomy mode enum: Mode 0–3 (per PRD section 18)
  - Unit tests для state transitions (valid + invalid)

---

### S0-07: gRPC proto definitions — session and run contracts

- **ID:** S0-07
- **Title:** gRPC proto definitions — session and run contracts
- **Subsystem:** proto / contracts
- **Why now:** gRPC — обязательный внутренний transport; proto-определения нужно иметь до начала разработки session service и orchestrator
- **Dependencies:** S0-01, S0-06
- **Acceptance criteria:**
  - `proto/session/v1/session.proto`: `CreateSession`, `GetSession`, `ResolveSessionKey`, `AcquireLease`, `ReleaseLease`, `CreateRun`, `UpdateRunState`, `GetRun`
  - `proto/run/v1/events.proto`: InputEvent message definition с fields per run-lifecycle-spec section 6
  - `proto/common/v1/types.proto`: shared enums (RunState, AutonomyMode, ErrorClass)
  - Go code generation работает через Makefile target `proto`
  - Generated code в `internal/gen/` или аналогичной директории

---

### S0-08: gRPC proto definitions — tool broker contract

- **ID:** S0-08
- **Title:** gRPC proto definitions — tool broker contract
- **Subsystem:** proto / contracts
- **Why now:** Tool Broker gRPC контракт нужен до начала разработки broker и runtimes
- **Dependencies:** S0-01, S0-06
- **Acceptance criteria:**
  - `proto/toolbroker/v1/broker.proto`: `ValidateToolCall`, `ExecuteToolCall`, `ListTools`, `GetToolContract`
  - `proto/toolbroker/v1/types.proto`: ToolCall, ToolResult, ToolContract, CredentialRef, ToolError messages (per tool-runtime-adr sections 7, 10, 11)
  - Go code generation работает

---

### S0-09: gRPC proto definitions — model transport contract

- **ID:** S0-09
- **Title:** gRPC proto definitions — model transport contract
- **Subsystem:** proto / contracts
- **Why now:** Model Transport Layer gRPC контракт нужен до начала разработки transport
- **Dependencies:** S0-01, S0-06
- **Acceptance criteria:**
  - `proto/transport/v1/transport.proto`: `StartRun`, `ContinueRun`, `SubmitToolResult`, `CancelRun` RPCs + `StreamEvents` server-streaming RPC
  - Messages: `StartRunRequest`, `ContinueRunRequest`, `SubmitToolResultRequest`, `CancelRunRequest`, `TransportEvent` (per model-transport-contract sections 10, 11)
  - Transport event types enum: `run_started`, `provider_session_bound`, `assistant_delta`, `assistant_final`, `tool_call_requested`, `tool_call_batch_requested`, `transport_warning`, `transport_error`, `run_cancelled`, `run_timed_out`, `run_completed`
  - Go code generation работает

---

### S0-10: Docker Compose baseline

- **ID:** S0-10
- **Title:** Docker Compose baseline
- **Subsystem:** deployment / infrastructure
- **Why now:** без docker-compose невозможно запускать PostgreSQL, Redis и интеграционные тесты; это нужно с первого дня
- **Dependencies:** S0-01
- **Acceptance criteria:**
  - `deploy/docker-compose.yml` с сервисами: `postgres` (с pgvector extension), `redis`
  - `deploy/docker-compose.dev.yml` override для dev: exposed ports, volumes для persistence
  - `.env.example` с документированными переменными
  - PostgreSQL healthcheck
  - Redis healthcheck
  - `Makefile` target `infra-up` и `infra-down`
  - После `make infra-up` оба сервиса доступны и проходят health check

---

### S0-11: SQL migrations baseline — sessions, runs, transcript

- **ID:** S0-11
- **Title:** SQL migrations baseline — sessions, runs, transcript
- **Subsystem:** storage / schema
- **Why now:** Session Service и Transcript Store — первые сервисы, которые будут разрабатываться; им нужна схема
- **Dependencies:** S0-04, S0-10
- **Acceptance criteria:**
  - Migration tool: используется `golang-migrate` или standalone SQL runner (минимальный)
  - `migrations/001_sessions.up.sql`: таблица `sessions` (session_key, user_id, channel, created_at, updated_at, metadata JSONB)
  - `migrations/002_runs.up.sql`: таблица `runs` с полями per run-lifecycle-spec section 23 (run_id, session_key, input_event_id, status, autonomy_mode, current_state, model_provider, provider_session_ref, lease_id, resumes_run_id, started_at, updated_at, finished_at, error_type, error_message)
  - `migrations/003_messages.up.sql`: таблица `messages` (message_id, session_key, run_id, role, content, tool_call_id, created_at, metadata JSONB)
  - `migrations/004_tool_calls.up.sql`: таблица `tool_calls` (tool_call_id, run_id, tool_name, args JSONB, status, runtime_target, started_at, finished_at, result JSONB, error JSONB)
  - `migrations/005_enable_pgvector.up.sql`: `CREATE EXTENSION IF NOT EXISTS vector`
  - Все миграции накатываются на чистую БД без ошибок
  - Makefile target `migrate-up` и `migrate-down`

---

### S0-12: Basic metrics package

- **ID:** S0-12
- **Title:** Basic metrics package
- **Subsystem:** internal / observability
- **Why now:** tech spec требует минимальных метрик (request counts, error counts, latency); задать паттерн нужно сразу
- **Dependencies:** S0-01
- **Acceptance criteria:**
  - Пакет `internal/metrics`
  - Prometheus-compatible HTTP endpoint `/metrics`
  - Helpers: `IncrCounter(name, labels)`, `ObserveHistogram(name, value, labels)`, `SetGauge(name, value, labels)`
  - Предопределённые метрики: `butler_requests_total`, `butler_errors_total`, `butler_request_duration_seconds`
  - Используется Go prometheus client (стандартная, минимальная библиотека)
  - Unit test: increment counter, check output

---

## Sprint 1 — Session Service, Run State Machine, Transcript Store

Цель: реализовать Session Service и Run State Machine — сердцевину Butler, через которую проходит каждый запрос. После Sprint 1 можно запускать run, проводить его через state transitions и записывать transcript.

---

### S1-01: Session Service — core implementation

- **ID:** S1-01
- **Title:** Session Service — core implementation
- **Subsystem:** Session Service
- **Why now:** Session Service — entry point для каждого входящего события; без него невозможен ни один run. **Критично для первого end-to-end.**
- **Dependencies:** S0-02, S0-03, S0-04, S0-05, S0-06, S0-07, S0-11
- **Acceptance criteria:**
  - `apps/orchestrator/internal/session/` (Session Service живёт внутри orchestrator process, per tech spec section 4.3 — не отдельный binary)
  - Реализация gRPC API: `CreateSession`, `GetSession`, `ResolveSessionKey`
  - Session lookup by session_key
  - Session creation with idempotency (per session_key)
  - PostgreSQL persistence для sessions
  - Structured logging для всех операций
  - Unit tests для session creation, lookup, idempotency
  - Integration test: create session → get session → verify in PostgreSQL

---

### S1-02: Lease model implementation

- **ID:** S1-02
- **Title:** Lease model implementation
- **Subsystem:** Session Service
- **Why now:** lease model гарантирует single active owner per session; без него невозможно безопасно запускать runs. **Критично для первого end-to-end.**
- **Dependencies:** S1-01, S0-05
- **Acceptance criteria:**
  - Redis-based lease implementation
  - `AcquireLease(session_key, run_id, owner_id, ttl) -> LeaseID | error`
  - `ReleaseLease(lease_id)`
  - `RenewLease(lease_id, ttl)`
  - TTL-based expiry (configurable, default 60s)
  - Lease conflict detection (already held by another run)
  - Unit tests для acquire, release, conflict, expiry
  - Integration test с Redis

---

### S1-03: Run state machine implementation

- **ID:** S1-03
- **Title:** Run state machine implementation
- **Subsystem:** Orchestrator / Run lifecycle
- **Why now:** run state machine — ядро execution flow; без неё невозможен orchestrator. **Критично для первого end-to-end.**
- **Dependencies:** S0-06, S0-11, S0-04, S1-01
- **Acceptance criteria:**
  - `internal/run/statemachine.go`: state machine с transitions per run-lifecycle-spec section 8
  - `CreateRun(session_key, input_event) -> Run`
  - `TransitionRun(run_id, from_state, to_state) -> error` с validation
  - PostgreSQL persistence: run record create + update
  - State transition event logging (structured log entry per transition)
  - Run record содержит все поля per run-lifecycle-spec section 23
  - Unit tests: all valid transitions, all invalid transitions rejected, terminal states are final
  - Integration test: create run → walk through full happy path state sequence → verify DB

---

### S1-04: Transcript Store — basic implementation

- **ID:** S1-04
- **Title:** Transcript Store — basic implementation
- **Subsystem:** Memory / Transcript
- **Why now:** transcript persistence — обязательная часть `finalizing` state; без неё невозможно завершить run. **Критично для первого end-to-end.**
- **Dependencies:** S0-04, S0-11, S0-06
- **Acceptance criteria:**
  - `internal/memory/transcript/store.go`
  - `AppendMessage(session_key, run_id, role, content, metadata) -> MessageID`
  - `AppendToolCall(run_id, tool_call) -> ToolCallID`
  - `GetTranscript(session_key, limit, offset) -> []Message`
  - `GetRunTranscript(run_id) -> []Message`
  - PostgreSQL persistence в таблицы `messages` и `tool_calls`
  - Append-only модель (no updates/deletes)
  - Unit tests + integration test

---

### S1-05: Input event normalization

- **ID:** S1-05
- **Title:** Input event normalization
- **Subsystem:** Ingress / Session Service
- **Why now:** все channels (Telegram, Web UI) должны нормализовать входящие события в единый формат InputEvent; нужно определить этот слой до реализации channel adapters
- **Dependencies:** S0-06, S0-07
- **Acceptance criteria:**
  - `internal/ingress/event.go`: InputEvent struct (per run-lifecycle-spec section 6: event_id, event_type, session_key, source, payload, created_at, idempotency_key)
  - `NormalizeEvent(source, rawPayload) -> InputEvent`
  - Idempotency key generation
  - Input deduplication check через Session Service (per run-lifecycle-spec section 20.1)
  - Unit tests

---

## Sprint 2 — Model Transport Layer (OpenAI), Basic Orchestrator

Цель: запустить модельный цикл — отправить context в OpenAI, получить ответ. После Sprint 2 можно пройти путь от InputEvent до assistant response.

---

### S2-01: Model Transport Layer — provider interface and types

- **ID:** S2-01
- **Title:** Model Transport Layer — provider interface and types
- **Subsystem:** Model Transport
- **Why now:** transport interface — основа для всех provider implementations; нужно закрепить контракт в Go коде до начала OpenAI реализации. **Критично для первого end-to-end.**
- **Dependencies:** S0-06, S0-09
- **Acceptance criteria:**
  - `internal/transport/provider.go`: Go interface `ModelProvider` с методами `StartRun`, `ContinueRun`, `SubmitToolResult`, `CancelRun` (per model-transport-contract section 7)
  - `internal/transport/types.go`: Go structs for TransportRunContext, ProviderSessionRef, TransportCommand, TransportEvent (per model-transport-contract sections 8–11)
  - `internal/transport/events.go`: все transport event types as Go constants
  - `internal/transport/errors.go`: normalized error types (per model-transport-contract section 16)
  - `internal/transport/capabilities.go`: CapabilitySnapshot struct (per model-transport-contract section 17)
  - Unit tests для event creation и error normalization

---

### S2-02: Model Transport Layer — OpenAI provider with WebSocket-first contract

- **ID:** S2-02
- **Title:** Model Transport Layer — OpenAI provider with WebSocket-first contract
- **Subsystem:** Model Transport
- **Why now:** OpenAI — primary cloud provider для V1; без рабочего transport невозможен end-to-end. **Критично для первого end-to-end.**
- **Dependencies:** S2-01, S0-02, S0-03
- **Acceptance criteria:**
  - `internal/transport/openai/provider.go`: реализация `ModelProvider` interface
  - Использует WebSocket-first transport там, где OpenAI API это поддерживает, с HTTP streaming fallback без изменения logical contract
  - `StartRun`: sends prepared `input_items` и `tools` в OpenAI transport backend
  - Streaming parsing: assistant deltas, tool call requests, final response
  - `SubmitToolResult`: sends tool output back через тот же normalized transport contract
  - `CancelRun`: best-effort cancel
  - CapabilitySnapshot: `supports_streaming=true`, `supports_tool_calls=true`, `supports_stateful_sessions=true`
  - Provider session ref tracking (response ID for stateful continuation)
  - Normalized error mapping: rate limit, auth error, timeout → transport error classes
  - Structured logging для всех API calls
  - Config: API key (через config, marked as secret), model name, base URL, timeouts
  - Unit tests с mock HTTP server для streaming responses
  - Integration test (opt-in, requires API key): send simple message, receive response

---

### S2-03: Orchestrator — basic run execution flow

- **ID:** S2-03
- **Title:** Orchestrator — basic run execution flow
- **Subsystem:** Orchestrator
- **Why now:** orchestrator — это ядро, которое связывает Session Service, Transport и Transcript Store. **Критично для первого end-to-end.**
- **Dependencies:** S1-01, S1-02, S1-03, S1-04, S2-01, S2-02
- **Acceptance criteria:**
  - `apps/orchestrator/internal/orchestrator/run.go`
  - Принимает InputEvent
  - Session resolution → lease acquisition → run creation (`created` → `queued` → `acquired`)
  - Context preparation (`preparing`): собирает input event payload + пустой memory bundle (заглушка)
  - Model interaction (`model_running`): вызывает transport `StartRun`
  - Processes streaming events: passes assistant_delta events (для будущей channel delivery)
  - On `assistant_final`: переход в `finalizing`
  - Finalizing: transcript persistence (user message + assistant response), run state → `completed`, lease release
  - Error handling: transport error → run `failed`, structured log
  - **НЕ реализует**: tool call loop, approval, memory retrieval, credential context — это Sprint 3+
  - Unit tests с mock transport
  - Integration test: full flow from InputEvent to completed run in DB

---

### S2-04: Orchestrator — gRPC API server and service wiring

- **ID:** S2-04
- **Title:** Orchestrator — gRPC API server and service wiring
- **Subsystem:** Orchestrator
- **Why now:** orchestrator должен быть запускаемым gRPC+REST сервером, чтобы channel adapters могли к нему обращаться
- **Dependencies:** S2-03, S0-03, S0-12
- **Acceptance criteria:**
  - `apps/orchestrator/main.go`: запуск gRPC server + HTTP server (REST API + metrics + health check)
  - gRPC service: `SubmitEvent(InputEvent) -> RunResponse`
  - REST endpoint: `POST /api/v1/events` → submits InputEvent
  - Health check: `GET /health` → checks PostgreSQL, Redis connectivity
  - Metrics endpoint: `GET /metrics`
  - Graceful shutdown
  - Dockerfile: `docker/orchestrator/Dockerfile` — multistage Go build
  - Structured logging на старте: service name, config summary (masked secrets)

---

## Sprint 3 — Telegram Adapter, Tool Broker Skeleton, First E2E

Цель: подключить Telegram как input/output channel и запустить первый end-to-end сценарий. Параллельно создать скелет Tool Broker.

---

### S3-01: Telegram Adapter — basic input/output

- **ID:** S3-01
- **Title:** Telegram Adapter — basic input/output
- **Subsystem:** Channel Adapter / Telegram
- **Why now:** Telegram — primary user channel. Без него нет взаимодействия с реальным пользователем. **Критично для первого end-to-end.**
- **Dependencies:** S2-04, S0-02, S0-03
- **Acceptance criteria:**
  - `apps/orchestrator/internal/channel/telegram/adapter.go` (адаптер живёт в orchestrator process)
  - Telegram Bot API integration (long polling для V1, webhook — позже)
  - Получает текстовые сообщения от пользователя
  - Формирует InputEvent (event_type=user_message, session_key derived from chat_id)
  - Отправляет InputEvent в orchestrator
  - Получает final response и отправляет обратно в Telegram
  - Конфигурация: bot token (secret), allowed_chat_ids
  - **НЕ реализует**: streaming delivery, inline keyboards, approval UX — это позже
  - Structured logging
  - Минимальная Telegram Go библиотека (одна, минимальная)
  - Unit tests для event normalization
  - Manual test: send message in Telegram → get response

---

### S3-02: Docker Compose — full MVP stack

- **ID:** S3-02
- **Title:** Docker Compose — full MVP stack
- **Subsystem:** deployment
- **Why now:** нужно иметь возможность запустить весь стек одной командой для первого e2e сценария. **Критично для первого end-to-end.**
- **Dependencies:** S2-04, S3-01, S0-10
- **Acceptance criteria:**
  - `deploy/docker-compose.yml` обновлён: `postgres`, `redis`, `orchestrator`
  - Orchestrator container depends_on postgres и redis с healthchecks
  - Миграции запускаются автоматически при старте (или отдельным init container)
  - `.env.example` содержит все необходимые переменные: DB URL, Redis URL, OpenAI API key, Telegram bot token
  - `make up` → весь стек поднимается и работает
  - `make down` → всё останавливается
  - `make logs` → unified log stream

---

### S3-03: First end-to-end acceptance test

- **ID:** S3-03
- **Title:** First end-to-end acceptance test
- **Subsystem:** e2e / verification
- **Why now:** верифицировать, что вертикальный срез действительно работает
- **Dependencies:** S3-01, S3-02
- **Acceptance criteria:**
  - Документированный manual test scenario: отправить сообщение в Telegram → получить ответ от OpenAI через Butler → проверить run в БД (`completed` state), transcript содержит user message и assistant response
  - Automated smoke test (опционально): HTTP POST to `/api/v1/events` с test InputEvent → verify run completes → verify transcript exists
  - Всё работает из docker-compose

---

### S3-04: Tool Broker — skeleton with Tool Registry

- **ID:** S3-04
- **Title:** Tool Broker — skeleton with Tool Registry
- **Subsystem:** Tool Broker
- **Why now:** Tool Broker — обязательный компонент; нужно иметь скелет до начала реализации runtimes. Не блокирует MVP, но блокирует Sprint 4.
- **Dependencies:** S0-06, S0-08, S0-02, S0-03
- **Acceptance criteria:**
  - `apps/tool-broker/main.go`: gRPC server
  - `apps/tool-broker/internal/registry/registry.go`: in-memory Tool Registry, loaded from config/static definition
  - gRPC: `ListTools` → returns registered tool contracts, `GetToolContract(tool_name)` → returns contract
  - gRPC: `ValidateToolCall(tool_call)` → schema validation against registered contract
  - gRPC: `ExecuteToolCall(tool_call)` → stub: returns "not implemented" error (routing not wired yet)
  - Tool contracts loaded from YAML/JSON config file (per tool-runtime-adr section 18)
  - Dockerfile: `docker/tool-broker/Dockerfile`
  - Health check endpoint
  - Structured logging
  - Unit tests: registry lookup, schema validation (valid + invalid args)

---

### S3-05: Working Memory — durable layer basic implementation

- **ID:** S3-05
- **Title:** Working Memory — durable layer basic implementation
- **Subsystem:** Memory / Working Memory
- **Why now:** Working Memory нужна для context preparation в orchestrator; дальше Sprint 4 расширит её использование
- **Dependencies:** S0-04, S0-11
- **Acceptance criteria:**
  - `migrations/008_memory_working.up.sql`: таблица `memory_working` (id, session_key, run_id, goal, entities JSONB, pending_steps JSONB, scratch JSONB, created_at, updated_at, status)
  - `internal/memory/working/store.go`: `Save(session_key, snapshot)`, `Get(session_key) -> WorkingMemorySnapshot`, `Clear(session_key)`
  - PostgreSQL persistence (durable layer per memory-model section 6.2)
  - Unit tests + integration test

---

## MVP cut line

**После Sprint 3 имеем:**
- Рабочий vertical slice: Telegram → Session → Orchestrator → OpenAI → Telegram response
- PostgreSQL persistence для sessions, runs, transcript
- Redis leases
- Run state machine с полной state transition validation
- Structured logging во всех компонентах
- Docker Compose для запуска одной командой
- Tool Broker skeleton с registry и validation
- Working Memory foundation
- Базовые метрики

**Критично для первого рабочего end-to-end сценария:**
- S1-01
- S1-02
- S1-03
- S1-04
- S2-01
- S2-02
- S2-03
- S2-04
- S3-01
- S3-02
- S3-03

**Чего ещё нет:**
- Tool call execution loop в orchestrator
- Browser / HTTP / Doctor runtimes
- Credential flow
- Approval UX
- Memory retrieval и memory pipeline
- Web UI
- Episodic / Profile memory
- WebSocket-first transport with approved fallback path where needed during staged provider implementation

---

## Sprint 4 — Tool Execution Loop, Browser & HTTP Runtime Skeletons

Цель: добавить в orchestrator tool call loop и подключить первые tool runtimes.

---

### S4-01: Orchestrator — tool call loop integration

- **ID:** S4-01
- **Title:** Orchestrator — tool call loop integration
- **Subsystem:** Orchestrator
- **Why now:** без tool loop агент не может использовать инструменты — ключевая capability Butler
- **Dependencies:** S2-03, S3-04
- **Acceptance criteria:**
  - Orchestrator обрабатывает `tool_call_requested` events от transport
  - Transition: `model_running` → `tool_pending` → `tool_running` → `awaiting_model_resume` → `model_running`
  - Sends tool call to Tool Broker via gRPC `ExecuteToolCall`
  - Receives tool result → calls transport `SubmitToolResult`
  - Supports multi-turn: model can request multiple sequential tool calls
  - Sequential-only execution (per run-lifecycle-spec section 11.3): serialize batch tool calls
  - Tool error → return as observable tool result to model (per run-lifecycle-spec section 12.5)
  - Transcript: tool_call и tool_result записываются
  - Timeout handling: per-tool-call timeout config
  - Unit tests с mock broker and transport
  - Integration test: full flow with mock tool that returns canned result

---

### S4-02: Tool Broker — runtime routing implementation

- **ID:** S4-02
- **Title:** Tool Broker — runtime routing implementation
- **Subsystem:** Tool Broker
- **Why now:** broker должен уметь маршрутизировать tool calls к runtime containers через gRPC
- **Dependencies:** S3-04
- **Acceptance criteria:**
  - Tool Registry содержит `runtime_target` per tool contract (gRPC address)
  - `ExecuteToolCall` → schema validation → policy check (autonomy mode, basic allow/deny) → route to runtime gRPC → normalize result → return
  - Timeout enforcement: configurable per tool class
  - Audit logging: log tool_call_id, tool_name, runtime_target, status, duration_ms
  - Error normalization: runtime errors → ToolError (per tool-runtime-adr section 22)
  - Unit tests with mock runtime
  - Integration test: broker → mock runtime → normalized result

---

### S4-03: gRPC proto definitions — tool runtime contract

- **ID:** S4-03
- **Title:** gRPC proto definitions — tool runtime contract
- **Subsystem:** proto / contracts
- **Why now:** нужен единый контракт между Tool Broker и runtime containers
- **Dependencies:** S0-08
- **Acceptance criteria:**
  - `proto/toolruntime/v1/runtime.proto`: `ExecuteTool(ExecuteToolRequest) -> ExecuteToolResponse`
  - `ExecuteToolRequest`: tool_name, args (JSON), execution_context (run_id, session_key), resolved_credentials (if any)
  - `ExecuteToolResponse`: status, result (JSON), error, duration_ms
  - Go code generation

---

### S4-04: Browser Runtime — skeleton with `browser.navigate` and `browser.snapshot`

- **ID:** S4-04
- **Title:** Browser Runtime — skeleton with `browser.navigate` and `browser.snapshot`
- **Subsystem:** Tool Runtime / Browser
- **Why now:** browser automation — ключевой tool class V1; нужно proof-of-concept минимум с двумя operations
- **Dependencies:** S4-03, S0-02, S0-03
- **Acceptance criteria:**
  - `apps/tool-browser/main.go`: gRPC server implementing `ExecuteTool`
  - Playwright-based browser backend (Go → Playwright via playwright-go or CDP)
  - Implements `browser.navigate(url)` → opens page, returns status + title
  - Implements `browser.snapshot()` → returns accessibility tree or text content of current page
  - Dockerfile: `docker/tool-browser/Dockerfile` с Playwright/Chromium dependencies
  - Result normalization: safe output, no raw HTML dumps
  - Structured logging
  - Health check
  - Integration test: navigate to example.com → snapshot → verify content

---

### S4-05: HTTP Runtime — skeleton with `http.request`

- **ID:** S4-05
- **Title:** HTTP Runtime — skeleton with `http.request`
- **Subsystem:** Tool Runtime / HTTP
- **Why now:** HTTP tool — второй обязательный tool class V1; нужен параллельно с browser
- **Dependencies:** S4-03, S0-02, S0-03
- **Acceptance criteria:**
  - `apps/tool-http/main.go`: gRPC server implementing `ExecuteTool`
  - Implements `http.request(method, url, headers, body)` → makes HTTP request, returns normalized response
  - Response normalization: status, headers (filtered), body (truncated if large), content_type
  - Domain allowlist enforcement (configurable)
  - Method policy (configurable: which methods allowed)
  - Payload size limits
  - Uses standard `net/http` client
  - Dockerfile: `docker/tool-http/Dockerfile`
  - Structured logging
  - Health check
  - Unit tests + integration test: request to httpbin or mock

---

### S4-06: Docker Compose update — broker and runtimes

- **ID:** S4-06
- **Title:** Docker Compose update — broker and runtimes
- **Subsystem:** deployment
- **Why now:** tool broker и runtimes должны быть в compose stack
- **Dependencies:** S4-02, S4-04, S4-05
- **Acceptance criteria:**
  - `deploy/docker-compose.yml`: добавлены `tool-broker`, `tool-browser`, `tool-http`
  - `tool-broker` depends_on: `tool-browser`, `tool-http` (healthchecks)
  - `orchestrator` depends_on: `tool-broker`
  - Network isolation: tool runtimes не имеют прямого доступа к postgres/redis (только broker имеет)
  - Все контейнеры стартуют и проходят health checks

---

## Sprint 5 — Credential Foundation, Doctor Skeleton, Memory Retrieval

Цель: заложить credential flow, добавить doctor runtime и начать использовать memory для context preparation.

---

### S5-01: Credential metadata store and Credential Broker — foundation

- **ID:** S5-01
- **Title:** Credential metadata store and Credential Broker — foundation
- **Subsystem:** Credentials
- **Why now:** credential-aware tool execution — обязательное требование V1; нужна foundation до реализации `credential_ref` в browser/http tools
- **Dependencies:** S0-04, S0-06
- **Acceptance criteria:**
  - `migrations/011_credentials.up.sql`: таблица `credential_records` (id, alias, secret_type, target_type, allowed_domains, allowed_tools, approval_policy, secret_ref, status, created_at, updated_at) + `credential_audit_logs` (run_id, tool_call_id, alias, field, tool_name, target_domain, decision, timestamp)
  - `internal/credential/store.go`: CRUD для credential records (без secret material)
  - `internal/credential/broker.go`: `Resolve(alias, field, context) -> resolved_value | policy_denied | not_found`
  - Policy checks: allowed_domains, allowed_tools, autonomy_mode (per credential-management sections 6.5, 9)
  - Audit logging per resolution attempt
  - Secret Store: V1 = PostgreSQL encrypted column (simple; per credential-management section 14.2 — финальный выбор отложен, начинаем с простого)
  - Unit tests: policy enforcement (allowed/denied scenarios), audit logging

---

### S5-02: Credential ref resolution in Tool Broker

- **ID:** S5-02
- **Title:** Credential ref resolution in Tool Broker
- **Subsystem:** Tool Broker / Credentials
- **Why now:** Tool Broker должен медиировать credential refs; без этого browser.fill и http.request с auth не работают
- **Dependencies:** S5-01, S4-02
- **Acceptance criteria:**
  - Tool Broker при `ExecuteToolCall`: если args содержат `credential_ref` objects → route через Credential Broker
  - Resolved values передаются в runtime через `resolved_credentials` field (per proto S4-03)
  - Если policy denied → return `credential_error` без выполнения tool
  - Audit: credential usage logged в credential_audit_logs
  - Agent never sees resolved value (value не возвращается в tool result)
  - Unit tests с mock Credential Broker

---

### S5-03: Doctor Runtime — skeleton with basic checks

- **ID:** S5-03
- **Title:** Doctor Runtime — skeleton with basic checks
- **Subsystem:** Tool Runtime / Doctor
- **Why now:** self-inspection — уникальный дифференциатор Butler; нужен хотя бы скелет до Web UI
- **Dependencies:** S4-03, S0-03
- **Acceptance criteria:**
  - `apps/tool-doctor/main.go`: gRPC server implementing `ExecuteTool`
  - `doctor.check_system`: checks PostgreSQL connectivity, Redis connectivity, config validation summary
  - `doctor.check_container`: lists expected containers and their health status (via Docker API socket or config)
  - `doctor.check_provider`: tests OpenAI API key validity (sends minimal request)
  - Uses configuration introspection interface from S0-03
  - Report format: list of checks, each with status (ok/warning/error), message, recommendation
  - Doctor не видит raw secrets (uses masked values from config introspection)
  - Dockerfile: `docker/tool-doctor/Dockerfile`
  - Structured logging
  - Unit tests + integration test

---

### S5-04: Memory retrieval — episodic and profile store basics

- **ID:** S5-04
- **Title:** Memory retrieval — episodic and profile store basics
- **Subsystem:** Memory
- **Why now:** orchestrator context preparation нуждается в memory bundle; пора заложить episodic и profile storage
- **Dependencies:** S0-04, S0-11
- **Acceptance criteria:**
  - `migrations/009_memory_episodic.up.sql`: таблица `memory_episodes` (per memory-model section 17.3: id, memory_type, scope_type, scope_id, summary, content, source_type, source_id, episode_start_at, episode_end_at, embedding vector(1536), tags, created_at, updated_at, confidence, status)
  - `migrations/010_memory_profile.up.sql`: таблица `memory_profile` (per memory-model section 17.2: id, key, value, scope_type, scope_id, summary, source_type, source_id, effective_from, effective_to, supersedes_id, created_at, updated_at, confidence, status)
  - `internal/memory/episodic/store.go`: `Save`, `GetByScope`, `SearchByEmbedding(vector, limit)` (pgvector similarity search)
  - `internal/memory/profile/store.go`: `Save`, `Get(key)`, `GetByScope`, `Update`, `Supersede`
  - Integration tests с pgvector similarity search

---

### S5-05: Orchestrator — memory-aware context preparation

- **ID:** S5-05
- **Title:** Orchestrator — memory-aware context preparation
- **Subsystem:** Orchestrator / Memory
- **Why now:** orchestrator должен собирать memory bundle per run-lifecycle-spec section 10
- **Dependencies:** S5-04, S3-05, S1-04
- **Acceptance criteria:**
  - Context preparation (`preparing` state) собирает: input event, session metadata, working memory snapshot, relevant profile entries, relevant episodic memories (top-N by embedding similarity)
  - Memory bundle формируется как structured data, не raw text dump
  - Configurable: max episodic memories count, max profile entries
  - If memory stores empty (new system) — gracefully proceeds with minimal context
  - Unit tests with mock memory stores

---

### S5-06: Docker Compose update — doctor runtime

- **ID:** S5-06
- **Title:** Docker Compose update — doctor runtime
- **Subsystem:** deployment
- **Why now:** doctor needs to be in the stack
- **Dependencies:** S5-03
- **Acceptance criteria:**
  - `deploy/docker-compose.yml`: добавлен `tool-doctor`
  - Doctor container has access to Docker socket (read-only, for container checks)
  - tool-broker registry updated with doctor tool contracts

---

## Sprint 6 — Telegram Streaming, Approval UX, Web UI Shell

Цель: улучшить UX: streaming ответов в Telegram, approval flow, начало Web UI.

---

### S6-01: Telegram Adapter — streaming response delivery

- **ID:** S6-01
- **Title:** Telegram Adapter — streaming response delivery
- **Subsystem:** Channel Adapter / Telegram
- **Why now:** без streaming пользователь ждёт полного ответа; плохой UX для длинных ответов
- **Dependencies:** S3-01, S2-03
- **Acceptance criteria:**
  - Orchestrator передаёт `assistant_delta` events в channel adapter
  - Telegram adapter: progressive message update (edit message with accumulated text)
  - Configurable: debounce interval for edits (default 500ms)
  - Final message = complete response
  - Graceful fallback: если edit fails, отправить новое сообщение

---

### S6-02: Telegram Adapter — approval UX with inline keyboards

- **ID:** S6-02
- **Title:** Telegram Adapter — approval UX with inline keyboards
- **Subsystem:** Channel Adapter / Telegram
- **Why now:** approval flow — обязательная часть lifecycle; без UX в Telegram нельзя подтверждать tool calls
- **Dependencies:** S3-01, S4-01
- **Acceptance criteria:**
  - Orchestrator в состоянии `awaiting_approval` отправляет approval request в channel adapter
  - Telegram adapter: отправляет сообщение с inline keyboard (Approve / Reject / Details) per PRD section 9.1.1
  - Callback data привязан к `tool_call_id`
  - Approve → orchestrator transitions to `tool_running`
  - Reject → orchestrator transitions to `cancelled`
  - Timeout: configurable (default 5 minutes), auto-reject with `approval_timeout`
  - Details button: показывает расширенное описание (tool name, target domain, credential alias if any, risk level)
  - Audit: approval request/response logged
  - Unit tests для callback parsing и timeout

---

### S6-03: Web UI — Nuxt.js project initialization and shell

- **ID:** S6-03
- **Title:** Web UI — Nuxt.js project initialization and shell
- **Subsystem:** Web UI
- **Why now:** Web UI — второй обязательный channel; нужно начать с shell, чтобы потом наращивать функционал
- **Dependencies:** S2-04
- **Acceptance criteria:**
  - `web/` directory: Nuxt.js project initialized
  - Pages: `/` (dashboard placeholder), `/sessions` (placeholder), `/memory` (placeholder), `/doctor` (placeholder), `/settings` (placeholder)
  - Layout: sidebar navigation, header with system status indicator
  - REST API client module configured (base URL from config)
  - `GET /health` → show green/red indicator
  - Dockerfile: `docker/web/Dockerfile` — Nuxt build + serve
  - Docker Compose: `web` service added, exposed on port 3000
  - No complex state management yet — just Nuxt built-in composables

---

### S6-04: Web UI — sessions and run history view

- **ID:** S6-04
- **Title:** Web UI — sessions and run history view
- **Subsystem:** Web UI
- **Why now:** пользователь должен видеть историю сессий и runs; это базовый observability requirement
- **Dependencies:** S6-03, S2-04
- **Acceptance criteria:**
  - REST API endpoints on orchestrator: `GET /api/v1/sessions`, `GET /api/v1/sessions/:key`, `GET /api/v1/runs/:id`, `GET /api/v1/runs/:id/transcript`
  - Web UI `/sessions` page: list of sessions with last activity time
  - Session detail: list of runs with state, timing, model provider
  - Run detail: state transitions timeline, transcript (messages + tool calls)
  - No edit capabilities — read-only

---

### S6-05: Web UI — doctor reports view

- **ID:** S6-05
- **Title:** Web UI — doctor reports view
- **Subsystem:** Web UI
- **Why now:** doctor reports — key self-hosted value proposition; нужно показывать их в UI
- **Dependencies:** S6-03, S5-03
- **Acceptance criteria:**
  - REST API: `POST /api/v1/doctor/check` → triggers doctor.check_system, returns report
  - REST API: `GET /api/v1/doctor/reports` → list of past reports
  - Web UI `/doctor` page: trigger check button, display report (checks list with status indicators, messages, recommendations)
  - Past reports list with timestamps

---

## Sprint 7 — Memory Pipeline, WebSocket Transport Upgrade, Hardening

Цель: запустить async memory pipeline и перейти на WebSocket-first transport для OpenAI.

---

### S7-01: Memory pipeline — async post-run extraction worker

- **ID:** S7-01
- **Title:** Memory pipeline — async post-run extraction worker
- **Subsystem:** Memory Pipeline
- **Why now:** memory extraction должна происходить async после run (per run-lifecycle-spec section 16); без этого память не наполняется
- **Dependencies:** S5-04, S5-05, S1-04, S0-05
- **Acceptance criteria:**
  - `internal/memory/pipeline/worker.go`: Redis-based job consumer
  - Orchestrator в `finalizing` enqueues post-run job (run_id, session_key)
  - Worker: reads transcript → calls LLM (lightweight model) for extraction → classifies candidates → writes to episodic/profile stores → updates embeddings (pgvector)
  - Extraction prompt: identify user preferences, important outcomes, system facts
  - Conflict resolution: profile updates supersede old values
  - Session summary generation/update
  - Structured logging per extraction
  - Integration test: complete run → verify memory items extracted

---

### S7-02: Model Transport — OpenAI WebSocket session hardening

- **ID:** S7-02
- **Title:** Model Transport — OpenAI WebSocket session hardening
- **Subsystem:** Model Transport
- **Why now:** базовый WebSocket-first path уже должен существовать к этому этапу; теперь нужно укрепить stateful session handling, fallback и recovery semantics
- **Dependencies:** S2-02
- **Acceptance criteria:**
  - `internal/transport/openai/websocket.go`: hardened WebSocket client for OpenAI Realtime API
  - Stateful session: single WebSocket connection per provider session, can persist across runs
  - Same `ModelProvider` interface — orchestrator не знает о деталях transport
  - Provider session ref management: create, reuse, detect loss
  - Fallback: if WebSocket connection fails, fall back to HTTP streaming (with `transport_warning` event)
  - Capability snapshot updated: `supports_stateful_sessions=true`
  - Structured logging for connection lifecycle
  - Integration test (opt-in): WebSocket session → send message → receive streaming response

---

### S7-03: Session summary — generation and retrieval

- **ID:** S7-03
- **Title:** Session summary — generation and retrieval
- **Subsystem:** Memory / Session
- **Why now:** session summary — compact context for orchestrator per run-lifecycle-spec section 17; improves response quality
- **Dependencies:** S7-01
- **Acceptance criteria:**
  - Session summary stored in sessions table (summary TEXT column, migration)
  - Memory pipeline worker: after extraction, generates/updates session summary via LLM
  - Orchestrator `preparing`: includes session summary in context bundle
  - Summary format: current goal, recent important events, open tasks, critical facts (per run-lifecycle-spec section 17.2)
  - Unit tests for summary integration in context bundle

---

### S7-04: Browser Runtime — full V1 tool set

- **ID:** S7-04
- **Title:** Browser Runtime — full V1 tool set
- **Subsystem:** Tool Runtime / Browser
- **Why now:** browser automation нуждается в полном наборе tools для реальных сценариев
- **Dependencies:** S4-04
- **Acceptance criteria:**
  - Additional tools: `browser.click(selector)`, `browser.fill(selector, value)`, `browser.type(selector, text)`, `browser.wait_for(selector, timeout)`, `browser.extract_text(selector)`, `browser.set_cookie(cookie)`, `browser.restore_storage_state(state)`
  - `browser.fill` and `browser.type` support `credential_ref` values (resolved by broker, passed in `resolved_credentials`)
  - All tools return normalized, safe output (no raw HTML/DOM)
  - Tool contracts registered in broker registry
  - Unit tests per tool + integration test: navigate → fill form → submit

---

### S7-05: HTTP Runtime — full V1 tool set

- **ID:** S7-05
- **Title:** HTTP Runtime — full V1 tool set
- **Subsystem:** Tool Runtime / HTTP
- **Why now:** HTTP tools нужны для полноценных web interactions
- **Dependencies:** S4-05
- **Acceptance criteria:**
  - Additional tools: `http.download(url, max_size)`, `http.parse_html(url_or_content, selector)`
  - `http.request` supports auth via `credential_ref` in `auth` field (resolved by broker)
  - Response size limits enforced
  - HTML parsing: extract text/structure from HTML response
  - Tool contracts registered
  - Integration tests

---

## Post-MVP priorities

### P1: Local model provider support
- Implement `ModelProvider` for local inference endpoints (ollama, llama.cpp, vllm)
- HTTP request/response transport (no WebSocket needed)
- Capability snapshot: `supports_tool_calls` may be false for some local models
- Config: local model URL, model name

### P2: Credential management UX
- `/cred add` command processing in Telegram
- Web UI credential management page
- Secure input flow (avoid storing secrets in chat history)
- Bootstrap: add credentials via Web UI or env variables initially

### P3: Autonomy mode enforcement
- Full policy engine in Tool Broker
- Mode 0–3 enforcement per tool call (per PRD section 18)
- Mode switching via Telegram command and Web UI
- Audit logging for mode changes

### P4: Web UI — memory management
- Memory browser: episodic, profile, working memory views
- Manual memory editing (add/update/delete profile facts)
- Memory search (pgvector similarity + keyword)

### P5: Doctor — advanced diagnostics
- `doctor.generate_report`: comprehensive system health report
- Memory diagnostics: index health, stale memories
- Tool runtime health monitoring
- Periodic automated checks (scheduled triggers)

### P6: Telegram enhancements
- Webhook mode instead of long polling
- Rich message formatting (markdown)
- File/image handling
- `/cancel` command for active runs

### P7: Web UI — chat interface
- Real-time chat via WebSocket to orchestrator
- Streaming response display
- Tool call visualization in chat

### P8: Rate limiting and resource constraints
- Per-tool resource limits in Docker (memory, CPU)
- Rate limiting in Tool Broker
- Model API cost tracking

### P9: Testing hardening
- Comprehensive e2e test suite: browser tool flow, credential-aware flow, doctor flow
- Load testing for concurrent sessions
- Chaos testing: runtime failure, database unavailability

### P10: Multi-model routing
- Support multiple OpenAI models (fast/smart routing)
- Model selection per task type
- Cost-aware model routing
