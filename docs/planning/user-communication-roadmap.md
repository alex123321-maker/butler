# План реализации общения Butler с пользователем

Дата: 2026-10-15. Статус: запланировано; реализация не выполнялась.
Основание: [подробная спецификация](../architecture/user-communication-spec.md).
Baseline: `origin/main b20206d`; deployed snapshot в этой работе не проверялся.

План расширяет существующие task/ingress/approval/outbox/authorization механизмы.
Закрытые #4–#7, #9, #11–#13 и #32 используются как основа, не переоткрываются.
#76 остаётся общим behavioral harness; здесь добавляются communication-сценарии.

## GitHub tracking

Общий issue: [#98](https://github.com/alex123321-maker/butler/issues/98).

| Этап | Issue | Зависимости |
| --- | --- | --- |
| C01 | [#99](https://github.com/alex123321-maker/butler/issues/99) | нет |
| C02 | [#100](https://github.com/alex123321-maker/butler/issues/100) | [#99](https://github.com/alex123321-maker/butler/issues/99) |
| C03 | [#101](https://github.com/alex123321-maker/butler/issues/101) | [#100](https://github.com/alex123321-maker/butler/issues/100) |
| C04 | [#102](https://github.com/alex123321-maker/butler/issues/102) | [#100](https://github.com/alex123321-maker/butler/issues/100), [#101](https://github.com/alex123321-maker/butler/issues/101) |
| C05 | [#103](https://github.com/alex123321-maker/butler/issues/103) | [#99](https://github.com/alex123321-maker/butler/issues/99), [#100](https://github.com/alex123321-maker/butler/issues/100) |
| C06 | [#104](https://github.com/alex123321-maker/butler/issues/104) | [#101](https://github.com/alex123321-maker/butler/issues/101), [#103](https://github.com/alex123321-maker/butler/issues/103) |
| C07 | [#105](https://github.com/alex123321-maker/butler/issues/105) | [#100](https://github.com/alex123321-maker/butler/issues/100), [#102](https://github.com/alex123321-maker/butler/issues/102), [#103](https://github.com/alex123321-maker/butler/issues/103), [#104](https://github.com/alex123321-maker/butler/issues/104) |
| C08 | [#106](https://github.com/alex123321-maker/butler/issues/106) | [#100](https://github.com/alex123321-maker/butler/issues/100), [#105](https://github.com/alex123321-maker/butler/issues/105) |
| C09 | [#107](https://github.com/alex123321-maker/butler/issues/107) | [#101](https://github.com/alex123321-maker/butler/issues/101), [#102](https://github.com/alex123321-maker/butler/issues/102), [#103](https://github.com/alex123321-maker/butler/issues/103), [#104](https://github.com/alex123321-maker/butler/issues/104), [#105](https://github.com/alex123321-maker/butler/issues/105), [#106](https://github.com/alex123321-maker/butler/issues/106) |

## Подтверждённые решения

- Четыре функции: message, photo, report, widget; явная публикация агентом.
- Все включённые каналы владельца получают одну каноническую публикацию.
- Telegram report — article / native Rich Message; Web — полный report.
- Widget V1: один/несколько вариантов и текст; составные формы позже.
- Ожидание до ответа/отмены с optional expiry; restart-safe.
- Первый submit фиксируется для всех каналов; изменение отдельным уточнением.
- Open reply obligation после model final получает bounded continuation,
  затем системный сбой при исчерпании; raw final fallback отсутствует.

## Последовательность и зависимости

Номер этапа — рекомендуемый порядок, а зависимости — обязательные prerequisites.
Независимая работа допустима только после готовности общего контракта.
Это описание порядка реализации, не поручение запускать субагентов сейчас.

| Этап | Результат | Зависимости |
| --- | --- | --- |
| C01 | Зафиксировать контракты общения и привязку каналов владельца | Можно начинать |
| C02 | Добавить Broker-инструмент общения и атомарные публикации | C01 |
| C03 | Доставлять сообщения и фотографии в Web и Telegram | C02 |
| C04 | Реализовать отчёты в Web и нативные Telegram Rich Messages | C02, C03 |
| C05 | Сохранять интерактивные вопросы и возобновлять работу после ответа | C01, C02 |
| C06 | Показать виджеты выбора и текста в Web и Telegram | C03, C05 |
| C07 | Отделить финал модели от публикации и контролировать обязанность ответить | C02, C04, C05, C06 |
| C08 | Добавить диагностику общения, миграцию и безопасное включение | C02, C07 |
| C09 | Пройти сквозную приёмку системы общения и включить explicit mode | C03, C04, C05, C06, C07, C08 |

C01 -> C02 создают контракт и durable foundation. После C02 доступны независимые
ветви доставки/отчётов (C03 -> C04) и interaction backend (C05).
C06 объединяет пользовательские виджеты с каналами. C07 меняет lifecycle только
после доступности всех четырёх функций. C08 завершает rollout/операционные
проверки; наблюдаемость и safety checks добавляются в каждом этапе, не откладываются.
C09 — обязательная итоговая приёмка, а не исследование.

## Этапы

### C01. Зафиксировать контракты общения и привязку каналов владельца

**Зависимости:** нет.

**Объём:**

- Ввести versioned DTO/schema для publication, report blocks, widget, reply obligation, adapter capabilities и событий.
- Добавить owner-managed channel bindings с verified identity, enabled state, version и immutable recipient snapshot; один owner может получать публикацию в Web и Telegram.
- Согласовать schema epoch/admission profile и routing без зависимости от исходного канала сессии; зафиксировать fixture corpus четырёх форматов.

**Готовность:**

- [ ] В схемах модели отсутствуют owner/chat/token/lease/grant полномочия; подмена owner и binding отклоняется.
- [ ] Один владелец имеет два проверенных канала; новый/rebound канал не получает старый backlog автоматически.
- [ ] Все пять решений владельца и native Telegram Rich Message отражены в контрактных fixtures; составные формы не входят в V1.

**Проверки:**

- Schema/DTO unit tests, owner-proof API tests, binding version/revoke/rebind integration tests.
- Проверка strict JSON, неизвестных полей, лимитов и backwards compatibility.

**Зоны изменений:** `docs/architecture/user-communication-spec.md`, `internal/owneridentity`, `apps/orchestrator/internal/session`, `apps/orchestrator/internal/api`.

**Приёмка спецификации:** AC14, AC15, AC16.

### C02. Добавить Broker-инструмент общения и атомарные публикации

**Зависимости:** C01.

**Объём:**

- Зарегистрировать communication.message/photo/report/widget как одну family с точными контрактами; вызвать fixed authenticated owning endpoint через обычные Prepare/Authorize/Execute.
- Создавать publication, transcript event, refs, obligations linkage, fan-out targets и durable events атомарно; widget continuation подключается на C05.
- Расширить существующий outbox и channel-only dispatcher на publication identity и representation manifests; не создавать конкурирующую очередь.
- Проверять текущие owner/task/source/privacy при commit и перед send; учитывать root/child budget и owner notification policy.
- Добавить узкое typed разрешение сообщить владельцу о препятствии/вопросе при unknown external effect, сохранив запрет остальных non-read действий.

**Готовность:**

- [ ] Публикация во время run переживает crash и не требует model final; accepted означает только сохранение.
- [ ] Replay exact effect identity возвращает прежнюю publication; другой payload конфликтует, lost internal ACK разрешается чтением durable receipt.
- [ ] Failure на любой записи откатывает транзакцию; сбой одного target не блокирует остальные каналы.
- [ ] UNKNOWN не вызывает resend либо tool/model replay; child и verified_task ограничения не обходятся.

**Проверки:**

- PostgreSQL rollback/crash/CAS/fencing integration; Broker direct-RPC denial и scoped delegation tests.
- Concurrent dispatcher claims, known absent retry, expired sending claim, ACK persistence failure.

**Зоны изменений:** `apps/tool-broker/internal/app`, `apps/tool-broker/internal/registry`, `internal/actionauth`, `apps/orchestrator/internal/communication (новый owning module)`, `apps/orchestrator/internal/deliveryoutbox`, `migrations`.

**Приёмка спецификации:** AC05, AC06, AC14, AC15, AC16, AC17.

### C03. Доставлять сообщения и фотографии в Web и Telegram

**Зависимости:** C02.

**Объём:**

- Добавить Web communication feed/detail и scoped SSE с cursor/reconnect; пользовательская лента отображает explicit publications.
- Реализовать message/photo rendering и передачу сохранённых media в Telegram без публичных URLs и секретов в model args.
- Сохранять independent ACK/manifest parts, ordered publication sequence и состояния по всем target bindings.

**Готовность:**

- [ ] Одно сообщение и photo с подписью появляются в обоих включённых каналах; canonical transcript не дублируется.
- [ ] Web available не называется прочитанным; Telegram sent требует корректный message/chat ACK.
- [ ] Рестарт, временная недоступность Telegram, oversized payload и неизвестный исход не приводят к повтору задачи или потере media.
- [ ] Owner revocation действует на feed, artifact read и отложенную отправку.

**Проверки:**

- Adapter contract tests, payload/media boundary tests, API authorization и Web rendering/reconnect tests.
- Live test-owner message/photo smoke с доказательствами и версиями клиентов; если недоступен — явно оставить acceptance открытым.

**Зоны изменений:** `apps/orchestrator/internal/channel/telegram`, `apps/orchestrator/internal/api`, `apps/orchestrator/internal/artifacts`, `web`, `apps/orchestrator/internal/deliveryoutbox`.

**Приёмка спецификации:** AC01, AC02, AC05, AC06, AC15.

### C04. Реализовать отчёты в Web и нативные Telegram Rich Messages

**Зависимости:** C02, C03.

**Объём:**

- Сохранять канонический report artifact с заголовком, summary, структурными блоками, изображениями и файлами.
- Создать безопасный Web renderer и Telegram mapping в InputRichMessage.blocks через sendRichMessage; перепроверить текущий Bot API.
- Сохранять детерминированный manifest частей; делить большой report по границам блоков с per-part ACK.
- Добавить readiness для Rich Message/media/document capabilities; запретить тихую замену отчёта обычной ссылкой/PDF.

**Готовность:**

- [ ] Один fixture report с таблицей, фото и файлом полноценно доступен в Web и нативной статье Telegram.
- [ ] Сверхлимитный report разделяется без потери содержимого; неделимый oversized block даёт явную ошибку.
- [ ] Обязательное вложение не сохранено/отозвано — успешная публикация или send не заявляются.
- [ ] Raw HTML/script не исполняются; private artifacts не становятся публичными ради Telegram.

**Проверки:**

- Golden rendering tests, Unicode/nesting/block/media/table limits, stored multipart ACK/recovery.
- Live Rich Message acceptance на тестовом Telegram-чате и Web: текст, изображения, document blocks, открытие файлов; screenshot и ACK оцениваются отдельно.

**Зоны изменений:** `apps/orchestrator/internal/artifacts`, `apps/orchestrator/internal/communication`, `apps/orchestrator/internal/channel/telegram`, `web`, `docs/architecture/user-communication-spec.md`.

**Приёмка спецификации:** AC03, AC04, AC05, AC06, AC15.

### C05. Сохранять интерактивные вопросы и возобновлять работу после ответа

**Зависимости:** C01, C02.

**Объём:**

- Реализовать open/resolved/expired/cancelled/superseded interactions и typed responses single/multi/text.
- Атомарно сохранять вопрос + data-only continuation до доставки; освобождать worker/lease при wait.
- Добавить control ingress ответа, не блокируемый собственным waiting input, с атомарным first-submit-wins и единственным resume event.
- Отделить no-expiry ожидание от обычного input TTL и active execution deadlines; сохранить spent budgets.
- Переиспользовать owning approval/input/owner-control primitives без превращения preference response в grant.

**Готовность:**

- [ ] Рестарт и ожидание более 24 часов сохраняют вопрос; ответ восстанавливает исходную task/session по текущим версиям и полномочиям.
- [ ] Ответ до завершения initial delivery не теряется; конкурирующие ответы дают один response и один resume.
- [ ] Cancel/amend/expiry закрывают старый вопрос; pause позволяет сохранить ответ без выполнения до resume.
- [ ] Conversation без Task поддерживается; fixed budgets и source guards сохраняются, tools не replay после восстановления.

**Проверки:**

- PostgreSQL concurrency/restart/fast-reply tests, DB clock expiry and waiting TTL tests.
- Fencing, fresh admission, cancelled task, expired mandate, stale source, source privacy, same-owner cross-channel proof и wrong-owner denial.

**Зоны изменений:** `apps/orchestrator/internal/communication`, `apps/orchestrator/internal/input`, `apps/orchestrator/internal/task`, `apps/orchestrator/internal/orchestrator`, `apps/orchestrator/internal/session`, `apps/orchestrator/internal/approval`, `migrations`.

**Приёмка спецификации:** AC07, AC09, AC10, AC11, AC14, AC16.

### C06. Показать виджеты выбора и текста в Web и Telegram

**Зависимости:** C03, C05.

**Объём:**

- Добавить single-choice, multi-choice и text widgets, optional option media, состояние ожидания и закрытия.
- Для multi-choice сохранять draft/toggle отдельно от Submit; синхронизировать версии и конечное решение между каналами.
- Привязывать Telegram text reply к конкретному interaction; при неоднозначности запрашивать выбор вопроса.
- Обновлять обе проекции после первого submit; late delivery уже закрытого widget рендерить закрытой.

**Готовность:**

- [ ] Сценарий выбора пиццы работает из обоих каналов, включая разные ответы почти одновременно.
- [ ] Draft не запускает continuation; Submit запускает его один раз; последующий ответ получает already_resolved.
- [ ] Отключённые/необновившиеся кнопки не позволяют повторное выполнение: backend проверяет статус.
- [ ] Составные формы отсутствуют; обычный ответ/выбор не заменяет payment approval.

**Проверки:**

- Web UI tests и Telegram callback/ForceReply contract tests; stale projection, double click, multi-choice CAS и ambiguous text tests.
- Live cross-channel widget acceptance с restart и восстановлением вопроса; fixtures без реальной покупки.

**Зоны изменений:** `web`, `apps/orchestrator/internal/channel/telegram`, `apps/orchestrator/internal/api`, `apps/orchestrator/internal/communication`.

**Приёмка спецификации:** AC07, AC08, AC10, AC11, AC16.

### C07. Отделить финал модели от публикации и контролировать обязанность ответить

**Зависимости:** C02, C04, C05, C06.

**Объём:**

- Добавить pinned explicit communication admission profile: assistant_delta/final не создают пользовательский send.
- Сохранить immutable attempt results, artifacts и separate task verification; связать explicit publications с obligation.
- Реализовать одно durable исправляющее продолжение на obligation revision в пределах существующих budget/authority/deadline.
- При отсутствии ответа после исправления — blocked + once-only безопасный system failure notice; без raw final fallback.
- Обновить prompt и context replay: опубликованное, внутренний итог и пользовательские ответы имеют разные роли; preserve verified_task policy.

**Готовность:**

- [ ] Progress не закрывает обязательство; result/blocked publication или waiting widget дают соответствующий переход.
- [ ] Прямой вопрос без Task также получает контроль ответа; quiet background trigger не создаёт лишнего сообщения.
- [ ] Final модели после уже опубликованного ответа не отправляет дубль; ошибка final persistence не повторяет внешний эффект.
- [ ] Корректный terminal event с пустым текстом после явной публикации или quiet run сохраняет typed итог; неожиданный обрыв stream остаётся ошибкой.
- [ ] Рестарт и смена run не обновляют allowance; verified_task не заставляется публиковать запрещённый unverified result.
- [ ] Purpose=result и tool ACK не помечают semantic task completion как проверенный.

**Проверки:**

- Unit/integration missing-final/missing-publication/duplicate final/frozen-save recovery tests.
- Behavioral harness: преждевременное «сейчас сделаю», подтверждённый результат, ложная result метка, полезный progress и quiet run; переиспользовать #76.

**Зоны изменений:** `apps/orchestrator/internal/orchestrator`, `apps/orchestrator/internal/completion`, `apps/orchestrator/internal/input`, `apps/orchestrator/internal/task`, `apps/orchestrator/internal/prompt`, `internal/transport`, `docs/architecture/atomic-result-delivery.md`.

**Приёмка спецификации:** AC01, AC12, AC13, AC16, AC17.

### C08. Добавить диагностику общения, миграцию и безопасное включение

**Зависимости:** C02, C07.

**Объём:**

- Показать publication, obligations, interactions и delivery per target в owner UI/Doctor; metrics и безопасные audit refs.
- Настроить конечные лимиты, adapter readiness, immutable admission mode и mixed-version claim fencing.
- Подготовить additive migrations, legacy result/outbox mapping и rollback с drain/resume принятых explicit данных.
- Проверить privacy/source revocation, binding disable/rebind, no-egress source и media access на всём lifecycle.
- Документировать operator rollout, UNKNOWN reconciliation, no-expiry wait и отличие acceptance/read/task completion.

**Готовность:**

- [ ] UI не скрывает failed/unknown Telegram за Web available и не выдаёт accepted за sent/read.
- [ ] Старые результаты и pending deliveries доступны; не происходит исторической повторной рассылки.
- [ ] Старый worker не подбирает explicit checkpoint; rollback сохраняет принятые вопросы и publications.
- [ ] Doctor не заявляет readiness по одному флагу; каждый неподдержанный adapter/media capability видим.
- [ ] Секреты, body и чувствительные refs не попадают в незащищённые метрики/ошибки.

**Проверки:**

- Migration upgrade/rollback tests на disposable DB, mixed-worker epoch tests, config/Doctor/API tests.
- Full privacy lifecycle и delayed delivery/read/reply tests; наблюдаемость проверять с induced failed/unknown targets.

**Зоны изменений:** `apps/orchestrator/internal/api`, `apps/orchestrator/internal/observability`, `apps/tool-doctor`, `internal/config`, `web`, `deploy`, `docs/architecture`, `apps/orchestrator/README.md`, `apps/tool-broker/README.md`.

**Приёмка спецификации:** AC05, AC09, AC11, AC15, AC18.

### C09. Пройти сквозную приёмку системы общения и включить explicit mode

**Зависимости:** C03, C04, C05, C06, C07, C08.

**Объём:**

- Собрать release acceptance matrix AC01–AC18 и воспроизводимые сценарии, включая pizza fixture, rich report, quiet background и crash boundaries.
- Проверить актуальный развёрнутый build, bindings, Bot API capability, migrations и owner policy; затем staged enable для тестового владельца.
- Разделить synthetic contract checks, live channel delivery, UI rendering, model behaviour и semantic task completion.
- Связать результаты с #76 и не дублировать весь общий harness; сохранить evidence и откат.

**Готовность:**

- [ ] AC01–AC18 имеют реальные результаты и ссылки на evidence; skips не считаются passes.
- [ ] Есть live Web + Telegram photo/report/widget с проверенным продолжением задачи и отсутствием дублей.
- [ ] Failure injection не повторяет внешние эффекты; UNKNOWN, no-expiry waiting, cancellation и forbidden export сохраняют контракт.
- [ ] Explicit mode включается только при полном supported profile; implementation issue не закрывается по одному mock ACK.
- [ ] Спецификация/README отражают фактически включённые возможности и оставшиеся ограничения.

**Проверки:**

- Targeted Go/PostgreSQL/API/Web suites и live Telegram/Web acceptance на тестовом владельце.
- Post-restart smoke, rollout/rollback rehearsal, bounded model behavioural scenarios с учётом полной стоимости.

**Зоны изменений:** `docs/testing`, `deploy`, `apps/orchestrator/internal/eval`, `docs/architecture/user-communication-spec.md`.

**Приёмка спецификации:** AC01, AC02, AC03, AC04, AC05, AC06, AC07, AC08, AC09, AC10, AC11, AC12, AC13, AC14, AC15, AC16, AC17, AC18.

## Общие правила исполнения

Каждый этап начинается со сверки актуального main, relevant specs и закрытых
implementation issues. Таблицы/файлы из плана — owning области, а не требование
создавать вторую реализацию существующего механизма.

Сохраняются Session/Orchestrator/Transport/Broker/Credentials/Memory/Channel
границы, PostgreSQL durable truth и Redis transient role. Никакой progress,
model final, queued publication или Bot API ACK не заменяет task verification.

Обязательны targeted tests и spec/README updates для реально изменённого поведения.
Сначала narrow checks; затем boundary/integration/UI проверки по изменённым
подсистемам. Миграции проверяются на отдельной disposable DB. Production settings,
покупки и отправка посторонним адресатам не входят в документационную задачу.

Компонентная готовность, доступность provider, live доставка, внешний вид,
поведение модели и completion поручения — отдельные строки evidence.
Секреты не включаются в отчёт. Непроверенное и skipped отмечаются явно.

## Отложено

Составные формы, дополнительные каналы, arbitrary UI code, voice и публичный
хостинг отчётов не блокируют C09. Добавление новых функций требует отдельного
scope и совместимых versioned контрактов.
