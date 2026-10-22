# Ограниченный owner rollout общения — 2026-10-22

По прямому поручению владельца PR #128 слит первым (`2cf62d39628817a68c510b804e55287f81ad9b92`),
затем #132 переведён на main и слит (`afc1f0a9c9843bf7cb9b9cc9600fa1038a18b304`).
Локальный main обновлён fast-forward; untracked prototypes сохранены.
У обоих исходных heads CI успешен; #132 имеет9/9 successful jobs.

Принятые constructors намеренно ограничивали БД суффиксом `_test`. Для реального
cutover добавлен отдельный strict operator profile, а не снята защита lab path.
Profile v1 фиксирует Database, OwnerID, BotID/ChatID/username, два binding ID и
их версии, приватные ссылки token/proxy, ControlURL и Mode=active|drain.
Неправильная БД/owner, неизвестные поля, публичные permissions, raw token поля и
внешний proxy bypass отклоняются. No-argument startup сохраняет legacy.
InitialUpdateID касается только нового cursor; рестарты не перематывают историю.

До deployment в отдельной PostgreSQL17 БД прошли96 targeted checks с race,
0 fail/skip, vet, локальные Windows checks и сборки. Это startup-specific
проверки поверх ранее принятой полной матрицы #132; native inference не повторялась
для unit/integration gate. Бинарники Core/Broker/runtimes собраны Go1.27.2,
Web собран из принятого source для существующего API адреса лабы.

Рабочая БД, Redis, owner proof, source history, spent/unknown budgets, mobile
receipts, schedules и независимый deletion anchor должны сохраниться. Перед
cutover сохраняются private pg_dump, current config и свежий deletion anchor;
primary не запускается параллельно. Cutover backlog Telegram фиксируется
отдельно; исторические тестовые публикации не импортируются в рабочую БД.
Core и Broker получают один scoped profile; Converter dependency hashes
должны соответствовать фактическому бинарнику, независимо от HTTP health.

Rollback: сначала Mode=drain с новым совместимым Core/Broker. Сохраняются
readers, принятые epoch1 Input/outbox и их budgets; новые канальные сообщения
не принимаются. Старый worker не должен потреблять epoch1; старый DB dump
никогда не восстанавливается поверх новых receipts. Перед любой последующей
сменой primary нужен новый dump и самый свежий deletion anchor.

На подготовке корневой раздел имел около9MiB свободного места. Сохранённый
Go build cache `/home/laba/.cache/go-build` перенесён в собственный storage root,
исходный путь оставлен symlink; освободилось около540MiB. Данные сервисов,
чужие images/volumes/logs не удалялись. Сборки, test PG и backups — на `/srv/storage`.

Этот rollout не закрывает #104–#107 и не объявляет полную готовность widgets,
Telegram approval UX, всех AC01–AC22 или semantic external-task verification.

## Результат cutover

Рабочий `butler-main` переведён на ограниченный owner profile
`message/photo/report`, Mode=active. Приватный operator startup принят в #133
(`851cc10886e39ab6a81f4e7e8d1fe1b13fb598f9`). Дополнительное исправление #134
слито после успешных четырёх CI jobs в
`3274da6541d401faf833d5d670847de6cc23e9e6`.
CI head: `06cd18c1d5be83b11c40bbfe083ade7eadb41859`;
[PostgreSQL communication/converter matrices](https://github.com/alex123321-maker/butler/actions/runs/39718118070),
[Web](https://github.com/alex123321-maker/butler/actions/runs/39718118186),
[discovery](https://github.com/alex123321-maker/butler/actions/runs/39718118266).

Первый native запрос выявил ошибку композиции guards: включённый converter
требовал worker ownership даже для завершённой публикации без converter
provenance. Публикация сохранилась, Web скрыл её, Telegram записал
`failed/policy_denied` без ACK. В #134 проверка ownership остаётся обязательной
для действительно закреплённых developer-return sources; обычные Catalog,
source, binding и export guards не отключены. Исправленный reader в drain
прочитал ту же публикацию без inference replay. Неудачная Telegram receipt
сохранена и не отправлялась повторно.

После активации свежий Web запрос «Новый режим общения: проверка v2» прошёл
через реальный model transport, owner approval и `communication.message`:
run `run-1792641184695577661-13c92fe414b65ddb`, Web `result_ready`,
Telegram `sent`, native ACK `42`. Оба представления показывают публикацию.
Подтверждения действий в текущей `mode_2` проходят через Web; отдельный
Telegram approval UX этим rollout не заявлен.

Нативный свежий Telegram input также завершён:
`run-1792641274441325994-8c4d6784afbd76c3`, ответ «Вход Telegram принят»
виден в обоих каналах, Telegram `sent` с ACK `44`, Web `result_ready`.
На финальной проверке 03:55:33 UTC все три epoch1 inputs завершены,
65 прежних epoch0 inputs и исторические owner_day rows неизменны,
неизвестные эффекты и budget policy сохранены. Все 16 постоянных сервисов
работают, все имеющие healthcheck — healthy. Неавторизованный profile API
возвращает 401; authenticated profile сообщает ровно три доступных метода.

Сборка production Go binaries: `f1881e683a79288c11fe3e8e70db55a5c4e9cb3c`.
Более поздние коммиты CI head меняют только CI и test fixture; production Go
код совпадает. Web собран из принятого #133 head, Web код после него не менялся.
Первый пакет оказался динамически связан с glibc и не запустил migrator в
Alpine; отказ случился до migrations, legacy был восстановлен. Затем все Go
бинарники пересобраны с `CGO_ENABLED=0` и проверены в фактических target images.
В статический пакет добавлен неизменённый существующий `tools.json`.

Первая CI попытка #134 обнаружила нестабильную подготовку deadline fault:
четырёхсекундный TTL истёк до захвата проверяемого lock. Исправлен именно
fixture: ожидание наблюдается через `pg_locks` на уже занятом соединении,
enqueue начинается перед неизменяемым deadline. Финальный сценарий повторён
три раза с PostgreSQL17/race, 3 pass, 0 skip; затем обе CI матрицы прошли.
Локальная полная converter выборка имела 61 pass и два явных ContainerE2E
topology skips; эти отдельные topology checks не выдаются за успешные.
Обязательные выбранные CI сценарии не допускают skips.

Приватные dump, config, fresh state/anchor, логи и числовые evidence находятся
в `/srv/storage/butler/agents/communication-owner-rollout-20261022`.
`deployed-main.json` различает merged source, binary build source и проверенный
CI head. Runtime SHA256 проверяются по `/proc/1/exe`, Converter profile — по
реальному executable. PostgreSQL/Redis и существующий owner token сохранены;
OAuth использует прежний canonical Manager. Telegram и model transport
используют обязательный proxy; внешний NO_PROXY bypass исключён.
Web UI проверяется через локальный SSH relay из-за системного browser proxy;
LAN HTTP адрес проверяется отдельно, production API origin не меняется.

У первого smoke сохранены 5/5 model attempts с input/output usage, но provider
`total_tokens` не был удержан до замены контейнера. Этот total остаётся
неизвестным; нулём он не считается. Для последующих smoke полный structured
usage log сохраняется до какой-либо замены Core.
Всего в этом rollout 15 model attempts, 15 input/output observations:
48 656 input и 462 visible output tokens. У последующих десяти попыток
сохранён provider total 34 246 tokens (включая скрытый output); total первых
пяти неизвестен. Эти значения не объявляются полным расходом счёта.
