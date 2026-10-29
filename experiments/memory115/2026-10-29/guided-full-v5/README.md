# Полный A/B/C: исправленный поиск и увеличенные лимиты

На laba завершены **57/57 стадий и48/48 контрольных вариантов**: четыре originals,
четыре notes, подготовка навыка, восемь случаев × A/B/C × два повторения.
Неисполненных стадий нет;51 завершена,6 неуспешны. Native collector вышел с0:
это завершение сбора, а не успешность всех задач или release qualification.

Профиль: **24 модельных хода и32 инструментальных обращения на задачу** вместо
12/11. Source task с одной note получает25 модельных допусков. Общий новый
независимый журнал ограничен1276 entries вместо640; соответствующие token,
input-byte, active-time и egress allowances заданы до admission.40 минут на
foreground и UTC-clamp сохранены. Значения одинаковы для всех вариантов;
лимиты активных задач не повышались. См. [протокол](../guided-full-v4/protocol.md).

## Итог по прежнему строгому критерию

| Вариант | Завершённые run | Подтверждённые задачи | Контрольные известные tokens | Полные известные tokens с подготовкой |
| --- | ---: | ---: | ---: | ---: |
| A: без опыта |14/16|14/16|389172|482514|
| B: одинаковые notes |15/16|14/16|1339684|1460558|
| C: те же notes + навыки/инструкция |13/16|13/16|1026481|1329435|

B−A: **0 дополнительных подтверждённых задач, +978044 известных full tokens**.
C−B: **−1 задача, −131123 известных full tokens**. Последнее не означает
денежной экономии: у C есть product-допуск с NULL usage, а invoice cost неизвестен.
A остаётся самым экономичным по наблюдаемым токенам на этом наборе.

Полные суммы каждой строки включают полностью необходимые для этого варианта
shared originals, B/C также notes, C также создание навыка. Их нельзя складывать
как уникальные расходы: общая подготовка фактически выполнена один раз.
Уникальная новая известная нижняя граница —3058291 full tokens.

B42 завершил run и корректно сохранил неопределённость предыдущей попытки, не
повторив подготовку. Это не доказало поддержку/неподдержку ready=maybe из текущего
probe или проверенного отрицательного наблюдения той же среды: original03 имел
owner_verdict=unknown. Поэтому run15/16 не превращён в task success15/16.
Task completion, run completion и action effect здесь различаются.

Относительно закрытого [v3](../guided-full-v3/README.md) строгие оценки изменились
14/13/11 →14/14/13. Одновременно изменились поиск, лимиты и заново сгенерированный
опыт; разницу нельзя приписать одному изменению. Двух повторений и оценки автора
недостаточно для независимой или статистической квалификации.

## Что реально проверено

В48 контролях `memory.search` завершился38 раз:26 `ok`,12 `empty`.
`memory.open` завершился35 раз:34 `ok`, один штатный `denied`.
**Прежних memory-tool UNKNOWN нет.** `memory.neighbors` с пустым fragment_key
был предложен моделью в C16 и отклонён до исполнения; это не сбой поиска.

**Шесть действительных Broker skills.load**, каждая с точным текущим run/task,
task/constraints revision и digest pin PostgreSQL, все до первого проектного
действия. Четыре — repeat/similar C в двух проходах, две дополнительные —
known-failure C. Последние сохранены как стоимость и не объявлены успехом.
При drift, beta-only scope, inspect-only и удалении исходника завершённых
загрузок неподходящего навыка не было. Тело не подставлялось обходом Broker.

Модель могла воспользоваться расширением: максимум24 model entries и24 external
calls на задачу. B18 остановлен именно `budget_model_turns_task`; потолок32
external calls не исчерпан. Большее число допусков не гарантировало выполнение
задачи и позволило длинное повторное чтение.

[Отдельный аудит](failure-and-memory-audit.json) сохраняет все остаточные отказы:

- A17/A40 и C41: unsupported prepare с effect_unknown; повтор/успех не выдуманы.
- B18: исчерпан model-turn budget.
- C16: ошибка подготовки точной авторизации после запроса neighbors с пустым
  fragment_key; до неё успешно выполнены шесть search, пять open и load.
- C43: `memory_source_stale` до следующего native/provider entry. Четыре предыдущих
  read-only вызова завершены; один model reservation остался с NULL usage.
  Точная первопричина freshness-отказа не воспроизводилась и не объявлена
  исправленной, ложной или следствием слабости модели. Защита не обходилась.

## Полный учёт и сохранность

Новых **281 native entries,281 known,0 native unknown;3058291 known full tokens**.
Product attempts: **282 =281 observed +1 NULL**, один без native entry. Этот
резерв не возвращён и не обнулён. Все49 allocated database exports сверены
первоначальными cost/quote-union/product/native/journal/domain validators.

Inclusive primary history: **1049 native =1045 known +4 unknown;
1051 product =1045 observed +6 NULL,2 without native;9848908 known full tokens**.
Отдельные25 запросов /90951 tokens и legacy NULL/4175/3389 не смешаны с primary.
Invoice cost неизвестен. Прежние провалы, расходы, UNKNOWN и результаты v3
не изменялись. Их архив/inventory повторно сверены с прежними SHA256.

Новый архив **1465 исходных файлов** закреплён:

- archive SHA256 `0cb84857f9747885881d65318e556dc052bcf5fa7e0e222b6089eb9ad203b552`;
- inventory SHA256 `807648b8ce76b11b710bdb0a50434066fc6ab976d54e56063fe74d41c287fb6a`.

Приватная PC-копия архива и inventory сверена по SHA256. Provider auth files
не включены. Завершение credential backends/всех процессов и opaque auth handback
подтверждены до остановки собственных PG и relay при нуле клиентов. Данные БД
и все setup/check logs сохранены; другой operator PG не затронут.

## Версии и границы

Collector source `f69d117e7bb747c5bb6a2a9888af6dfd300528f9` содержит исправление
поиска16c24eba и явную kernel-инструкцию загрузки навыка. Source6623 blobs и пять
артефактов закреплены до auth; plan9743229d…, historya84f7e4d…,
actual recipient appb0605504…. Bound carrier451e0802, operator1cbdc055…;
[полные pins](binding.json). Все8 commitments теперь снова UNBOUND и проверены
на отказ prepare/native до filesystem/DB/auth.

Предварительный v4 preseal заменён до auth/модели из-за старого значения640 в
служебном поле при действительном1276. Его plan и нулевой расход сохранены.
Первый offline review wrapper указывал старый reader и дал SKIP; он не принят
за PASS. Логи сохранены, исправленный запуск требует отсутствие SKIP и оба JSON.

Отдельный reader5e922838… не меняет collector или результаты. Если Flow вернул
nil, он доказывает единственный run по current input/task/owner/session и
captured run task revision; более поздняя task revision не даёт новую authority.
Так восстановлены факты C16/C43 без изготовления ExecutionResult/final/success.
Pure regression отвергает foreign/stale/ambiguous rows; реальный сохранённый C16
прошёл проверку чтений и load/pin. Предыдущий неудачный reader log сохранён.
Python stdlib используется только для офлайн-проекции JSON, не для core,
SQL, auth или генерации. Оценка/критерии не подгонялись после результатов.

Bounded canary/source-reference scan прошёл по256 native control entries во
всех48 случаях; совпадений запрещённых маркеров нет. Это не полная семантическая
privacy-приёмка. Настоящие App.New/Run/Doctor выбранной композиции, coherent #92,
независимое качество и реальные owner tasks остаются отдельными критериями.

**Вывод: функциональная загрузка и исправленный поиск подтверждены; преимущество
опыта/навыков по качеству и полной стоимости не доказано. #115 остаётся OPEN/NO_GO,
PR draft, deployment/defaults не менялись. Дальнейшее увеличение потолков само по
себе не обосновано; остаток включает freshness guard и ненужное повторное чтение.**

Доказательства: [учёт](cost-review.json), [оценка48 случаев](coordinator-assessment.json),
[load/pins](broker-load-proof.json), [чтение и отказы](failure-and-memory-audit.json),
[raw seal](raw-seal-guided-full-v5.json), [post-exit gate](review-run-guided-full-v5-exit.json),
[остановка](private-stop-guided-full-v5.json), [failed-case reader](failed-case-projection.json).

[Архив офлайн-аудита](audit-seal-guided-full-v5.json) содержит65 файлов; его приватная PC-копия также сверена. [UNBOUND deny proof](operator-unbound-guided-full-v5.json) подтверждает отсутствие нового допуска.
