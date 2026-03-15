# Butler — Credential Management and Deferred Secret Resolution

## 1. Статус документа

**Тип документа:** Architecture Subspec / Security and Tooling
**Версия:** 0.1
**Статус:** Draft
**Связанный документ:** Butler PRD + Architecture Specification

---

## 2. Назначение

Этот документ формально описывает подсистему управления учётными данными и отложенного разрешения секретов в Butler.

Подсистема предназначена для безопасного использования паролей, токенов, cookies и других учётных данных в инструментах Butler без раскрытия этих данных агенту и без помещения их в модельный контекст.

---

## 3. Цели

1. Агент не видит секретные значения.
2. Пользователь может явно разрешить использование конкретных учётных данных в конкретном запросе.
3. Инструменты могут использовать секреты через единый механизм.
4. Авторизация работает одинаково для browser tools и HTTP tools.
5. Использование секретов контролируется политиками, доменами и режимами автономности.
6. Все обращения к учётным данным аудируются.

---

## 4. Архитектурные принципы

### 4.1 Secrets never enter model context

LLM не должен получать пароль, токен, cookie value, storage state content или другие реальные секретные значения.

### 4.2 User works with aliases

Пользователь работает с alias, а не с сырым секретом внутри обычного запроса агенту.

### 4.3 Resolution happens only in the system layer

Разрешение `credential_ref` в фактическое значение выполняет только Credential Broker на системном слое перед исполнением инструмента.

### 4.4 Policy before secret access

До раскрытия секрета runtime-инструменту должны быть проверены approval policy, allowed domains, allowed tools и глобальный autonomy mode.

### 4.5 Auditability is mandatory

Каждое использование alias должно оставлять audit trail без записи самого секрета.

### 4.6 Provider auth is a separate secret class

Учётные данные model providers (например, OpenAI Codex refresh token или GitHub Copilot device-flow credentials) не должны смешиваться с user-facing credential aliases.
Они хранятся и обновляются в системном control-plane слое Butler и выдаются transport слою только как runtime auth material.

---

## 5. Базовые сущности

### 5.1 Credential Alias

Пользовательский идентификатор набора учётных данных.

Примеры:

* `Пиццерия`
* `GitHub`
* `РабочаяПочта`

Alias используется в командах пользователя и в вызовах инструментов.

### 5.2 Credential Record

Внутренняя запись, связанная с alias.

Содержит:

* `id`
* `alias`
* `secret_type`
* `target_type`
* `allowed_domains`
* `allowed_tools`
* `approval_policy`
* `secret_ref`
* `status`
* `created_at`
* `updated_at`

### 5.3 Secret Store

Хранилище фактических секретов.

Хранит:

* логины
* пароли
* токены
* cookies
* storage state

Секреты не хранятся в открытом виде в контексте агента или в обычных tool arguments.

### 5.4 Credential Broker

Сервис, который:

* принимает ссылку на alias;
* проверяет политику использования;
* разрешает alias в секретное значение;
* выдаёт секрет только runtime инструмента;
* ведёт аудит.

### 5.5 Deferred Credential Reference

Типизированная ссылка на учётные данные, передаваемая в аргументах тулзы вместо реального секрета.

Пример:

```json
{
  "type": "credential_ref",
  "alias": "Пиццерия",
  "field": "password"
}
```

Поддерживаемые поля:

* `username`
* `password`
* `token`
* `cookie_bundle`
* `storage_state`

---

## 6. Основной функционал

### 6.1 Добавление учётных данных

Пользователь добавляет alias и связанные с ним секреты через отдельную команду или UI.

Минимально сохраняются:

* alias
* тип секрета
* домен или целевая система
* логин / пароль или другой секрет
* политика подтверждения

### 6.2 Передача alias в запросе

Пользователь может явно передать alias в конкретный запрос.

Пример:
`/cred "Пиццерия" Закажи маргариту`

После разбора сообщения в orchestrator формируется отдельный credential context, не смешанный с обычным текстом запроса.

### 6.3 Credential discovery

Агент узнаёт о доступных credential aliases двумя способами:

#### 6.3.1 Явный credential context от пользователя

Когда пользователь передаёт alias в запросе (например, `/cred "Пиццерия" Закажи маргариту`), orchestrator формирует credential context и передаёт агенту metadata выбранного alias: alias name, secret type, allowed domains, allowed tools. Секретные значения в credential context не входят.

Это основной и обязательный способ credential discovery в V1.

#### 6.3.2 Metadata lookup (optional)

Агент может запросить список доступных aliases через отдельный metadata lookup, возвращающий только безопасную информацию:

* alias
* secret_type
* allowed_domains
* status

Lookup не возвращает секретные значения, approval policies и внутренние идентификаторы. Это позволяет агенту предложить пользователю подходящий alias, если пользователь не указал его явно.

#### 6.3.3 Ограничение V1

В V1 агент **не** может самостоятельно выбирать и использовать alias без явного указания пользователя. Даже при наличии metadata lookup, credential_ref в tool call допускается только для alias, явно переданного пользователем в текущем запросе или подтверждённого пользователем в ходе диалога.

### 6.4 Использование alias агентом

Агент не получает секрет.
Агент получает только:

* alias
* тип секрета
* допустимый target hint
* допустимые способы использования

### 6.4 Передача secret reference в tool args

Если агенту нужно заполнить логин или пароль, он передаёт в tool не значение, а typed reference object.

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

### 6.5 Разрешение секрета в runtime

При исполнении tool call:

1. Tool Broker проверяет, разрешён ли alias.
2. Проверяется соответствие текущего домена и allowed domains.
3. Проверяется режим автономности и approval policy.
4. Credential Broker разрешает `credential_ref` в фактическое значение.
5. Значение передаётся напрямую в runtime инструмента.
6. Агенту возвращается только безопасный результат.

#### V1 baseline implemented in Sprint 5.2

В текущем baseline Butler:

* `ToolCall.credential_refs` остаётся модельно-видимой typed reference структурой без секрета;
* Tool Broker авторизует каждую ссылку по alias metadata, tool name, target domain и autonomy mode;
* после авторизации broker разрешает `secret_ref` только в системном слое;
* runtime получает секреты отдельно в gRPC-поле `resolved_credentials`, а не внутри `args_json`;
* первая поддержанная runtime-инъекция реализована для `http.request`.

---

## 7. Поддерживаемые сценарии

### 7.1 Browser Automation

Инструменты браузера должны поддерживать credential references в аргументах:

* `browser.fill`
* `browser.type`
* `browser.set_cookie`
* `browser.restore_storage_state`

Сценарий:

* агент кликает кнопку входа;
* находит поля формы;
* передаёт `credential_ref` в fill/type;
* runtime подставляет реальные значения;
* результат возвращается без секрета.

### 7.2 HTTP / Web

HTTP tool должен поддерживать auth references.

Пример:

```json
{
  "tool": "http.request",
  "args": {
    "method": "GET",
    "url": "https://api.example.com/orders",
    "auth": {
      "type": "credential_ref",
      "alias": "Пиццерия",
      "field": "token"
    }
  }
}
```

Runtime сам добавляет нужный заголовок или другой auth material.

---

## 8. Правила безопасности

### 8.1 Агент не видит секрет

LLM никогда не получает:

* пароль
* токен
* cookie value
* storage state content

### 8.2 Секрет не должен проходить через обычный tool result

В output запрещено возвращать:

* реальные credential values;
* сырые cookies;
* токены;
* чувствительные значения из заполненных полей.

### 8.3 Alias ограничивается областью использования

Для каждого alias задаются:

* разрешённые домены;
* разрешённые tool classes;
* разрешённые типы действий;
* политика подтверждения.

### 8.4 Разрешение ссылок выполняет только системный слой

Ни агент, ни сам tool logic не должны самостоятельно запрашивать “сырой пароль”.
Разрешение `credential_ref` делает только Credential Broker.

### 8.5 Логи маскируются

В логах допускается:

* alias
* field
* tool name
* domain
* факт использования

В логах запрещено:

* фактическое значение секрета

---

## 9. Политики использования

### 9.1 Approval Policy

Для alias задаётся политика:

* `always_confirm`
* `confirm_on_mutation`
* `auto_read_only`
* `manual_only`

### 9.2 Режимы автономности

Подсистема должна учитывать глобальный режим автономности Butler.

Пример:

* в `Read/Assist` использование credential alias запрещено;
* в `Propose` требуется подтверждение;
* в `Guarded Execute` разрешены безопасные сценарии;
* в `Extended Autonomy` разрешение определяется policy.

---

## 10. Поддерживаемые типы секретов

1. `username_password`
2. `api_token`
3. `cookie_bundle`
4. `storage_state`
5. `oauth_token`

`oauth_token` может использоваться и для tool-facing aliases, и для provider auth, но provider auth credentials не проходят через alias selection / credential_ref flow.

---

## 11. Контракт аргументов

### 11.1 Literal value

```json
{
  "type": "literal",
  "value": "test@example.com"
}
```

### 11.2 Credential reference

```json
{
  "type": "credential_ref",
  "alias": "Пиццерия",
  "field": "username"
}
```

Все инструменты, работающие с чувствительными значениями, должны уметь принимать не только literal values, но и credential references.

---

## 12. Жизненный цикл выполнения

### 12.1 Сценарий browser login

1. Пользователь передаёт alias `Пиццерия`.
2. Агент открывает сайт.
3. Агент кликает кнопку входа.
4. Агент вызывает `browser.fill` для логина с `credential_ref`.
5. Агент вызывает `browser.fill` для пароля с `credential_ref`.
6. Tool Broker валидирует вызовы.
7. Credential Broker разрешает ссылки.
8. Browser runtime заполняет поля.
9. Агент отправляет форму.
10. Runtime возвращает безопасный результат.

---

## 13. Минимальный набор команд

### 13.1 Управление учётными данными

* `/cred add <alias>`
* `/cred list`
* `/cred show <alias>` — только метаданные, без секрета
* `/cred revoke <alias>`
* `/cred update <alias>`

### 13.2 Использование в запросе

* `/cred "<alias>" <user task>`

---

## 14. Минимальные требования к хранилищу

### 14.1 Credential metadata

Хранится в PostgreSQL:

* alias
* тип
* домены
* политики
* статус
* ссылки на секрет

#### V1 baseline implemented in Sprint 5

Credential metadata now has a durable PostgreSQL table `credentials` with the following minimum fields:

* `alias`
* `secret_type`
* `target_type`
* `allowed_domains`
* `allowed_tools`
* `approval_policy`
* `secret_ref`
* `status`
* `metadata`
* `created_at`
* `updated_at`

Important constraints:

* `secret_ref` points to runtime-only secret material and is not the raw secret itself;
* credential CRUD operates only on metadata and policy fields;
* authorization decisions can be made from metadata before any future secret resolution path runs.

### 14.2 Secret material

Хранится в отдельном Secret Store:

* локальное зашифрованное хранилище;
* либо внешний secret provider.

#### V1 baseline resolver

До появления полноценного encrypted Secret Store Butler V1 использует минимальный системный resolver по `secret_ref` схеме `env://ENV_VAR_NAME`.

Это означает:

* PostgreSQL хранит только metadata и `secret_ref`;
* фактическое значение читается Tool Broker только из переменной окружения runtime/process layer;
* raw secret не записывается в transcript, config introspection и model-visible tool args.

---

## 15. Аудит

Каждое использование alias должно записывать:

* `run_id`
* `tool_call_id`
* `alias`
* `field`
* `tool_name`
* `target_domain`
* `decision`
* `timestamp`

---

## 16. Итоговая модель

1. Пользователь работает с alias.
2. Агент видит только alias и метаданные.
3. Агент передаёт в tools typed `credential_ref`.
4. Tool Broker и Credential Broker foundation проверяют допустимость использования по alias, tool, domain и autonomy mode.
5. Credential Broker разрешает ссылку в runtime.
6. Runtime использует секрет без раскрытия агенту.
7. Агент получает только безопасный результат.

---

## 17. Открытые решения

1. Финальный выбор Secret Store backend: локальное зашифрованное хранилище (encrypted at rest) или внешний secret provider (Vault, SOPS, etc.).
2. Детализация approval policy model: точные правила взаимодействия approval policy alias с глобальным autonomy mode, приоритеты при конфликтах.
3. Rotation и update flow: как обновлять секреты без прерывания активных сессий, нужен ли grace period для старых значений.
4. Bootstrap UX для `/cred add`: через какой канал пользователь безопасно вводит секрет (Telegram, Web UI, CLI), как защитить ввод от попадания в историю чата.
5. Masking и redaction policy: точные правила маскирования в логах, tool outputs и диагностических отчётах; какие поля маскируются, какой формат маски.
6. Предпочтение browser session state vs raw password: когда использовать `storage_state` / `cookie_bundle` вместо `username_password`, политика выбора по умолчанию.

## 18. Current masking baseline for memory safety

Current Butler baseline now applies memory-focused sanitization before memory extraction prompts and before durable memory persistence for memory-derived records.

Minimum covered categories:

* bearer tokens and API tokens;
* passwords and password-like fields;
* cookies and storage-state-like blobs;
* connection strings / DSNs with embedded credentials;
* common credential-like JSON/header fields such as `authorization`, `access_token`, `refresh_token`, `api_key`, `client_secret`, `cookie`, and `connection_string`.

This sanitization is intended to protect memory stores and extraction prompts. It does not change Transcript Store raw audit semantics, which remain the source-of-truth history layer.
