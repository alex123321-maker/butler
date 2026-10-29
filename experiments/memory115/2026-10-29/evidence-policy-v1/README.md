# Выбор доказательств и остановка бесплодного чтения: model-free v1

На laba реализована и проверена предварительная политика чтения опыта.
**Новых обращений к внешней модели:0.** Прежние A/B/C, failed/UNKNOWN/NULL,
расходы и их оценки не изменялись. Рабочий Butler не обновлялся; #115 остаётся
OPEN/NO_GO, PR158 — draft. Польза для качества и реальные token savings не
заявляются: для них потребуется отдельное нативное сравнение.

## Что изменилось

Поиск применимого типизированного урока возвращает в существующем `locator`
компактный `experience_evidence_v1`: исторические verification/owner_verdict и
6 точных диапазонов исходника — verification, verdict, unknowns, actual actions,
failed calls, model notes. Можно непосредственно открыть нужное наблюдение вместо
последовательного чтения environment/usage/всей provenance. Диапазон содержит
Unicode offset, max_chars≤8192 и total_chars; `memory.open` использует тот же
source/revision/access/hash с пустым fragment_key и обычной повторной проверкой.
Текст не переписывается и не становится новой принятой записью/цитатой.

Историческая проверенность явно называется `historical_task_verified`; она не
отмечает текущую задачу успешной. UNKNOWN и effect_absent не превращаются в
доказательство допустимости или недопустимости payload. Указатели создаются лишь
после полного Catalog/lesson proof и проверки настоящей typed metadata, а не по
форме JSON или тексту модели. Обычные источники сохраняют прежние locator.

Core получает фиксированную инструкцию выбирать конкретный недостающий факт,
проверять actual evidence раньше model_notes и прекращать поиск отсутствующего
в исходном UNKNOWN доказательства. Инструкция восстанавливается без дублирования;
данные памяти и текущая воля владельца не превращаются в kernel instruction.

Для выбранного experience consumer проверка Core до новой авторизации/dispatch:

- до8 записанных попыток `memory.search/open/neighbors` на текущий run;
- до3 поисков; два последовательных поиска без новых точных версий источников
  прекращают discovery, даже при перефразировании query;
- полностью уже прочитанный диапазон не открывается заново. Учитываются Unicode,
  объединение диапазонов, разрывы, частичное перекрытие и EOF;
- failed/unconfirmed попытки не возвращают allowance и не доказывают новые данные;
  только completed receipt с подтверждённым Broker effect задаёт novelty/coverage;
- состояние восстанавливается из PostgreSQL именно текущего run. Новые объекты
  сервиса и перезапуск не обнуляют его. Другой run/owner не заимствует историю.

Discovery exhaustion допускает открытие уже найденного непрочитанного диапазона;
другие разрешённые действия задачи остаются доступны. Если модель всё же предлагает
нарушающее правило чтение, обычный LoopError/stopBudget прекращает run с сохранённым
checkpoint и незавершённой задачей. Это сознательно консервативная граница, которая
может остановить и полезную сложную работу; успешный ответ не подделывается.
Storage/source/authority ошибки не маскируются под отсутствие новых доказательств.
Наружу идёт фиксированная метка, trusted unwrap сохраняет причину.

Это политика сериализованных Core proposals, не новый rate limiter приватного
HTTP endpoint. Постоянный Broker по-прежнему авторизует и исполняет допущенные
вызовы. Его каталог не сокращается ради оптимизации, поэтому уже закреплённые
зависимости не становятся неприменимыми как в старом C43. Существующие глобальные
лимиты, actual costs, источники, privacy, lease, task/constraint и release guards
сохранены. Нет миграции, новой зависимости, настройки/тайного счётчика в памяти
процесса или включения отклонённого adaptive87. При отсутствии experience consumer
policy и новая инструкция не устанавливаются. Release qualification остаётся false.

## Проверка на laba

Отдельный root `/srv/gitea/butler-acceptance/issue115-evidence-20261029-v1`,
source base4ed0dac7 плюс проверяемый delta, новые PostgreSQL/pgvector DB/schema.
Go1.27.2 и PG17 образы закреплены, modules readonly/GOPROXYoff, network=none
(Go/PG используют общий изолированный namespace), без provider auth и host ports.
Работающие сервисы и старые базы не использовались для тестовых записей.

- [PG/reader/experience и actual Broker](validation-v3.log): race-проверки
  correction/deletion/ancestor applicability, C43 dependency prefilter, обычные
  источники, отрицательный feedback и настоящие приватные Broker dispatch.
- [Финальная узкая проверка и vet](final-v4.log): выбор полей/исходный текст,
  Unicode offsets, union/overlap/gaps/EOF, failed/unconfirmed spend, новый source
  revision, состояние после реконструкции сервиса, foreign run, отсутствие policy
  при typed nil. Core stop проверен до authorizer/executor, восстановление guidance
  и ошибки без raw storage details. Новые обязательные проверки не пропускались.
- Orchestrator package прошёл с `-race`; его старые интеграции общего task store
  и PG+Redis имеют отдельные env prerequisites и в этом package run не исполнялись.
  Их отсутствие не засчитано за новые native/operational доказательства. Старый
  внешний backup test сознательно исключён из общего memory package run: данный
  change не меняет схему/backup/storage writes,3 настоящие restore фазы выполнены
  и сохранены в предыдущем [C43 отчёте](../c43-stale-fix-v1/README.md).
- Первая actual Broker проверка нашла `{}` вместо locator из-за несовпадения
  порядка JSON keys с canonical sanitizer. Исправлена сериализация, а не защита;
  [неудачный v1 журнал сохранён](rejected-locator-v1.log), финальный Broker test PASS.
  Scripted in-process fixtures не являются внешними модельными обращениями.

## Записанные первые шаги старых контролей

Из неизменённых raw выгрузок взяты все48 контролей и73 memory reads, все73
corroborated confirmed. [Проектор](project-sealed-reads.py) только читает старые
JSON; [Go replay](prefix-replay.go.txt) исполняет настоящую новую функцию политики,
без модели и без возобновления старых run. [Результат](read-prefix-replay.json),
[журнал](prefix-replay.log).

| Старый вариант | Recorded memory reads | Допустимые первые чтения | Причина следующей остановки |
| --- | ---: | ---: | --- |
| C16 |11|8|read_bound|
| B18 |23|8|read_bound|
| C41 |11|6|search_exhausted|
| B42 |22|8|read_bound|

Другие44 записанные цепочки не остановлены. Ни один положительный записанный
контроль не получил дополнительной остановки. Это ретроспективный screen на уже
известном корпусе, а не независимый holdout. После первой изменённой операции
старое продолжение модели не является контрфактическим результатом. Нельзя
приписать сэкономленные tokens, новый task success или исправленный старый UNKNOWN.
Чтение полей также не доказывает смысловую правильность ответа будущей модели.

## Сохранность и расходы

Прежний v5 остаётся A14/16, B14/16, C13/16. Его уникальные известные полные
tokens3058291 и inclusive primary9848908 не пересчитаны. Inclusive остаётся
1049native/1045known/4unknown и1051product/1045observed/6NULL/2without-native.
Отдельные25/90951 diagnostics и legacy NULL/4175/3389 сохраняются отдельно.
Invoice cost неизвестен. Синтетические test-provider usage не добавляются в
журнал реального native расхода.

Старые raw/inventory/audit/C43 архивы проверяются по прежним SHA256, новые журналы
и тестированный source сохраняются отдельным архивом. Собственный новый PG
останавливается при0 клиентах; все старые результаты остаются на месте. Итоговый
receipt и точные source hashes: [verification.json](verification.json).

Остановка своего PG при0 клиентах подтверждена. Все16 испытанных Go-файлов
совпали с локальным checkout по SHA256. Новый архив522240bytes скопирован на этот
ПК в `D:\CodexData\private\butler115-evidence-policy-v1`; обе копии имеют SHA256
`2cf5829f46d13d66666e7277fdb516ae61765757dbadbab5ca4e16e179728841`.
[Backup receipt](backup-receipt.json). Python здесь — только stdlib read-only
проекция запечатанных синтетических JSON; сервисная логика и проверки политики — Go.
