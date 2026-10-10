# Butler — Memory Model Specification

## 1. Статус документа

**Тип документа:** Architecture Subspec / Memory Model
**Версия:** 0.2
**Статус:** Baseline
**Связанный документ:** Butler PRD + Architecture Specification

---

## 2. Назначение

Этот документ формально описывает memory architecture (архитектуру памяти) системы Butler (дворецкий).

Цель модели памяти — дать Butler устойчивую, управляемую и расширяемую основу для:

* долгоживущего контекста;
* персонализации;
* воспроизведения прошлых эпизодов;
* поддержки tool-heavy workflows (сценариев с большим числом вызовов инструментов);
* self-inspection / doctor use cases (сценариев самодиагностики).

Модель памяти должна обеспечивать предсказуемость и контроль, а не сводиться к одному неструктурированному массиву истории или к одному vector index (векторному индексу).

---

## 3. Основные принципы

### 3.1 Memory is not chat history

Память не равна истории чата. История — лишь один из источников данных для memory pipeline (конвейера памяти).

### 3.2 Different memory classes serve different purposes

Разные типы памяти решают разные задачи. Нельзя пытаться заменить Profile Memory (профильную память) через vector search (векторный поиск), а Working Memory (рабочую память) — через transcript replay (повтор истории транскрипта).

### 3.3 Structured truth, retrieved relevance

Истина о памяти хранится в структурированном виде. Векторный поиск используется для релевантного извлечения, но не является главным источником истины.

### 3.4 Memory is owned by Butler

Даже если transport layer (транспортный слой) модели использует stateful WebSocket sessions (сессии с состоянием по WebSocket), память всегда остаётся внутренней подсистемой Butler.

### 3.5 Every stored memory item must have provenance

Любая единица памяти должна быть связана с источником происхождения: session, run, message, tool result или system event.

---

## 4. Цели памяти

Система памяти должна обеспечивать:

1. **Continuity (непрерывность контекста)** — пользователь должен ощущать, что агент “помнит” важные вещи между разговорами.
2. **Task support (поддержку задач)** — агент должен удерживать текущее состояние длительной задачи.
3. **Personalization (персонализацию)** — агент должен учитывать предпочтения пользователя.
4. **Recall (воспроизведение)** — агент должен уметь поднимать релевантные прошлые эпизоды.
5. **Operational self-knowledge (операционное знание о себе)** — агент должен помнить состояние собственной конфигурации и типовых проблем.
6. **Auditability (аудируемость)** — должно быть понятно, откуда взялся каждый сохранённый факт.

---

## 5. Не-цели

В текущую модель памяти не входят:

* полноценная knowledge graph (графовая база знаний);
* неконтролируемое автоматическое саморасширение памяти;
* хранение скрытых рассуждений модели как части долговременной памяти;
* безусловное сохранение всего подряд.

---

## 6. Классы памяти

Butler использует 4 основных класса памяти.

### 6.1 Transcript Store

#### Назначение

Полная история взаимодействий и системных событий.

#### Источники

* user messages (сообщения пользователя);
* assistant messages (сообщения агента);
* tool calls (вызовы инструментов);
* tool outputs (результаты инструментов);
* system inserts (системные вставки);
* doctor / diagnostics outputs (результаты диагностики).

#### Свойства

* источник истины для истории;
* максимально append-only модель;
* полный аудит;
* не используется как прямой prompt payload целиком.

#### Примеры

* пользователь попросил открыть сайт;
* был вызван browser.navigate;
* doctor обнаружил недоступность Redis.

---

### 6.2 Working Memory

#### Назначение

Краткоживущий слой состояния текущей задачи или активной сессии.

#### Содержимое

* текущая цель;
* активные сущности;
* незавершённые шаги;
* временные промежуточные результаты;
* task scratch state (рабочий контекст задачи).

#### Свойства

* short-lived (краткоживущая);
* допускает перезапись и обновление;
* рабочая область поручения связана с конкретным task; transcript остаётся у session, transient scratch — у run;
* не должна автоматически становиться долгосрочной памятью.

#### Два слоя хранения

Working Memory физически разделена на два слоя:

* **Durable layer (PostgreSQL, `tasks.working_json`)** — план, незавершённые действия и scratch конкретного поручения, с compare-and-swap `working_revision`. Цель, критерии и версионированные ограничения хранятся отдельно в той же task record. Финальный ответ модели не очищает эту область. `memory_working` сохраняется как legacy session snapshot для совместимости и аудита; новые task runs и обычный разговор не наследуют его цель. Реализованный контракт: [Durable tasks](durable-tasks-spec.md).
* **Transient layer (Redis)** — эфемерное состояние в процессе исполнения: in-progress execution state, task scratchpads, промежуточные данные текущего run. Теряется при перезапуске и не считается источником истины.

Durable layer отвечает на вопрос "где мы остановились", transient layer — "что происходит прямо сейчас внутри выполнения".

Текущий baseline: transient Working Memory хранится в Redis под отдельными ключами session+run с явным TTL. Этот слой используется для scratchpads и checkpoint state во время выполнения, автоматически очищается при successful completion и истекает по TTL для timeout/abandon scenarios.

#### Примеры

* “сейчас пользователь настраивает Telegram adapter”;
* “агент уже проверил конфигурацию Docker, осталось проверить Postgres”.

---

### 6.3 Episodic Memory

#### Назначение

Хранение значимых завершённых эпизодов, которые могут быть полезны в будущих похожих ситуациях.

#### Содержимое

* важные успешные действия;
* типовые ошибки;
* полезные сценарии решения проблем;
* завершённые контекстные эпизоды.

#### Свойства

* сохраняется после завершения run или группы run;
* может индексироваться в vector index;
* должна иметь summary (краткое описание), timestamps (временные метки) и source references (ссылки на источник).

#### Примеры

* “14 марта 2026 агент успешно диагностировал проблему с WebSocket transport из-за неверного API key”;
* “пользователь ранее уже просил разнести tools по Docker-контейнерам по классам”.

---

### 6.4 Profile Memory

#### Назначение

Хранение устойчивых и относительно стабильных сведений о пользователе и инстансе Butler.

#### Содержимое

* пользовательские предпочтения;
* технические предпочтения;
* особенности окружения;
* выбранные архитектурные договорённости;
* конфигурационные особенности инстанса.

#### Свойства

* долговременная;
* структурированная;
* должна поддерживать обновление, вытеснение и версионирование;
* не должна зависеть только от similarity search (поиска по сходству).

#### Примеры

* пользователь предпочитает русский язык;
* проект использует PostgreSQL + pgvector;
* система self-hosted и docker-oriented.

---

## 7. Логическая модель памяти

### 7.1 Источник истины

Butler использует structured persistence (структурированное хранение) как основной источник истины:

* PostgreSQL для долговременных данных;
* Redis или аналог для краткоживущего состояния;
* pgvector как retrieval layer (слой извлечения), а не как primary truth.

### 7.2 Принцип слоёв

* **Transcript Store** отвечает на вопрос: “что реально произошло?”
* **Working Memory** отвечает на вопрос: “что происходит сейчас?”
* **Episodic Memory** отвечает на вопрос: “что из прошлого опыта здесь похоже и полезно?”
* **Profile Memory** отвечает на вопрос: “что важно знать о пользователе и системе в долгую?”

---

## 8. Предлагаемая схема хранения

### 8.1 PostgreSQL

В PostgreSQL рекомендуется хранить:

* sessions
* runs
* messages
* tool_calls
* tool_outputs
* memory_working
* memory_episodes
* memory_profile
* memory_chunks
* memory_links

#### Current baseline

The current Butler baseline now includes durable PostgreSQL tables for:

* `memory_working`
* `memory_profile`
* `memory_episodes`
* `memory_chunks`
* `memory_links`

`memory_profile` is used for structured long-lived facts addressed by `scope_type`, `scope_id`, and `key`.

`memory_episodes` stores completed summaries plus pgvector embeddings so Butler can perform semantic retrieval without treating pgvector as the primary source of truth.

`memory_chunks` stores durable chunked long-form memory such as selected tool-output summaries and doctor-derived memory documents in PostgreSQL, while `memory_links` stores provenance-safe references separately from retrieval-facing content.

### 8.2 Redis

В Redis рекомендуется хранить:

* transient working memory state (эфемерное рабочее состояние в процессе исполнения, не дублирующее durable snapshot из `memory_working`);
* locks / leases;
* short-lived execution caches;
* active task scratchpads.

**Примечание:** Redis не хранит durable working memory. Рабочая область поручения сохраняется в PostgreSQL (`tasks.working_json`); `memory_working` — legacy session view. Redis содержит только эфемерный execution state, который допустимо потерять при перезапуске.

### 8.3 pgvector

Opt-in auxiliary inference is specified in [Local Model Service](local-model-service.md).
Its model/revision/preprocessing/dimension identity defines a separate embedding
space; installing the service does not switch existing memory indexes or providers.
Issue #25 still requires reversible side-by-side reindex and production evaluation.

Migration 021 uses dimensionless vector columns to support different embedding
providers. It removes the old fixed-dimension IVFFlat indexes; current semantic
queries scan these columns. Future indexes require fixed-dimension expressions
and provider-specific predicates. HNSW does not remove that requirement.

pgvector используется для semantic retrieval (семантического извлечения) по:

* episodic memory summaries;
* selected transcript summaries;
* long document chunks;
* selected tool outputs.

---

## 9. Что индексируется в vector search

### 9.1 Индексировать рекомендуется

1. Episodic memory summaries
2. Summaries длинных диалогов
3. Knowledge / document chunks
4. Длинные tool outputs, если они могут переиспользоваться
5. Doctor reports summaries

### 9.2 Индексировать не рекомендуется

1. Весь raw transcript целиком
2. Любые короткие сообщения без фильтрации
3. Шумовые tool outputs
4. Временные рабочие значения из Working Memory

### 9.3 Причина

Без фильтрации retrieval quality (качество извлечения) быстро деградирует: поиск начинает возвращать шум, а не действительно полезную память.

---

## 10. Retrieval model (модель извлечения)

### 10.1 Базовый принцип

Retrieval должен быть hybrid (гибридным), а не чисто векторным.

### 10.2 Компоненты retrieval

1. **Metadata filtering (фильтрация по метаданным)**
   tenant_id, session scope, memory class, timestamps, source type.

2. **Structured lookup (структурированный поиск)**
   точные факты, профильные настройки, известные ключи конфигурации.

3. **Vector similarity retrieval (векторное извлечение)**
   поиск похожих эпизодов и релевантных документов.

4. **Optional keyword / full-text retrieval (опциональный полнотекстовый поиск)**
   полезен для технических ключей, логов, конфигурационных строк.

### 10.3 Сбор памяти перед run

Целевой контракт [Persistent Assistant Spec](persistent-assistant-spec.md)
сохраняет четыре класса памяти. Working Memory принадлежит поручению, если run
с ним связан, и не очищается при окончании незавершившей поручение попытки.
Session хранит transcript и порядок; task state, approvals и mandates —
операционные записи PostgreSQL, а не результат semantic retrieval.

Summary должна иметь source revision и проверяемую границу message sequence.
Контекст содержит summary, хвост после границы и отдельные действующие ограничения
с source refs. Старый worker не перезаписывает более новую границу. Обязательные
ограничения и свежий ввод имеют приоритет над второстепенной памятью; overflow и
ошибки чтения обрабатываются явно. Базовое разделение tasks/runs (#4) и сборка
контекста (#10) реализованы ниже; изменение ограничений относится к #9.

Перед запуском model loop (цикла модели) orchestrator должен запросить у memory service memory bundle (пакет памяти):

* session summary;
* working memory snapshot;
* relevant episodic memories;
* relevant profile memory entries;
* при необходимости retrieved document chunks.

#### Current store baseline

At the current implementation stage:

* profile retrieval is structured lookup by scope from `memory_profile`;
* episodic retrieval is vector similarity search from `memory_episodes` using pgvector distance ordering;
* `internal/memory/service` owns bundle assembly policy, scope ordering, and store-specific retrieval for profile, episodic, working, and session-summary inputs;
* orchestrator requests that structured bundle and assembles the final system prompt itself when relevant entries exist.

#### Current hybrid retrieval baseline

Current bundle assembly now combines:

* structured profile lookup by ordered scope;
* session summary and working memory as highest-priority bundle items;
* vector episodic retrieval when embeddings are available;
* lightweight keyword summary matching as an optional fallback / complement when supported by the store;
* explicit bundle-budget ordering across summary, working, profile, and episodic sections.

When embeddings or optional keyword matches are unavailable, bundle assembly degrades gracefully and still returns deterministic summary / working / profile context within budget.

The Transcript Store assigns a durable per-session `message_sequence` using the
locked `transcript_heads` row; ordering does not depend on client timestamps or
on commit order of a global sequence. A separate `source_revision` changes when
any message is edited/deleted, invalidating summaries that could retain its content.
Message identity, session and sequence are immutable. This metadata belongs to
the Transcript Store and does not transfer run ordering/leases from Session Service.

Execution-owned transcript writes carry Session Service's PostgreSQL ownership
token. Appending a message or tool call checks its run/session binding and holds
a shared ownership-row lock across the write. Superseded or expired workers
cannot append delayed model output or tool results after a new epoch takes over.
Administrative/background audit writes remain separate from run execution.

`internal/memory/summary` owns `session_summaries`: summary text, format version,
source revision, last covered message ID and sequence. Publication checks the
source identity/revision and uses a monotonic boundary CAS under the same head
lock as append/source invalidation. A late inference cannot replace newer coverage.
The memory worker summarizes the previous verified prefix plus the complete new
session message tail, separately from per-run profile/episode extraction. It skips
already covered history. Empty/oversized summary input leaves coverage unchanged;
it never assigns full coverage to abbreviated or per-run-only input.

Run preparation validates that boundary against its loaded transcript snapshot.
Missing/legacy/edited/future boundaries replay the complete history; verified
coverage supplies the full sanitized summary plus every nonempty user/assistant
message after it, without a fixed ten-message window or assistant abbreviation.
The legacy `sessions.summary` field remains for rollback/audit and is not trusted
as coverage. Durable task goal, constraints/revision, criteria and working state
are attached separately and cannot be replaced by a model summary. Each new run
starts with fresh provider state; tool continuation within a run retains its ref.

History or coverage read failure blocks model preparation explicitly. Context
admission uses `BUTLER_CONTEXT_BUDGET_TOKENS` (default 65536): one conservative
token unit per encoded UTF-8 byte, a 1024-unit protocol allowance and per-item/tool
overhead. Observability records estimator, estimate, allowance and removed optional
sections. This is an admission estimate, not a provider-native token count or bill;
operators must account for provider-specific wrappers and model/output capacity.
Retrieved chunks, episodes, profile and tool descriptions are removed first. If
mandatory history, covered summary, task constraints and base context cannot fit,
the run fails with an explicit budget error rather than losing a prohibition.
Optional prompt shortening and pipeline logging remain Unicode-safe. Amendment
and authority revocation semantics belong to #9; cumulative turn limits to #13.

---

## 11. Memory pipeline (конвейер памяти)

### 11.1 После завершения run

После завершения каждого run должны выполняться следующие шаги:

1. **Persist transcript**
   сохранить полную историю хода.

2. **Extract memory candidates**
   извлечь кандидатов в факты, эпизоды, профильные записи.

3. **Classify candidates**
   определить тип памяти: working / episodic / profile / ignore.

4. **Resolve conflicts**
   выяснить, это новый факт, обновление старого факта, временное состояние или шум.

#### Current pipeline baseline

The async memory worker now separates:

* extraction;
* candidate classification into profile / episodic / working / document / ignore;
* conflict resolution before durable writes;
* explicit ignore handling for low-confidence or noise candidates.

Current baseline now persists accepted document candidates into `memory_chunks`, and may additionally ingest selected long-form tool outputs and doctor reports through explicit allowlist-style logic in the async memory pipeline.

5. **Write structured memory**
   записать результат в соответствующий memory store.

6. **Update retrieval index**
   обновить vector index и, если нужно, full-text representations.

### 11.2 Кандидаты памяти

Кандидатом памяти может быть:

* явно выраженное предпочтение пользователя;
* результат важного действия;
* диагностическая проблема;
* конфигурационный факт;
* завершённый эпизод;
* полезный long-form output (длинный содержательный вывод).

---

## 12. Правила сохранения

### 12.1 Сохранять стоит

* подтверждённые пользовательские предпочтения;
* устойчивые свойства окружения;
* значимые диагностические результаты;
* важные завершённые сценарии;
* полезные паттерны решения.

### 12.2 Не стоит сохранять автоматически

* случайные разовые пожелания без повторяемости;
* весь диалог без отбора;
* шумовые технические детали;
* скрытые промежуточные рассуждения модели;
* большие outputs без признаков будущей полезности.

#### Current sanitization baseline

Before memory extraction and before durable memory writes for memory classes, Butler now applies a sanitization pass that redacts credential-like values such as bearer tokens, passwords, cookies, storage-state blobs, and DSNs / connection strings. This keeps Transcript Store as raw audit history while preventing raw secret material from entering Working, Profile, Episodic memory, and session summaries.

---

## 13. Принципы конфликтов и обновлений

### 13.1 Profile conflicts

Если новый профильный факт конфликтует со старым, система должна:

* либо обновить старое значение;
* либо пометить старое как superseded (замещённое);
* либо сохранить обе версии при явной временной привязке.

#### Current profile conflict baseline

Current pipeline baseline now applies deterministic profile conflict handling:

* same key + same value => keep the stronger version and suppress weaker duplicates;
* same key + conflicting value => higher-confidence candidate supersedes the current active version;
* superseded entries remain queryable through profile history for audit/version review.

### 13.2 Episodic duplication

Похожие эпизоды могут дедуплицироваться или связываться как variants (варианты).

#### Current episodic dedup baseline

Current pipeline baseline now suppresses near-duplicate episodic candidates with the same canonical summary and similar content, while preserving materially different candidates as variants linked to the same canonical summary.

### 13.3 Working Memory replacement

Рабочая область обновляется с явной версией. Завершение run не очищает незавершённое поручение; состояние меняется через task module. Хранить каждый scratch checkpoint как Episodic Memory не требуется.

---

## 14. Привязка к сессиям и scope (области)

Каждая memory item должна иметь scope:

* global system scope;
* user scope;
* session scope;
* run scope;
* doctor / system scope.

Это нужно для того, чтобы:

* не смешивать временное и постоянное;
* не смешивать пользовательское и системное;
* лучше ограничивать retrieval.

---

## 15. Self-inspection memory

Отдельный важный класс использования памяти для Butler — запоминание собственного состояния.

### 15.1 Что относится сюда

* результаты doctor checks;
* повторяющиеся проблемы конфигурации;
* известные особенности инстанса;
* состояние внутренних компонентов;
* сведения о доступности containers / services.

### 15.2 Где хранить

* краткоживущие технические состояния — в Working Memory;
* устойчивые особенности окружения — в Profile Memory;
* завершённые значимые диагностические случаи — в Episodic Memory.

---

## 16. Prompt integration (интеграция в контекст модели)

### 16.1 Что не делать

Не передавать модели всю историю и всю память целиком.

### 16.2 Что делать

Перед каждым run формировать memory-aware context bundle (контекстный пакет с учётом памяти), включающий:

* compact session summary;
* active working state;
* selected profile facts;
* 1..N релевантных episodic memories;
* при необходимости document chunks.

### 16.3 Цель

Сделать memory injection (вставку памяти) контролируемой и дешёвой по токенам.

---

## 17. Рекомендуемые поля memory records

### 17.1 Общие поля

Для всех классов памяти рекомендуется иметь:

* `id`
* `memory_type`
* `scope_type`
* `scope_id`
* `summary`
* `content`
* `source_type`
* `source_id`
* `provenance`
* `created_at`
* `updated_at`
* `confidence`
* `status`

### 17.4 Provenance and links baseline

Current baseline now stores explicit `provenance` JSON on durable Working, Profile, and Episodic memory records.

`provenance` is intended to hold safe source references such as:

* originating `run_id`;
* source class like `run`, `tool_result`, `doctor_report`, `system_event`, or `memory_pipeline`;
* non-secret-safe reference handles for transcript or tool-output lineage.

Related references are stored separately in `memory_links`, which maps a durable memory record to safe target references such as runs, messages, tool calls, or doctor reports without copying sensitive source payloads into retrieval-facing records.

### 17.2 Дополнительные поля для Profile Memory

* `key`
* `value`
* `effective_from`
* `effective_to`
* `supersedes_id`

### 17.3 Дополнительные поля для Episodic Memory

* `episode_start_at`
* `episode_end_at`
* `embedding`
* `tags`

---

## 18. Минимальный V1 объём

Для Butler V1 memory subsystem должна обязательно включать:

* Transcript Store;
* Working Memory;
* Episodic Memory;
* Profile Memory;
* PostgreSQL как primary store;
* Redis как short-lived state store;
* pgvector как vector retrieval layer;
* базовый extraction pipeline;
* базовый retrieval pipeline.

---

## 19. Риски

### 19.1 Over-saving

Если сохранять слишком много, retrieval деградирует и память превращается в шум.

### 19.2 Under-structuring

Если всё хранить как неструктурированный текст, система теряет предсказуемость.

### 19.3 RAG-only trap

Если пытаться заменить всю память одним vector search, система плохо работает с точными фактами, профилем и обновлениями.

### 19.4 Memory drift

Если не решать конфликты и устаревание, агент начнёт опираться на неверные старые факты.

## 20. Current retention baseline

Current Butler baseline now applies explicit housekeeping policies:

* transient Working Memory remains Redis TTL-based and self-expires automatically;
* stale durable Working Memory snapshots can be pruned by `updated_at` age;
* inactive / superseded Profile Memory versions can be removed after retention windows;
* Episodic Memory and document chunks can be pruned by age while keeping the highest-confidence recent items per scope;
* housekeeping operations are safe to rerun and report deletion counts for observability.

## 21. Current observability baseline

Current Butler baseline now includes memory-specific observability signals:

* counters for memory jobs, memory writes, retrieval hits/misses, and doctor checks;
* queue depth gauge for the memory pipeline backlog;
* structured logs with memory-oriented fields such as `memory_type`, `source_type`, and `run_id` on persistence paths;
* doctor memory reporting for queue health, pgvector readiness, pipeline worker status, and embedding provider availability.

## 21.1 Current operator/browser baseline

Current Butler baseline now exposes read-only operator visibility for durable memory:

* orchestrator serves `GET /api/v1/memory?scope_type=...&scope_id=...` for scope-based inspection;
* the endpoint returns durable Working, Profile, Episodic, and Chunk memory records when present;
* provenance-safe related references are returned via `memory_links` instead of copying raw source payloads;
* Web UI `/memory` provides the corresponding browser for operators and future build agents.

---

## 21.2 Current pipeline operational baseline

The async memory pipeline is now fully operational end-to-end:

* **Embedding provider** (`internal/memory/embeddings/`): pluggable embedding provider supporting both OpenAI-compatible and Ollama backends. The provider produces embedding vectors for episodic memory and document chunk semantic search.
  * **OpenAI provider** (`provider.go`): HTTP client for OpenAI `/v1/embeddings` endpoint, default model `text-embedding-3-small` (1536 dimensions). Requires an API key.
  * **Ollama provider** (`ollama.go`): HTTP client for Ollama `/api/embed` endpoint, default model `nomic-embed-text` (768 dimensions). No API key required; runs locally via Docker. Recommended for self-hosted deployments without an OpenAI API key.
  * **Vector dimensions** are configurable at startup via `BUTLER_MEMORY_EMBEDDING_DIMENSIONS` (defaults to 1536). The database migration `021_flexible_vector_dimensions` removes fixed `vector(1536)` column types to support any dimension size.
* **LLM caller** (`internal/memory/pipeline/llmcaller.go`, `internal/memory/pipeline/codexcaller.go`): lightweight extraction callers for either OpenAI chat completions or OpenAI Codex responses auth, depending on which provider credentials Butler has available.
* **Pipeline worker** (`internal/memory/pipeline/worker.go`): async worker started in `app.go` that dequeues post-run jobs from Redis, reads the transcript, calls LLM extraction, classifies/resolves candidates, generates embeddings, and writes profile entries, episodic memories, document chunks, and session summaries.
* The pipeline now supports two embedding provider backends: OpenAI (requires API key) and Ollama (local, no API key). When configured with `BUTLER_MEMORY_EMBEDDING_PROVIDER=ollama`, Butler uses a local Ollama instance for embeddings while still using the available LLM provider (OpenAI or Codex) for extraction. This enables full episodic and chunk memory without a paid OpenAI API key.

### Configuration keys

| Environment variable | Default | Description |
|---|---|---|
| `BUTLER_MEMORY_EMBEDDING_PROVIDER` | `openai` | Embedding backend: `openai` or `ollama` |
| `BUTLER_MEMORY_EMBEDDING_MODEL` | `text-embedding-3-small` | Embedding model name (provider-specific) |
| `BUTLER_MEMORY_EMBEDDING_DIMENSIONS` | `1536` | Expected embedding vector dimensions |
| `BUTLER_OLLAMA_URL` | `http://ollama:11434` | Ollama API base URL (used when provider is `ollama`) |
| `BUTLER_MEMORY_EXTRACTION_MODEL` | `gpt-4o-mini` | LLM model for memory extraction |
| `BUTLER_MEMORY_PIPELINE_ENABLED` | `true` | Enable/disable the async memory pipeline worker |
| `BUTLER_MEMORY_PIPELINE_POLL_TIMEOUT_SECONDS` | `5` | How long the worker blocks waiting for a job |
| `BUTLER_MEMORY_PIPELINE_MAX_RETRIES` | `3` | Max retries for a failed pipeline job |

### Doctor checks

The memory doctor (`/api/v1/doctor/check`) now reports:

* `queue`: Redis queue health and current depth;
* `pgvector`: whether the `vector` PostgreSQL extension is enabled;
* `pipeline_worker`: whether the worker is enabled and running;
* `embedding_provider`: whether an embedding provider is configured.

---

## 22. Открытые решения

1. ~~Точная SQL schema для memory tables.~~ — **Текущий baseline реализован** через `memory_working`, `memory_profile`, `memory_episodes`, `memory_chunks`, `memory_links`; дальнейшая эволюция schema остаётся допустимой отдельными миграциями.
2. ~~Политика дедупликации episodic memory.~~ — **Решено:** near-duplicate эпизоды подавляются, materially different варианты сохраняются как variants по canonical summary policy.
3. ~~Политика сохранения doctor outputs.~~ — **Решено:** doctor/tool-derived long-form data допускается только через явную allowlist-style ingestion policy с предварительной sanitization и записью в chunk memory.
4. Формат session summary и точные правила его обновления остаются частично открытыми; текущий baseline использует compact async-updated summary path через memory worker.
5. ~~Приоритет retrieval между profile, episodic и document memory.~~ — **Решено:** bundle assembly применяет явный порядок summary → working → profile → episodic → chunks в пределах budget policy.
6. Использовать ли отдельный reranking layer (слой переранжирования) в будущем.

---

## 23. Итоговый тезис

Память Butler должна быть **многослойной, структурированной и управляемой**.
Она не равна ни чату, ни RAG, ни векторной базе.

Правильная модель для Butler:

* Transcript Store фиксирует факты истории;
* Working Memory удерживает текущую задачу;
* Episodic Memory даёт воспроизведение прошлого опыта;
* Profile Memory фиксирует стабильные знания о пользователе и системе;
* pgvector помогает находить релевантное, но не становится единственной правдой о памяти.

Именно такая схема подходит для self-hosted, long-lived personal agent system (долгоживущей персональной агентной системы), которой должен стать Butler.


Owner amendments are appended as user transcript data and preserved in full protected task constraints with provenance; obsolete working plans are archived before clearing the new revision. See [Owner Controls](task-owner-controls-spec.md).
