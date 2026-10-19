# Этап 1: исправление шести дефектов веб-инструментов

Дата: 19 октября 2026, Asia/Tomsk. База: `405a096b8eecfc3ad2fe1dce100c7e8a3ab9d312`.
Работа выполнена в отдельном worktree `web-tools-audit`, ветка
`codex/web-tools-audit`. Исходные критерии — [аудит](web-tools-audit-2026-10-19.md).

## Реализовано и проверено

| Дефект | Исправление | Проверка через Broker |
|---|---|---|
| W1: script/style в readable text | HTML walker пропускает эти поддеревья | Реальный HTTP fixture с JS/CSS; в результате ровно `Useful article` |
| W2: web.extract теряет HTTP-ошибку | URL mode требует HTTP2xx; `reason:http_status` и числовой status в error details, без error body | 404, 429, 500 и redirect302 остаются явными неуспешными чтениями, со своими кодами |
| W3: скрытая обрезка | Чтение limit+1; `metadata.truncated`, безопасная UTF-8 граница; extract сохраняет metadata | Ответ `2 MiB + 128 bytes` проходит обе gRPC границы в ограниченном виде с `truncated:true` через fetch и extract |
| W4: binary-as-text | MIME allowlist text/JSON/XML, sniffing при отсутствии MIME, отказ для binary/invalid UTF-8/NUL | application/pdf возвращает validation failure, без body; unit tests также проверяют missing MIME, PNG, octet-stream и неверный UTF-8 |
| H1: неподдержанный selector выглядит как no-match | Проверка простой грамматики перед HTTP execution | `article h1` возвращает validation failure; счётчик HTTP recipient не меняется |
| H2: matches:null | Пустой результат — инициализированный массив | `.absent` даёт `matches:[]`, `count:0`; действующая output schema Broker принимает результат |

Дополнительные постоянные regression checks: точный лимит без ложного truncated,
обрезка на середине Unicode rune, пустой HTTP204 документ, корректные selector
формы, private/no-authority отказ, сохранение status/provider/final URL у
успешного extract. JSON-expanded результат более 3 MiB получает явный отказ
до доставки через gRPC. Self-hosted JSON envelope более 2 MiB отклоняется
вместо разбора обрезанного объекта; это отдельная граница от текстового excerpt.

## Контракт и Broker

`web.extract` переведён на revision2 с объявленными `status_code`, `final_url`,
`mime_type`, `provider`, `metadata` в output schema. Аргументы URL/html/text
сохранены. Contract hash/approvals для новой схемы пересчитываются обычным
механизмом; старые approvals не переводятся в новую ревизию.

В исходном Broker любая runtime failure заменялась общей unknown-effect
ошибкой, что стирало HTTP-статус и причину отказа. Теперь коррелированный
HTTP/WebFetch read с validation error сохраняет message (до512 bytes) и details
(до1024 bytes), если отсутствует failed-result body и transport/schema error.
Статус остаётся `failed`; journal/evidence остаются `effect_unknown`, retryable
принудительно false. Это сохранение диагностики, а не подтверждение эффекта.

Проверены отдельные отрицательные случаи: POST failure, потеря ответа, чужой
tool_call_id, oversized diagnostic и internal runtime error сохраняют generic
unknown response. Невалидный success schema не становится подтверждённым
результатом. Existing lost external ACK test подтверждает отсутствие повторного
внешнего действия, network-cap test — отказ до dispatch.

Согласованы [Tool ADR](../architecture/tool-runtime-adr.md#web-result-correctness)
и README трёх сервисов. Зависимости и production topology не менялись.

## Что именно означает проверка через Broker

`TestWebToolsBrokerSixDefectsIntegration` использует production `broker.Server`
на loopback gRPC, production runtime router и два отдельных процесса настоящих
runtime test binaries. Эти процессы обслуживают собственный gRPC и исполняют
текущую HTTP/WebFetch реализацию, не fake response executor.

Схемы загружаются из настоящего `configs/tools.json`; меняются только временные
runtime targets. Используются настоящие PostgreSQL intents/grants, effect
journal/evidence и budget controller. Контекст текущих task/run/session/lease
читается из отдельных fixture-таблиц. Тестовый owner approval привязан к
действительному подготовленному action hash. Проверены12 разрешённых dispatch,
12 бюджетных reservations и отсутствие dispatch без authorization.

Синтетическими остаются owner decisions и публичный hostname, который test-only
resolver направляет к единственному loopback recipient. Это проверка Broker
RPC/authority/storage/runtime границ, без product `app.New` bootstrap, внешнего
аккаунта, owner UI, model inference или изменения действующего deployment.

## Результаты

Числа — test/subtest events, а не количество независимых пользовательских задач.

| Запуск | PASS | FAIL | Обязательные SKIP |
|---|---:|---:|---:|
| HTTP/WebFetch unit suites под race detector | 90 | 0 | 0 |
| Broker regression + PG/gRPC/runtime acceptance под race detector | 22 | 0 | 0 |
| go vet HTTP/WebFetch/Broker | успешно | 0 | 0 |

Unit-команда исключает лишь entrypoints двух subprocess harness: они
запускаются настоящими дочерними процессами внутри Broker acceptance, а не
считаются отдельными unit-success. Package events `[no test files]` не включены
в числа и не считаются пропущенными критериями.

Набор Broker также выполняет `TestReadFailureDiagnosticsDoNotConfirmEffectsOrPermitRetry`,
`TestCompletedRuntimeResultMustSatisfySchemaBeforeEvidence`,
`TestNetworkCapDeniesAllBrokerAdmissionPathsBeforeDispatch`,
`TestBrokerPostgresLostExternalAcknowledgementStopsDuplicateDispatch`.

Ранее обнаруженное несовпадение Doctor/FunctionProposal из полного registry
suite относится к исходному аудиту; этот этап не заявляет прохождение всех
тестов репозитория или исправление этой отдельной проблемы.

## Изоляция на общей лабе

Собственный root: `/srv/storage/butler/agents/web-tools-stage1-20261019-a7f9`.
Ни общий Compose, ни его контейнеры, PostgreSQL, `/srv/storage/butler/current`,
служебные токены, чужие agent roots и настройки не изменялись.

Каждый запуск использует собственные build/tmp/Docker caches, PID/CPU/RAM limits
и lock только внутри этого root. Общий Go module cache смонтирован read-only.
Образы разрешаются в image IDs и записываются в лог; образы не собираются и
общие tags не перезаписываются. PostgreSQL имеет уникальное имя, свой bind data
directory и `network none`; тестовый контейнер использует его network namespace.
Нет published ports и отдельной Docker bridge сети. Cleanup удаляет только
созданный этим запуском PG container; fixture processes завершаются тестами.

Использованные images:

- Go: `sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61`.
- PostgreSQL: `sha256:ac08538c6f8b9904c33c8224c5e5706dbe760aca29db1d096972b4052c22a75d`.

Оба окончательных запуска используют snapshot `20261019-022206-048`.
Он содержит все runtime/contract/Broker изменения и regression tests, включая
различение допустимого разреза UTF-8 rune и невалидного байта у границы лимита.

## Воспроизведение и retained evidence

Использовать новый собственный agent root, затем
[web-tools-stage1-lab.sh](../../scripts/dev/web-tools-stage1-lab.sh):

```powershell
pwsh -NoProfile -File scripts/dev/lab.ps1 -Action Sync -RemoteRoot /srv/storage/butler/agents/web-tools-stage1-NEW-ID
ssh laba 'bash /srv/storage/butler/agents/web-tools-stage1-NEW-ID/current/scripts/dev/web-tools-stage1-lab.sh /srv/storage/butler/agents/web-tools-stage1-NEW-ID unit'
ssh laba 'bash /srv/storage/butler/agents/web-tools-stage1-NEW-ID/current/scripts/dev/web-tools-stage1-lab.sh /srv/storage/butler/agents/web-tools-stage1-NEW-ID broker'
```

Последние локальные логи находятся в `.cache/web-tools-stage1/`, полные
серверные логи — в собственном root (`unit-verified.log`, `broker-verified.log`
и `reports/`). Предыдущие успешные промежуточные проверки также сохранены.

| Локальный лог | SHA256 |
|---|---|
| `unit-verified.log` | `84fac52cefbc807499fc039e8ded6a702936a821319e9c973ee9bfe1c0f38d4b` |
| `broker-verified.log` | `2d8a799b2d57fc5e9972554d3a7d06004384b356930bec80d96a35492b63a0c5` |

Все изменения пока локальные, без merge или production rollout. Исходный
`D:\Repository\butler` сохранён на том же commit с чистым status.

Пользовательские решения о серверном браузере без fixed-origin ограничения,
Web viewer с ручным управлением и отложенном single_tab записаны в
[плане](../planning/tool-simplification-plan.md). Это следующий этап, он не
объявлен реализованным текущими шестью исправлениями.
