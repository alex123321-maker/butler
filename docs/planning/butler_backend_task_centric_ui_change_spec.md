# Butler — спецификация изменений backend (бэкенда) под новый task-centric web UI (веб-интерфейс вокруг задач)

## 1. Назначение документа

Этот документ описывает не абстрактный backend (бэкенд) для нового интерфейса, а **изменение уже существующей системы Butler** в состояние, необходимое для нового web UI (веб-интерфейса).

Цель документа:

- зафиксировать текущее состояние Butler как исходную точку;
- показать разрыв между текущей системой и целевым task-centric UI (интерфейсом вокруг задач);
- описать минимально достаточные изменения backend (бэкенда), чтобы новый фронтенд мог быть реализован без переписывания платформы с нуля;
- дать AI-кодеру понятный план эволюции системы.

Документ не заменяет архитектурные спекы Butler. Он является **прикладной спецификацией миграции текущего backend (бэкенда)** под новый интерфейс прозрачности и контроля.

---

## 2. Исходная система: что уже есть сейчас

### 2.1. Общая архитектура

Текущий Butler — это monorepo (монорепозиторий) с Go-сервисами, Nuxt web UI (веб-интерфейсом на Nuxt), PostgreSQL, Redis, gRPC-контрактами и Docker Compose (оркестрацией через Docker Compose).

На текущем этапе система уже включает:

- orchestrator (оркестратор) как основной backend (бэкенд);
- Session Service (сервис сессий) внутри orchestrator (оркестратора);
- run lifecycle (жизненный цикл выполнения);
- transcript storage (хранение транскрипта);
- memory pipeline (конвейер памяти);
- Telegram adapter (адаптер Telegram);
- Tool Broker (брокер инструментов);
- doctor/runtime/settings/provider auth flows (диагностику, настройки и авторизацию провайдеров);
- базовый Nuxt web shell (веб-каркас на Nuxt) с view endpoints (эндпоинтами просмотра) для sessions/runs/transcript/memory/doctor/settings.

### 2.2. Главный пользовательский поток

Сейчас реальный основной поток выглядит так:

1. Пользователь пишет в Telegram.
2. Telegram adapter (адаптер Telegram) нормализует update (обновление) в `InputEvent`.
3. Orchestrator (оркестратор) создаёт или находит session (сессию).
4. Создаётся run (выполнение).
5. Run проходит state machine (машину состояний): `queued -> acquired -> preparing -> model_running -> ...`.
6. Transcript (транскрипт) и working memory (рабочая память) сохраняются.
7. Partial/final delivery (частичная/финальная доставка ответа) уходит обратно в Telegram.
8. Post-run memory pipeline (послеруновый конвейер памяти) запускается асинхронно.

### 2.3. Текущая публичная модель backend (бэкенда) для web UI

Сейчас web UI (веб-интерфейс) в основном работает с такими сущностями:

- `session`
- `run`
- `transcript`
- `memory scope`
- `doctor report`
- `settings`
- `provider auth state`

То есть текущий UI и текущие read models (модели чтения) backend (бэкенда) по сути **сессие- и run-центричны**, а не task-centric (ориентированы не на задачу как продуктовую сущность, а на низкоуровневый жизненный цикл выполнения).

### 2.4. Что уже хорошо и должно быть сохранено

Новый UI не требует уничтожать текущую архитектуру. Наоборот, в текущей системе уже есть сильный фундамент:

- явная state machine (машина состояний) run;
- Telegram как первичный канал;
- transcript persistence (надёжное хранение транскрипта);
- memory model (модель памяти);
- doctor/settings/provider endpoints (эндпоинты диагностики, настроек и провайдеров);
- tool loop (цикл работы инструментов);
- approval state (состояние подтверждения) в lifecycle (жизненном цикле);
- working/transient memory (рабочая и временная память);
- CORS и web API (веб API) для Nuxt UI (интерфейса на Nuxt).

Именно поэтому целевая стратегия — **не переписать Butler**, а надстроить поверх существующего run-centric core (ядра вокруг выполнений) новый task-centric presentation layer (слой представления вокруг задач).

---

## 3. Что есть сейчас в API

### 3.1. Уже существующие REST endpoints (REST-эндпоинты)

Сейчас orchestrator (оркестратор) уже публикует такие HTTP endpoints (HTTP-эндпоинты):

- `GET /health`
- `GET /metrics`
- `POST /api/v1/events`
- `GET /api/v1/settings`
- `PUT /api/v1/settings/{key}`
- `DELETE /api/v1/settings/{key}`
- `GET /api/v1/settings/restart`
- `POST /api/v1/settings/restart`
- `GET /api/v1/providers`
- provider auth endpoints (эндпоинты авторизации провайдеров)
- `GET /api/v1/memory`
- `GET /api/v1/sessions`
- `GET /api/v1/sessions/{key}`
- `GET /api/v1/runs/{id}`
- `GET /api/v1/runs/{id}/transcript`
- `GET /api/v1/doctor/reports`
- `POST /api/v1/doctor/check`

### 3.2. Что реально использует текущий web UI (веб-интерфейс)

Текущий web UI (веб-интерфейс) уже ходит в backend (бэкенд) за:

- health check (проверкой здоровья системы);
- sessions list (списком сессий);
- session detail (деталями сессии);
- run detail (деталями выполнения);
- run transcript (транскриптом выполнения);
- memory browser (браузером памяти);
- doctor reports (диагностическими отчётами);
- settings и provider auth (настройками и авторизацией провайдеров).

### 3.3. Что важно понять про текущую модель

Текущий API уже достаточен для:

- технической прозрачности;
- просмотра run history (истории выполнений);
- диагностики;
- просмотра памяти;
- управления настройками.

Но он **ещё недостаточен** для нового интерфейса, потому что новый интерфейс хочет показывать:

- product-level task (задачу продуктового уровня), а не только run;
- approvals (подтверждения) как самостоятельные объекты;
- artifacts (артефакты) как самостоятельные объекты;
- activity/audit (активность и аудит) как самостоятельный поток;
- richer system overview (более богатую системную сводку);
- memory management (управление памятью), а не только просмотр;
- состояния «ждёт подтверждения в web UI (веб-интерфейсе)» и «ждёт ответа в Telegram» как явные UX-статусы, а не просто внутренние состояния исполнения.

---

## 4. Ключевой разрыв между текущей системой и целевым интерфейсом

## 4.1. Сейчас основная публичная сущность — session/run

Это хорошо для инженерного наблюдения, но плохо для продуктового UX (пользовательского опыта).

Пользователь нового интерфейса мыслит не так:

- «я хочу посмотреть session (сессию)»;
- «я хочу открыть raw transcript (сырой транскрипт)».

Он мыслит так:

- какая у меня была задача;
- что по ней происходит;
- требуется ли от меня действие;
- какой получился результат;
- что пошло не так;
- что система по этому поводу запомнила.

### 4.2. Сейчас approval flow (поток подтверждений) не является полноценной durable entity (долговечной сущностью)

С точки зрения lifecycle (жизненного цикла) approval (подтверждение) уже существует.
Но по факту текущая реализация держит pending approvals (ожидающие подтверждения) через in-memory ApprovalGate (внутрипроцессную память), а Telegram callback (обратный вызов из Telegram) просто резолвит ожидание по `tool_call_id`.

Это даёт рабочий Telegram UX (пользовательский опыт в Telegram), но **не даёт backend-модели**, на которую можно опереться для полноценного раздела Approvals в web UI (веб-интерфейсе).

### 4.3. Сейчас нет самостоятельной сущности artifact (артефакта)

Итоговые outputs (результаты) есть, но они хранятся фрагментарно:

- как assistant final message (финальное сообщение ассистента) в transcript (транскрипте);
- как doctor report (диагностический отчёт);
- как tool result (результат инструмента) в transcript tool calls (вызовах инструмента в транскрипте).

Новой UI-модели нужен отдельный слой `artifact`.

### 4.4. Сейчас нет activity feed (ленты активности) и audit-oriented read model (модели чтения для аудита)

Есть structured logging (структурированные логи), transcript (транскрипт), doctor reports (диагностические отчёты), credential audit (аудит учётных данных), но **нет единого activity endpoint (эндпоинта активности)** и нет нормализованной user-facing activity model (модели активности, пригодной для пользователя).

### 4.5. Сейчас memory endpoint (эндпоинт памяти) — только read-only (только для чтения)

Новый UI требует не только browse (просмотр), но и:

- confirm (подтверждение);
- reject (отклонение);
- edit (редактирование);
- delete (удаление);
- hide/disable/use policy flags (флаги скрытия, запрета и политики использования).

### 4.6. Сейчас system overview (системная сводка) фрагментирован

Есть:

- `GET /health`
- doctor reports (диагностические отчёты)
- settings/provider state (настройки и состояние провайдеров)

Но нет одного нормального `System` API (системного API) для UI-страницы `System` и overview-блоков (сводных блоков на главной).

---

## 5. Целевой принцип изменений

## 5.1. Не переписывать execution core (ядро исполнения)

Нельзя делать вид, что текущего Butler не существует. Основная ошибка была бы такой:

- ввести новую «правильную» сущность task (задачи);
- начать заново переписывать orchestrator (оркестратор), Telegram, memory pipeline (конвейер памяти), tool loop (цикл инструментов);
- получить огромный параллельный backend (бэкенд), дублирующий run lifecycle (жизненный цикл выполнения).

Это неверно.

### 5.2. Правильный подход

Правильный подход:

- **ядро исполнения остаётся run-centric (вокруг выполнений)**;
- **публичная UI-модель становится task-centric (вокруг задач)**;
- task (задача) на первом этапе — это не новая независимая бизнес-сущность, а **UI/read-model overlay (слой представления/модель чтения) над текущим run**.

Иными словами:

- внутренняя execution unit (единица исполнения) по-прежнему `run`;
- внешняя UI-сущность для пользователя — `task`.

### 5.3. На первом этапе: `Task = primary run-centric view (основное представление выполнения)`

На первом этапе не нужно invent fully separate task engine (изобретать отдельный движок задач).

Нужно сделать так:

- новый UI говорит термином `task`;
- backend (бэкенд) собирает `task` из уже существующих данных `run + session + transcript + memory + delivery + approval + doctor/tool outputs`.

Это даст:

- минимальный разрыв с текущей системой;
- быстрый путь к новому UI;
- меньше риска сломать работающий Telegram flow (поток через Telegram).

---

## 6. Что нужно изменить в backend (бэкенде) обязательно

# 6.1. Добавить новый task-centric read API (модель чтения вокруг задач)

Нужен **новый набор UI-oriented endpoints (ориентированных на интерфейс эндпоинтов)**.

Рекомендуемый путь:

- не ломать старые `sessions/runs/transcript` endpoints (эндпоинты);
- оставить их как low-level/debug endpoints (низкоуровневые/отладочные эндпоинты);
- для нового UI ввести новый слой, например:
  - `/api/v2/overview`
  - `/api/v2/tasks`
  - `/api/v2/tasks/{id}`
  - `/api/v2/tasks/{id}/activity`
  - `/api/v2/tasks/{id}/artifacts`
  - `/api/v2/approvals`
  - `/api/v2/artifacts`
  - `/api/v2/activity`
  - `/api/v2/system`
  - `/api/v2/memory/...`

Почему `v2`:

- потому что меняется не просто shape (форма) ответа, а сама публичная модель интерфейса;
- `v1` уже фактически означает technical operator views (технические представления оператора) вокруг sessions/runs.

# 6.2. Добавить нормализованную сущность task (задачи) как read model (модель чтения)

Каждый task (задача) должен содержать минимум:

- `task_id` — на первом этапе равен `run_id`;
- `run_id`
- `session_key`
- `source_channel` (`telegram`)
- `source_message_preview`
- `source_message_full`
- `status` — продуктовый статус для UI;
- `run_state` — исходное внутреннее состояние run;
- `current_stage`
- `needs_user_action`
- `user_action_channel` (`web`, `telegram`, `none`)
- `waiting_reason`
- `started_at`
- `updated_at`
- `finished_at`
- `model_provider`
- `autonomy_mode`
- `outcome_summary`
- `error_summary`
- `risk_level`
- `telegram_delivery_state`
- `has_approvals`
- `has_artifacts`
- `memory_scope_refs`

Важно:

`status` для UI не должен слепо повторять `run_state`.

Нужен mapping layer (слой преобразования), например:

- `created/queued/acquired/preparing/model_running/tool_pending/tool_running/awaiting_model_resume` -> `in_progress`
- `awaiting_approval` + approval available in web -> `waiting_for_approval`
- `awaiting_approval` + approval only in telegram -> `waiting_for_reply_in_telegram`
- `completed` -> `completed`
- `failed` -> `failed`
- `cancelled` -> `cancelled`
- `timed_out` -> `completed_with_issues` или `failed` в зависимости от policy (политики)

# 6.3. Добавить task overview endpoint (сводный эндпоинт задач)

Нужен endpoint (эндпоинт), который отдаёт overview data (сводные данные) для главной страницы.

Пример состава ответа:

- `attention_items`
- `active_tasks`
- `recent_results`
- `system_summary`
- `approvals_pending_count`
- `tasks_waiting_for_telegram_reply_count`
- `failed_tasks_count`
- `degraded_services_count`

Этот endpoint (эндпоинт) должен быть собран на backend (бэкенде), а не на клиенте, чтобы UI не склеивал 6–8 разных API-вызовов в одну бизнес-сводку.

# 6.4. Добавить полноценную persistence model (модель хранения) для approvals (подтверждений)

Это одно из самых важных изменений.

Сейчас approval (подтверждение) фактически живёт как:

- internal lifecycle state (внутреннее состояние жизненного цикла);
- in-memory wait (ожидание в памяти процесса);
- Telegram callback resolution (резолв через callback из Telegram).

Для нового UI этого недостаточно.

Нужна отдельная durable entity (долговечная сущность), например таблица `approvals`.

Минимальные поля:

- `approval_id`
- `run_id`
- `session_key`
- `tool_call_id`
- `status` (`pending`, `approved`, `rejected`, `expired`, `failed`)
- `requested_via` (`telegram`, `web`, `both`)
- `resolved_via` (`telegram`, `web`, `system`)
- `tool_name`
- `args_json`
- `risk_level`
- `summary`
- `details_json`
- `requested_at`
- `resolved_at`
- `resolved_by`
- `resolution_reason`
- `expires_at`

Изменение логики:

- перед `ApprovalGate.Wait()` backend (бэкенд) обязан создать durable approval record (долговечную запись подтверждения);
- Telegram adapter (адаптер Telegram) и будущий web approval endpoint (веб-эндпоинт подтверждения) должны резолвить **не только in-memory gate (ожидание в памяти)**, но и persistent approval record (запись подтверждения в хранилище);
- UI должен читать approvals только из durable storage (долговечного хранилища).

# 6.5. Добавить web approval actions (действия подтверждения из web UI)

Нужны endpoints (эндпоинты):

- `GET /api/v2/approvals`
- `GET /api/v2/approvals/{id}`
- `POST /api/v2/approvals/{id}/approve`
- `POST /api/v2/approvals/{id}/reject`

Ключевое требование:

web approval (подтверждение из веб-интерфейса) должно проходить через тот же orchestration path (путь оркестрации), что и Telegram approval (подтверждение из Telegram), а не через отдельный «хак» на фронтенде.

То есть backend (бэкенд) должен иметь единый approval resolution service (сервис резолва подтверждений), который:

- валидирует статус;
- пишет audit trail (аудит);
- обновляет approval record (запись подтверждения);
- резолвит in-memory waiter (ожидание в памяти), если он существует;
- корректно обрабатывает дубликаты и гонки.

# 6.6. Добавить artifacts (артефакты) как first-class objects (первоклассные объекты)

Нужна отдельная сущность `artifact`.

Сейчас у нас уже есть несколько классов данных, которые можно превратить в artifacts (артефакты):

- финальный ответ ассистента;
- doctor report (диагностический отчёт);
- важные tool outputs (результаты инструментов);
- возможно session summary (сводка сессии);
- структурированные выводы из tool calls (вызовов инструментов);
- ссылки и списки ресурсов.

Минимальная таблица `artifacts`:

- `artifact_id`
- `run_id`
- `session_key`
- `artifact_type` (`assistant_final`, `doctor_report`, `tool_result`, `summary`, `report`, `links`, `structured_output`)
- `title`
- `summary`
- `content_text`
- `content_json`
- `content_format`
- `source_type`
- `source_ref`
- `created_at`

Что важно:

- не все transcript messages (сообщения транскрипта) должны стать artifacts (артефактами);
- artifact (артефакт) — это **результат, полезный для повторного доступа**, а не просто событие.

# 6.7. Добавить task activity / audit model (модель активности и аудита задачи)

Нужна отдельная сущность activity (активности), не равная transcript (транскрипту).

Transcript (транскрипт) отвечает на вопрос:

- кто что сказал или какой tool result (результат инструмента) был записан.

Activity feed (лента активности) отвечает на вопрос:

- что происходило с задачей как с процессом.

Нужна таблица, например `task_activity`:

- `activity_id`
- `run_id`
- `session_key`
- `activity_type`
- `title`
- `summary`
- `details_json`
- `actor_type` (`system`, `agent`, `user`, `telegram_adapter`, `web_ui`)
- `created_at`
- `severity` (`info`, `warning`, `error`)

Примеры activity types (типов активности):

- `task_received`
- `lease_acquired`
- `context_prepared`
- `model_started`
- `tool_requested`
- `approval_requested`
- `approval_resolved`
- `tool_completed`
- `tool_failed`
- `assistant_stream_started`
- `assistant_final_delivered`
- `memory_pipeline_enqueued`
- `memory_pipeline_completed`
- `task_completed`
- `task_failed`

Именно эта сущность должна кормить:

- вкладку `Timeline`;
- вкладку `Activity`;
- блоки «последние действия» на overview (главной).

# 6.8. Добавить richer task detail endpoint (более богатый эндпоинт деталей задачи)

Нужен endpoint (эндпоинт), который отдаёт **нормализованную карточку задачи**, а не только `run`.

Пример:

`GET /api/v2/tasks/{id}`

Ответ должен содержать разделы:

- `task`
- `summary_bar`
- `source`
- `status`
- `current_stage`
- `waiting_state`
- `result`
- `error`
- `artifacts`
- `approvals`
- `memory_refs`
- `timeline_preview`
- `debug_refs`

Важно:

`Task detail (детали задачи)` должен собираться на backend (бэкенде) из нескольких слоёв:

- run record (записи выполнения);
- run metadata (метаданных выполнения);
- transcript (транскрипта);
- activity records (записей активности);
- approvals (подтверждений);
- artifacts (артефактов);
- memory references (ссылок на память);
- delivery state (состояния доставки в канал).

# 6.9. Сохранить transcript/debug endpoints (эндпоинты транскрипта/отладки), но понизить их роль

Старые endpoints (эндпоинты):

- `GET /api/v1/runs/{id}`
- `GET /api/v1/runs/{id}/transcript`

нужно оставить.

Но в новой системе они должны считаться:

- debug endpoints (отладочными эндпоинтами);
- low-level diagnostics (низкоуровневой диагностикой);
- источником для вкладки `Debug`, но не для основной UX-модели.

# 6.10. Добавить memory write API (API записи в память)

Текущий memory endpoint (эндпоинт памяти) пригоден только для просмотра.

Для нового UI нужно минимум:

- `GET /api/v2/memory`
- `GET /api/v2/memory/{id}`
- `PATCH /api/v2/memory/{id}`
- `DELETE /api/v2/memory/{id}`
- `POST /api/v2/memory/{id}/confirm`
- `POST /api/v2/memory/{id}/reject`

Тут нельзя делать «лёгкий фронтовый хак». Нужна нормальная доменная политика:

- какие memory types (типы памяти) редактируемы;
- как хранится confirmation state (состояние подтверждения);
- как помечаются inferred entries (выведенные записи);
- как отличать soft delete (мягкое удаление) от hard delete (жёсткого удаления).

Рекомендуемый подход:

- profile/episodic/chunk memory (профильная/эпизодическая/фрагментная память) редактируются через status/override model (модель статусов и переопределений), а не прямым неконтролируемым переписыванием сырых исторических записей;
- UI работает с current effective memory view (актуальным эффективным представлением памяти), а backend (бэкенд) сохраняет audit trail (аудит).

# 6.11. Добавить system overview endpoint (системный сводный эндпоинт)

Нужен endpoint (эндпоинт), который объединит:

- health summary (сводку здоровья системы);
- latest doctor status (последний статус диагностики);
- provider connectivity (связность провайдеров);
- queue/backlog stats (метрики очередей и бэклога);
- failing integrations (ошибающиеся интеграции);
- stale pending approvals (зависшие подтверждения);
- runs in failed/timed_out states (выполнения в состояниях ошибки/таймаута).

Пример:

`GET /api/v2/system`

Важно:

Это не должен быть просто прокси к `GET /health` и `GET /api/v1/doctor/reports`.
Это должна быть **агрегированная операторская сводка**.

# 6.12. Добавить activity endpoint (эндпоинт активности) глобального уровня

Нужен endpoint (эндпоинт), который возвращает cross-system activity feed (общесистемную ленту активности).

Пример:

`GET /api/v2/activity`

Фильтры:

- `entity_type`
- `severity`
- `actor_type`
- `run_id`
- `session_key`
- `since`
- `until`

Он нужен для страницы `Activity` и частично для overview (главной).

# 6.13. Добавить transport for UI live updates (транспорт для живых обновлений интерфейса)

Для нового UI желательно иметь push transport (push-транспорт), а не только polling (опрос).

Рекомендуемый минимальный вариант:

- `SSE (Server-Sent Events — серверные события)`

Почему не сразу `WebSocket (веб-сокеты)`:

- UI не является чатом;
- основная потребность — подписка на обновления задач, approvals (подтверждений), system status (системного статуса), activity (активности);
- `SSE (серверные события)` проще и дешевле по реализации на текущем этапе.

Пример:

- `GET /api/v2/stream?topics=overview,tasks,approvals,system`

События:

- `task.updated`
- `approval.created`
- `approval.resolved`
- `artifact.created`
- `memory.updated`
- `system.updated`
- `activity.created`

Это не блокер для самого первого экрана, но очень желательно для хорошего UX.

---

## 7. Какие изменения в текущем backend (бэкенде) НЕ нужны

## 7.1. Не нужен новый task execution engine (движок исполнения задач)

Нельзя заводить отдельную state machine (машину состояний) task (задачи), которая живёт независимо от run.

На первом этапе это создаст:

- дублирование статусов;
- гонки состояния;
- рост сложности;
- вторую правду вместо одной.

### 7.2. Не нужно убирать sessions/runs/transcript

Они всё ещё полезны:

- для debug (отладки);
- для инженерной диагностики;
- как внутренняя истина исполнения.

### 7.3. Не нужно ломать Telegram-first model (модель с Telegram как точкой входа)

Новый backend (бэкенд) не должен открывать путь к созданию задач из web UI (веб-интерфейса).

Даже если появятся новые UI endpoints (эндпоинты интерфейса), они должны быть:

- read-heavy (ориентированными на чтение);
- approval/memory/settings oriented (ориентированными на подтверждения, память и настройки);
- но не task creation endpoints (не эндпоинтами создания задач).

### 7.4. Не нужно срочно переводить ingestion (приём событий) в асинхронную очередь ради нового UI

Текущий synchronous execution path (синхронный путь исполнения) — ограничение, но не главный блокер для нового task-centric UI (интерфейса вокруг задач).

Для нового UI критичнее:

- durable approvals (долговечные подтверждения);
- artifacts (артефакты);
- activity feed (лента активности);
- overview aggregation (агрегация данных для главной);
- task read model (модель чтения задач).

Асинхронный event ingestion (приём событий) можно оставить как отдельный следующий шаг.

---

## 8. Предлагаемые новые таблицы и изменения схемы

## 8.1. Новые таблицы

### `approvals`

Назначение: durable approval storage (долговечное хранение подтверждений).

### `approval_events`

Назначение: audit trail (аудит) по подтверждениям.

### `artifacts`

Назначение: first-class outputs (результаты как первоклассные объекты).

### `task_activity`

Назначение: нормализованная timeline/activity model (модель таймлайна и активности).

### `delivery_events` или `channel_delivery_events`

Назначение: явная фиксация доставок в Telegram/web channels (каналы Telegram/веб).

Это нужно, чтобы UI мог честно показывать:

- ответ отправлен в Telegram;
- ожидание ответа пользователя именно в Telegram;
- approval request (запрос подтверждения) отправлен;
- final response (финальный ответ) доставлен;
- delivery failed (доставка не удалась).

## 8.2. Возможные расширения существующих таблиц

### Таблица `runs`

Можно добавить либо реальные колонки, либо derived fields (вычисляемые поля через read model):

- `source_channel`
- `source_message_preview`
- `needs_user_action`
- `user_action_channel`
- `outcome_summary`
- `risk_level`
- `ui_status`

Мой рекомендуемый путь:

- не раздувать таблицу `runs` слишком рано;
- оставлять внутреннюю таблицу `runs` execution-centric (ориентированной на исполнение);
- а UI-поля собирать в read model layer (слое моделей чтения).

### Таблица `messages` / `tool_calls`

Их не нужно ломать. Они остаются базой для debug/transcript view (представления транскрипта и отладки).

### Таблица memory stores (хранилищ памяти)

Понадобятся дополнительные поля или связанная таблица для:

- `confirmation_state`
- `effective_status`
- `edited_by_user`
- `suppressed`
- `expires_at`

Но это нужно вводить аккуратно, чтобы не разрушить текущую модель provenance (происхождения данных).

---

## 9. Новый API-каталог для фронтенда

Ниже — рекомендуемый минимальный API-каталог для нового UI.

## 9.1. Overview

### `GET /api/v2/overview`

Возвращает:

- `attention_items`
- `active_tasks`
- `recent_results`
- `system_summary`
- `counts`

## 9.2. Tasks

### `GET /api/v2/tasks`

Фильтры:

- `status`
- `source_channel`
- `needs_user_action`
- `waiting_reason`
- `provider`
- `from`
- `to`
- `query`
- `limit`
- `offset`
- `sort`

### `GET /api/v2/tasks/{id}`

Возвращает нормализованную детальную карточку задачи.

### `GET /api/v2/tasks/{id}/activity`

Возвращает timeline/activity (таймлайн/активность).

### `GET /api/v2/tasks/{id}/artifacts`

Возвращает связанные artifacts (артефакты).

### `GET /api/v2/tasks/{id}/debug`

Возвращает ссылки или low-level payloads (низкоуровневые полезные нагрузки):

- `run`
- `transcript`
- `tool_calls`
- `provider_session_ref`
- `raw states`

## 9.3. Approvals

### `GET /api/v2/approvals`

### `GET /api/v2/approvals/{id}`

### `POST /api/v2/approvals/{id}/approve`

### `POST /api/v2/approvals/{id}/reject`

## 9.4. Artifacts

### `GET /api/v2/artifacts`

Фильтры:

- `type`
- `run_id`
- `session_key`
- `query`

### `GET /api/v2/artifacts/{id}`

## 9.5. Memory

### `GET /api/v2/memory`

Поддерживает:

- `scope_type`
- `scope_id`
- `memory_type`
- `status`
- `confirmation_state`
- `query`

### `GET /api/v2/memory/{id}`

### `PATCH /api/v2/memory/{id}`

### `DELETE /api/v2/memory/{id}`

### `POST /api/v2/memory/{id}/confirm`

### `POST /api/v2/memory/{id}/reject`

## 9.6. Activity

### `GET /api/v2/activity`

Возвращает глобальную ленту активности.

## 9.7. System

### `GET /api/v2/system`

Возвращает:

- `health`
- `doctor`
- `providers`
- `queues`
- `degraded_components`
- `pending_approvals`
- `recent_failures`

## 9.8. Stream

### `GET /api/v2/stream`

Формат: `SSE (Server-Sent Events — серверные события)`

---

## 10. Изменения по сервисным слоям

## 10.1. Orchestrator service layer (сервисный слой оркестратора)

Нужно добавить:

- approval persistence hooks (хуки записи подтверждений);
- artifact creation hooks (хуки создания артефактов);
- activity event emission (эмиссию событий активности);
- delivery event emission (эмиссию событий доставки);
- UI status derivation helpers (вспомогательные преобразования статусов для интерфейса).

Важно:

Эти изменения должны быть **добавочными**, а не переписывающими текущую execution logic (логику исполнения).

## 10.2. API layer (слой API)

Нужно добавить новый пакет view/read handlers (обработчиков чтения), отдельно от текущих low-level handlers (низкоуровневых обработчиков):

- `overview.go`
- `tasks.go`
- `approvals.go`
- `artifacts.go`
- `activity.go`
- `system.go`
- `memory_write.go`
- `stream.go`

## 10.3. Persistence layer (слой хранения)

Нужно добавить:

- approval repository (репозиторий подтверждений);
- artifact repository (репозиторий артефактов);
- activity repository (репозиторий активности);
- delivery events repository (репозиторий событий доставки);
- memory command service (командный сервис для записи в память).

---

## 11. Как меняются существующие разделы UI с точки зрения backend (бэкенда)

## 11.1. Overview

Сейчас:

- собирается только из health + placeholder cards (заглушек карточек).

Нужно:

- отдельный aggregated endpoint (агрегированный эндпоинт).

## 11.2. Sessions

Сейчас:

- самостоятельный первичный экран.

Нужно:

- оставить как debug/operator view (представление для отладки и оператора), но убрать из центральной продуктовой модели.

## 11.3. Run detail

Сейчас:

- главный детальный экран технического уровня.

Нужно:

- сохранить как debug tab (вкладку отладки);
- основной task detail (детали задачи) строить отдельно.

## 11.4. Memory

Сейчас:

- read-only browser (браузер только для чтения).

Нужно:

- полноценное управление памятью.

## 11.5. Doctor

Сейчас:

- уже достаточно сильный раздел.

Нужно:

- интегрировать его в `System` и `Overview`, но сам раздел можно сохранить почти без больших концептуальных изменений.

## 11.6. Settings

Сейчас:

- уже один из наиболее зрелых разделов.

Нужно:

- оставить;
- возможно упростить представление под новый дизайн;
- backend (бэкенд) в этой части менять минимально.

---

## 12. Пошаговый план внедрения

## Phase 1 (Этап 1) — безболезненный переход на новый UI-read layer (слой чтения для нового интерфейса)

Сделать:

1. `api/v2/tasks`
2. `api/v2/tasks/{id}`
3. `api/v2/overview`
4. `api/v2/system`
5. `api/v2/activity`

На этом этапе:

- approvals (подтверждения) ещё можно частично читать из текущих состояний и из новых event logs (журналов событий),
- artifacts (артефакты) можно собирать минимально из assistant final и doctor reports (финальных ответов ассистента и doctor reports).

## Phase 2 (Этап 2) — durable approvals and artifacts (долговечные подтверждения и артефакты)

Сделать:

1. таблицу `approvals`
2. web approval actions (действия подтверждения из веба)
3. таблицу `artifacts`
4. нормальную связь task <-> approvals <-> artifacts

Это критический этап для нового UX.

## Phase 3 (Этап 3) — memory write API and full activity model (API записи в память и полная модель активности)

Сделать:

1. memory write endpoints (эндпоинты записи в память)
2. audit-safe edit model (безопасную с точки зрения аудита модель редактирования)
3. полноценную activity table (таблицу активности)
4. глобальный activity feed (ленту активности)

## Phase 4 (Этап 4) — live updates (живые обновления)

Сделать:

1. `SSE (Server-Sent Events — серверные события)`
2. подписки на tasks/approvals/system/activity (задачи/подтверждения/систему/активность)

---

## 13. Главные продуктовые правила, которые backend (бэкенд) обязан защищать

## 13.1. Web UI не создаёт задачи

Backend (бэкенд) не должен вводить endpoints (эндпоинты), через которые новый web UI начнёт создавать пользовательские задачи.

## 13.2. Telegram остаётся источником пользовательских запросов

Даже если в будущем появятся internal actions (внутренние действия), пользовательские task requests (запросы задач) по-прежнему должны идти через Telegram.

## 13.3. Approval в web UI — это контроль, а не параллельный канал постановки задач

Подтверждение действия допустимо.
Создание новой задачи из веба — нет.

## 13.4. Debug data (отладочные данные) должны оставаться доступны, но не быть основной моделью UI

Backend (бэкенд) должен одновременно поддерживать:

- operator-friendly technical views (дружественные оператору технические представления);
- user-friendly task-centric views (дружественные пользователю представления вокруг задач).

---

## 14. Итоговое архитектурное решение

Целевое решение выглядит так:

### Слой 1. Execution core (ядро исполнения)

Остаётся почти без концептуальных изменений:

- Session Service (сервис сессий)
- Run lifecycle (жизненный цикл выполнения)
- Orchestrator (оркестратор)
- Tool Broker (брокер инструментов)
- Memory pipeline (конвейер памяти)
- Telegram adapter (адаптер Telegram)

### Слой 2. Durable UI support model (долговечная модель поддержки интерфейса)

Добавляется:

- `approvals`
- `artifacts`
- `task_activity`
- `delivery_events`
- memory write model (модель записи в память)

### Слой 3. UI-oriented read API (ориентированный на интерфейс API чтения)

Добавляется:

- `overview`
- `tasks`
- `approvals`
- `artifacts`
- `activity`
- `system`
- `memory v2`
- `stream`

### Главный принцип

**Внутренняя правда исполнения остаётся run-centric (вокруг выполнений), но внешний backend contract (контракт бэкенда) для нового web UI становится task-centric (вокруг задач).**

Это даёт минимально рискованный путь миграции, совместимый с уже существующим Butler.

---

## 15. Что делать AI-кодеру в первую очередь

Порядок работы должен быть таким:

1. Не трогать текущие `v1` endpoints (эндпоинты) без необходимости.
2. Добавить новые `v2` read endpoints (эндпоинты чтения) под новый UI.
3. Ввести durable approvals (долговечные подтверждения).
4. Ввести artifacts (артефакты).
5. Ввести task activity (активность задачи).
6. После этого расширять memory write API (API записи в память).
7. Только потом добавлять `SSE (Server-Sent Events — серверные события)`.

Если делать наоборот и начинать, например, с `SSE (серверных событий)` или полной переплавки run model (модели выполнения), проект сильно усложнится без продуктовой отдачи.

---

## 16. Traceability (прослеживаемость)

Этот документ основан на изучении текущих исходников и архитектурных документов Butler, в частности:

- корневой `README.md`
- `apps/orchestrator/README.md`
- `apps/orchestrator/internal/app/app.go`
- `apps/orchestrator/internal/api/server.go`
- `apps/orchestrator/internal/api/views.go`
- `apps/orchestrator/internal/api/memory.go`
- `apps/orchestrator/internal/api/settings.go`
- `apps/orchestrator/internal/orchestrator/service.go`
- `apps/orchestrator/internal/orchestrator/approval.go`
- `apps/orchestrator/internal/channel/telegram/adapter.go`
- `docs/architecture/run-lifecycle-spec.md`
- `docs/planning/butler-implementation-roadmap.md`
- `web/README.md`
- `web/composables/useApi.ts`
- текущие страницы `web/pages/*`

То есть документ описывает **эволюцию реальной текущей системы**, а не выдуманный backend (бэкенд) с чистого листа.
