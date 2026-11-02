# Наблюдение страницы после неизвестного браузерного действия — #163

В исходниках добавлен ограниченный срез: после сохранённого ответа UNKNOWN
для click/fill/type Core может один раз прочитать ту же вкладку через новый
`browser.freeze_snapshot`. Затем run остаётся blocked / effect_unknown.
Это сохранение наблюдения для последующего разбора; модель не возобновляется,
remote acceptance и успешный login по DOM не объявляются. Полная интерактивная
приёмка #163 ещё не выполнена.

Изменены Orchestrator/Input recovery, контракт/политика Broker и браузерный
runtime. PostgreSQL остаётся владельцем intent/grant/checkpoint; новых миграций,
зависимостей и сервисов нет. Наблюдение не копирует исходные значения полей или
credential refs, использует новый точный grant и расход, текущую authority и
пятиисекундный дочерний deadline. Новое согласование не подставляется: при его
необходимости чтение не выполняется. Потерянный ответ исходного действия не
запускает автоматический probe.

Runtime закрывает HTTP/CONNECT соединения и блокирует все маршруты перед
чтением документа. Он сохраняет masking, не навигирует и не вызывает input/
submit handlers. Контекст остаётся замороженным; GET form/link не могут его
разморозить. Старый snapshot сохраняет прежний смысл. Все owner/epoch/domain/
expiry проверки остаются. При resource quarantine чтение также запрещено:
локальный ACK завершённой операции и UNKNOWN внешнего эффекта — разные факты.

Checkpoint хранит исходные call/effect refs и отдельный детерминированный read
call. Маркер сохраняется и на обычной границе dispatch нового чтения. Recovery
из observing/dispatch/observed, а также malformed/null marker, блокирует run
до обращения к модели. История, старый UNKNOWN, grant и расходы не очищаются.

## Проверено

Изолированный стенд на ssh laba, сеть container-private, одноразовый pgvector17
в tmpfs. Shared Compose, provider/settings, действующие Input и старые journals
не менялись. Последний источник: snapshot20261029-001451-040, родитель21488db0;
точные production hashes и Docker image IDs сохранены рядом с результатами.
Локальная проверка каждого из13 hashes против текущих исходников прошла.

- Go1.27.2 / PostgreSQL / race: **40 tests/subtests PASS, 0 FAIL, 0 SKIP**.
- Настоящий Chromium: **11 tests/subtests PASS** — включая фоновые GET/POST,
  отсутствие навигации, masking, невозможность thaw, прежние формы/GET readers.
- Настоящий Broker + PG + resource boundary + Chromium: **2 PASS**. Получатель
  фиксирует один POST; отдельное чтение завершается, старый эффект остаётся
  UNKNOWN. Повтор того же grant и новая мутация не вызывают второй POST;
  расход остаётся трём реально допущенным операциям. Неизвестный локальный
  исход сохраняет карантин и запрещает даже frozen read.
- Vet и сборка обоих тестовых бинарников прошли. Core unit tests проверяют
  отсутствие model resume/final, lost ACK, отказ/изменение разрешения, чужой
  результат, отдельный read grant, запрет второго probe и ограничение deadline.
- PG recovery fault injection проверяет observing/dispatch/observed и null
  marker: после рестарта нет модели или повторной отправки. Это тесты настоящих
  checkpoint/lease/task границ со stub-provider, не нативная LLM-приёмка.

Сохранены все шесть попыток. Первая обнаружила ошибку доменного cap в новом
тесте; вторая — отсутствие migration102 в прежнем resource test fixture;
третья — реальную несовместимость новой output schema (`const` без поддержанного
type). Fixture получил каноническую пустую таблицу истории; схема исправлена
на typed enum, добавлен отдельный registry regression. Четвёртая прошла Broker
проверки; пятая — весь финальный набор. Дополнительная шестая проверка общей
resource-композиции дала39 PASS/0 FAIL, но wrapper завершился с exit1 из-за
одного SKIP: чрезмерно широкий PG-фильтр захватил отдельный неизменённый
`TestSingleTabResourceActualChromiumNativeHostIntegration`. Он требует другого
native-host стенда и не проверялся в этом срезе. Его исключение из будущего
PG-фильтра указано явно; шестая попытка не переименована в PASS и не повторялась
ради показателей. Обязательные53 проверки server-browser имеют0 SKIP.

## Время и границы результата

В последней полной проверке монотонные Linux uptime интервалы составили:

| Этап | Время, с |
| --- | --- |
| PG/race | 13,30 |
| vet | 0,72 |
| сборка browser.test | 0,97 |
| сборка broker.test | 1,11 |
| Chromium | 6,76 |
| Broker/PG/Chromium | 1,97 |

Это время тестовых команд с прогретым кешем, не скорость нативного агента.
Новых обращений к настоящей модели — **0**, повторов baseline ради метрик — **0**.
Прежние20 прогонов/188 полных шагов вместе с размышлениями модели сохранены
без изменения; см. [отчёт времени](browser-action-timings-2026-10-28.md).

Безопасная сводка с временами всех неудачных и успешных фаз:
[validation-summary.json](browser-baseline/uncertain-observation/validation-summary.json).
[Source hashes](browser-baseline/uncertain-observation/production.sha256),
[образы](browser-baseline/uncertain-observation/source-images.txt).
Полные исходные логи сохранены на стенде в
`/srv/storage/butler/agents/server-browser-outcome-20261029/reports/`.

PR основан на #175. Этот срез **не merged и не deployed**. Старый live Core и
Broker/browser продолжают работать на прежних закреплённых сборках. Оставшиеся
POST/login/consent slots baseline остаются UNEXECUTED: этот код не даёт права
обойти старый неизвестный write и ещё не реализует безопасное продолжение
следующих изменений. Следующий обязательный шаг #163 — проверяемое разрешение
неизвестного исхода и ограниченное продолжение, а затем нативная приёмка через
штатный credential owner flow. Общий browser qualification остаётся NO_GO.

Воспроизведение: scripts/dev/browser-outcome-tests.sh OWNED_ROOT all;
browser/broker режимы оставлены для узких повторов после исправления тестов;
resources проверяет общую PG-композицию, явно исключая другой native-host e2e.
Каждая попытка создаёт новый каталог, старые результаты не перезаписываются.
