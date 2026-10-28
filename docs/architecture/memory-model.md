# Butler — Memory Model Specification

When the existing skill catalog is configured for a typed task, Orchestrator adds
fixed kernel guidance to review descriptions and load an applicable exact digest
before solving the task. Missing applicability information may require search;
an already loaded current procedure need not be loaded again. Owner prohibitions,
compatibility, revocation, current source and Broker authority checks still govern
use. Descriptions and procedure bodies remain procedural DATA. Recovery replaces
the fixed guidance and reloads current pinned DATA; neither text nor a model
selection grants permissions or proves task completion.

Catalog's explicit `WithRunObservationPool` composition clones a Store with its
existing business pool and exact manifest intact, including restore/deletion
failure injections. Only the existing token-only observation path selects the
reserved capacity. Grants, source state, deletion anchors and authority checks
remain unchanged; the original Store is not mutated.

Migration 103 changes only the query plan of summary currentness checks. A STABLE
PL/pgSQL function checks empty summary metadata in the current statement snapshot;
otherwise it evaluates the original recursive roots, transcript heads, dependency
sets and original-source metadata. No accepted fact or authority result is cached,
and correction, deletion, privacy and revision fences remain authoritative.

New epoch1 explicit taskless requests use two recent completed published
exchanges as historical reference plus all current-run observations. Unfinished
old requests/internal finals/old tool results are excluded without deleting
transcript. Current input stays verbatim; typed tasks retain full history and
constraints. Exact interaction continuation remains owned by Communication.
Complete transcript is pinned/privacy checked before selecting context. Old
epoch0 inputs retain replay policy. A kernel UTC clock and current-request rule
prevent notification timestamps and unfinished history from defining the goal.
Automatic document context requires lexical term/prefix overlap with the current
query using the existing heuristic. This favors precision and can omit
paraphrases; explicit semantic search retains recall. Omissions are diagnosed.
Profile preferences and source/privacy guards are unchanged.

The disabled-by-default [issue97 synthetic research driver](../../cmd/effort-eval/README.md)
uses an independent local PostgreSQL fixture journal. Primary source observations,
model claims/hypotheses and current goal versions remain distinct; quoted claims
are not promoted into source facts. It adds no product memory class, profile
entry, compaction/retrieval consumer or production working-memory policy.

Credential vault form bodies, PINs, decrypted account fields and OTP candidates
never enter Transcript, Working, Episodic, Profile or vector memory. Protected
owner/device endpoints bypass ordinary event/message ingestion. The approved
request reference and field/status metadata may resume a task. Explicit SMS
relay sends a transient login candidate through a private device endpoint before
ordinary notification normalization; the ordinary queue retains its masked
version. Ciphertext and content-free receipts are vault state, not memory sources.

The [mobile memory/search contract](mobile-memory-search.md) adds task-scoped
notification context, atomically searchable notes and explicit local automatic
index maintenance. Vector publication remains separate from fact acceptance.

[Memory Doctor](../../internal/memory/doctor/README.md) reports actual constructed
feature composition separately from operator requests, code-owned qualification,
supported provider profile and current durable worker/owner-policy controls. None
of these observations proves answer quality or grants source/job/action authority.
Configuration identities include the effective composition; current source and
deletion-anchor guards survive disabling new admission or retiring a derivative.

The gated [verified task compaction consumer](../../internal/memory/compaction/README.md)
uses durable independent model jobs and exact prefix coverage. Every user event
remains verbatim; assistant/tool excerpts must match primary bytes and tool states
are attached by code. Newest four events stay raw. Summaries are historical DATA:
current typed goals/constraints, approvals and effect journal remain authoritative.
Repeated compaction regenerates primary events, avoiding recursive fact drift.
Failure never hides history; finite input/output limits can require an explicit
context-budget failure. Migration072 links copied session inputs to privacy
deletion and invalidates coverage on tool state changes as well as message edits.

Issue78's [memory-eval protocol](../../cmd/memory-eval/README.md) measures truth,
actual-context evidence, calibrated refusal, current instruction consistency and
separate user outcomes. Its independent public V2 controls and owned-source/index/
native collector extend issue33 without another memory service. Missing retrieval
may justify refusal but cannot count as recovered usefulness. Unknown tokenizer,
invoice, physical-storage and effects remain unknown and deny promotion; synthetic
QA grants no owner confirmation or execution authority.

## Планируемое происхождение коммуникаций

Dormant backend реализует typed kinds и отдельную identity final. Explicit
`attempt_final` хранится с role=system, visibility=internal, published=false;
extractor/compactor сохраняют эту provenance, а не представляют её публичным
ответом. Publication role=assistant не доказывает доставку. Миграция090 стирает
payload, question/answer и outbox representations при удалении canonical
message через Catalog, сохраняя identities и hashes. См.
[проверки](../ai/communication-core-acceptance-20261021.md); startup остаётся legacy.

[User Communication](user-communication-spec.md) сохраняет одну каноническую
publication/response в Transcript независимо от числа каналов. Delivery receipts
и внутренний model final отличаются от опубликованного текста; ответы виджетов
сохраняют actor/source provenance. Это целевое дополнение не вводит новый класс
памяти, не превращает ответы в policy и не разрешает повторное извлечение
отозванных sources из delivery history.

До первой исполняемой publication целевой foundation C02 разделяет
publication/attempt_final/owner_input/interaction_response по typed identity,
kind и visibility независимо от role. Transcript final writer выбирает только
собственный итог, не все assistant-записи run. Replay, compaction, extraction,
summaries и episodic consumers сохраняют provenance: внутренний итог не является
сообщением, уже отправленным владельцу. Разрешённые технические факты могут
использоваться во внутренней памяти с этим типом происхождения; user feed не
подменяет ими публикации. Проверяются несколько publications + иной/пустой
terminal final + persistence recovery и последующая compaction. Это планируемое
изменение readers/writer, а не уже реализованное свойство main.

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

Типизированные уроки выполнения #90 остаются внутри этого класса: наблюдения
действий/эффектов, отдельное подтверждение владельца, версии среды и инструментов,
ошибки и неизвестные исходы. См. [контракт опыта](episodic-experience.md).
Модельная заметка не является подтверждением успеха или разрешением действия.

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
Migration041 and `internal/memory/semantic` add separate immutable embedding spaces
and staged generations with source revision/hash, scope and live-state filtering.
They support reviewed coverage/quality verification, activation CAS, reversible
selection of still-current ready generations, and restart-resumable indexing.
Unchanged title/content hashes can reuse exact-space vectors across new generations
after citation/path changes; current source revisions and access are still checked.
Deleting a chunk removes its derived vectors; incompatible dimensions/revisions
never enter the legacy columns. Source refs and private data origin accompany
retrieval; Secret-class inputs cannot reach embedding/reranking IPC.

Every source read/publication requires a typed `SourceGuard`, default denied. Its
`CatalogGuard` derives catalog links and current owner/run/job authority from actual
PostgreSQL chunk IDs, not provenance JSON. Catalog/authority rows are locked in
sorted order before chunks in the caller transaction; no second connection is
needed. Optional catalog provenance fields are citations rather than permissions.

Typed `LiveAdmission` and `JobAdmission` now reserve each actual local query/rank
or indexing attempt through current ownership/independent job authority and shared
root/mandate/owner-day budgets. A pinned internal service proves locality separately
from explicit local_no_billing pricing. The optional bundle reader remains disabled
until the production #22/#23 composition installs these adapters and schedules
owned jobs. It does not revive completed run ownership. The source guard and current
metadata checks do not replace #22 contradiction, acceptance or backup tombstone
semantics. Fixed Russian/code fixture quality is measured separately from
production acceptance; reranking is optional and default-off.

The live local query/rank watchdog shares Core's reserved authority connection;
business-pool occupancy cannot consume its200ms proof window. The250ms observer
still checks current lease, task/mandate and exact source stamps and rolls back.
Allowance admission and returned-result acceptance keep their original business
transactions, committed applicability pins and immutable reservations. A failed
observation cancels inference with a fixed phase/class cause; raw source/SQL
errors are excluded and original parent deadlines are not extended.

The two local-index in-flight source/chunk observations keep their complete
catalog graph, current job claim, source-body and exact chunk proof at the
unchanged250ms interval and200ms proof deadline. Their private job checker cannot
bind run pins or execute a caller-supplied checker; these SELECT/row-lock
transactions publish no business state and finish with bounded rollback. Full
allowance admission, ordinary source reads, returned-result acceptance and
publication retain their existing commit boundaries. A failed proof or rollback
still cancels; late local results remain unusable and every attempted quote stays
charged. A controlled commit-delay reproduction does not establish the cause of
any historical generic `index_rejected` receipt.

The optional `HybridReader` adds current exact identifier/path/version matching
and Russian/simple PostgreSQL full-text search (derived GIN migration042), then
reciprocal rank fusion with separate vector candidates. Only positions are fused;
exact technical hits stay protected. Bare absent identifiers return no source.
Duplicate content and per-source caps preserve a diverse bounded candidate set.
Every candidate needs current source authority before text-model IPC. Optional
Qwen reranking has complete correlated outputs and per-scope sorting; unavailable
or oversized ranking keeps base evidence with fixed diagnostics. Authority/budget
denial, Secret data, cancellation or corrupt/partial scores cannot use that fallback.
The adapter uses the admission's returned bounded context and rejects late results.
Index/rank IPC binds chunk ID/scope/revision, full content/citation hash and embedding
payload hash; the exact text/title must match the snapshot. Current catalog plus
live chunk identity is checked after allowance waits and before returned output is
accepted. Periodic cancellation bounds queued/in-flight revocation; publication
still needs fresh transactional proof. The all-migration owned-admission smoke
exercises actual local-only mandate, independent index job, live query/rank,
reservations, current citations and physical vector deletion on the same service.

Final citations include original chunk/document refs and current revision, with
explicit clipped UTF-8 excerpts and a finite serialized byte/token allowance.
This allowance covers retrieved evidence; the caller's full prompt has its own
transport/budget checks. Retrieval never changes structured owner rules. The
fixed real suite measures support-source coverage; it does not verify generative
answers or justify activating the still-unwired production reader.

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

* current confirmed/effective profile lookup by authorized scope, ranked by
  lexical query applicability and then stable scope/key/text ordering;
* session summary and working memory as highest-priority bundle items;
* vector episodic retrieval when embeddings are available;
* lightweight keyword summary matching as an optional fallback / complement when supported by the store;
* `memory-context-v3`: mandatory summary/working state, then deterministic
  document/profile/episode round-robin, an optional conservative UTF-8 JSON byte
  allowance (8192 by default) inside the complete context allowance, per-class
  quotas and a secondary item cap. Mandatory history is not rejected by the
  smaller optional quota. With documents present, profile and episodes each
  consume at most one third of the optional allowance.
  The product default is four bounded document results. Zero lexical overlap
  leaves authorized profiles as lower-priority candidates within their quota.
  Deduplication uses profile scope/key/value/provenance or document source/
  revision/title/excerpt/provenance, preserving independent equal-valued facts.
  Oversized optional records are omitted whole. Mandatory
  overflow is explicit. Safe diagnostics contain version/counts/reason codes.

`QueryPolicy` separately permits local query use and external query release.
Assigned local-only tasks retain local SQL and proven pinned local inference;
unproven inference is skipped with an explicit limitation, without external
fallback. A local search does not permit cloud generation. Existing current
run/source/task guards remain authoritative before/after search and on provider
release. Profile eligibility is re-read after slow retrieval; selection never
fills a quota using withdrawn/foreign records. See the
[bundle service contract](../../internal/memory/service/README.md).

Selected profile evidence retains its exact scope type/ID and source handles in
both bundle text and Orchestrator model sections. Missing scope is explicitly
unknown, not inferred from a key or its wording. Profile sections are not
character-truncated: a trailing exception or source qualifier belongs to the
record. Full-context admission may remove optional records whole and still
prioritizes current owner input and typed task constraints. Descriptive scope
metadata does not grant authority, resolve competing preferences, or promote an
extracted candidate. The combined prompt preview may retain its display limit;
the actual model consumes the complete admitted sections. Development evidence
and limits: [issue76 profile context](../testing/issue-76-profile-context.md).

The [issue76 factorial comparison](../testing/issue-76-context-factorial-results-20261023.md)
supports an explicit `BundleRequest.ProfileTaskGoal` hint for current active tasks.
Only ties in the existing current-message lexical profile score use the goal's
lexical score; existing scope/key/text ties and quotas otherwise remain intact.
The goal is current typed task DATA, never inferred from an old session snapshot
or a retrieved document. It grants no access/authority. The Orchestrator supplies
it only after validating the explicit task's ID, owner, session and active state
and reading its current authority. After retrieval it rechecks task/constraint
revisions, goal and release policy before admitting the bundle. Conversation,
preview and callers without the authority reader leave it empty. The experiment
found a small answer improvement with no observed paired loss on eight new
controls; earlier ambiguous proposal behavior remains a limitation, not a
universal safety certificate. Additional preference prompt guidance stays dormant.

Hybrid queries above 2048 UTF-8 bytes skip optional lookup with `query_input_limit`
and no SQL/inference, preserving the full owner message for ordinary preparation.
Access/Secret/configuration failures and current authority/source release checks
remain blocking. Historical v2 comparison artifacts retain their executed pins.

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
Complete input admission leaves `min(4096, allowance/8)` additional conservative
units for answer/tool continuation. Optional tool summary and then profile,
episodes and documents are removed one item at a time, preserving remaining
document evidence rather than discarding an entire class. If
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

Selected mobile notes and their immutable exact evidence have indefinite retention.
Their class rows are exempt from age/count housekeeping. Raw notification archives
default to360 days; automatic archive_expire preserves retained notes, while
explicit owner forgetting erases their evidence and derived indexes. Observation
retention does not imply Profile acceptance or infinite event validity. See
[the implemented contract](mobile-memory-search.md#archive-expiry-and-retained-evidence).

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
4. Session summary uses versioned verified transcript coverage and source/deletion fences. Legacy model summary workers remain disabled until independently owned admission; summaries never replace current authority.
5. Приоритет retrieval версионирован в `memory-context-v3`: обязательные summary/working и отбор document/profile/episode по применимости, размеру и квотам; прежние версии сохранены в историческом сравнении.
6. Использовать ли отдельный reranking layer (слой переранжирования) в будущем.

---

## 23. Итоговый тезис

### Инструментальное чтение по потребности

Optional operator composition exposes bounded `memory.search/open/neighbors`
through Tool Broker, in addition to the prepared memory bundle. The catalog and
PostgreSQL remain authoritative; version1 full-text search has no external model
call and does not depend on a semantic generation. Source references remain DATA;
current source/ancestor/access/privacy and owning task/run/input guards apply to
every read and subsequent provider release. This does not enable write consumers
or accept profile/episodic claims. Contract:
[memory read control](../../internal/memory/readcontrol/README.md).

### Проверка полезности памяти

Текущая [смысловая методика](../ai/memory-eval-semantic-methodology.md)
проверяет обоснованность утверждений, полноту, актуальность, неопределённость
и согласованность с поручением отдельно от машинного формата. Независимая
рецензия привязана к точным корпусу/методике/input/output; код проверяет
подлинность цитат и полноту матрицы, не выдавая это за доказанное смысловое
следование. Ошибка старого lexical scorer, отсутствие факта в retrieval,
ошибка ответа при видимых доказательствах и provider failure различаются.
Режим oracle предоставляет заранее проверенные достаточные источники;
он не получает эталон ответа. Свежая проверка использует обычный Catalog,
owned index и сборщик bundle, отдельный процесс чтения после перезапуска
PostgreSQL. Это проверка document memory, не всех слоёв памяти платформы.
Исторические strict scores ниже сохраняются со своими версиями; их общий
NO_GO не заменяет смысловую оценку. Времена и ресурсы — отдельные измерения.

Issue78 добавляет `cmd/memory-eval` поверх существующего `internal/acceptance`
(issue33). До модельного сравнения фиксируются версия набора и разметки,
контрольная часть, исходники, код, модель, промпт, конфигурация памяти и конечные
лимиты. Режимы без долговременной памяти, полного допустимого контекста и
актуального retrieval используют одинаковое текущее поручение и модель.
Непоместившийся baseline явно пропускается; усечённая история не считается
полным контекстом. Удалённые тела и секретные случаи не входят в model input.

Качество поиска, корректность ответа и цитирования, применение актуального факта,
соблюдение поручения и независимо подтверждённый результат действия — разные
измерения. Все расходы чтения/записи, embedding, извлечения, сводок,
консолидации, повторов, ошибок и фона включаются с явными неизвестными.
Критические нарушения полномочий, приватности, остановки и удаления запрещают
продвижение. Модель-судья не заменяет проверки исходников и реальных эффектов.

Первый набор синтетический; fixture проверяет стенд и текущий сборщик bundle
с фиксированными ответами тестового хранилища. Это не доказательство качества
модели или production readiness. Независимая разметка/контроль и исходные
runner/ledger доказательства остаются обязательны. Память и её полномочия этот
offline контур не меняет. См. [методику и ограничения](../../cmd/memory-eval/README.md).

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

## Source sensitivity and derived authority

Persistent provenance retains transcript/tool/child/document references. A stored claim of owner instruction/public classification cannot mint authority: extracted/retrieved material is private data by default. Secret-class sources fail before extraction, summaries, model input and retrieval; logs/parser errors cannot echo model output. Sensitivity is inherited by derivatives. Suppression/expiry is distinct from physical deletion; source removal must include derived records and backups under the retention policy in [agent security boundaries](agent-security-boundaries.md). Migrations046–049 implement versioned source/claim decisions, authenticated owner correction/export, structured original assets and transitive privacy redaction with a persistent anti-rollback deletion anchor. See [versioned sources](versioned-memory-sources.md), which supersedes the legacy pipeline operational description above. Production legacy model extraction/summary jobs remain disabled. Opt-in owned local indexing and current-run query use exact sources, independent current claims and shared budget admission; completion never borrows an old lease.

Optional [structured local extraction](structured-memory-extraction.md) publishes
only source-bound candidates under current durable job admission. Exact literal
fields, fragment-scoped rule evidence and unresolved action/exception scope are
separate; extraction scores cannot accept a belief or grant a mandate. Empty
results are durably complete, retries preserve IDs, and publication rechecks the
source graph. The measured Russian quality gate fails, so production scheduling
remains off despite the separate source-preservation acceptance test.

Optional [voice data](russian-voice.md) uses the same catalog original assets and
transitive immutable derivatives. ASR text is a preview, not a confirmed fact or
permission. Owner-reviewed ordinary input is queued by opaque binding and resolved
under the new current lease/source pins; privacy deletion includes prepared runs
before their first transcript message. Voice endpoints enforce their own finite
access lifetime, including audio ancestors of presented text. Owner catalog
administration retains/export-controls the source until explicit physical deletion;
expiry itself does not promise purging assets or backups.

Prepared code-copy provenance uses the same original Catalog graph, as specified
in [isolated workspaces](isolated-code-workspaces.md). Its immutable task DATA
contains no code/source body and grants no fact, policy or selectable ancestor
access. Catalog deletion transitively tombstones these contexts and atomically
blocks/queues affected filesystem copies. Fixed-root privacy erasure records a
durable completion only after actual removal; lost acknowledgement remains
pending across restart. This component is implemented, while workspace
Broker/runtime/model admission and cleanup scheduling remain disabled/pending.

# Pending-only evaluation lineage

The separately authorized [background model job path](background-memory-jobs.md)
extends the durable auxiliary queue with finite policies, current task/source
proof, shared per-attempt accounting and atomic immutable catalog DATA results.
It does not enable legacy extraction or confirm memory claims. Consumers require
their own quality/security gates; the optional exact-quote probe proves plumbing.

Memory observations use the additive `memory-observations-v1` contract described
in [the operator runbook](../operations/memory-observability.md). Component
configuration/connection, timestamped checks, request outcomes and quality proof
are separate. The production bundle service shares the `/metrics` registry;
search and context selection have independent completion/latency/count/reason
metrics with fixed label vocabularies. Legacy hit/miss semantics remain intact.
Source guards preserve content-free stale/deletion reasons. Zero document hits
remain unknown unless the permitted corpus is positively established empty or
present. SQL fallback preserves results and semantic limitations independently.

Read-only, bounded PostgreSQL observations recover auxiliary job backlog and
build/verified/active index states after restart. Owner Doctor reports scope-check
generation metadata and retain existing generation evidence separately from
unverified assistant quality. They neither run inference nor enable legacy
workers. Configured pipelines/readers cannot advertise unobserved liveness as
healthy. Missing/expired checks and failed necessary stores affect aggregation.

An independently reviewed continuation can reuse original retrieval captures
after a failed native prefix. It must preserve the complete original scored
matrix, paid failures, all charges and original absolute expiry; only admission
of unpaid inputs is allowed. Rebase current source/task authority through the
Catalog, without replaying old receipts as new execution or repeating inference.
Source/binary/original-byte lineage and composite ledger correlation are required.

A separate explicit owner mandate may authorize a new prospective window after
expiry. MemoryRenewal binds the new source/binary/window, exact ordered unpaid
input digest, original parent pins and accumulated spend. The current renewal is
limited to five remaining inputs and at most30minutes. It neither edits old
expiries nor retries any consumed observation. Current leases/source guards are
new receipts. Clean retained provider failures remain in the unchanged scored
denominator; they do not block admission of the next distinct authorized input.
Authority, protocol or retention failures still seal collection immediately.

## Prospective adaptive consumer (#87)

The default-off bounded read plan, metadata-only opened receipts, current source
guards and independent adoption/NO_GO protocol are specified in
[adaptive memory retrieval](adaptive-memory-retrieval.md). This does not qualify
the consumer or activate main; current scoped accepted preferences remain loaded.

Retrieved document text remains eligible when the source has no title. The
final prompt formatter uses a neutral Document label; an absent title must not
silently discard an already authorized, selected fragment. Empty evidence text
is still omitted. Source provenance and currentness checks remain mandatory.

Standalone ordinary run source observations select no additional roots and bind
no new data. The token-only current authority/source/temporal proof retains its
row locks until explicit bounded rollback; there is no observer mutation to
commit. The foreground provider source guard allows at most two seconds for a
complete proof; trusted explicit bounds may retain200ms..2s. Parent deadlines
and cancellation clamp it. Full pre-HTTP/publication owning transactions remain
mandatory. Trusted compound owner/auxiliary-checker contexts
retain their existing commit semantics. Optional fixed phase/pool diagnostics
must omit source text, SQL parameters, raw errors and credential material.

The active experience adapter additionally verifies current Broker contracts and
fresh owning applicability after that standalone Catalog proof. When its store
is installed it declares the existing trusted compound2s ceiling; without that
store it preserves the default2s or explicit inherited bound. Invalid inherited bounds
remain invalid. All final/tool DATA is held through the fresh proof, with the
parent deadline, Stop and cancellation retained. No Broker RPC runs under SQL
locks. Background supervisors retain their separate200ms proof deadline; the
foreground250ms observation interval is unchanged. This
does not establish a latency SLO. See the episodic experience contract.
