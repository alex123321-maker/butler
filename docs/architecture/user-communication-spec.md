# Butler — общение агента с пользователем

Версия 0.12, 2026-10-23. **Статус: согласованный продуктовый контракт;
#127/#128/#132 в main; message/photo/report приняты в обоих каналах.
Ограниченный owner rollout требует отдельного operator profile.**
Полный контракт ниже остаётся целевым; доступность каждого метода определяется
готовностью и системным admission, а не наличием DTO.

Исходный анализ — `origin/main b20206d` от 2026-10-15; ревизия плана
2026-10-21 сверена с `origin/main ab28dd88`. Текущий ограниченный owner rollout
message/photo/report документируется отдельно; C05 не обновляет его автоматически.

### Граница текущей реализации

Core резервирует одно из `BUTLER_POSTGRES_MAX_CONNS` соединений для текущих
проверок lease/Task authority и token-only Catalog currentness, оставляя минимум два для бизнес-операций.
Минимум Core при запуске — три соединения; суммарный максимум, execution budgets
и сроки не увеличиваются. Долгий Telegram poll или Web feed не занимают этот
резерв. Проверки читают PostgreSQL и продолжают запрещать работу при отзыве,
истечении или недоступности authority. Кэш разрешений не вводится.

Продолжение после ответа (#142) читает канонический ответ под совместными
блокировками Session/run. Длительное чтение старой Web-ленты не требует
исключительного доступа к этим же строкам. Текущие lease/fence, Input claim,
Task/authority и Catalog остаются обязательными; после ожидания источников
повторно проверяются claim и срок lease. Это чтение DATA, не разрешение действия.
Изменения состояния, публикации и эффекты сохраняют прежние owning writers.
Для чтения ответа, подготовки, обновления run lease и финального сохранения
fencing входит в ту же транзакцию, что и соответствующая запись; срок lease
проверяется снова перед commit. Это оставляет соединение для watchdog даже
при пуле из двух соединений. Ожидание lease mutation по-прежнему происходит
вне пула; Task, claim, source и authority проверки не ослабляются.
Отказ продолжения хранит фиксированные phase/reason `interaction_resume_*`;
отмена по ownership watchdog сохраняет owning cause и при recovery. Heartbeat
различает `input_claim_lost` и `input_claim_renewal_failed`, не раскрывая SQL,
credential/body или заменяя независимую ошибку provider/истёкший deadline.

Для остановленного taskless explicit Input доступен узкий owner control:
`POST /api/v3/communications/inputs/{input_id}/close-failed` с прежним Bearer
proof, одним стабильным `X-Butler-Delivery-ID` и strict JSON `binding_id`,
`binding_version`, `expected_checkpoint`, `expected_error_code`. Текущие
owner/session и verified Web binding обязательны. Только `blocked` без claim,
живой lease и executing write может стать `failed`; UNKNOWN по умолчанию запрещён.
Для раннего `run_created/execution_stopped` отказа до `AdmitRun` владение
проверяется по durable accepted Input/session и текущему Web binding. Исключение
без admission разрешено только для совпадающих Web/Telegram source и session
channel; admission/obligation/grant не создаются. Остальные guard сохраняются.
Для остановки собственной неопределённой локальной публикации владелец может
явно передать `acknowledge_unknown_effects` — точный список её effect IDs. Это
допустимо только для уже `effect_unknown` prepare-эффектов `communication.*`;
executing и другие UNKNOWN (HTTP/browser/commit) остаются запрещены. UNKNOWN,
grants, evidence, reservations и доставка не меняются, эффект не признаётся
ABSENT/confirmed и не запускается повторно. Запрос прекращается с ошибкой,
а отдельная сверка эффекта остаётся обязательной перед новой попыткой этого
же эффекта. Точные IDs записываются в owner transition и события
`input.uncertainty_retained` / `effect.uncertainty_retained`. Task закрывается
прежним Task controller. Переход run в failed и `input.closed_failed` audit
атомарны с этими событиями. Обязательство получает `closed_with_failure`; question/answer,
checkpoints/errors, publications, reservations, observations и NULL usage
сохраняются. Повтор принятого close возвращает failed без второго audit;
содержимое body не может выбрать completed, retry, grant или новые allowances.
Этот control не является модельным инструментом, не воспроизводит эффект и
не означает успешную доставку/выполнение. Рабочая приёмка и rollout требуют
отдельных доказательств; наличие кода не переводит полный V1 в ready.

C07 (#105) добавляет к принятому ответу фиксированную kernel-инструкцию о фазе
продолжения исходного запроса. Она не содержит question/answer bytes: эти поля,
включая канонические подписи options, остаются private preference DATA с Catalog
provenance. После ответа этап вопроса завершён; повтор уже отвеченного вопроса
является behavioral failure. Новый вопрос допустим для существенно иной недостающей
информации. Exact-action approval, budgets, quiet/verified_task и owner verification
сохраняются. Повторная подготовка и approval recovery не дублируют инструкцию и
не превращают текст ответа в полномочие. Один успешный live опыт не доказывает
отсутствие ошибок модели; все неуспешные окна также входят в evidence и стоимость.
Проверка C07 использует отдельный content verdict вместе с owning kernel records:
purpose=result/ACK не доказывают содержательность. Публичный synthetic corpus и
self-review не объявляются независимой holdout-оценкой. Приёмка и фактический
статус rollout указаны [в отчёте C07](../ai/communication-obligations-acceptance-20261022.md).
Owner pause/resume переносит тот же interaction marker и потраченные allowances
в новый owning Input как для уже принятого, так и для позднего ответа. Новый run
читает canonical ответ через отдельный store reader после admission: только
перенесённый контроллером Input, совпадающие owner/session/task/revisions, текущие
claim/lease и Catalog позволяют получить DATA. Старый run/grant не возобновляется,
public payload не выбирает исторический вопрос. Это также проверяется настоящим
Core.Execute после двух порядков answer/resume, а не только статусом очереди.

C05 (#104) добавляет opt-in `Widgets` в private operator/lab composition:
single-choice, multi-choice и text в Web/Telegram. Полная приёмка фиксируется
[отдельной приёмкой C05](../ai/communication-widgets-acceptance-20261022.md); существующий рабочий message/photo/report профиль не
обновляется автоматически. Новая capability требует явно квалифицированных
binding snapshots; включение флага не переписывает старые admissions/receipts.

Миграция095 хранит общий multi-choice draft с отдельным `draft_version`,
короткий callback locator, идемпотентные channel-scoped receipts и журнал
изменений известного Telegram ACK. Draft/toggle не создаёт transcript response,
resume intent или grant. Submit проходит прежний `RespondTx`; только первый
ответ разрешает прежний owning Input. Повтор delivery возвращает тот же receipt,
другой payload конфликтует, stale version требует обновления. Чтение Web
показывает question/state/draft/answer; текст решения находится внутри вопроса,
а не дублируется в ленте сырым JSON.
После проверки ответа восстановленный prepared context сохраняет его как private
preference DATA до нового model turn. Узкий owning checkpoint writer сохраняет
`interaction_ready` и его identity: crash не теряет wakeup, последующий exact-action
approval не возвращает модель к вопросу без уже принятого ответа. Повторная
подготовка заменяет только DATA того же interaction, не добавляет второй ответ,
не меняет owner instruction и не восстанавливает старые tool grants.
Перед release ответа в model context owning worker читает канонический
`message:response:<interaction>` через тот же Catalog AccessTx. Текущие source,
scope, dependency и lease проверки захватывают этот source в `memory_run_sources`;
provenance ссылается на него, а не на канальный interaction locator. Ответ DATA
содержит отдельные `interaction_ref`, `publication_ref` и `source_ref`; schema
явно различает publication reply link и Catalog evidence refs. Последующее
удаление ответа запрещает cached context/publication; text проходит обычное
маскирование credential material. Callback refs не становятся Catalog authority.

Telegram callbacks проверяют numeric bot, private chat, sender, binding/version,
сохранённый sent ACK и текущий interaction; callback token не является authority.
Текстовый ответ требует явного reply на сохранённый вопрос. В Bot API10.3
используется непустой InlineKeyboardMarkup с `force_reply: true` и неактивным
индикатором ожидания (`disabled: {}`), без callback: он открывает reply и остаётся
редактируемым. Пустая keyboard на реальном API интерпретируется как standalone
ForceReply и запрещает edit; это отдельно проверено native contract probe.
Неизменяемый флаг сохраняется и при terminal edit;
сам terminal state проверяется backend. Inline buttons — канальное представление
тех же данных. Option media проходит прежние
Artifact/Catalog/export guards до publication и перед отправкой.

Редактирование проекции сохраняется отдельно от первоначальной доставки, по
notification ID и версии состояния. Worker сериализует edits одного ACK;
потеря ответа/процесса оставляет UNKNOWN без нового send или inference replay.
Ошибки edit не открывают вопрос заново. Web показывает неподтверждённое edit;
любая устаревшая кнопка всё равно проверяется backend. Поздняя исходная доставка
считывает terminal state и не создаёт активных кнопок закрытого вопроса.
Read/authorization исходной доставки и edit сначала берут shared session lock,
затем source/export guards могут читать run. Это тот же session → run порядок,
что у Respond/Resume: renderer не удерживает run, ожидая session владельца.
Locks освобождаются до канального HTTP; порядок не ослабляет policy проверки.
Privacy erasure удаляет производные drafts/receipts; rollback schema запрещён
при принятой widget history, для неё сохраняется compatible drain.

Mapping сверено с [InlineKeyboardButton](https://core.telegram.org/bots/api#inlinekeyboardbutton),
[InlineKeyboardMarkup](https://core.telegram.org/bots/api#inlinekeyboardmarkup) и
[editMessageText](https://core.telegram.org/bots/api#editmessagetext), 2026-10-22.

При совместном включении converters retained message/report не должен требовать
живую lease уже завершённого unrelated run. Developer-return guard сначала
проверяет фактически captured package provenance; только при её наличии
применяет прежнюю owned lifecycle проверку. Catalog/source/binding/media guards
сохраняются всегда. Policy-denied delivery, выявленная до исправления, остаётся
failed; перезапуск не превращает её в sent и не повторяет её автоматически.

По поручению владельца от 2026-10-22 обычные Core/Broker binaries поддерживают
явный `-communication-profile /private/profile.json`. Без аргумента сохраняется
legacy startup. Private strict JSON v1 фиксирует точные Database/OwnerID,
числовые BotID/ChatID, username, binding IDs/versions обоих каналов, ссылки
TokenFile/BotAPIProxyFile, authenticated ControlURL и Mode=active|drain.
Неизвестные поля, неверные значения, публично читаемый профиль и несовпадение
рабочей БД/owner запрещают запуск. Lab constructors по-прежнему требуют `_test`;
JSON/настройки/модель не могут снять этот запрет. Канальная привязка проверяется
через getMe до admission. Model HTTPS_PROXY обязателен, включая discovery/OAuth;
NO_PROXY не должен обходить прокси для внешних provider endpoints.

Это ограниченный message/photo/report профиль для одного владельца, а не
объявление готовности widgets/approval UX или всего V1. Существующий Task
Controller, approvals, Catalog, budgets и Broker неизменны. Только новые
доказанные канальные входы получают epoch1; старые runs/history не мигрируют.
InitialUpdateID используется только при создании нового numeric bot cursor;
оператор сохраняет pending backlog при cutover. Restart не перезаписывает
существующий cursor. Перед переключением останавливается единственный старый
primary, сохраняются dump, fresh deletion anchor и rollback configuration.
Откат начинается с Mode=drain на совместимом новом binary, а не с запуска
старого worker поверх живых epoch1 inputs/outbox/interactions.

По новому поручению владельца Web PR #131 интегрируется в изолированную
App-композицию #132. `WebBindingID` / `WebBindingVersion` — доверенные operator
параметры отдельного `_test` профиля; обычный `New` не включает этот путь.
Owner proof существующего HTTP API должен соответствовать тому же владельцу.
Web reader и planner используют актуальный Catalog/export guard и обязательные
retained media. Только такая композиция устанавливает `Store.WebAvailable`:
новые Web targets сохраняются как `result_ready`, а API представляет их как
`available`, без ACK/прочтения. Старые pending targets не меняются и не рассылаются.
Telegram по-прежнему отправляет только собственную закреплённую очередь.

Owner-authenticated `/api/v3/communications[/publication:ref]` читает полные
canonical payloads и независимые состояния/состав всех текущих targets. Cursor
покрывает просмотренные publication sequences, включая недоступные записи.
Отозванная запись возвращается без payload, files и manifests. `/media` разрешает
только attachment из этой разрешённой публикации; повторно проверяет sources,
binding, исходный run и SHA-256. Bytes передаются с no-store/nosniff, файлы — как
attachment; изображения ограничены PNG/JPEG. Произвольных URL/host paths нет.
Typed renderer экранирует текст, сохраняет все блоки, таблицы, captions и файлы;
HTML, скрипты и вложенные executable структуры не поддерживаются. Оба planners
используют одну shape-проверку плоских report blocks и прямоугольных таблиц.

SSE `/events?cursor=...` принимает также `Last-Event-ID` и передаёт только scoped
invalidation, без текста/refs. REST остаётся источником истины. Клиент после
reconnect и каждого pulse перечитывает загруженные страницы, включая delivery
changes/privacy, удаляет недоступные bytes/object URLs и не дублирует transcript.
Owner proof передаётся только через Authorization, не через URL/SSE. Смена proof
останавливает stream, запросы, очищает publications/receipts/drafts и media URLs.
HTTP instrumentation сохраняет Flusher; это проверяется настоящим HTTP stream.
HTTP request context следует App.Run, поэтому shutdown/restart отменяет открытые
SSE в пределах grace. Ошибки Telegram polling повторяют только чтение с
отменяемой задержкой1–30s и сохранённым cursor; они не останавливают Web/Core.
Sender/outbox остаётся независимым, UNKNOWN не переотправляется таким retry.
Единственная missing-response коррекция продолжает тот же run с новым актуальным
tool set и user-role reminder; системный prompt остаётся неизменным. Reminder не
создаёт новый owner Input и не расширяет полномочия.
Доверенный operator-параметр `DrainOnly` сохраняет чтение, обработку уже принятых
epoch-1 Inputs и outbox, но транзакционно отклоняет новые сообщения. Повтор того
же delivery identity восстанавливает прежний receipt. Profile сообщает
`drain_only`; Web блокирует новый ввод, сохраняя историю и вложения. Это режим
отдельной композиции, а не автоматический откат epoch-1 очереди к legacy worker.
Разрешённые Telegram exports не устанавливают `protect_content`: сохранение и
открытие файлов входят в контракт отчёта. Согласно [Bot API](https://core.telegram.org/bots/api#sendrichmessage),
этот флаг запрещает forwarding/saving. Privacy проверяется до экспорта; уже
полученную пользователем копию нельзя отозвать очисткой Butler history.

Новый Web conversation endpoint принимает только текст и owner delivery identity;
он не выбирает последнюю Task. Reply явно pin-ит publication/binding/version и
продолжает исходный canonical target через существующий Input/Controller.
`web_message` proof с BotID=0 сохраняется в owning transaction и защищён epoch 1;
Telegram proof по-прежнему требует закреплённый положительный BotID. Public
payload не устанавливает proof. Неподтверждённый Web профиль блокирует отправку;
только явный 404 обычного startup оставляет прежний legacy ingress.

Изолированный native App lab может передать programmatic `ProviderAuthStore`:
это reference на обычный Manager store с узкими refresh-only правами, а не
raw credential/config flag. JSON profiles не принимают это поле. Так проверяется
настоящий App.Run без копирования refresh-token lineage в тестовую БД. Native
App/model/UI приёмка отражается отдельно в evidence; само наличие lab не есть pass.

В #130 добавлена отдельная тестовая App/Broker-композиция: programmatic
`NewForTelegramCommunication` / `NewForCommunication` и operator commands на
изолированном `_test` PostgreSQL. Обычные main/New/config не переключены.
Admission ограничен закреплённой owner/bot/chat беседой; новые сообщения не
выбирают последнюю Task, replies разрешаются по сохранённому sent ACK.
Owner Stop/Pause/Resume/amend/prepare-only используют существующий Controller
с повторной проверкой binding в его транзакции. Недоступные widget/approval UX
не объявляются готовыми; существующие approval mechanisms не обходятся.
Source/export guard читает актуальный Catalog и полный dependency graph, а также
фиксированную operator privacy policy для конкретного bot binding. Private
export требует явного `AllowPrivate`; secret/stale/revoked запрещены всегда.
Вложения повторно проверяют policy источников как отправляющего, так и исходного
run. После process death между Input commit и cursor commit новый poller
переигрывает стабильную delivery identity без второго Input. Web ведётся в #129;
фактически пройденные native/UI gates указываются отдельно в evidence #130.
Для тестового Telegram-клиента operator profile может задать `BotAPIProxyFile`:
приватный файл с HTTP(S) proxy URL. Соединение идёт через CONNECT к фиксированному
`api.telegram.org:443` с обычной проверкой TLS; token остаётся внутри HTTPS.
Environment proxy не подхватывается неявно, redirects запрещены. Proxy credentials
не входят в модельные аргументы, публикации или сообщения об ошибках. Тестовый
native harness использует существующий приватный proxy без изменения production.
Native model harness также требует этот `HTTPS_PROXY`: до запуска проверяет
фактический proxy selection для discovery/inference/OAuth, исключает внешние
адреса из тестового `NO_PROXY` и отменяет попытку соединиться в обход proxy.
В диагностике остаются только счётчики соединений и фиксированные auth phases /
HTTP statuses. Такая настройка не доказывает успешную авторизацию или inference.
Перед native запуском существующий `cmd/provider-auth diagnose-auth` выполняет
обычный Manager Resolve/refresh под общим operator lock, с проверкой отсутствия
текущей production работы. Ротация сохраняется в исходной зашифрованной строке;
копия старой авторизации не импортируется. Тест использует штатный минутный
expiry margin. Тестовый запрет на refresh удалён: во время inference обычный
Manager также может продлить token через узкий store adapter, допускающий только
зашифрованную запись `provider-auth-refresh` для Antigravity. Остальные keys,
imports, plaintext и deletion запрещены. Наблюдатель проверяет
фактический HTTP socket, включая reused HTTP/2, и не отменяет локальные обращения
к Broker/PostgreSQL. Неопределённый refresh/persistence требует reconciliation.
Core и Broker тестового процесса получают одну и ту же finite budget policy;
persisted scope limits не сбрасываются при переходе границы процесса. Mode 2
выбирается только для изолированного native fixture; его synthetic approvals
разрешаются существующим approval service и не доказывают готовность Telegram UX.
Миграция093 закрепляет приватный immutable ingress epoch/proof внутри owning
Input/control транзакции. Модельный/public payload не выбирает этот режим.
Миграция094 хранит fixed transport clarification receipts с claim до сети;
неопределённые send/finish не повторяются. Доказанное отсутствие reply target
отличается от generic guarded SQL/policy failure, который сохраняет cursor.

После merge #127 начат #101 и Telegram-объём native Rich Messages из #102.
Codec `channel/telegramrich` остаётся отдельным модулем; основной
`channel/telegram` получил явный `NewExplicitAdapter` с durable poller/outbox,
подавлением model-final/status fallback и отказом от legacy rows в новом пути.
Обычный Bootstrap не вызывает этот конструктор и сохраняет legacy режим.
Выборка outbox, recovery истёкших claims/попыток, send authorization и Finish
ограничены точным binding ID/version/channel/recipient. Общий owner или chat ID
не даёт права потреблять очередь другого бота или Web.
Production профиль не меняется. В предыдущей scoped поставке #101
и #102 закрываются только в подтверждённом backend/Telegram объёме; Web остаток
сохранён в [#129](https://github.com/alex123321-maker/butler/issues/129),
настоящая App/Core/model-композиция и остаточная пользовательская приёмка — в
[#130](https://github.com/alex123321-maker/butler/issues/130). Полный AC01–AC22
контракт ниже не ослабляется таким разделением.

Новый adapter profile по умолчанию пуст. `Capabilities.Methods()` показывает
только реализованные методы; report требует полного native rich/photo/document
mapping. Отсутствие capability отказывает до publication writes, без fallback.
Завершённый report сохраняет отдельный canonical `communication_report` artifact
в одной транзакции с publication, transcript и всеми outbox parts. Его identity
производна от publication, а не от Telegram part; privacy очищает existing JSON.

Опциональный `Store.PlanDelivery` строит immutable части в одной транзакции
с publication/transcript/outbox. Photo/report admission требует этого hook;
композиция без него сохраняет message/widget-only foundation. Новый Telegram
planner сохраняет message части по 4096 UTF-16 units; photo — отдельный текст
и индивидуальные uploads с caption до 1024 units; report — native blocks через
`sendRichMessage`. Каждая часть имеет собственный outbox row/ACK. Рестарт
продолжает только сохранённые части; UNKNOWN не разрешает автоматический replay.

Миграция091 расширяет существующий Artifact Store типом `communication_media`
и `private_media_v1`: bytes/base64, name, MIME, SHA-256, current owner/source/
privacy policy. Референсы не являются URLs/host paths. Лимит artifact 10 MiB;
для PNG/JPEG проверяются MIME/декодирование/размеры. Rollback отказывается при
наличии media history. Уже существующий privacy lifecycle очищает content_json.
Manifest не копирует bytes и pin-ит ref/name/MIME/size/digest. Sender разрешает
bytes заново и повторяет owning claim/binding/source guard после этих чтений
и подготовки multipart непосредственно перед сетью. Без guard send запрещён.

Локальные пределы report: 32 KiB manifest, 50 blocks, 20 MiB uploads на часть,
100 частей на публикацию. Деление только по границам блоков; неделимый oversized
block, отсутствующее обязательное вложение или неподдержанный renderer дают
ошибку до publication writes. Это пределы Butler, не утверждение о лимитах API.
Raw HTML не исполняется; fallback на ссылку/PDF и платные рассылки отсутствуют.

Новый ingress проверяет private chat/user/bot и отдельную binding/version,
ответ на sent ACK поступает в существующий canonical Input/Task Controller.
`Poller.ProcessOnce` хранит bot/binding-scoped cursor в PostgreSQL (миграция092),
сериализует getUpdates и фиксирует его только после owning Input/control commit.
Потеря ответа между commit-ами воспроизводит исходную delivery identity.
Generic denial может скрывать storage error и не сдвигает cursor. Malformed
owner input/ambiguity не угадывают Task. Rollback отказывает для consumed cursor.
Poller и новые conversation admissions не подключены к обычному Bootstrap.
См. [адаптер](../../apps/orchestrator/internal/channel/telegramrich/README.md),
[изолированный lab](../../apps/orchestrator/cmd/telegram-rich-lab/README.md).

Далее зафиксирован объём foundation #127; утверждения photo/report contract-only
относятся к этой предыдущей поставке и композиции без нового planner.

По поручению владельца реализуются ядро #99–#100 и затем backend ожидания #103.
Есть strict versioned schemas четырёх методов, typed transcript/final, bindings,
атомарные message/widget publications в существующем outbox, exact dispatched
Broker control, reply obligations, одно исправление и durable blocking questions.
Миграции 089–090 добавляют эти записи и удаление копий при privacy redaction.
Review этой backend поставки добавил повторную проверку goal/mandate, точного
private Input claim и сроков после export-policy waits; terminal proof требуется
и для непустого explicit final. Quiet/verified_task после ответа на optional
вопрос завершается без требования нового result. Ошибка единственного correction
закрывает obligation с failure. Просроченный/закрытый open-widget payload не
отправляется; maintenance выбирает только actionable вопросы до batch limit.
См. [результаты review и критерии закрытия issues](../ai/communication-core-review-20261021.md).
См. [модуль](../../apps/orchestrator/internal/communication/README.md) и
[проверки и ограничения](../ai/communication-core-acceptance-20261021.md).

Следующая backend итерация добавляет owner-authenticated HTTP handlers без
регистрации маршрутов, ACK/publication reply resolver в canonical Input очередь
и version1 corpus четырёх форматов с native Rich Message expectation. `AdmitRun`
и optional Core admission hook закрепляют профиль после подготовки Task/sources
до загрузки каталога; startup hook не устанавливает. Потерянный ACK, каждый write
publication/response и handoff проверяются на PostgreSQL с process death.
Недопустимый export одного target хранится как existing outbox `failed` с
`policy_denied` и пустым payload; разрешённые targets продолжают доставку.
Response проверяет current channel policy; срок send claim/question проверяется
повторно после policy wait. См. [полную backend матрицу](../ai/communication-backend-full-acceptance-20261021.md).

Это программные точки композиции и изолированные проверки, не включённая
пользовательская функция. Bootstrap не регистрирует tools, private endpoint,
admission, sender, owner-control hook или maintenance loop; флага включения нет.
По умолчанию worker поддерживает epoch 0 и не берёт epoch 1. Web и существующий
Telegram adapter не изменяются. Photo/report пока только contracts; каналов,
вложений, канальных callbacks/renderers, UI и полной AC01–AC22 приёмки
эта поставка не заявляет. ChannelGuard обязателен для acceptance и send;
будущее подключение должно предоставить настоящую policy экспорта и artifacts.

Связанные контракты: [постоянный помощник](persistent-assistant-spec.md),
[durable execution](durable-execution-spec.md), [atomic result/delivery](atomic-result-delivery.md),
[run lifecycle](run-lifecycle-spec.md), [tools](tool-runtime-adr.md),
[action authorization](action-authorization-contract.md),
[owner controls](task-owner-controls-spec.md), [бюджеты](execution-budget-contract.md),
[план реализации](../planning/user-communication-roadmap.md).

GitHub tracking: [общий issue #98](https://github.com/alex123321-maker/butler/issues/98),
этапы #99–#107 и зависимости перечислены в плане реализации.

## 1. Продуктовые решения

Владелец подтвердил следующие требования в обсуждении 2026-10-15:

1. Агент явно вызывает инструмент общения, когда есть что сообщить.
   Завершение модели не является командой публикации.
2. Четыре функции: короткое сообщение, фотография с сообщением, большой
   отчёт с изображениями/файлами, интерактивный вопрос.
3. Одна публикация доставляется во все подключённые и включённые каналы
   этого владельца: в первой версии Web и Telegram.
4. Telegram-отчёт — нативный article / Rich Message. Web показывает полный отчёт.
5. Виджеты V1: выбор одного варианта, нескольких вариантов, текстовый ответ.
   Составные формы из нескольких полей отложены.
6. Виджет ждёт до ответа или отмены задачи; конкретный вопрос может иметь срок.
   Ожидание переживает перезапуск.
7. Первый действительный отправленный ответ фиксируется для всех каналов.
   Изменение решения — отдельное уточнение агенту.
8. Прямой запрос результата создаёт обязательство ответить. Если модель
   завершилась без публикации результата, Butler продолжает работу в пределах
   бюджета. При исчерпании сообщает системный сбой, сохраняя незавершённость.
   Автоматической публикации финального текста модели как fallback нет.

Инженерные решения ниже конкретизируют эти требования. Ключевые слова MUST /
должен обозначают требования целевого режима, а не состояние текущего main.

## 2. Что уже есть и что расширяется

| Механизм baseline | Использование в этой работе |
| --- | --- |
| PostgreSQL tasks, task revisions, owner controls (#4, #9) | Привязка коммуникаций к задаче, отмена, изменение условий |
| Durable ingress и approval continuation (#5, #6) | Сохраняемое ожидание и новый вход после ответа |
| Exact authorization, effect journal, budgets (#7, #11, #13) | Обычный Broker-путь и ограниченное продолжение |
| Completion coordinator и delivery outbox (#12) | Публикации с собственной идентичностью и доставкой по каналам |
| Artifacts и source/privacy catalog | Изображения, документы, provenance, проверка доступа |
| Owner-configured `verified_task` delivery | Существующее более строгое ограничение публикаций |
| DeliverySink и observability Hub | Точки интеграции и live-обновления; Hub не является durable bus |

Outbox уже отделяет сохранение результата от Telegram-send, учитывает UNKNOWN,
а Web-readiness не выдаёт за прочтение. Не нужно создавать второй независимый
доставщик с более слабыми гарантиями. Текущие rows ориентированы на run result;
их контракт требуется расширить на публикацию во время исполнения, media,
rich blocks, взаимодействия и несколько адресатов одного владельца.

В baseline обычный финал по-прежнему связан с формированием уведомления;
исключение `verified_task` не является универсальным инструментом общения.
Новый backend реализован отдельно и не подключён к baseline.

## 3. Границы ответственности

| Подсистема | Ответственность |
| --- | --- |
| Модель | Выбор момента, формата, текста и вопроса в рамках разрешённого каталога |
| Orchestrator / Task | Выполнение, ответные обязательства, pause/resume, task revisions, бюджеты |
| Session Service | Каноническая сессия, identity, порядок и live ownership |
| Tool Broker | Схема, точное разрешение, эффекты, маршрутизация, credential mediation |
| Communication module | Канонические публикации, взаимодействия, транзакции, события |
| Channel delivery | Долговечные попытки, адресаты, ACK, retry, UNKNOWN |
| Channel adapter | Отображение payload и нормализация подтверждённого пользовательского ввода |
| Artifacts / Catalog | Сохранённое содержимое, происхождение, privacy и разрешённое чтение |
| Model Transport | Нормализованные события модели; он не решает, что публиковать |

Communication — Go-модуль внутри существующего orchestrator. Broker вызывает
его через фиксированный аутентифицированный internal control endpoint по
существующему паттерну owning tools. Новый контейнер и внешняя event-платформа
для V1 не требуются. Это явное расширение списка owning control tools,
не разрешение вызывать канал мимо Broker.

Направление публикации:
`Model -> Orchestrator -> Tool Broker -> Communication -> PostgreSQL outbox -> adapters`.

Направление ответа:
`Adapter -> authenticated interaction ingress -> PostgreSQL decision + continuation
-> Session Service -> Orchestrator -> Model`.

Доставщик не имеет интерфейсов provider/tool execution. Ответ виджета не
исполняет browser action прямо из callback handler.

## 4. Модель данных

Имена таблиц ниже целевые; номер миграции выбирается при реализации по текущему
main. Каждый модуль сохраняет владение своими записями.

| Сущность | Обязательные поля и смысл |
| --- | --- |
| Publication | ID, schema version, owner, canonical session, optional task + revisions, origin run/tool/effect, kind, purpose, immutable payload/hash, artifact/source refs, created_at, publication sequence |
| Delivery target | Publication, channel binding ID/version, pinned recipient, representation version/hash, part index, state, attempts, deadline, claim, ACK |
| Interaction | ID, publication ID, owner/session/task/input binding, question schema/version, status, optional expiry, reply policy, continuation ref |
| Interaction draft | Owner + interaction + revision, временный выбор до Submit; durable там, где нужен межканальный sync |
| Interaction response | ID, actor proof metadata, channel/delivery identity, schema version, typed answer, accepted_at |
| Reply obligation | Accepted input или task/revision, required/quiet classification, state, response publication refs, continuation count, budget scope |
| Communication event | Event ID, aggregate ID/version, owner/session scope, type, time, opaque refs |
| Channel binding | Owner, adapter, recipient, verified identity, enabled state, version, capability/profile refs |

IDs, owner, session, task versions, lease, execution mode и адресаты определяются
системой. Модель не задаёт их как полномочия в аргументах.

Публикация неизменна после commit. Исправление — новая связанная публикация;
системное изменение состояния виджета обновляет его projection/version, сохраняя
исходный вопрос и принятое решение. Редактирование текста сообщений агентом и
произвольное удаление отправленного содержимого не входят в V1.

Один publication transcript event сохраняется на одну публикацию. Web и Telegram
delivery receipts не превращаются в два assistant-сообщения. Входящий ответ также
записывается один раз. Технический model final хранится отдельно как внутренний
результат попытки и не выглядит в обычной ленте как уже отправленное сообщение.
Скрытые reasoning traces в этот контракт не входят.

### 4.1 Публикация и внутренний итог в transcript и памяти

До первой исполняемой публикации C02 MUST разделить тип записи и роль автора:
`publication`, `attempt_final`, `owner_input`, `interaction_response` имеют
разные typed identities и visibility. Имена полей уточняются в C01; одна лишь
роль `assistant` или изменение prompt не определяют факт общения.

В проверенном main `internal/memory/transcript/completion.go:AppendFinalTx`
считает любые assistant-записи run кандидатами на единственный финал и требует
совпадения текста. Этот selector непригоден для нескольких публикаций и отдельного
итога в одном run. Explicit final persistence MUST искать собственную immutable
result/message identity и kind, не конфликтовать с publications и не дописывать
их повторно. Legacy recovery сохраняет прежние проверки; история не переписывается.

User feed и обычная история общения показывают publications и owner responses;
attempt_final остаётся типизированным внутренним результатом. Replay, compaction,
extraction, summaries и episodic consumers MUST сохранять это различие: внутренний
текст не становится утверждением «это уже сообщено владельцу». Полезные технические
факты допустимы в разрешённом внутреннем контексте со своей provenance. Отзыв
source/privacy действует на оба вида записей. Регрессия «две publications + иной
или пустой terminal final + recovery + compaction» входит в раннюю приёмку C02/C03.

## 5. Контракт инструмента

Одна логическая tool family `communication` имеет четыре типизированные функции:

- `communication.message`
- `communication.photo`
- `communication.report`
- `communication.widget`

Раздельные имена функций позволяют иметь строгие схемы без большого union
необязательных полей. Для пользователя это один инструмент общения.

Общие модельные поля: `schema_version=1`, `purpose`, содержимое функции,
необязательные `reply_to_publication_ref`, `source_refs`.
Допустимые purpose для первых трёх функций: `progress|result|blocked`.
Widget всегда `question`. Purpose — заявленный смысл, не доказательство
выполнения задачи. Система сохраняет собственные verification/effect states.

### 5.1 Message и photo

`message` принимает непустой `text`. Короткий ответ тоже может быть result.
`photo` принимает непустой `text` и `images[]` с `artifact_ref`, `alt`
и необязательной подписью. Ссылки должны разрешаться в готовые image artifacts.
Несколько изображений имеют явный порядок.

Payload не содержит base64, локальных путей, bot token, chat_id, произвольного
URL для серверного скачивания или учётных данных. Получение внешней картинки —
отдельное авторизованное действие; после него инструмент получает artifact ref.

### 5.2 Report

Поля: `title`, краткий `summary`, `blocks[]`, необязательные `attachments[]`.
Канонический документ хранится в Butler и не зависит от Telegram HTML.

Блоки V1: paragraph, heading, list, quote, code, table, divider, image,
document. Текст поддерживает ограниченные inline spans: plain, emphasis, strong,
code и безопасная ссылка. Image/document содержат artifact ref; tables — явные
строки/колонки. Raw HTML/JavaScript не являются исполняемым содержимым.
Неизвестные типы и превышение лимитов дают ошибку до commit, без тихой обрезки.

Report artifact имеет версию, hash и ссылки на все составляющие. Summary не
заменяет документ и не скрывает отсутствующее вложение. Артефакты должны быть
полностью сохранены и разрешены до публикации; сбой обязательного вложения не
даёт успешного ACK публикации.

### 5.3 Widget

Пример условного payload; названия/цены в реальном вопросе берутся из наблюдения:

```json
{
  "schema_version": 1,
  "question": "Какую пиццу желаете?",
  "input": {
    "type": "single_choice",
    "options": [
      {"id": "margherita", "label": "Маргарита"},
      {"id": "pepperoni", "label": "Пепперони"}
    ],
    "allow_text_answer": true
  },
  "wait_for_response": true
}
```

`single_choice`: непустые options с уникальными IDs, label, optional description
и image artifact ref. Ответ — один option ID либо текст, если он разрешён.

`multi_choice`: те же options плюс min/max selections. Выбор галочек остаётся
черновиком; только явный Submit фиксирует набор. Повторное нажатие toggle не
является отдельным пользовательским решением.

`text`: prompt, min/max length, optional placeholder. Пользовательский текст
проходит обычную классификацию доверия; поле ответа не меняет правила ядра.

`expires_at` необязателен. Его отсутствие означает отсутствие срока именно у
вопроса, но не бессрочную execution/mandate authority. В V1 все widgets блокируют
свою execution branch: `wait_for_response` отсутствует либо равен true.
Значение false отклоняется как invalid_payload до любого commit. Неблокирующие
вопросы отложены до отдельного контракта доставки ответа работающему исполнителю.

Составные формы, произвольные UI-компоненты и исполняемый frontend-код от модели
отложены. System approval может использовать тот же renderer, сохраняя отдельные
типизированные action-binding и проверку разрешения.

### 5.4 Результат вызова

```json
{
  "status": "accepted",
  "publication_ref": "publication:...",
  "interaction_ref": null,
  "delivery": [
    {"channel": "web", "state": "available"},
    {"channel": "telegram", "state": "pending"}
  ]
}
```

У widget возвращается interaction ref. Точные получатели, секретные URLs, proof,
claim IDs и grants не выдаются модели.

`accepted` означает durable commit. Это не `sent`, не прочтение и не
подтверждение semantic completion. Ошибки: invalid_payload, artifact_not_ready,
source_unavailable, policy_denied, stale_context, capability_unavailable,
quota_exceeded, persistence_unavailable. Ошибки имеют безопасные коды.

Повтор того же Broker effect/tool identity и payload возвращает сохранённый
результат. Другой payload под тем же identity конфликтует. Новый tool call не
считается повтором автоматически; content hash сам по себе не определяет intent.
При неопределённом ответе internal endpoint Broker сначала разрешает outcome по
durable operation ID, а не запускает новую публикацию.

## 6. Публикация, политика и транзакция

Обычный Broker prepare/authorize/claim сохраняется. Создание публикации —
локальный durable effect `prepare`; фактический channel send — отдельный
внешний эффект delivery worker. Prepare здесь не даёт права отправлять: точное
разрешение публикации заранее включает закреплённые destinations и media scope.

Мандат owner-communication разрешает обычные обращения к самому владельцу без
approval на каждый текст, если действующие policy это допускают. Его конфигурация
не создаётся моделью. Ни public URLs, ни source instructions не добавляют
получателей. Child agents по умолчанию возвращают результат родителю; прямые
публикации требуют явно выделенной capability в делегировании и общего бюджета.

Неопределённый эффект прежнего внешнего действия не должен лишать владельца
сообщения о препятствии или уточняющего вопроса. Broker получает узкое typed
исключение для разрешённой owner communication при unknown-effect stop.
Оно связано с фиксированным recipient scope, текущими источниками и finite
communication allowance; не объявляет публикацию read и не разблокирует покупку,
HTTP mutation, произвольный адресат или другой non-read инструмент.

Под текущим ownership/task/source guard одна транзакция фиксирует:
publication + canonical transcript event + response obligation linkage +
interaction/continuation при необходимости + destinations/outbox + event.
Никаких внешних сетевых send внутри транзакции.
Для widget эта же транзакция фиксирует owning Input checkpoint/wait intent,
ожидание Run/Task (если Task есть) и obligation; детали handoff — в разделе 10.

Доставщик после commit заново проверяет владельца, binding, актуальность
source/privacy и правила публикации. Он не заимствует execution lease уже
завершённого run. Короткая read authority для delivery не разрешает новые tools.

При owner amendment старые непосланные progress/questions становятся
superseded; неактуальные result требуют новой проверки. Исторический
подтверждённый эффект остаётся фактом. Отмена не стирает уже отправленное.

## 7. Шина событий и адаптеры

PostgreSQL — журнал принятых публикаций/ответов и источник восстановления.
In-process events / Redis wakeups могут ускорять доставку; потеря такого сигнала
не теряет публикацию. Общий журнал наблюдаемости не заменяет transactional outbox.

Типы событий V1: publication.created, delivery.changed, interaction.created,
interaction.draft_changed, interaction.resolved, interaction.expired,
interaction.cancelled, publication.redacted. Event envelope содержит opaque refs
и версии; body загружается только через проверку доступа.

Fan-out создаёт delivery targets для всех enabled bindings владельца в момент
commit. Повторная отправка не расширяет этот snapshot. Новый канал видит историю
в UI, но не получает автоматически весь backlog. Отключение binding останавливает
его pending deliveries; rebind на другого адресата не перенаправляет их.

Адаптер регистрирует versioned capabilities: message/photo/report/widget,
ограничения payload, media types, rendering version, supported reply controls.
Отсутствующая возможность отображается как unsupported; данные не отбрасываются.
Production V1 supported profile требует оба полноценные адаптера. Перед запуском
Doctor показывает несовместимую конфигурацию. C02/C03 проверяют минимальный
explicit profile только в изолированном fixture/test-owner окружении с системным
allowlist готовых методов. Это не owner-wide/default enable и не готовность V1.
Неготовые функции отсутствуют в модельном каталоге; прямой вызов отклоняется до
commit. Legacy admission не получает communication tools. Readiness и schema epoch
реализуются вместе с foundation, а C08 проверяет весь rollout/rollback.

Внутри канала сохраняется порядок publication sequence и частей сообщения.
UNKNOWN блокирует дальнейшие зависимые части, а не все задачи и каналы владельца.
System stop/error notification имеет отдельную ограниченную control lane.

REST feed с cursor является восстанавливаемым источником для Web. SSE доставляет
новые события, поддерживает reconnect/replay по event ID либо resync из feed.
Web availability, открытие страницы и человеческое прочтение не смешиваются.

## 8. Доставка и неопределённость

Состояния target: pending, sending, sent, available (Web), failed,
delivery_unknown, suppressed, superseded. Состояния payload/задачи отдельны.

Стабильный ключ включает publication, binding version, representation version,
part index. Rendered payload/hash и manifest фиксируются до сетевой попытки.
Каждая часть имеет собственный ACK. Агрегат показывает все каналы: Web available
не маскирует Telegram failed/unknown.

Используется существующая политика bounded claims/retries:
подтверждённое отсутствие отправки допускает retry; timeout, потерянный ACK или
истёкший sending claim переходят в UNKNOWN. В Telegram нет обещания exactly-once.
UNKNOWN нельзя автоматически исправлять повторным вызовом tool или модели.
Ручная повторная доставка требует owner control с явным предупреждением о
возможном дубле и сохраняет связь с исходной попыткой.

Изменение projection известного message ID тоже является отдельной операцией.
Ошибка редактирования закрытого widget не открывает interaction заново.
При поздней доставке исходного widget адаптер обязан показать его текущий
terminal state, а callback handler повторно проверить серверный статус.

## 9. Telegram article / Rich Message

Требование владельца: отчёты доставляются нативным Rich Message через
`sendRichMessage`. Реализация ориентируется на Bot API 10.3 или совместимый
endpoint с нужными media/document blocks. Обычная ссылка на Web или PDF не
считается выполнением этого требования.

Адаптер преобразует Butler report blocks в `InputRichMessage.blocks`; изображения
и документы становятся соответствующими media blocks. Подтверждение — Message
с корректными chat/message ID. Draft API не является durable delivery.

На дату проверки официальные ограничения rich message: 32 768 символов,
500 блоков, 16 уровней вложенности, 50 media, 20 колонок таблицы.
Валидация конкретного Telegram payload учитывает форматный счётчик API.
Большой отчёт делится по границам блоков на нумерованные Rich Messages с общим
publication ID. Непомещающийся неделимый блок даёт явную ошибку представления;
весь документ остаётся доступен в Web. Ничего не обрезается молча.

Private artifacts передаются сервером разрешённым способом upload/file reuse;
для доставки не нужно публиковать общедоступный artifact URL. Bot-scoped file
IDs хранятся вне model payload; текущие права проверяются и при reuse.

Документация Telegram проверена 2026-10-15:
[sendRichMessage](https://core.telegram.org/bots/api#sendrichmessage),
[InputRichMessage](https://core.telegram.org/bots/api#inputrichmessage),
[Rich Message limits](https://core.telegram.org/bots/api#rich-message-limits),
[InputRichBlockPhoto](https://core.telegram.org/bots/api#inputrichblockphoto),
[InputRichBlockDocument](https://core.telegram.org/bots/api#inputrichblockdocument).
При реализации нужна повторная проверка фактического endpoint и реальных клиентов.

## 10. Widget lifecycle и обратный путь

Состояния: open -> resolved | expired | cancelled | superseded.
Waiting execution — отдельное состояние Task/Input, не удерживаемая goroutine.

Создание вопроса коммитится **до** появления кнопок в каналах. Под текущими
ownership/session/run/task guards, в общем согласованном порядке блокировок,
одна транзакция сохраняет publication, interaction, data-only continuation,
owning Input checkpoint `awaiting_interaction`, wait intent Run/Task и
`waiting_for_user` obligation, outbox и события. У question-only conversation
те же гарантии для Input/Run, без искусственной Task. Сам lease освобождается
после commit; continuation содержит данные и refs, не Go stack и не секреты.

Core распознаёт durable suspension до обычного сохранения tool result и нового
model turn, в том числе при потерянном internal ACK. Между commit и освобождением
старого worker ответ уже может быть принят: response/ready marker сохраняется
монотонно. Поздний Finish/suspend старого worker использует expected checkpoint
version/CAS и MUST NOT заменить готовность обратно на waiting. Готовность не даёт
второму исполнителю работать одновременно: resume требует освобождения/истечения
старого ownership и свежего fence. Оба порядка «ответ → Finish» и «Finish → ответ»,
crash и потеря ACK должны давать ровно одно продолжение без повторного tool.

Ответ принимается через отдельный authenticated interaction control path,
который не стоит за собственным ожидающим input в FIFO. Он атомарно:
проверяет actor/owner и binding; под тем же порядком owning locks сверяет
schema/revision, task state и expiry по DB clock; CAS open->resolved; записывает
ответ/транскрипт и единственный resume intent в durable ingress; создаёт события
обновления. Input handoff не сбрасывает ready marker при позднем выходе worker.

Уникальность accepted response по interaction ID плюс channel delivery identity
исключает два продолжения. Повтор идентичного submit возвращает прежний receipt.
Конкурирующий другой ответ получает already_resolved; его содержимое не заменяет
принятое. Только полная отправка multi-choice фиксирует результат.

Текст привязывается к widget по явному reply/interaction reference. Если привязка
неоднозначна, система просит выбрать вопрос, не угадывает по последнему сообщению.
Отдельное обычное сообщение остаётся самостоятельным owner input/control.

Новый worker получает текущие lease, task/constraint/authority versions, catalog,
источники и бюджет. Старый provider session не является authority. Записанные
tool results не исполняются повторно. Перед покупкой проверяются актуальные
цены/наличие и необходимые разрешения.

No-expiry interaction не наследует 24-часовой timeout обычного accepted input.
Для waiting_for_user требуется явное расширение durable input lifetime:
время без исполнителя не расходует active execution timeout; spent budgets не
обнуляются. После ответа continuation получает новый ограниченный execution
window по текущей policy. Истёкший mandate блокирует действие, но не теряет ответ.
Это изменение должно быть реализовано в owning Input/Task, не обходом timestamp.

Owner pause сохраняет ожидание; ответ может сохраниться, но не запускает работу
до resume. Cancel закрывает вопрос и запрещает продолжение. Amendment делает
вопрос старой revision superseded; новый вопрос создаёт агент после проверки.
Expiry сохраняет состояние и один управляющий event; повторного опроса по таймеру
и циклических напоминаний без отдельной policy нет.

Для V1 допускается один открытый blocking widget на execution branch.
Неблокирующих вопросов в V1 нет. Ответы сериализует Session
Service; отвечать из Web на Telegram-вопрос разрешает подтверждённая owner
identity, но не совпадение отображаемого имени.

Выбор пиццы — preference DATA. Он не подтверждает оплату. Approval продолжает
проверять exact action, args hash, resource, revisions и authority. Нажатие кнопки
в ordinary widget не создаёт grant.

### 10.1 Обычные ответы из другого канала

Fan-out сам по себе не объединяет сессии владельца. Для каждого подтверждённого
канального представления хранится приватное соответствие binding/version +
chat/message ID -> publication -> canonical session/task и исходные revisions.
Web передаёт publication ref через owner-authenticated ingress; Telegram reply
разрешается через сохранённый ACK mapping, а не отображаемое имя или args модели.

Обычный ответ на message/photo/report, например «переделай второй пункт», получает
проверенный target исходной публикации даже из другого канала. Ingress повторно
проверяет текущего owner, binding/source/privacy и состояние/revisions Task;
старый reply не восстанавливает отозванные права и не меняет constraints молча.
Input с explicit target поступает в каноническую Session Service очередь один раз.
Текст «продолжай» без доказанного target не выбирает последнюю задачу владельца.
При неоднозначности применяется существующее разрешение task target владельцем;
новое сообщение в выбранной conversation остаётся обычным новым input.

Явный reply на открытый widget проходит typed interaction path раздела 10;
обычный ответ на публикацию не становится автоматически interaction response.
Cancel/amend/pause/resume проходят existing authenticated owner-control path и не
стоят за собственным waiting input. Неоднозначный control не получает расширенный
scope. Новая очередь, lease authority из канала или автоматическое слияние всех
session одного owner не создаются.

## 11. Обязательство ответить и завершение

`communication_mode=explicit` закрепляется системой при admission; модель его
не переключает. В нём assistant_delta/final остаются внутренними transport
событиями. Model final сохраняется как результат попытки, без implicit outbox.

Завершение попытки, состояние obligation и постановка нужного исправляющего
continuation фиксируются согласованно в owning completion/input transaction.
Повтор сохранения frozen final не вызывает модель и не создаёт второй continuation.
Пустой финальный текст допустим при корректном terminal transport event, когда
результат уже явно опубликован или запуск имеет quiet policy. Тогда сохраняются
typed terminal metadata и ссылки на публикации, без выдуманного публичного текста.
Неожиданное закрытие stream не становится успешным завершением. Старое требование
обязательного непустого final изменяется только для нового admission profile.

При прямом conversational input по умолчанию создаётся reply obligation, даже
если Task отсутствует. Для длительной задачи обязательство связано с task/input
и переживает отдельные runs. Quiet background trigger имеет явную owner policy;
не всякий внутренний wakeup создаёт новое обязательство.

Состояния obligation: open, waiting_for_user, response_committed,
closed_with_failure, cancelled. Delivery state хранится отдельно.
Progress ничего не закрывает. Blocking widget атомарно переводит связанное
обязательство в ожидание; принятый ответ возвращает его в open для продолжения
при действующей policy (pause откладывает выполнение, cancel закрывает его).
Result либо содержательное blocked-сообщение связывает ответ с обязательством.
Постановка доставки закрывает необходимость повторно генерировать ответ, но
недоставленные targets остаются операционной проблемой.

Если модель завершилась при open obligation без blocker/widget/result, ядро
создаёт идемпотентное bounded continuation с указанием недостающего ответа.
В V1 максимум **одно исправляющее продолжение на obligation revision**;
оно расходует обычные turn/token/time budgets. Сброс run, рестарт или смена
канала не обновляют allowance. Правильный результат нового действия не должен
дублировать уже подтверждённый внешний эффект.

Если исправление невозможно или снова не дало ответа, task/input остаётся
blocked с communication_result_missing; система публикует один безопасный
failure notice. В нём нет raw model/provider errors. Неисправность самой
доставки видна в owner UI/Doctor; гарантировать уведомление по сломанному каналу
невозможно.

Kernel notice имеет стабильную identity и отдельный allowance максимум один
сверх 20 agent publications. Модель не может запросить этот allowance.
Если нет разрешённого канала или export policy отказала, obligation всё равно
закрывается с failure и durable диагностическим событием; повторного inference
или raw-final fallback нет. Owner UI/Doctor для этого события относится к C08.

Контроль obligation структурный: он не гарантирует содержательный ответ.
Purpose=result не подтверждает критерии Task. Семантическая проверка результата,
effect evidence и owner-confirmed completion сохраняются. «Сейчас сделаю» с
ложной меткой result может обойти простую структурную проверку: это отдельный
behavioral acceptance failure, а не доказательство готовности. Eval обязан
проверять соответствие текста фактическому выполнению.

Режим `verified_task` сохраняет более строгую owner policy: tools не могут
обойти запрет progress/unverified result. Разрешённые вопросы/blocked interventions
доступны по policy; запрещённая публикация даёт явный policy_denied. Для него
не запускается бессмысленное исправление с требованием запрещённого сообщения.
После owner verification системная verified-notice транзакция использует общий
publication/delivery механизм, сохраняя прежний смысл и once-only identity.

## 12. Identity, privacy, storage и API

Binding создаёт/изменяет аутентифицированный владелец. Telegram требует numeric
From.ID + разрешённый chat, Web — существующее owner proof. Display name, model
args и заголовок с actor ID не являются доказательством. Секреты остаются на
сервере в существующей credential/config подсистеме.

Все read/send/reply пути проверяют owner, current source/privacy и artifact
scope. Fan-out не расширяет разрешённый export: если источник нельзя передать
в Telegram, этот target получает suppressed с причиной policy_denied.
Владелец видит, почему каналы расходятся. Модель не снимает ограничение.

Бинарные данные хранятся через artifact layer на self-hosted managed storage;
PostgreSQL владеет metadata, hashes, references и жизненным циклом. Atomic publish
ссылается только на готовые объекты; orphan staging убирается отдельно.
Переезд на внешний object storage не является обязательным условием.

Удаление/отзыв источника инвалидирует pending send, report access и exports.
Для уже доставленного содержимого нельзя обещать отзыв с устройства получателя;
best-effort delete/edit сохраняется отдельным результатом. Model resume и память
используют канонические source refs и не восстанавливают отозванный текст из ACK.

Предлагаемые owner-authenticated API:

| Endpoint | Контракт |
| --- | --- |
| GET /api/v3/communications | Cursor feed по owned session/task |
| GET /api/v3/communications/{id} | Payload, разрешённые artifacts, состояние по каналам |
| GET /api/v3/communications/events | Scoped SSE; reconnect/resync |
| GET /api/v3/interactions/{id} | Вопрос, schema, draft и terminal state |
| POST /api/v3/interactions/{id}/draft | Expected revision; не submit |
| POST /api/v3/interactions/{id}/responses | Typed answer, expected version, stable delivery ID |
| GET/POST /api/v3/channel-bindings | Owner-only управление разрешёнными bindings |

Backend C01/C04 имеет отдельно создаваемый, не зарегистрированный handler
`CommunicationOwnerControls` для GET/POST bindings, POST ordinary replies и
POST typed responses. Он использует существующий `OwnerControlAuth`; Web
recipient фиксируется как `web:<authenticated owner>`, body/header owner ID
не является proof. POST требует stable delivery ID и strict bounded JSON object.
Обычный `POST /api/v3/communications/replies` принимает Web publication ref либо
Telegram ACK из trusted adapter с binding/version. Неоднозначность — 409,
никакой «последней задачи». Ответ на widget требует typed response path.
Распознанные существующим owner classifier stop/pause/resume/prepare-only команды
проходят существующий Task Controller, вне waiting FIFO. Binding/ACK/source proof
повторяется в control transaction; expected Task revision берётся из publication.
Отозванный execution mandate не мешает владельцу остановить задачу. Без controller
composition API отказывает явно, а не передаёт такую команду обычному worker.
Остальные endpoints таблицы, feed/SSE/drafts и канальные callbacks остаются C05.

Adapters нормализуют Telegram callbacks/replies в тот же service API.
HTTP 202 publication acceptance не означает delivery ACK. Invalid schema — 400,
нет доступа — 403/404 без утечки existence, stale/resolved conflict — 409,
bounded quota — 429. Дубли идентичного запроса возвращают сохранённый receipt.

## 13. Лимиты и наблюдаемость

Owner config задаёт конечные лимиты публикаций, байтов, media и open interactions.
Разумные стартовые V1 defaults для fixtures: message 4 000 Unicode characters;
photo до 10 images; report до 200 000 characters, 2 000 blocks, nesting 8,
20 table columns, 100 media; widget до 20 options, text answer до 4 000 characters;
не более 20 agent publications на accepted request и один open blocking widget
на execution branch; общее число открытых вопросов ограничено owner policy.
Это Butler limits, не заявления о лимитах каналов. Media byte limits наследуются
из artifact policy и пересекаются с текущими adapter limits.

Превышение возвращается модели до частичного commit. Канальные ограничения
проверяются при построении manifest; большой report может дать несколько rich
частей. Неограниченная генерация сообщений из повторного wakeup запрещена.

Метрики: accepted publications, delivery latency/backlog, failed/unknown по
каналам, retry counts, interaction age, response conflicts, resume latency,
missing-result continuations, blocked obligations, rendered payload limits.
Labels не содержат owner text, secret, arbitrary URL или безграничные IDs.
Audit хранит stable refs и версии; payload читается только по разрешению.

Doctor показывает migration/tool/adapter readiness, bindings, Rich Message
совместимость и недоставленные результаты. Наличие подключённого адаптера не
равно успешной реальной доставке или визуальной корректности.

## 14. Миграция и совместимость

Внедрение поэтапное и additive. Исторические results, transcripts и outbox IDs
сохраняются; их не переписывают в новые публикации с повторной отправкой.
Mapping legacy delivery остаётся читаемым.

Полный production `explicit` admission profile включается только после готовности
tools, каналов, interactions и obligation control. До этого C02/C03 обязаны
проверить минимальный message flow на изолированном fixture/test owner: публикация,
продолжение tools, terminal final без implicit send, bounded missing-response
recovery и сохранение legacy поведения. Неготовые методы закрыты readiness gate;
тестовый профиль не обходится моделью и не объявляется поддержанным V1.
Режим ортогонален Task
`result_delivery=attempt|verified_task`. Legacy admissions продолжают прежний
путь; для одной попытки никогда не работают два автоматических финальных send.

На mixed-version deployment старый worker не должен брать explicit checkpoint.
Capability/schema epoch и admission flag обеспечивают это. Rollback выключает
новые admissions, но оставляет совместимый drain/resume worker для принятых
публикаций и вопросов. Down migration отказывает при живых данных; no-expiry
вопросы нельзя молча удалить ради downgrade.

Prompt instructs: публиковать только полезные updates; применять правильный
формат; не повторять опубликованный результат в конце; ждать явного ответа;
не утверждать доставку по accepted; не объявлять успех по ACK tool.
Prompt не заменяет enforcement и не раскрывает внутренние summaries в канал.

## 15. Приёмка

| ID | Сценарий и проверяемый результат |
| --- | --- |
| AC01 | Progress опубликован, затем агент продолжает tools; final не даёт второго сообщения |
| AC02 | Photo + caption из сохранённого artifact видны в обоих каналах |
| AC03 | Report с текстом, таблицей, картинкой и файлом открыт в Web и в нативном Telegram Rich Message |
| AC04 | Большой report разбит по блокам без потери текста/media, ACK каждой части сохранён |
| AC05 | Telegram недоступен: Web доступен, task/effect не повторяется, retry только delivery |
| AC06 | Crash после publication commit и до ACK: восстановление без повторной генерации; UNKNOWN не resend |
| AC07 | Widget показан в двух каналах; конкурентные ответы дают одно решение и одно продолжение |
| AC08 | Multi-choice toggles — draft; только Submit фиксирует набор; text reply привязан к нужному вопросу |
| AC09 | Restart и ожидание более 24 часов не теряют no-expiry вопрос; execution budgets не обновляются |
| AC10 | Ответ до выхода initial worker/окончания доставки: оба порядка response/Finish, crash и lost ACK сохраняют один resume; поздняя копия widget закрыта |
| AC11 | Cancel/amend/expiry/pause и stale authority не запускают устаревшее действие |
| AC12 | Модель завершилась без ответа: одно bounded исправление; затем explicit failure без успеха task |
| AC13 | Quiet background run без новости даёт ноль публикаций и ноль исправляющих циклов |
| AC14 | Повтор tool/response identity идемпотентен; другой payload конфликтует |
| AC15 | Отзыв binding/source/artifact запрещает pending send, export и stale replay |
| AC16 | verified_task, child scope и approval сохраняют authority; выбор пиццы не разрешает покупку |
| AC17 | Уже опубликованный результат + final-save failure не создаёт вторую публикацию |
| AC18 | Mixed version/rollback сохраняет старые receipts и принятые interactions |
| AC19 | Несколько publications и отличный/пустой terminal final одного run не конфликтуют; replay/compaction не выдают внутренний итог за сообщение владельцу |
| AC20 | Обычный reply на message/report из другого канала продолжает исходный explicit target; ambiguity, wrong owner и stale revisions не выбирают задачу молча |
| AC21 | Ранний изолированный explicit message flow продолжает tools и завершается без дубля; readiness не допускает неготовые методы и не меняет legacy admission |
| AC22 | Widget V1 принимает только blocking wait; false отклоняется до publication/continuation/outbox commit |

Unit tests проверяют схемы, rendering и state transitions. PostgreSQL integration
проверяет транзакции, CAS, restart, fencing и outbox. API/adapter tests проверяют
typed wire и authorization. Behavioral harness проверяет правильный момент
публикации, фактическую завершённость и отсутствие спама.

Нужен отдельный live acceptance на тестовом владельце в Telegram и Web с
зафиксированными версиями клиентов и доказательствами отображения/ответа.
Синтетический Bot API ACK не доказывает живую доставку; screenshot сам по себе
не доказывает успешное продолжение задачи. Реальный заказ/оплата для pizza
acceptance не нужны: разрешённый fixture проверяет остановку перед commit.
Пропуски и недоступные проверки фиксируются отдельно от passes.

## 16. Не входит в V1

Неблокирующие вопросы, составные формы, arbitrary executable widgets, сообщения посторонним адресатам,
голосовые ответы, пользовательский HTML runtime, автоматическая публичная
публикация отчётов, автоматическая покупка по preference-кнопке и бесконечные
напоминания. Новые каналы подключаются через тот же versioned adapter contract.
