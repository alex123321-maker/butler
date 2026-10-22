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
Фактический результат deployment записывается после cutover и проверки.
