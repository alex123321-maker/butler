# Ревизия веб-инструментов: фактические проверки

Исторический baseline: шесть дефектов ниже затем исправлены и проверены
через Broker в [этапе 1](web-tools-stage1-2026-10-19.md). Исходные наблюдения
и неуспешные пробы сохранены здесь без переписывания их результатов.

Дата: 19 октября 2026, Asia/Tomsk. Исходный commit: `405a096b8eecfc3ad2fe1dce100c7e8a3ab9d312`,
на начало проверки совпадал с `origin/main`. Рабочая ветка: `codex/web-tools-audit`.

## Вердикт

Веб-инструменты имеют работающие реализации. Подтверждены чтение настоящих
публичных страниц, HTTP GET/download текстового ответа, серверный Chromium,
Broker → Chromium и ограниченный guarded user-tab с PostgreSQL/native host.
При этом считать весь веб-набор корректным и универсально применимым нельзя:
есть шесть воспроизведённых дефектов результатов, ограничения браузерного
origin и неподтверждённая сквозная готовность действующего развёртывания.

Текущий аудит не исправляет runtime-логику. Он сопровождает
[план упрощения](../planning/tool-simplification-plan.md). Единственное изменение
продуктового поведения в ветке — Converters перенесён из Workspace в Advanced.

## Где и как проверялось

- Локальный worktree: `D:\CodexData\home\worktrees\web-tools-audit\butler`.
- Собственный lab root: `/srv/storage/butler/agents/web-tools-audit-20261019`.
- Исходный lab snapshot: `snapshots/20261019-014850-483`; audit script/fixtures
  затем добавлялись только в этот собственный snapshot.
- Go `1.27.2-alpine`; существующий `butler-lab-tool-browser` использован как
  образ с Node/Chromium, тестовые Go-бинарники собраны из указанного commit.
- Отдельные bounded контейнеры (1–2 CPU, 768 MiB–3 GiB, PID limits), без
  публикации портов, без account/model credentials, без native inference.
- PostgreSQL создана только для этого аудита: собственный контейнер, network
  none, база `web_audit_test`, данные в собственном lab root. Тесты используют
  его network namespace. Контейнер удалён после выполнения; evidence сохранено.
- Общие Go-модули и Playwright browsers подключены read-only; build/npm/tmp
  cache принадлежат audit root. Общий Compose и рабочая БД не менялись.
- Browser tests используют реальные Chromium/JS/native host, но синтетические
  страницы, authority fixtures и тестовые credentials. Они не доказывают
  пользовательский owner bind на действующем аккаунте или успешный реальный платёж.

## Результаты запусков

Числа ниже — события тестов и подтестов, а не независимые продуктовые критерии.

| Проверка | Результат | Что доказывает / ограничение |
|---|---|---|
| Существующие узкие Go suites: HTTP, WebFetch, browser, browser-local, Broker/registry, egress, tab control | 142 pass, 1 fail, 43 skip | Компоненты и policy guards; общий запуск НЕ зелёный. Большинство skips требуют PostgreSQL/Chromium/специального harness |
| `TestBrowserSessionE2E` | 8 подтестов + parent PASS | Настоящий Chromium: форма, контекст, auth masking, preparation freeze, stale DOM, expiry/crash, блокировка запрещённых сетевых действий |
| `TestBrokerBrowserSequenceE2E` | PASS | Broker routing → runtime → Chromium, последовательность действий на fixture |
| PostgreSQL policy/resource subset | 34 pass, 0 fail, 2 skip | Подготовка/журнал/бюджет, resource и guarded-tab проверки; два Chromium cases выполнены отдельно ниже |
| `TestResourceBrokerActualChromiumPageCASIntegration` | PASS | Реальные PG/Broker/Chromium: open, stale-page отказ, отсутствие POST и сохранение uncertain resource |
| `TestSingleTabResourceActualChromiumNativeHostIntegration` | 10 подтестов + parent PASS | Guarded `status/click`, ручные изменения, смена страницы/origin/host, lost ACK/restart, два исполнителя, installed native fixture |
| Дополнительные audit expectations, итоговый v3 | 7 pass, 6 fail, 0 skip | Четыре настоящих публичных чтения, mixed batch fixture; шесть дефектов ниже. Это НЕ acceptance PASS |
| Web typecheck, ESLint, Stylelint, build | PASS | Изменение навигации компилируется и соответствует правилам проекта |
| Существующий `generated-converters.spec.ts` в Chromium | 1 PASS | Страница конверторов и owner controls сохранены; скриншот проверен, Converters находится в Advanced |

В первом Web-запуске браузер не стартовал: образ содержит Playwright 1.52,
frontend требует 1.58.2 и browser revision1208. Это ошибка окружения проверки,
не дефект переноса пункта. Повторный запуск использовал уже существующий
read-only cache с revision1208 и прошёл. Новая установка браузера на общий host
не выполнялась. Скрипт воспроизведения использует этот read-only cache.

Единственный FAIL существующего unit-набора:
`TestFunctionCandidatePassesOrdinaryBrokerSchemaOnly` в registry.
`functionproposal.ToProposal` ждёт `doctor.check_system` revision1, текущий
каталог содержит revision2. Тест останавливается на первом Doctor-кандидате;
это не доказательство отказа `web.fetch`. Несовпадение существовало в исходном
commit, поскольку в этой ветке эти файлы не изменены. Его нельзя скрывать
суммарным «все тесты прошли».

## Подтверждённые публичные чтения

Исполнялся текущий Go runtime с обычным egress/DNS policy, без fixture resolver
для этих четырёх запросов. Это компонентные чтения, не live owner-run через
действующий Broker.

| Операция | URL | HTTP | Проверенное содержимое |
|---|---|---:|---|
| `web.fetch` | `https://example.com/` | 200 | `Example Domain`, 340 bytes text, plain HTTP provider |
| `web.fetch` | `https://go.dev/doc/` | 200 | `Documentation`, 13 418 bytes text, plain HTTP provider |
| `http.request` GET | `https://example.com/` | 200 | `Example Domain`, 577 bytes body |
| `http.download` | `https://example.com/` | 200 | Тот же HTML body, 577 bytes; артефакт не создаётся |

Успех этих запросов не подтверждает article-quality extraction, поддержку
JavaScript, anti-bot обход, authenticated website, Jina или self-hosted provider.
Fallback chain покрыта существующим fake-provider unit test; внешний Jina и
реальный self-hosted extractor в данном аудите отдельно не проверялись.

## Шесть воспроизведённых дефектов

### W1 — исполняемый код и CSS попадают в readable text

`TestAuditReadableTextExcludesScriptAndStyle`:

```text
expected: Useful article
actual:   STYLE_NOISE Useful article SCRIPT_NOISE
```

`extractReadableText` обходит все text nodes, включая `script/style`.
Последствие: шум, лишний контекст и ложное представление о видимом тексте.
Это не самостоятельное доказательство prompt-injection exploit.
Источник: `apps/tool-webfetch/internal/runtime/server.go`, `extractReadableText`.

### W2 — URL-режим web.extract теряет HTTP-ошибку

`TestAuditWebExtractPreservesHTTPFailure`: fixture отвечает HTTP404, runtime
возвращает `completed` и только:

```json
{"content_text":"Missing document","url":"http://external.test"}
```

Статус и признак ошибки исчезают. Агент не может отличить извлечённый документ
от текста страницы ошибки. `web.fetch` сам сохраняет `status_code`; дефект
здесь именно в преобразовании ответа через URL-ветку `executeExtract`.

### W3 — большие страницы обрезаются без отметки

`TestAuditLargePageReportsTruncation`: ответ размером `2 MiB + 128 bytes`
возвращается как `2 097 152 bytes` без `truncated` в результате/metadata.
Причина: `io.LimitReader` без чтения дополнительного байта и признака неполноты.
Нельзя считать такой результат полным чтением документа.

### W4 — бинарный MIME выдаётся как readable text

`TestAuditBinaryResponseIsNotReadableText`: `application/pdf` с бинарными
байтами превращается в `content_text`, включая replacement character.
Это не PDF reader и не byte-preserving download. Нужен явный unsupported-MIME
результат или отдельный артефактный путь.

### H1 — неподдержанный selector маскируется под отсутствие совпадений

`TestAuditUnsupportedSelectorIsExplicit`: inline `content` содержит
`<article><h1>Found heading</h1></article>`, selector — `article h1`.
Текущий parser поддерживает только tag / .class / #id / tag.class / tag#id,
но вместо явного отказа возвращает `completed`, count0.
Нужно отклонять неподдержанную грамматику; добавление полноценного CSS-движка
не является обязательным решением.

### H2 — корректное отсутствие совпадений нарушает output schema

`TestAuditNoMatchIsSchemaValidEmptyArray`: допустимый selector `.absent`
по `<h1>Present</h1>` возвращает:

```json
{"count":0,"matches":null,"selector":".absent"}
```

Каталог объявляет `matches` массивом, следовательно правильный результат —
`[]`. Нулевой результат поиска не должен превращаться в schema failure.
Источник H1/H2: `apps/tool-http/internal/runtime/server.go`,
`executeParseHTML` / `extractTextBySelector`; schema — `configs/tools.json`.

## Матрица применимости

| Задача | Сейчас |
|---|---|
| Небольшая публичная HTML/text страница по точному URL | Работает; WebFetch extraction требует исправления W1–W4 |
| Простой API по публичному HTTP(S) адресу | Runtime работает, credential handling покрыт fixture tests; реальный provider/account не проверен |
| HTML selector из маленького документа | Поддерживается узкая грамматика; H1/H2 делают пограничные результаты некорректными |
| Несколько URL | Runtime batch сохраняет успешный item и отказ для private URL; end-to-end бюджет/authority для всей batch этой пробой не доказан |
| JS-страница в одном origin | Есть настоящий Chromium, JS и session operations; успешный synthetic сценарий доказан |
| Многошаговая same-origin форма | Работает в fixture; использовать prepare-first. Click без независимого receipt может остаться unknown, это не бизнес-успех |
| Сайт с внешними CDN, cross-origin SSO, popup/foreign frame | Универсальная поддержка отсутствует: fixed-origin policy блокирует необходимые ресурсы/переходы |
| SPA с POST-загрузкой данных / WebSocket | Read-only browser baseline GET/HEAD и запрет WebSocket могут ломать сценарий; нельзя обещать обычный полнофункциональный браузер |
| Пользовательская вкладка | Guarded `status/click` доказаны на реальной extension/native-host fixture; живой пользовательский bind и остальные legacy команды не квалифицированы этим аудитом |
| PDF, архив, настоящий бинарный файл | Reader не умеет понимать PDF; текущий download возвращает строку body, не артефакт |
| Localhost/LAN/self-hosted API с private IP | Обычная публичная egress policy запрещает; отдельный разрешённый профиль нужен как самостоятельная задача |
| Поиск информации без известного URL | `web.search` отсутствует; текущие инструменты читают URL, но не реализуют поисковую систему |

Redirect-following запрещён egress-клиентом. Это осознанная граница, но
применимость к распространённым URL-переадресациям ограничена. Нужен честный
redirect outcome; нельзя просто включить `http.Client` redirects без проверки
каждого нового адреса и полномочий.

## Каталог и реальное развёртывание

В `configs/tools.json` 39 статических контрактов: browser14, single_tab15,
http3, webfetch3, doctor4. Это НЕ количество tools в каждом model request.
Broker фильтрует по current run/task/mandate/delegation/readiness; отдельно
добавляются optional kernel/extension/converter contracts.

Дополнительные отличия режимов: resourceStore включает `GuardedTabOnly` и
скрывает `browser.*`; workspaceStore без resourceStore включает
`BrowserResourcesUnavailable`. Поэтому нельзя объявить tool доступным по одному
наличию имени в JSON или по healthy runtime container.

При начальном read-only осмотре runtime-контейнеры были запущены. На завершающем
осмотре Orchestrator, Broker и Web оказались `Exited (0)`, OOM у Orchestrator —
false, FinishedAt `2026-10-18T18:54:48.330781517Z` (19 октября 01:54:48 Томск).
API на `127.0.0.1:8088` был недоступен. Причина остановки не установлена;
shared host используется другими исполнителями. Общие сервисы не
перезапускались, их состояние не исправлялось этой задачей.

Итог: доступность реального owner-run и live extension heartbeat на основном
развёртывании не подтверждена. Успешные изолированные тесты не подменяют этот
критерий и не доказывают совпадение production binary с текущим `main`.

## Воспроизведение и evidence

Использовать новый собственный root, не общий `/srv/storage/butler` и не root
другого агента. Сначала прочитать [скрипт](../../scripts/dev/web-tools-audit.sh).
Он не запускает Compose; `probes` временно добавляет audit tests только в
собственный snapshot и удаляет их после выполнения. Запуски внутри root
сериализуются. Режим probes намеренно возвращает nonzero при найденных дефектах.

```powershell
pwsh -NoProfile -File scripts/dev/lab.ps1 -Action Sync -RemoteRoot /srv/storage/butler/agents/web-tools-audit-NEW-ID
ssh laba 'bash /srv/storage/butler/agents/web-tools-audit-NEW-ID/current/scripts/dev/web-tools-audit.sh /srv/storage/butler/agents/web-tools-audit-NEW-ID unit'
ssh laba 'bash /srv/storage/butler/agents/web-tools-audit-NEW-ID/current/scripts/dev/web-tools-audit.sh /srv/storage/butler/agents/web-tools-audit-NEW-ID browser'
ssh laba 'bash /srv/storage/butler/agents/web-tools-audit-NEW-ID/current/scripts/dev/web-tools-audit.sh /srv/storage/butler/agents/web-tools-audit-NEW-ID policy'
ssh laba 'bash /srv/storage/butler/agents/web-tools-audit-NEW-ID/current/scripts/dev/web-tools-audit.sh /srv/storage/butler/agents/web-tools-audit-NEW-ID probes'
ssh laba 'bash /srv/storage/butler/agents/web-tools-audit-NEW-ID/current/scripts/dev/web-tools-audit.sh /srv/storage/butler/agents/web-tools-audit-NEW-ID web'
```

Fixtures: [WebFetch](../../scripts/dev/web-tools-audit-fixtures/webfetch_test.go.txt),
[HTTP](../../scripts/dev/web-tools-audit-fixtures/http_test.go.txt).
Это внешние audit reproductions, не постоянно падающие тесты стандартного suite.
При исправлении дефектов переносить их expectations в обычные regression tests.

Полные логи сохранены в собственном remote root (`*-console*.log`, `reports/`)
и локально в `.cache/web-tools-audit/`. Последний валидный набор:

| Локальный лог | SHA256 |
|---|---|
| `unit.log` | `b98dfe7b40fd0b535d2345b3eb0d7e94a22b1ed67e1acdbd44e0e3d7b813a7b9` |
| `browser.log` | `4529689fa1079126f4ae32b0b8d129a26f239187b17ec90beaf2ae7854c372de` |
| `policy.log` | `dcf7c98338aad569408e049fac3a029cc706d22eeba10105ae9f564ec2ac87a8` |
| `probes-v3.log` | `277ad5120495cff7b6055439b9faf12896aaef823340ea9f805f5489921511c7` |
| `web-v2.log` | `0330fe4a3942364601bb80c960e4e69d86ccd7f0ce342d3e35042e19e9240f4a` |

`converters-advanced.png` — проверенный screenshot итоговой страницы.
Старые неуспешные попытки также сохранены: первоначальная проба selector имела
ошибочное имя входа `html`; её мнимый PASS исключён. Итоговые v2/v3 передают
правильное `content` и воспроизводят H1. Первый Web environment failure также
не удалялся и не засчитан как проверка UI.
