# Butler — Tooling and Execution Specification

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

* fully autonomous self-extending tools;
* произвольную загрузку стороннего кода в runtime;
* прямой доступ LLM к секретам;
* исполнение инструментов без централизованного broker layer;
* смешение execution layer и memory layer.

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
* `single_tab.*` не поддерживает `credential_ref`, cookie injection, `storage_state` restore и другие secret-bearing browser args;
* policy boundary строится вокруг session-bound capability и broker-side prereq checks, а не вокруг domain allowlist.

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
