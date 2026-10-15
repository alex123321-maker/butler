# Независимый prepared/native entry gate semantic trial

Дата: 2026-10-15. Рецензент: `/root/memory_independent_review`.
Автор реализации: `/root`.

**SCOPED_PREPARED_NATIVE_ENTRY_READY: PASS.**
Допускается ровно один execute для указанного ниже свежего root, в пределах
ранее разрешённых пользователем новых сценариев. Это готовность собрать
оригиналы, не semantic quality PASS и не общая приёмка будущего результата.

## Единственный разрешённый root и точные pins

Root:
`/srv/storage/butler/agents/memory-semantic-trial-20261015b`.

Release:
`c8193fa7eee04ff4097a4c8943447f4dfe080976`.

Prepared source:
`/srv/storage/butler/agents/memory-eval-semantic-20261015/snapshots/20261015-164602-865`.

| Материал | Независимо прочитанный/пересчитанный SHA256 |
| --- | --- |
| Source tree, 2617 files | `43887f96dd72bb41683faaff22a65c70630bb5b109d7b1dcaf00e319933c9e6c` |
| build/collector.test | `e956488c2ea57aa09e87c406bcb91dff6949d20c1db082d86ee71d1aadbbee25` |
| build/memory-eval | `740ed985fecda78261392b5e46cd86ba12ed3ebb4c715a97a8e8dd3e014690b1` |
| metadata/semantic-pins.json | `ae49adc56028de91908e086f516c0bfb3a4b7a2524d4bca2f7974c973c93c936` |
| metadata/independent-review.sha256 | `35df8cceba7fb5c6d08cf265f9192df13661f3fe7282ba4f24414f181e4cdfdd` |
| Corpus | `9992c0ae196aa85ebdd1b0869aa435d733296f43164a30c8e989d012c5ea973b` |
| Methodology | `ab41d8ec5040804d9060cdcf58762d3ef342329a9d2174533535141c42c75132` |
| Rubric preflight review | `373b1f0e1f9f823f561bd36e1b200f56d102285760d7da11c0b113d39566571b` |
| Calibration review | `d208529a9ada3358f599938bd6246a07f4f9987cc757e355b1d2d791dcce0973` |
| Independent source review | `203c8a20718ff339b1bec9d0f3f617a3b8d84b1c967632b7e2bcdb30c4b5a237` |
| Prepared launcher | `a967c03c5d716954319c4c4dbb2f4744480b72dcde9452be35ca228e07b7b9d0` |

Runner image ID:
`sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61`.

Source tree пересчитан независимо совместимым с acceptance.TreeHash
алгоритмом: сортированные relative paths, NUL, SHA256 raw file bytes,
LF; исключены только .git/.cache/node_modules/reports, symlinks не
допускаются. Corpus/method/review/core-helper/test pins совпали.
Core helper остаётся79f6256b, test c0b7c167: исправления с41PASS
не изменены.

На момент сверки entered отсутствовал. Private принадлежит UID10001,
mode0700; оба бинарника mode0755. Build diagnostics пуст.
Свободно830459904 bytes на / и127793283072 bytes на /srv/storage
в момент проверки; это операционный снимок, не гарантия будущего места.

## Последнее исправление подготовки

Предыдущий triala, по сообщению оператора, завершил offline prepare
ошибкой permission denied при запуске /build/memory-eval до создания
БД и до auth/embedding/native. Он сохранён и не подменяет этот root.

Единственное изменение launcher относительно принятого source review:
pins-only docker run выполняется с UID:GID владельца нового артефакта.
Это необходимо для обхода mode0700 build directory во время подготовки.
Проверен точный git diff; отдельный bash -n нового launcher прошёл.

Actual write/read containers сохраняют UID10001, cap-drop ALL,
no-new-privileges, readonly rootfs/source/build/metadata/auth leaf mounts,
ограниченный tmpfs, 2CPU/1536MiB/128PIDs и прежние phase network/proxy
ограничения. Offline UID исправление не переносится в actual phase.

## Конечный допуск и исполнимые условия

Разрешены12 случаев ×3arms плюс два предобъявленных повтора:
максимум38 native inputs. Каждый выполняется один раз; availability
failure сохраняется и не заменяется повтором. Общая память строится
реальным ограниченным Catalog/index/embedding/read контуром в свежей
собственной БД, без импорта старых наблюдений или переоткрытия окон.

Writer фиксирует общий Deadline=Started+4h. Reader использует тот же
Deadline после writer exit и PostgreSQL restart; продление не разрешено.
Модельный turn ограничен пятью минутами. Safety/auth/authority/retention
failure останавливает entry с сохранением prefix; внешняя остановка и
SQL seal остаются исполнимым best effort, результат которого сохраняется.

Фактические контейнеры ещё не запускались при этой prepared сверке.
Допуск условен на принятие actual execution metadata уже проверенным Go
validator перед private backend/token/native auth каждой фазы:
exact image/args, UID, caps/security, mounts/source leaf refs, ресурсы,
network set, native proxy и local bypass. Несоответствие должно завершить
фазу до private credential access и model spend. Отдельный остановленный
runtime snapshot до entry для этого уже проверенного executable gate
не требуется.

Этот документ создан в review worktree после подготовки и не добавляется
в закреплённый source snapshot. Source hash43887f96 и prepared binaries
должны остаться неизменными.

## Что проверяется после сбора

Нужна отдельная корреляция writer exit → PostgreSQL restart → new read,
actual runtime receipts, exact input/original output/events/version/usage,
всех38 planned entries и расходов с SQL ledger и window counter/expiry.
Failed и skipped сохраняются отдельно; им не выдумываются semantic
verdicts. Повторы с одним capture оценивают модель при одинаковом input,
не второй независимый retrieval.

Смысловые оценки будущих originals ещё не выставлены. Приёмка
документного harness, диагностическое качество ответов, доступность
провайдера, цены по заявленному условию подписки и actual task effects
остаются разными выводами. Рецензент не читал credentials/key/import/
encrypted settings и не выполнял inference/auth/embedding. Независимая
модельная рецензия также может ошибаться.
