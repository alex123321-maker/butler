# План реализации общения Butler с пользователем

Дата ревизии: 2026-10-21. Статус: запланировано; реализация не выполнялась.
Основание: [подробная спецификация](../architecture/user-communication-spec.md).
Исходный baseline: `origin/main b20206d`; ревизия сверена с `ab28dd88`.
Deployed snapshot в этой документационной работе не проверялся.

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
| C04 | [#103](https://github.com/alex123321-maker/butler/issues/103) | [#99](https://github.com/alex123321-maker/butler/issues/99), [#100](https://github.com/alex123321-maker/butler/issues/100) |
| C05 | [#104](https://github.com/alex123321-maker/butler/issues/104) | [#101](https://github.com/alex123321-maker/butler/issues/101), [#103](https://github.com/alex123321-maker/butler/issues/103) |
| C06 | [#102](https://github.com/alex123321-maker/butler/issues/102) | [#100](https://github.com/alex123321-maker/butler/issues/100), [#101](https://github.com/alex123321-maker/butler/issues/101) |
| C07 | [#105](https://github.com/alex123321-maker/butler/issues/105) | [#100](https://github.com/alex123321-maker/butler/issues/100), [#101](https://github.com/alex123321-maker/butler/issues/101), [#102](https://github.com/alex123321-maker/butler/issues/102), [#103](https://github.com/alex123321-maker/butler/issues/103), [#104](https://github.com/alex123321-maker/butler/issues/104) |
| C08 | [#106](https://github.com/alex123321-maker/butler/issues/106) | [#100](https://github.com/alex123321-maker/butler/issues/100), [#105](https://github.com/alex123321-maker/butler/issues/105) |
| C09 | [#107](https://github.com/alex123321-maker/butler/issues/107) | [#101](https://github.com/alex123321-maker/butler/issues/101), [#102](https://github.com/alex123321-maker/butler/issues/102), [#103](https://github.com/alex123321-maker/butler/issues/103), [#104](https://github.com/alex123321-maker/butler/issues/104), [#105](https://github.com/alex123321-maker/butler/issues/105), [#106](https://github.com/alex123321-maker/butler/issues/106) |

## Подтверждённые решения

- Четыре функции: message, photo, report, widget; явная публикация агентом.
- Все включённые каналы владельца получают одну каноническую публикацию.
- Telegram report — article / native Rich Message; Web — полный report.
- Widget V1: один/несколько вариантов и текст, только blocking wait; неблокирующие вопросы и составные формы позже.
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
| C01 | Зафиксировать typed transcript, каналы и обратную маршрутизацию | Можно начинать |
| C02 | Добавить атомарные публикации и ранний explicit lifecycle | C01 |
| C03 | Пройти ранний сквозной message flow; добавить фото и обычные ответы | C02 |
| C04 | Сохранять blocking вопросы и безопасно передавать исполнение после ответа | C01, C02 |
| C05 | Показать виджеты выбора и текста в Web и Telegram | C03, C04 |
| C06 | Реализовать отчёты в Web и нативные Telegram Rich Messages | C02, C03 |
| C07 | Интегрировать все форматы с obligations и проверить поведение модели | C02, C03, C04, C05, C06 |
| C08 | Добавить диагностику общения, миграцию и безопасное включение | C02, C07 |
| C09 | Пройти полную приёмку и подготовить включение production explicit mode | C03, C04, C05, C06, C07, C08 |

C01 -> C02 -> C03 дают первый обязательный сквозной результат: explicit message,
продолжение tools, terminal final без дубля и bounded missing-response recovery.
Разделение transcript/final, память, readiness и epoch относятся к C01/C02,
а не откладываются до C07/C08. Проверка идёт на изолированном fixture/test owner;
production/default admissions остаются legacy до полной готовности.

Рекомендуемый следующий порядок: C04 (#103) -> C05 (#104) -> C06 (#102).
Backend вопросов C04 независим после C02; reports C06 независим после C03.
Номера GitHub issue сохранены, изменились номера этапов #102–#104.
C07 объединяет obligations всех четырёх функций и behavioral acceptance.
C08 завершает rollout/операционные проверки; safety и наблюдаемость добавляются
в каждом этапе. C09 — итоговая AC01–AC22 приёмка. Реальное production enable
требует отдельного разрешения владельца; этот план его не выдаёт.

## Этапы

### C01. Зафиксировать typed transcript, каналы и обратную маршрутизацию

**Зависимости:** нет.

**Объём:**

- Ввести versioned DTO/schema для publication, report blocks, widget, reply obligation, adapter capabilities и событий.
- Разделить publication/attempt_final/owner_input/interaction_response по identity, kind и visibility; определить selectors final persistence, replay, compaction и extraction. Роль assistant не доказывает публикацию.
- Добавить owner-managed channel bindings с verified identity, enabled state, version и immutable recipient snapshot; один owner может получать публикацию в Web и Telegram.
- Определить ACK mapping канального сообщения на publication и canonical session/task; обычный cross-channel reply, ambiguity и owner controls проходят owning ingress без слияния сессий.
- Согласовать schema epoch/admission profile и routing без зависимости от исходного канала сессии; зафиксировать fixture corpus четырёх форматов.
- Widget V1 принимает только отсутствующий/true wait_for_response; false отклоняется до commit. Зафиксировать Input/Interaction checkpoint versions и общий порядок owning locks.

**Готовность:**

- [ ] В схемах модели отсутствуют owner/chat/token/lease/grant полномочия; подмена owner и binding отклоняется.
- [ ] Один владелец имеет два проверенных канала; новый/rebound канал не получает старый backlog автоматически.
- [ ] Все решения владельца и native Telegram Rich Message отражены в fixtures; составные формы и nonblocking questions не входят в V1.
- [ ] Несколько publications и отдельный final одного run имеют разные identities; обычный reply имеет доказанный target либо явную неоднозначность.

**Проверки:**

- Schema/DTO unit tests, owner-proof API tests, binding version/revoke/rebind integration tests.
- Проверка strict JSON, неизвестных полей, лимитов и backwards compatibility.
- Contract fixtures typed transcript/visibility, cross-channel reply target и invalid wait=false без записей.

**Зоны изменений:** `docs/architecture/user-communication-spec.md`, `docs/architecture/memory-model.md`, `internal/owneridentity`, `internal/memory/transcript`, `internal/memory/compaction`, `apps/orchestrator/internal/session`, `apps/orchestrator/internal/api`.

**Приёмка спецификации:** AC14, AC15, AC16, AC19, AC20, AC22.

### C02. Добавить атомарные публикации и ранний explicit lifecycle

**Зависимости:** C01.

**Объём:**

- Зарегистрировать communication.message/photo/report/widget как одну family с точными контрактами; вызвать fixed authenticated owning endpoint через обычные Prepare/Authorize/Execute.
- Включать в каталог только готовые методы pinned explicit test profile; legacy не получает tools, неготовые методы отказывают до commit. Production V1 пока не включать.
- Реализовать ранний explicit lifecycle для message: delta/final не публикуются, attempt result сохраняется отдельно; непустой/пустой typed terminal, quiet и одно bounded missing-response continuation с safe failure работают без ожидания reports/widgets.
- Изменить AppendFinalTx selector на identity/kind финала, сохранить legacy recovery. Адаптировать transcript readers, replay, compaction/extraction и user feed к разным видам записей.
- Создавать publication, transcript event, refs, obligations linkage, fan-out targets и durable events атомарно; widget continuation подключается на C04.
- Расширить существующий outbox и channel-only dispatcher на publication identity и representation manifests; не создавать конкурирующую очередь.
- Проверять текущие owner/task/source/privacy при commit и перед send; учитывать root/child budget и owner notification policy.
- Добавить узкое typed разрешение сообщить владельцу о препятствии/вопросе при unknown external effect, сохранив запрет остальных non-read действий.
- Добавить schema epoch/claim fencing и конечные лимиты с первой исполняемой публикации, а не только в C08.

**Готовность:**

- [ ] Публикация во время run переживает crash и не требует model final; accepted означает только сохранение.
- [ ] Replay exact effect identity возвращает прежнюю publication; другой payload конфликтует, lost internal ACK разрешается чтением durable receipt.
- [ ] Failure на любой записи откатывает транзакцию; сбой одного target не блокирует остальные каналы.
- [ ] UNKNOWN не вызывает resend либо tool/model replay; child и verified_task ограничения не обходятся.
- [ ] Две message publications + иной/пустой terminal final сохраняются без конфликта; frozen recovery не повторяет публикацию; память не выдаёт внутренний final за отправленное сообщение.
- [ ] Missing-response и quiet policies работают для message; неготовые tools и legacy calls отказывают до эффекта.

**Проверки:**

- PostgreSQL rollback/crash/CAS/fencing integration; Broker direct-RPC denial и scoped delegation tests.
- Concurrent dispatcher claims, known absent retry, expired sending claim, ACK persistence failure.
- PostgreSQL transcript/final identity и compaction/replay regression; minimal message obligations, readiness/epoch и unchanged legacy tests.

**Зоны изменений:** `apps/tool-broker/internal/app`, `apps/tool-broker/internal/registry`, `internal/actionauth`, `internal/memory/transcript`, `internal/memory/compaction`, `internal/memory/pipeline`, `apps/orchestrator/internal/communication (новый owning module)`, `apps/orchestrator/internal/deliveryoutbox`, `apps/orchestrator/internal/completion`, `apps/orchestrator/internal/input`, `apps/orchestrator/internal/orchestrator`, `apps/orchestrator/internal/prompt`, `internal/transport`, `migrations`.

**Приёмка спецификации:** AC05, AC06, AC12, AC13, AC14, AC15, AC16, AC17, AC19, AC21.

### C03. Пройти ранний сквозной message flow; добавить фото и обычные ответы

**Зависимости:** C02.

**Объём:**

- Добавить Web communication feed/detail и scoped SSE с cursor/reconnect; пользовательская лента отображает explicit publications.
- Реализовать message/photo rendering и передачу сохранённых media в Telegram без публичных URLs и секретов в model args.
- Сохранять independent ACK/manifest parts, ordered publication sequence и состояния по всем target bindings.
- Реализовать authenticated обычный reply на publication из Web/Telegram через ACK mapping в исходную canonical session/task; ambiguity и owner controls сохраняют действующие проверки.
- Пройти ранний сквозной explicit message сценарий до reports/widgets: отправка -> продолжение tools -> отличный/пустой terminal final без дубля; missing-response и quiet ветви отдельно.

**Готовность:**

- [ ] Одно сообщение и photo с подписью появляются в обоих включённых каналах; canonical transcript не дублируется.
- [ ] Web available не называется прочитанным; Telegram sent требует корректный message/chat ACK.
- [ ] Рестарт, временная недоступность Telegram, oversized payload и неизвестный исход не приводят к повтору задачи или потере media.
- [ ] Owner revocation действует на feed, artifact read и отложенную отправку.
- [ ] Cross-channel reply на обычное сообщение имеет исходный target; wrong owner, stale target и неоднозначность не выбирают task молча.
- [ ] Ранний explicit flow доказан на изолированном test owner без production enable; неподдержанные методы скрыты/отклоняются.

**Проверки:**

- Adapter contract tests, payload/media boundary tests, API authorization и Web rendering/reconnect tests.
- Live test-owner message/photo smoke с доказательствами и версиями клиентов; если недоступен — явно оставить acceptance открытым.
- Reply mapping/revoke/rebind/ambiguity tests; ранний model behaviour и final-save/replay/compaction evidence отдельно от adapter ACK.

**Зоны изменений:** `apps/orchestrator/internal/channel/telegram`, `apps/orchestrator/internal/api`, `apps/orchestrator/internal/artifacts`, `apps/orchestrator/internal/input`, `apps/orchestrator/internal/session`, `web`, `apps/orchestrator/internal/deliveryoutbox`.

**Приёмка спецификации:** AC01, AC02, AC05, AC06, AC12, AC13, AC15, AC17, AC19, AC20, AC21.

### C04. Сохранять blocking вопросы и безопасно передавать исполнение после ответа

**Зависимости:** C01, C02.

**Объём:**

- Реализовать open/resolved/expired/cancelled/superseded interactions и typed responses single/multi/text.
- Атомарно сохранять вопрос, data-only continuation, Input awaiting_interaction checkpoint, Run/Task wait intent и obligation до доставки; Core останавливает обычный tool-result/model continuation даже при lost internal ACK.
- Добавить control ingress ответа, не блокируемый собственным waiting input, с атомарным first-submit-wins и единственным resume event.
- Ready marker ответа монотонен: поздний Finish/suspend старого worker проверяет checkpoint version/CAS и не возвращает ready в waiting. Resume ждёт освобождения/истечения старого ownership и свежего fence.
- Отделить no-expiry ожидание от обычного input TTL и active execution deadlines; сохранить spent budgets.
- Переиспользовать owning approval/input/owner-control primitives без превращения preference response в grant.

**Готовность:**

- [ ] Рестарт и ожидание более 24 часов сохраняют вопрос; ответ восстанавливает исходную task/session по текущим версиям и полномочиям.
- [ ] Ответ до завершения initial delivery не теряется; конкурирующие ответы дают один response и один resume.
- [ ] Оба порядка response/Finish, crash и lost ACK дают один resume без одновременного исполнения двух worker и без повтора tool.
- [ ] Cancel/amend/expiry закрывают старый вопрос; pause позволяет сохранить ответ без выполнения до resume.
- [ ] Conversation без Task поддерживается; fixed budgets и source guards сохраняются, tools не replay после восстановления.

**Проверки:**

- PostgreSQL concurrency/restart/fast-reply tests: response до Finish, Finish до response, crash каждой границы, lost internal ACK, checkpoint CAS и fresh ownership; DB clock expiry and waiting TTL tests.
- Fencing, fresh admission, cancelled task, expired mandate, stale source, source privacy, same-owner cross-channel proof и wrong-owner denial.

**Зоны изменений:** `apps/orchestrator/internal/communication`, `apps/orchestrator/internal/input`, `apps/orchestrator/internal/task`, `apps/orchestrator/internal/orchestrator`, `apps/orchestrator/internal/session`, `apps/orchestrator/internal/approval`, `migrations`.

**Приёмка спецификации:** AC07, AC09, AC10, AC11, AC14, AC16.

### C05. Показать виджеты выбора и текста в Web и Telegram

**Зависимости:** C03, C04.

**Объём:**

- Добавить single-choice, multi-choice и text widgets, optional option media, состояние ожидания и закрытия.
- Для multi-choice сохранять draft/toggle отдельно от Submit; синхронизировать версии и конечное решение между каналами.
- Привязывать Telegram text reply к конкретному interaction; при неоднозначности запрашивать выбор вопроса.
- Обновлять обе проекции после первого submit; late delivery уже закрытого widget рендерить закрытой.

**Готовность:**

- [ ] Сценарий выбора пиццы работает из обоих каналов, включая разные ответы почти одновременно.
- [ ] Draft не запускает continuation; Submit запускает его один раз; последующий ответ получает already_resolved.
- [ ] Отключённые/необновившиеся кнопки не позволяют повторное выполнение: backend проверяет статус.
- [ ] Только blocking widgets; false отклоняется до commit. Составные формы отсутствуют; обычный ответ/выбор не заменяет payment approval.

**Проверки:**

- Web UI tests и Telegram callback/ForceReply contract tests; stale projection, double click, multi-choice CAS и ambiguous text tests.
- Live cross-channel widget acceptance с restart и восстановлением вопроса; fixtures без реальной покупки.

**Зоны изменений:** `web`, `apps/orchestrator/internal/channel/telegram`, `apps/orchestrator/internal/api`, `apps/orchestrator/internal/communication`.

**Приёмка спецификации:** AC07, AC08, AC10, AC11, AC16, AC22.

### C06. Реализовать отчёты в Web и нативные Telegram Rich Messages

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

### C07. Интегрировать все форматы с obligations и проверить поведение модели

**Зависимости:** C02, C03, C04, C05, C06.

**Объём:**

- Расширить работающий с C02/C03 lifecycle на photo/report/blocking widget, включая wait->response->open, pause/cancel/amend и multi-run obligations.
- Проверить общее одно durable исправление на obligation revision, общие budgets/authority/deadline и once-only safe failure для всех форматов.
- Завершить prompt/context/memory интеграцию, quiet, verified_task и child policies. Typed transcript и отсутствие implicit final send уже являются foundation C02, не новым механизмом C07.
- Отдельно проверить смысл ответа: purpose=result структурно связывает obligation, но не доказывает ни содержательный ответ, ни semantic task completion.

**Готовность:**

- [ ] Progress не закрывает обязательство; result/blocked publication или waiting widget дают соответствующий переход.
- [ ] Прямой вопрос без Task также получает контроль ответа; quiet background trigger не создаёт лишнего сообщения.
- [ ] Final модели после уже опубликованного ответа не отправляет дубль; ошибка final persistence не повторяет внешний эффект.
- [ ] Корректный terminal event с пустым текстом после явной публикации или quiet run сохраняет typed итог; неожиданный обрыв stream остаётся ошибкой.
- [ ] Рестарт и смена run не обновляют allowance; verified_task не заставляется публиковать запрещённый unverified result.
- [ ] Purpose=result и tool ACK не помечают semantic task completion как проверенный.
- [ ] «Сейчас сделаю» с purpose=result фиксируется как behavioral failure, даже если структурная obligation закрыта; этот пробел не маскируется техническими passes.

**Проверки:**

- Unit/integration missing-final/missing-publication/duplicate final/frozen-save recovery tests.
- Behavioral harness: преждевременное «сейчас сделаю», подтверждённый результат, ложная result метка, полезный progress и quiet run; переиспользовать #76.

**Зоны изменений:** `apps/orchestrator/internal/orchestrator`, `apps/orchestrator/internal/completion`, `apps/orchestrator/internal/input`, `apps/orchestrator/internal/task`, `apps/orchestrator/internal/prompt`, `internal/transport`, `docs/architecture/atomic-result-delivery.md`.

**Приёмка спецификации:** AC01, AC12, AC13, AC16, AC17, AC19, AC20, AC21, AC22.

### C08. Добавить диагностику общения, миграцию и безопасное включение

**Зависимости:** C02, C07.

**Объём:**

- Показать publication, obligations, interactions и delivery per target в owner UI/Doctor; metrics и безопасные audit refs.
- Проверить конечные лимиты, adapter readiness, immutable admission mode и mixed-version claim fencing, введённые с C02; завершить operator configuration полного профиля.
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

### C09. Пройти полную приёмку и подготовить включение production explicit mode

**Зависимости:** C03, C04, C05, C06, C07, C08.

**Объём:**

- Собрать release acceptance matrix AC01–AC22 и воспроизводимые сценарии, включая pizza fixture, rich report, quiet background, typed transcript, обычный cross-channel reply и crash boundaries.
- Проверить актуальный развёрнутый build, bindings, Bot API capability, migrations и owner policy; подготовить staged production enable, выполняемый только по отдельному разрешению владельца.
- Разделить synthetic contract checks, live channel delivery, UI rendering, model behaviour и semantic task completion.
- Связать результаты с #76 и не дублировать весь общий harness; сохранить evidence и откат.

**Готовность:**

- [ ] AC01–AC22 имеют реальные результаты и ссылки на evidence; skips не считаются passes.
- [ ] Есть live Web + Telegram photo/report/widget с проверенным продолжением задачи и отсутствием дублей.
- [ ] Failure injection не повторяет внешние эффекты; UNKNOWN, no-expiry waiting, cancellation и forbidden export сохраняют контракт.
- [ ] Explicit mode включается только при полном supported profile; implementation issue не закрывается по одному mock ACK.
- [ ] Спецификация/README отражают фактически включённые возможности и оставшиеся ограничения.

**Проверки:**

- Targeted Go/PostgreSQL/API/Web suites и live Telegram/Web acceptance на тестовом владельце.
- Post-restart smoke, rollout/rollback rehearsal, bounded model behavioural scenarios с учётом полной стоимости.

**Зоны изменений:** `docs/testing`, `deploy`, `apps/orchestrator/internal/eval`, `docs/architecture/user-communication-spec.md`.

**Приёмка спецификации:** AC01, AC02, AC03, AC04, AC05, AC06, AC07, AC08, AC09, AC10, AC11, AC12, AC13, AC14, AC15, AC16, AC17, AC18, AC19, AC20, AC21, AC22.

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

Неблокирующие вопросы, составные формы, дополнительные каналы, arbitrary UI code, voice и публичный
хостинг отчётов не блокируют C09. Добавление новых функций требует отдельного
scope и совместимых versioned контрактов.
