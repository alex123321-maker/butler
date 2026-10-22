# C07 — продолжение и обязательства ответа (#105)

Статус: реализация и приёмка в работе. Native/CI результаты ниже дополняются только
после фактического выполнения. Изменение не включает новый production profile.

## Наблюдение и гипотеза (#76)

На main `83c6ddd8` рабочий multi-choice после принятого Submit передал canonical
answer DATA в provider, но модель повторно предложила тот же widget. Вторая
публикация не была разрешена; исходный failed Input и расходы сохранены.
Это отдельный behavioral failure, не session/run lock race из #137.

Гипотеза: восстановленный запрос сохраняет первоначальное «задай вопрос», но не
объясняет завершённый этап; canonical answer раньше не содержал labels/options.
Исправление: fixed kernel phase guidance отдельно от недоверенных данных вопроса
и ответа, восстановленная canonical mapping. Оно не меняет полномочия или бюджеты.
Регрессии: guidance survives exact-action approval/provider rebuild; poison в
question/answer не становится инструкцией; повторная подготовка не дублирует DATA.

## Матрица критериев

| Критерий | Воспроизводимая проверка | Результат |
|---|---|---|
| progress/result/photo/report, прямой input без Task | `TestCommunicationAllResultFormatsPreserveObligationAndUnverifiedCompletionIntegration` | pending |
| widget wait→response→result, quiet/verified_task | `TestCommunicationActualCoreFreshContextResumeIntegration` и native оба клиента | pending |
| final без implicit send, frozen-save replay | `TestCommunicationPublicationAtomicIdentityAndSeparateFinalIntegration`, format matrix, completion regressions | pending |
| typed empty/nonempty terminal, broken stream | `TestCommunicationRequiresFinalAndTerminalAndSuppressesDeltas`, `TestCommunicationNonemptyFinalRequiresTerminalProofIntegration` | pending |
| correction/restart и новый owner continuation не пополняют allowance | `TestCommunicationCorrectionIsBoundedIntegration`, `TestCommunicationTaskPauseResponseAndFreshResumeIntegration` | pending |
| строгая policy/child/authority/cancel/amend | существующие communication guard/control/authority matrices | pending |
| result tag/ACK не доказывают semantic completion | format matrix (verdict remains unverified), behavioral review | pending |
| premature promise/false result/useful progress/quiet | `TestCommunicationBehaviorSeparatesStructuralAndSemanticResults` + retained content reviews | pending |

## Behavioral review

`internal/acceptance.ReviewCommunication` расширяет существующий контур оценки,
не запускает модель и не является автоматическим semantic judge. Content verdict
должен следовать из отдельного просмотра результата и evidence, а не из метки tool.
Неполные измерения, повтор вопроса и duplicate result не проходят. Synthetic checks
проверяют правила оценки, не измеряют live качество. Для native каждого окна
фиксируются Input/run, transcript, publications/obligation, delivery/terminal edit,
semantic review, полный учёт попыток/observed usage, source и binary hashes.

## Проверки и rollout

Focused unit/PG/race, CI, self-review и native Web/Telegram: pending.
Рабочие owner profile/history/limits не меняются этой реализацией. Полная
готовность, диагностика/историческая лента и production rollout — #106–#107.
