# Приёмка memory-eval: полная матрица попыток

Сравнение issue78 завершено на ssh laba: новое однократное окно выполнило все
пять оставшихся входов, четыре ответа и один таймаут. Машинный аудит —
**PASS_CORRELATED_COMPLETE_ATTEMPT_MATRIX_WITH_RETAINED_FAILURES**.
Числовые критерии качества/расходов — **NO_GO**, продвижение запрещено.
Независимая приёмка стенда — **PASS_HARNESS_ACCEPTANCE**; итоговый вердикт находится в
[отдельном заключении](memory-eval-v2-renewed-five-final-independent-review-20261015.md).

| Исходные входы | no_ltm | full_context | retrieval | Всего |
| --- | ---: | ---: | ---: | ---: |
| Завершённые ответы | 20 | 19 | 19 | 58 |
| Неудачные допущенные попытки | 2 | 1 | 3 | 6 |
| Заранее объявленные пропуски | 2 | 4 | 2 | 8 |
| Невыполненные входы | 0 | 0 | 0 | 0 |
| Исходный знаменатель | 24 | 24 | 24 | 72 |

Все64 eligible inputs попытаны ровно по одному разу. Сохранены53 исходных
auxiliary admissions; закрытый счётчик117/3442427 не превышает новый предел.
Новое окно2026-10-15T07:25:18.766787Z—07:55:18.766787Z не меняет прежние
deadlines, failures и расходы. Старая граница2026-10-14T20:16:58.345536Z
сохранена. Повторов и дополнительных auxiliary inference не было.

## Доступ к провайдеру

Auth-preflight: один control-plane Resolve за2818мс, Infer0. OAuth refresh
HTTP200, durable encrypted persistence и paid discovery HTTP200 прошли.
Native использовал тот же PostgreSQL auth-store и сохранённую rotation.
Диагностика содержит только фиксированные стадии/исходы/HTTP-коды.
Точную причину старого provider_unavailable10036мс его исходная трасса не
раскрывает; подтверждения отправки inference HTTP нет. Новый успешный preflight
не устраняет это историческое неизвестное.

| Новый вход | Результат | Длительность, мс |
| --- | --- | ---: |
| h11/full_context | completed | 61522 |
| h11/retrieval | provider_timeout, пустой final, usage/version неизвестны | 120018 |
| h12/no_ltm | completed | 93293 |
| h12/full_context | completed | 16496 |
| h12/retrieval | completed | 18292 |

Все58 полных ответов имеют gemini-3.8-flash-n при запросе
gemini-3.8-flash-high. Восемь пропусков сохраняют исходные причины; ограниченный
full-context baseline не объявлен полностью сопоставимым.

Сравнение по неизменному строгому рубрикатору; каждый arm сохраняет исходные
24 случая, включая ошибки и объявленные пропуски:

| Метрика | no_ltm | full_context | retrieval |
| --- | ---: | ---: | ---: |
| Корректный ответ или допустимый отказ | 16 | 7 | 8 |
| Правильный актуальный факт | 5 | 7 | 7 |
| Соблюдение текущего поручения | 16 | 15 | 15 |
| Ложное положительное утверждение | 4 | 12 | 11 |
| Измеренный p95, мс | 19620 | 83575 | 120014 |
| Числовой gate | NO_GO | NO_GO | NO_GO |

Отказы no_ltm не означают восстановление полезного факта. Retrieval recall/MRR
измерены отдельно на12 случаях; корректный chunk сам по себе не доказывает
правильный ответ. Внешний пользовательский успех не подтверждён ни для одного arm.

## Расходы и границы вывода

Сохранены1030 expense rows по всем девяти стадиям; missing_stages0 во всех
arms. Их446 логических I/O+model/control-plane calls превышают неизменный
оценочный порог280. Это другая величина, чем117 native+auxiliary admissions.
Один auth Resolve включён в background. Семь failed expense rows — шесть
generation и одна историческая write, а не семь неудачных native inputs.

Известный token subtotal:36987 input,3391 visible output,190938 provider-reported
total; разбивка дополнительных token dimensions не измерена. Invoice, usage
неудачных native попыток, embedding tokenizer,
физическое хранение/хост и реальные внешние результаты поручений неизвестны.
Учёт всех стадий не означает измеренную полную денежную стоимость.

Замороженный lexical rubric отклоняет часть допустимых перефраз и имеет
пересекающиеся action enums. Его числа и critical markers не равны полной
смысловой оценке или доказанным нарушениям ядра. Критерии после наблюдения не
менялись. Публичные синтетические controls нельзя использовать для дальнейшего
подбора; реальные случаи требуют новой проверенной человеком разметки.

## Артефакты и проверки

- [Полный отчёт](../../testdata/acceptance/results/ru-memory-v2-complete-20261015.json),
  SHA256053c3032091be6cc58ae68ecd37dec6489b02e18864c34b3f395928d9f27b593:
  model_quality_evidence=true, promotion_allowed=false, statusNO_GO.
- [Машинный аудит](../../testdata/acceptance/results/ru-memory-v2-complete-runner-audit-20261015.json),
  SHA256a7884cde71d2cf9eb35d3555ebc5c839d5db6e6def690730737f899928cad3a8.
- [Старый неполный отрицательный отчёт](../../testdata/acceptance/results/ru-memory-v2-partial-20261015.json)
  и [его независимая сверка](memory-eval-v2-final-independent-evidence-20261015.md)
  сохранены как история, без переименования в полную приёмку.
- [Source proof](memory-eval-v2-renewed-five-source-readiness-20261015.md):
  22 целевых real-PG/race и46 CLI/auth/race PASS, zero fail/skip, vet/bash PASS
  на6928447; отдельный pgx timestamp-codec test/vet/bash PASS наa3d66736.
  Зависимости collector/auditor/auth/launcher между ними идентичны.
- Отдельные [prepared/auth](memory-eval-v2-renewed-five-prepared-auth-review-20261015.md)
  и [native entry](memory-eval-v2-renewed-five-native-readiness-20261015.md) gates
  сохранены вместе с actual runtime proof и owning PostgreSQL originals.

Исторические авторские хэши четырёх новых заключений и их хэши после удаления
одного лишнего LF в EOF сопоставлены в [заметке о сериализации](memory-eval-v2-review-serialization-20261015.md).

Исполненный release a3d66736dcc5e7ebf94a8b1d614b0e538f387eb8,
snapshot20261015-142056-119,
sourceSHA256d9824a33875f194085673f46aede24eea593f46ef990b8b9ea0a0855091d22f7,
collectorSHA2566bf79d67bf279dee49230538676cbbdd53595bd4f651bd973b94df9936f75980.
Исходный scoring Plan90ed/2a6 сохранён с отдельной цепочкой последующих runtime.
Нативные PG/оригиналы находятся в
/srv/storage/butler/agents/memory-eval-v2-renewed-five-20261015;
network-none CLI audit завершился без inference. Документационные коммиты после
исполнения не выданы за исполненный source. Основной deployment не изменялся.

Контракт/CI-набор, команды свежего запуска, read-only audit и связь behavior_refs
с #76 описаны в [README](../../cmd/memory-eval/README.md).
