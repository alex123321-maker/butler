# Butler — Memory Model Specification

## 1. Статус документа

**Тип документа:** Architecture Subspec / Memory Model
**Версия:** 0.1
**Статус:** Draft
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
* связана с конкретной session или run;
* не должна автоматически становиться долгосрочной памятью.

#### Два слоя хранения

Working Memory физически разделена на два слоя:

* **Durable layer (PostgreSQL, таблица `memory_working`)** — логический snapshot рабочего состояния задачи: текущая цель, активные сущности, незавершённые шаги. Сохраняется при каждом значимом обновлении состояния и переживает перезапуск системы.
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

#### V1 baseline implemented in Sprint 5

The current Butler baseline now includes durable PostgreSQL tables for:

* `memory_working`
* `memory_profile`
* `memory_episodes`

`memory_profile` is used for structured long-lived facts addressed by `scope_type`, `scope_id`, and `key`.

`memory_episodes` stores completed summaries plus pgvector embeddings so Butler can perform semantic retrieval without treating pgvector as the primary source of truth.

### 8.2 Redis

В Redis рекомендуется хранить:

* transient working memory state (эфемерное рабочее состояние в процессе исполнения, не дублирующее durable snapshot из `memory_working`);
* locks / leases;
* short-lived execution caches;
* active task scratchpads.

**Примечание:** Redis не хранит durable working memory. Логический snapshot рабочего состояния задачи сохраняется в PostgreSQL (`memory_working`). Redis содержит только эфемерный execution state, который допустимо потерять при перезапуске.

### 8.3 pgvector

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
* orchestrator requests that bundle and injects the returned compact system memory prompt before model execution when relevant entries exist.

#### Current hybrid retrieval baseline

Current bundle assembly now combines:

* structured profile lookup by ordered scope;
* session summary and working memory as highest-priority bundle items;
* vector episodic retrieval when embeddings are available;
* lightweight keyword summary matching as an optional fallback / complement when supported by the store;
* explicit bundle-budget ordering across summary, working, profile, and episodic sections.

When embeddings or optional keyword matches are unavailable, bundle assembly degrades gracefully and still returns deterministic summary / working / profile context within budget.

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

Document candidates may be classified and observed even when durable document-chunk persistence is not yet implemented.

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

Working Memory всегда может быть обновлена или очищена без необходимости хранить все её промежуточные версии как долгосрочную память.

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

---

## 20. Открытые решения

1. Точная SQL schema для memory tables.
2. Политика дедупликации episodic memory.
3. Политика сохранения doctor outputs.
4. Формат session summary и правила его обновления.
5. Приоритет retrieval между profile, episodic и document memory.
6. Использовать ли отдельный reranking layer (слой переранжирования) в будущем.

---

## 21. Итоговый тезис

Память Butler должна быть **многослойной, структурированной и управляемой**.
Она не равна ни чату, ни RAG, ни векторной базе.

Правильная модель для Butler:

* Transcript Store фиксирует факты истории;
* Working Memory удерживает текущую задачу;
* Episodic Memory даёт воспроизведение прошлого опыта;
* Profile Memory фиксирует стабильные знания о пользователе и системе;
* pgvector помогает находить релевантное, но не становится единственной правдой о памяти.

Именно такая схема подходит для self-hosted, long-lived personal agent system (долгоживущей персональной агентной системы), которой должен стать Butler.
