# Butler Task-Centric UI — Implementation Backlog

## 1. Назначение

Этот бэклог объединяет задачи из:

- `docs/planning/butler_web_ui_ai_implementation_spec.md`
- `docs/planning/butler_backend_task_centric_ui_change_spec.md`

Цель: дать пошаговый, исполнимый план внедрения нового task-centric Web UI и необходимых backend-изменений без переписывания execution core.

---

## 2. Обязательные инварианты

- Telegram-first сохраняется: Web UI **не** создаёт задачи.
- Внутренняя истина исполнения остаётся run-centric.
- Внешний UI-контракт становится task-centric через новый API.
- Любые новые UI-статусы выводятся из run state через явный mapping layer.
- Web UI остаётся control console, не chat interface.

---

## 3. Формат задач

- **ID** — уникальный идентификатор.
- **Приоритет** — P0 (критично), P1 (важно), P2 (улучшение).
- **Зависимости** — задачи, которые должны быть выполнены раньше.
- **Результат** — конкретный артефакт задачи.
- **Критерии приёмки** — проверяемые условия готовности.

---

## 4. Эпик A — Backend task-centric read layer (api/v2) [✅ Готов]

### A-01: Каркас task-centric API [✅ Выполнено]
- **Приоритет:** P0
- **Зависимости:** нет
- **Результат:** базовый HTTP-роутинг для task-centric endpoints
- **Критерии приёмки:**
  - Добавлены новые endpoints для task-centric UI.
  - Добавлен README/раздел с описанием API-контрактов.

### A-02: Task read model + mapping run state -> ui status [✅ Выполнено]
- **Приоритет:** P0
- **Зависимости:** A-01
- **Результат:** доменная модель `task` для списка/деталей
- **Критерии приёмки:**
  - Реализован mapping layer из run lifecycle в UI-статусы.
  - Модель включает поля: `task_id`, `run_id`, `session_key`, `status`, `run_state`, `needs_user_action`, `user_action_channel`, `waiting_reason`, `started_at/updated_at/finished_at`, `outcome_summary`, `error_summary`, `risk_level`.
  - Покрытие unit-тестами переходов и edge-cases (`timed_out`, `awaiting_approval`, terminal states).

### A-03: `GET /api/tasks` [✅ Выполнено]
- **Приоритет:** P0
- **Зависимости:** A-02
- **Результат:** endpoint списка задач с фильтрацией и сортировкой
- **Критерии приёмки:**
  - Поддержаны фильтры: `status`, `needs_user_action`, `waiting_reason`, `source_channel`, `provider`, `from`, `to`, `query`, `limit`, `offset`, `sort`.
  - Возвращается нормализованный task list (без низкоуровневого debug payload).
  - Unit + integration тесты на фильтры и пагинацию.

### A-04: `GET /api/tasks/{id}` [✅ Выполнено]
- **Приоритет:** P0
- **Зависимости:** A-02
- **Результат:** task detail агрегированный из run/transcript/activity/approvals/artifacts
- **Критерии приёмки:**
  - Ответ содержит секции: `task`, `summary_bar`, `source`, `waiting_state`, `result`, `error`, `timeline_preview`, `debug_refs`.
  - Для Telegram-источника явно отражается ожидание ответа в Telegram.
  - Integration тест покрытия in-progress/completed/failed/waiting states.

### A-05: `GET /api/overview` [✅ Выполнено]
- **Приоритет:** P0
- **Зависимости:** A-03, A-06, B-06
- **Результат:** агрегированный overview endpoint
- **Критерии приёмки:**
  - Возвращает `attention_items`, `active_tasks`, `recent_results`, `system_summary`, `counts`.
  - UI не собирает бизнес-сводку из 6–8 запросов на клиенте.
  - Тесты на корректную агрегацию и частичные деградации источников.

### A-06: `GET /api/system` [✅ Выполнено]
- **Приоритет:** P1
- **Зависимости:** A-01
- **Результат:** агрегированная операторская сводка
- **Критерии приёмки:**
  - В ответе есть: `health`, `doctor`, `providers`, `queues`, `pending_approvals`, `recent_failures`, `degraded_components`.
  - Это не прокси `GET /health`.
  - Integration тесты с деградированным и healthy состоянием.

### A-07: `GET /api/activity` (global feed) [✅ Выполнено]
- **Приоритет:** P1
- **Зависимости:** B-06
- **Результат:** глобальная лента активности
- **Критерии приёмки:**
  - Фильтры: `entity_type`, `severity`, `actor_type`, `run_id`, `session_key`, `since`, `until`.
  - Не дублирует transcript; отражает процессные события.
  - Unit/Integration тесты фильтрации.

### A-08: `GET /api/tasks/{id}/debug` [✅ Выполнено]
- **Приоритет:** P2
- **Зависимости:** A-04
- **Результат:** debug endpoint для низкоуровневой информации о задаче
- **Критерии приёмки:**
  - Возвращает raw run state, полный transcript, tool call payloads, provider details.
  - Отделён от user-facing task detail.
  - Предназначен для operator/debug режима, не для основного UI.

---

## 5. Эпик B — Durable approvals, artifacts, activity [✅ Готов]

### B-01: Миграции таблиц `approvals` и `approval_events` [✅ Выполнено]
- **Приоритет:** P0
- **Зависимости:** A-01
- **Результат:** durable persistence для approval flow
- **Критерии приёмки:**
  - Созданы таблицы с индексами по `run_id`, `status`, `requested_at`.
  - Статусы минимум: `pending/approved/rejected/expired/failed`.
  - Миграции проходят на чистой БД и rollback сценарии валидны.

### B-02: Approval persistence hooks в orchestrator [✅ Выполнено]
- **Приоритет:** P0
- **Зависимости:** B-01
- **Результат:** запись durable approval перед `ApprovalGate.Wait()`
- **Критерии приёмки:**
  - Любой pending approval существует в БД до отправки в канал.
  - Telegram callback и Web API резолвят единый approval resolution service.
  - Есть защита от дублей/гонок (idempotency + status guard).

### B-03: `/api/approvals` + web actions approve/reject [✅ Выполнено]
- **Приоритет:** P0
- **Зависимости:** B-02
- **Результат:**
  - `GET /api/approvals`
  - `GET /api/approvals/{id}`
  - `POST /api/approvals/{id}/approve`
  - `POST /api/approvals/{id}/reject`
- **Критерии приёмки:**
  - Web approval проходит тот же orchestration path, что и Telegram.
  - Все решения пишутся в audit trail.
  - Integration тесты конкурентных approve/reject запросов.

### B-04: Миграция и модель `artifacts` [✅ Выполнено]
- **Приоритет:** P0
- **Зависимости:** A-01
- **Результат:** таблица `artifacts` + репозиторий
- **Критерии приёмки:**
  - Поддержаны типы минимум: `assistant_final`, `doctor_report`, `tool_result`, `summary`.
  - Артефакты создаются по правилам полезного результата (не любой transcript event).
  - Есть связь с `run_id`/`session_key`.

### B-05: `/api/artifacts` и связь с задачами [✅ Выполнено]
- **Приоритет:** P1
- **Зависимости:** B-04, A-04
- **Результат:**
  - `GET /api/artifacts`
  - `GET /api/artifacts/{id}`
  - `GET /api/tasks/{id}/artifacts`
- **Критерии приёмки:**
  - Фильтры по типу, `run_id`, `session_key`, `query`.
  - Task detail получает список связанных артефактов.

### B-06: Миграция и модель `task_activity` [✅ Выполнено]
- **Приоритет:** P0
- **Зависимости:** A-01
- **Результат:** таблица активности и эмиссия ключевых событий
- **Критерии приёмки:**
  - Логируются типы: `task_received`, `model_started`, `approval_requested`, `approval_resolved`, `task_completed`, `task_failed` (минимум).
  - Сущность отделена от transcript.
  - Есть индексы по `run_id`, `created_at`, `severity`.

### B-07: `GET /api/tasks/{id}/activity` [✅ Выполнено]
- **Приоритет:** P1
- **Зависимости:** B-06
- **Результат:** таймлайн задачи для вкладок Timeline/Activity
- **Критерии приёмки:**
  - Возвращаются нормализованные user-facing события.
  - Поддержана пагинация и сортировка по времени.

### B-08: Delivery events (`channel_delivery_events`) [✅ Выполнено]
- **Приоритет:** P1
- **Зависимости:** A-01
- **Результат:** явная фиксация доставки в Telegram/Web
- **Критерии приёмки:**
  - Можно показать: «отправлено в Telegram», «ожидаем reply в Telegram», «delivery failed».
  - Используется в task detail и overview attention block.

---

## 6. Эпик C — Memory v2 write model [✅ Готов]

### C-01: Memory policy model (editable/confirmable/suppressible) [✅ Выполнено]
- **Приоритет:** P0
- **Зависимости:** A-01
- **Результат:** доменные правила изменения памяти
- **Критерии приёмки:**
  - Определены редактируемые типы памяти и ограничения.
  - Определены `confirmation_state`, `effective_status`, `suppressed`, `expires_at`.
  - Зафиксировано distinction между soft/hard delete.

### C-02: `/api/memory` read/write endpoints [✅ Выполнено]
- **Приоритет:** P0
- **Зависимости:** C-01
- **Результат:**
  - `GET /api/memory`
  - `GET /api/memory/{id}`
  - `PATCH /api/memory/{id}`
  - `DELETE /api/memory/{id}`
  - `POST /api/memory/{id}/confirm`
  - `POST /api/memory/{id}/reject`
- **Критерии приёмки:**
  - Все write-операции аудируются.
  - Не допускается неконтролируемая перезапись provenance.
  - Integration тесты confirm/reject/edit/delete.

---

## 7. Эпик D — Live updates для Web UI [✅ Готов]

### D-01: `GET /api/stream` (SSE) [✅ Выполнено]
- **Приоритет:** P1
- **Зависимости:** A-03, B-03, B-06, A-06
- **Результат:** серверный SSE-стрим для UI
- **Критерии приёмки:**
  - Поддержаны topics: `overview`, `tasks`, `approvals`, `system`, `activity`.
  - События минимум: `task.updated`, `approval.created`, `approval.resolved`, `artifact.created`, `memory.updated`, `system.updated`, `activity.created`.
  - Документирован fallback на manual refresh/polling.

---

## 8. Эпик E — Frontend foundation (Nuxt SPA, FSD, design system) [✅ Готов]

### E-01: Структура FSD в `apps/web` [✅ Выполнено]
- **Приоритет:** P0
- **Зависимости:** нет
- **Результат:** каркас `app/pages/widgets/features/entities/shared`
- **Критерии приёмки:**
  - SSR отключён для основного продуктового режима (internal SPA).
  - Структура каталогов соответствует спецификации UI.
  - Базовый app shell (sidebar + topbar + content).
  - **Sidebar navigation widget** реализован как `widgets/sidebar` с навигацией по основным страницам (Overview, Tasks, Approvals, Artifacts, Memory, Activity, System, Settings).
  - Sidebar поддерживает collapsed/expanded состояния.

### E-02: Design tokens + theme (dark-only) [✅ Выполнено]

- **Приоритет:** P0
- **Зависимости:** E-01
- **Результат:** централизованные токены цветов/spacing/typography/radius/shadows + Tailwind theme extension
- **Критерии приёмки:**
  - Нет хардкода hex/spacing/radius в продуктовых компонентах.
  - Поддерживается только dark theme в MVP.
  - Добавлены guardrails линтера против inline styles и произвольных цветов.

### E-03: `shared/ui` primitives (обёртки над PrimeVue) [✅ Выполнено]

- **Приоритет:** P0
- **Зависимости:** E-02
- **Результат:** минимум `AppButton`, `AppCard`, `AppTable`, `AppBadge`, `AppPanel`, `AppDialog`, `AppInput`, `AppSelect`, `AppTabs`, `AppEmptyState`, `AppSkeleton`, `AppAlert`, `AppTooltip`, `AppIconButton`
- **Критерии приёмки:**
  - PrimeVue импортируется только в `shared/ui`.
  - Ограниченный и стабильный API у базовых компонентов.

### E-04: API client и entity adapters [✅ Выполнено]

- **Приоритет:** P0
- **Зависимости:** E-01
- **Результат:** `shared/api/client` + `entities/*/api` для task-centric контрактов
- **Критерии приёмки:**
  - `widgets/pages` не знают деталей HTTP-запросов.
  - TypeScript-типы описаны для response/view-model.
  - `any` отсутствует (кроме документированных исключений).

### E-05: Pinia stores для задач/overview/approvals/system/activity/memory [✅ Выполнено]

- **Приоритет:** P1
- **Зависимости:** E-04
- **Результат:** централизованный state слой
- **Критерии приёмки:**
  - Глобальные фильтры и системное состояние живут в Pinia.
  - Локальное состояние ограничено UI toggles/inputs.

---

## 9. Эпик F — Frontend страницы нового UI [✅ Готов]

### F-01: Overview page [✅ Выполнено]

- **Приоритет:** P0
- **Зависимости:** E-03, E-04, A-05
- **Результат:** экран с attention/active tasks/recent results/system summary
- **Критерии приёмки:**
  - Нет task creation/chat controls.
  - Реализованы loading/empty/error/partial error состояния.

### F-02: Tasks page [✅ Выполнено]

- **Приоритет:** P0
- **Зависимости:** E-03, E-04, A-03
- **Результат:** task-centric таблица/список с фильтрами
- **Критерии приёмки:**
  - Поддержаны фильтры из спецификации.
  - Видимы текстовые статусы (`waiting_for_approval`, `waiting_for_reply_in_telegram` и т.д.).

### F-03a: Task detail page — shell и summary bar [✅ Выполнено]

- **Приоритет:** P0
- **Зависимости:** F-02, A-04
- **Результат:** базовый каркас страницы задачи с summary bar
- **Критерии приёмки:**
  - Summary bar отображает status, risk level, timing, source channel.
  - Реализованы loading/empty/error состояния.
  - Явно показано, если действие возможно только через Telegram.
  - Базовая структура tabs (без реализации содержимого вкладок).

### F-03b: Task detail page — timeline, conversation, debug tabs [✅ Выполнено]

- **Приоритет:** P0
- **Зависимости:** F-03a, B-07, B-05, A-08
- **Результат:** полноценные вкладки Timeline, Conversation, Debug
- **Критерии приёмки:**
  - Timeline показывает activity events из B-07.
  - Вкладка Conversation — read-only transcript view.
  - Debug tab доступен только в operator/debug режиме и использует A-08.
  - Артефакты связанные с задачей отображаются (B-05).

### F-04: Approvals page [✅ Выполнено]

- **Приоритет:** P0
- **Зависимости:** B-03
- **Результат:** список pending approvals + действия approve/reject
- **Критерии приёмки:**
  - Всегда виден риск и контекст связанной задачи.
  - Действия отражают статус и ошибки исполнения.

### F-05: Artifacts page [✅ Выполнено]

- **Приоритет:** P1
- **Зависимости:** B-05
- **Результат:** библиотека артефактов с preview/detail
- **Критерии приёмки:**
  - Артефакты доступны отдельно от timeline.

### F-06: Memory page (management) [✅ Выполнено]
- **Приоритет:** P0
- **Зависимости:** C-02
- **Результат:** просмотр/фильтры/confirm/reject/edit/delete памяти
- **Критерии приёмки:**
  - Явно различаются типы memory и confirmation state.
  - Источник записи всегда отображается.

### F-07: Activity page [✅ Выполнено]

- **Приоритет:** P1
- **Зависимости:** A-07
- **Результат:** аудит-лента с фильтрами
- **Критерии приёмки:**
  - Экран не перегружен низкоуровневыми payload по умолчанию.

### F-08: System page [✅ Выполнено]

- **Приоритет:** P1
- **Зависимости:** A-06
- **Результат:** операторская прозрачность системы
- **Критерии приёмки:**
  - Отдельно видны degraded zones и активные предупреждения.

### F-09: Settings page (адаптация к новой IA) [✅ Выполнено]
- **Приоритет:** P2
- **Зависимости:** E-03
- **Результат:** структурированный экран настроек в новом UI-слое
- **Критерии приёмки:**
  - Settings не становится «свалкой».
  - Ключевые policy toggles доступны без глубокого погружения.
  - **Примечание:** если потребуется UI для memory confirmation policy или approval policy раньше — выделить F-09a как P1.

---

## 10. Эпик G — QA, guardrails, CI [✅ Готов]

### G-01: Playwright baseline для новых страниц [✅ Выполнено]
- **Приоритет:** P0
- **Зависимости:** F-01..F-04 (минимум)
- **Результат:** e2e/smoke/navigation/critical-state/screenshot тесты
- **Критерии приёмки:**
  - Для каждой новой страницы есть smoke + navigation + critical state + screenshot.
  - CI сохраняет screenshots всегда, traces при падениях.

### G-02: Линтеры и ограничения архитектуры UI [✅ Выполнено]
- **Приоритет:** P0
- **Зависимости:** E-02, E-03
- **Результат:** ESLint/Stylelint/TS guardrails
- **Критерии приёмки:**
  - Блокируется merge при inline styles, произвольных цветах, прямом PrimeVue-импорте вне `shared/ui`, `any` без исключения.

### G-03: Contract tests API [✅ Выполнено]
- **Приоритет:** P1
- **Зависимости:** A/B/C
- **Результат:** проверка стабильности форматов API
- **Критерии приёмки:**
  - Фиксируются обязательные поля и backward-compatible эволюция.
  - Отдельно проверяются UX-критичные поля (`needs_user_action`, `waiting_reason`, `telegram_delivery_state`).

---

## 11. Рекомендованный порядок внедрения (release sequence)

1. **Wave 1 (P0 backend foundation):** A-01, A-02, A-03, A-04, A-05
2. **Wave 2 (P0 durable control):** B-01, B-02, B-03, B-04, B-06
3. **Wave 3 (P0 frontend foundation + guardrails):** E-01, E-02, E-03, E-04, G-02
4. **Wave 4 (P0 frontend pages):** F-01, F-02, F-03a, F-03b, F-04
5. **Wave 5 (memory management):** C-01, C-02, F-06
6. **Wave 6 (operator completeness):** A-06, A-07, A-08, B-05, B-07, B-08, F-05, F-07, F-08
7. **Wave 7 (live updates + hardening):** D-01, E-05, F-09, G-01, G-03

---

## 12. Связь с существующим roadmap

Данный бэклог является **продолжением** существующего `docs/planning/butler-implementation-roadmap.md`.

### Завершённые предпосылки (Sprint 6–7)
- Sprint 6: Web UI shell, sessions view, basic doctor view
- Sprint 7: Enhanced doctor output, credential listing, Telegram integration debugging

### Позиционирование текущего бэклога
Этот бэклог представляет **Sprint 8+** — эволюцию Web UI из debug-oriented в task-centric operator console:
- Существующий `web/` каталог адаптируется под FSD-структуру
- Вводится task-centric API для UI
- Durable approvals/artifacts/activity заменяют transient Redis-only state

### Миграционная стратегия
- Wave 1–2: Backend готовит task-centric read model
- Wave 3–4: Frontend реализует новые страницы
- Wave 5+: Постепенное расширение функциональности

---

## 13. Definition of Done для бэклога

Бэклог считается реализованным, когда:

- Все P0 задачи закрыты.
- Новый UI полностью работает на task-centric API.
- Telegram-first ограничения технически защищены (нет endpoint для создания задач из Web UI).
- Durable approvals и memory write flow аудируются и покрыты интеграционными тестами.
- CI блокирует нарушения дизайн-системы и критичных пользовательских сценариев.
