# Ревью серверного браузера

19 октября 2026. Проверена реализация на `2ed4b5db` относительно актуального
`main` `57156dd5`. Общий стек на лабе не изменялся. Исправления сохраняют обычные
Broker grants, credential mediation, public egress и operator domain caps.

## Найденные и исправленные замечания

| Приоритет | Дефект и последствие | Исправление | Регрессия |
| --- | --- | --- | --- |
| P1 | Вызов, допущенный до takeover и задержанный в транспорте, мог дойти после release. Snapshot/close не имеют observation CAS. | Broker передаёт приватный dispatch epoch; runtime сравнивает его под session mutex. Release продвигает runtime epoch. | Реальный Broker/runtime gRPC: старый epoch отклоняется после Take/Return; текущий snapshot и продолжение проходят. Unit проверяет также отсутствие, дубликат и некорректный epoch. |
| P2 | Старый snapshot/extract, записанный после Take/Return, мог очистить `fresh_needed` нового поколения. | Обновление выполняется только для epoch, сохранённого в контексте Broker admission. | PG integration записывает старый completed result после release: флаг остаётся true, старое предложение не получает dispatch. |
| P1 | Frame token проверял только главный документ. Изменение cross-origin iframe не мешало клику по устаревшему изображению. | Capture до/после JPEG и проверка перед вводом включают identity/revision всех документов и состав дерева фреймов. | Настоящий Chromium: независимый child DOM меняется с подтверждением recipient; ввод со старым кадром отклоняется, дополнительного POST нет. |
| P2 | Накопленный Enter мог отправиться после неопределённого результата предыдущего ввода. | Любая ошибка control request очищает keyboard queue; unmount также очищает очередь и адрес. | Playwright держит text request, ставит Enter в очередь, возвращает 409 и подтверждает отсутствие последующей отправки клавиши. |
| P2 | После отмены задачи владелец терял возможность release; ручные background writes оставались разрешены до TTL. | Release текущего controller проверяет owner mapping/hash/epoch, но не требует активной задачи. Это только снятие существующей write capability. | Реальный PG/Broker/Chromium: cancelled task запрещает frame и чужой release; правильный controller успешно завершает управление. |
| P1 | Последовательная замена короткого private input prefix раньше полного значения могла оставить суффикс пароля в model text. | Все raw/encoded совпадения ищутся в исходном тексте; маскируется объединение перекрывающихся диапазонов. | Реальный Chromium посимвольно вводит пароль, затем открывает отражающую его страницу. Model text должен быть в точности `[REDACTED]`, без префиксов/суффиксов. |

Потерянный release ACK по-прежнему оставляет durable owner fence закрытым.
Явное восстановление Take/Return проверено с новым runtime epoch. До ACK takeover
уже выполнявшаяся операция может завершиться; совместного ввода владельца и
агента нет. Запущенный inference не отменяется задним числом.

## Проверки после исправлений

Изолированный root `/srv/storage/butler/agents/server-browser-20261019-a7f9`,
финальные неизменяемые исходники `snapshots/20261019-130847-076`. Отдельный Web
protocol тест запускался на `snapshots/20261019-130315-438`; после него Web исходники
не менялись. Проверки используют
`scripts/dev/server-browser-lab.sh`. Только собственные контейнеры и caches,
network-none PG, без опубликованных host ports и без Compose общего стека.

- `browser`: PASS, `TestBrowserSessionE2E` со всеми восемью subtests и
  `TestServerBrowserChromium`, включая iframe и посимвольную password regression.
- `web-real`: PASS, production Web build/typecheck/lint, 1/1 Playwright и
  `TestServerBrowserBrokerIntegration`; обязательные acceptance проверки не пропущены.
- `web`: PASS, typecheck/lint и 1/1 Playwright, включая очередь Enter после 409.
- `unit`: Go `-race`: 277 pass events, 0 fail, 74 явно опциональных skips;
  `go vet` изменённых backend/runtime/Core пакетов — PASS, общий exit 0.

Журналы в `reports/` этого root:

- `browser-20261019T060913Z-2889759.log`;
- `web-real-20261019T060922Z-2891169.log`;
- `web-20261019T060420Z-2869326.log`;
- `unit-20261019T060959Z-2894781.log`.

Run/task/lease и approval решения заданы fixtures; PG, runtime Chromium и
production owner API/Broker реальные. Нет проверки внешнего аккаунта или живого
model provider. Ограничения продукта и полный исходный протокол проверки описаны
в [отчёте реализации](server-browser-2026-10-19.md). Ранее существовавший конфликт
doctor revision в function-proposal тесте явно исключён из unit-команды, а не
объявлен исправленным. Опциональные PG/fixture skips в unit не заменяют acceptance.

Для включения после merge по-прежнему нужны migration086, dedicated runtime token
и optional server-browser Compose overlay. В ходе этой задачи overlay не применялся.
