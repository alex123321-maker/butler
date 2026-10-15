# Независимая исходниковая готовность semantic memory trial

Дата: 2026-10-15. Рецензент: `/root/memory_independent_review`.
Автор реализации: `/root`.

**SCOPED_SOURCE_READINESS: PASS. OFFLINE_PREPARE_ALLOWED.**
Два оставшихся замечания закрыты, исправленный код и целевой proof
сопоставлены независимо. Этот вердикт разрешает подготовку отдельного
бинарника и metadata; **native admission остаётся WITHHELD до отдельной
сверки actual prepared source/binary/runtime pins конкретного свежего root**.
Старый gate, sealed окна и прежние failed попытки не переоткрываются.

## Привязки проверенного кода

Сверка выполнена после завершения тестов. Все девять изменённых/новых Go
blobs текущего worktree совпадают по raw SHA256 с проверенным snapshot
`/srv/storage/butler/agents/memory-eval-semantic-20261015/snapshots/20261015-163929-581`.
Будущий commit с этим документом должен отдельно получить source-tree и
compiled-binary pins; настоящий документ не приписывает ещё не созданный
commit или бинарник предыдущему запуску тестов.

| Файл | SHA256 raw bytes |
| --- | --- |
| memory_eval_semantic_native_test.go | `79f6256b6d11fd229540d6461d21d5ef845362bd97aba980940d76d56d979d80` |
| memory_eval_semantic_test.go | `c0b7c1679bf6a28d0c9a8c9f09ce11f871271f28f37fe856649e4d148321ab28` |
| memory_eval_collector_test.go | `ce5329850b5ca40e5559242accaeda7da8805bfa69d81e7d01f99946cd5d28fd` |
| memory_eval_native_test.go | `e67daa22798478b8fddb4f117e2205a65cab6ef8b78dc064459256c0ab1ab6b3` |
| cmd/memory-eval/main.go | `9c8dba72e1ebe7e8096f333cae870b5cded81cba17a2661c425b27c7eb0ed8b7` |
| cmd/memory-eval/semantic.go | `cd7f07b5289870839bc5e96a635ece9fa9e771f9ca7a2f0b624ecd2aa4f2504a` |
| cmd/memory-eval/semantic_test.go | `7f68920fd81116dfed08bc5106f52b279e62f63418206964a4d231491480d6f9` |
| internal/acceptance/memory_semantic.go | `ba5c7af90a6e2f1dd19c3f97ff6655ad521441d7b8dd98ddd9198ab0d4227560` |
| internal/acceptance/memory_semantic_test.go | `c3ca56f3d41ca949e7b5dc99b23f72120745c34f437554ecff6799e51bb26c35` |
| scripts/dev/memory-eval-semantic-lab.sh | `67bc3c9cf42f94d29af67a2ed8b87ad026eaca6a2de831b7e17860212f28487a` |

Последний shell отличается от проверенного в snapshot
`34336cd63996af24903938d47082ce2249eec2821609c537c2f3fb74b917a4db`
ровно добавлением второго аргумента memory-eval к chmod755. Это делает
CLI исполнимым для UID10001 при offline audit. Сверка полного diff и
отдельный bash -n новых bytes прошли; Go bytes не менялись.

Корпус остаётся
`9992c0ae196aa85ebdd1b0869aa435d733296f43164a30c8e989d012c5ea973b`,
методика —
`ab41d8ec5040804d9060cdcf58762d3ef342329a9d2174533535141c42c75132`.
Их смысловая разметка и калибровка приняты в
[memory-eval-semantic-preflight-review-20261015.md](memory-eval-semantic-preflight-review-20261015.md).
Прежняя переоценка 58 originals и десять calibration labels повторно не
выполнялись и не изменялись.

## Исправления, проверенные независимо

1. **Runtime до private state.** Go проверяет source/binary/corpus/method
   pins и actual execution metadata до чтения local token, native import
   и нового auth store. Проверяются image/command, UID10001, 2CPU,
   1536MiB, 128PIDs, readonly rootfs, отсутствие privileged/CapAdd,
   CapDrop=ALL, no-new-privileges и ограниченный noexec/nosuid tmpfs.
   Полный набор bind mounts сверяется по type, source, destination и RW:
   source/build/metadata и только два разрешённых auth leaf refs readonly,
   private writable. Для writer разрешены изолированная trial network и
   local-inference; read добавляет backend и требует фиксированные
   proxy/local bypass. Значения credential material не экспортируются.

2. **Сохранение prefix при SQL ошибке.** Defer finishSemantic установлен
   до capture/native spend. Успешный writer передаёт исходное открытое
   окно read; ошибка или конечный read закрывает его. errors.Join
   позволяет попытаться записать measurements и ledger даже после
   неудачного SQL close, а ошибка удержания дополнительно вызывает seal.
   Потеря SQL не превращается в success и не препятствует доступной
   файловой записи уже потраченного prefix.

3. **Ограниченное расписание без замены результатов.** Один и тот же
   collectMemorySemanticSamples используется actual entry и fixed38
   fixture. Provider availability failure сохраняется как failed и
   позволяет перейти к другому предобъявленному input; safety/auth/
   retention failure прекращает entry. Попытка не повторяется.
   fillUnused вызывается после учтённого sample и добавляет нулевые
   not_used строки только действительно не вызванным стадиям.

4. **Достаточная конечная длительность.** Writer сохраняет Started и
   Deadline=Started+4h, reader загружает тот же Deadline. Отдельный
   модельный turn ограничен пятью минутами; test timeout245m и внешний
   timeout250m обеспечивают конечное завершение/остановку. Shell trap
   останавливает контейнеры, пытается закрыть SQL window и сохраняет
   результат внешнего seal. Неудача этого best effort остаётся ошибкой,
   а не доказательством успешного закрытия.

5. **Проверка после перезапуска.** Shell дожидается успешного выхода
   writer, перезапускает собственный PostgreSQL и создаёт отдельный
   read process. Reader вновь открывает pool/manifest и вызывает
   актуальный captureRead вместо подстановки writer capture.
   Действительное выполнение этих фаз ещё предстоит проверить по
   runtime receipts; исходниковый порядок не назван завершённым trial.

6. **Честные arms и результат.** Model projection не включает rubric,
   expected/required facts, oracle selection rules или review labels.
   Oracle получает только заранее выбранные достаточные оригиналы.
   Обычный retrieval передаёт actual prompt с пустыми source bodies.
   Повторы используют тот же context и измеряют повтор ответа модели.
   Pipeline latency теперь одинаково учитывает shared capture для
   retrieval и retrieval_repeat; actual расходы поиска не удваиваются.

7. **Исторические результаты сохранены.** Free-answer schema и marker
   pending independent review выделены отдельной веткой. Старые sealed
   originals и strict scores не переписываются. Offline scorer требует
   уникальные полные input/output bindings и ровно одну рецензию для
   каждого completed; failed/skipped не получают выдуманных semantic
   verdicts. Отсутствующая planned observation вызывает ошибку полного
   scoring, а не сокращение знаменателя ради успеха.

## Фактически прочитанный test proof

Финальный proof:
`/srv/storage/butler/agents/memory-eval-semantic-20261015/reports/memory-semantic-verify-20261015T094106Z-2068364`.

- complete: PASS; SHA
  `c26de83abdc9496cd1301470918ec39ecca1cf389ef0ae1c6504da1800d1c431`.
- targeted.jsonl: 37 test/subtest PASS, 0FAIL, 0SKIP; SHA
  `c299ba79bf24a77389bba6b7661d7a9fd9aa1b918d1609fc58b9bef2c5d6ee55`.
- cli.jsonl: 4 test/subtest PASS, 0FAIL, 0SKIP; SHA
  `02300e9477a984f42cdf33d6d29a18c3d3c1102137dec542d8898225fa806625`.
- Итого41 test/subtest PASS. Package PASS строки считаются отдельно,
  не прибавлены к числу тестов. diagnostics.log пуст.
- Реальный PG/fake-provider fixed38 сохраняет второй provider timeout,
  выполняет третий и все остальные entries один раз, сохраняет38
  observations/attempts и ровно один failed.
- SQLFailureStillRetainsFileIntegration: PASS0.76s. После настоящего
  fake-embedding prefix закрывается pool; finishSemantic возвращает
  SQL error и всё равно сохраняет measurements с расходами.
- Reopened pool/manifest, ранняя write embedding failure, ранняя native
  construction failure, runtime drift negatives, projection isolation
  и free-schema checks прошли. Race/vet выполнены по операторскому
  протоколу этого целевого proof; JSON результаты проверены независимо.
  Новый shell дополнительно проверен рецензентом bash -n.

Предыдущие093804Z/093934Z proofs сохранены отдельно. Последняя проверка
охватывает добавленный fillUnused и SQL-failure test; им не приписывается
успех до их появления в исходниках.

## Граница допуска

Разрешена подготовка одного свежего trial на этих проверенных runtime
bytes, с отдельной сверкой exact source tree, compiled collector/CLI,
review/corpus/method pins, ресурсов и actual control plane. Будущий
native gate должен назвать конкретный root и максимум38 предобъявленных
native inputs без retries; подготовка сама не разрешает inference.

Проверка не утверждает качество будущих ответов, доступность провайдера,
полный task effect или поддержку всех путей памяти Butler. Измеряемая
milliseconds — wall clock entry коллектора, включающий его проверки,
input/auth и получение ответа; это не выделенная provider-server latency.
Invoice/host overhead и более широкие эффекты остаются отдельно.

F09 использует реальный Catalog.Delete для безопасного placeholder до
индексирования: это deletion fence/tombstone, а не доказательство очистки
ранее существовавших embedding derivatives. F01/f05 проверяют датированные
исправления в отдельных документах, а не замену revision одного source.
Данный документный контур не доказывает все transcript/profile/
consolidation/update/delete пути. Независимый модельный рецензент также
может ошибаться.
