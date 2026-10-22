# C07 — продолжение и обязательства ответа (#105)

Функциональная приёмка завершена. Реализация — [PR #138](https://github.com/alex123321-maker/butler/pull/138).
CI и self-review привязываются к конкретному финальному head в PR; закрытие #105
допускается только после их успеха. Это не независимое maintainer approval.
Рабочий owner profile не обновляется этой поставкой.

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

Ревью дополнительно выявило потерю canonical ответа при owner pause/resume:
новый Input получал allowances, но не marker уже принятого ответа. Новый
`TestCommunicationOwnerResumeNewRunRetainsCanonicalAnswerIntegration` на предыдущем
candidate воспроизвёл обе потери (answer до/после resume): provider не получил DATA.
Исправление переносит marker через owning Input methods и читает ответ под текущими
guards до старта нового run; чужой Input и старая task revision отклоняются.

## Матрица критериев

| Критерий | Воспроизводимая проверка | Результат |
|---|---|---|
| progress/result/photo/report, прямой input без Task | `TestCommunicationAllResultFormatsPreserveObligationAndUnverifiedCompletionIntegration` | PASS Linux PostgreSQL/race |
| widget wait→response→result, quiet/verified_task | `TestCommunicationActualCoreFreshContextResumeIntegration` и native оба клиента | PASS; quiet/verified — controlled Core/PG cases |
| final без implicit send, frozen-save replay | `TestCommunicationPublicationAtomicIdentityAndSeparateFinalIntegration`, format matrix | PASS; более широкие completion regressions — в CI |
| typed empty/nonempty terminal, broken stream | `TestCommunicationRequiresFinalAndTerminalAndSuppressesDeltas`, `TestCommunicationNonemptyFinalRequiresTerminalProofIntegration` | PASS |
| correction/restart и новый owner continuation не пополняют allowance | `TestCommunicationCorrectionIsBoundedIntegration`, `TestCommunicationTaskPauseResponseAndFreshResumeIntegration` | PASS |
| ответ до/после owner resume, новая Task revision и Catalog | `TestCommunicationOwnerResumeNewRunRetainsCanonicalAnswerIntegration` | PASS 6 случаев + parent; before-fix обе positive ветки FAILED |
| строгая policy/child/authority/cancel/amend | существующие communication guard/control/authority matrices | PASS |
| result tag/ACK не доказывают semantic completion | format matrix (verdict remains unverified), behavioral review | PASS |
| premature promise/false result/useful progress/quiet | `TestCommunicationBehaviorSeparatesStructuralAndSemanticResults` + retained content reviews | PASS правила оценки; не оценка точности модели |

## Behavioral review

`internal/acceptance.ReviewCommunication` расширяет существующий контур оценки,
не запускает модель и не является автоматическим semantic judge. Content verdict
должен следовать из отдельного просмотра результата и evidence, а не из метки tool.
Неполные измерения, повтор вопроса и duplicate result не проходят. Synthetic checks
проверяют правила оценки, не измеряют live качество. Для native каждого окна
фиксируются Input/run, transcript, publications/obligation, delivery/terminal edit,
semantic review, полный учёт попыток/observed usage, source и binary hashes.

## Реальные клиенты и model loop

Все окна проходили через отдельные фактические App.Run, Broker и provider с
каноническим encrypted auth store/обычным renewal, PostgreSQL и Redis, отдельного
бота `butler_widgets_lab_20261022_bot`, actual Telegram Web и deployed Web bundle.
Для Web использован loopback-only relay к изолированному API; production build,
API config и user credentials не изменялись. Telegram и provider — через proxy.

| Окно | Исходный запрос и действие владельца | Результат |
|---|---|---|
| Web multi, до дополнительного owner-resume fix | Обычная просьба выбрать сыр/грибы без option IDs; Telegram toggle + Web toggle + Telegram Submit | Input completed; один вопрос, один результат; ACK38/39 и ACK42; оба клиента |
| Telegram text, тот же первый build | Обычная просьба уточнить подачу; native ForceReply «Подать горячей и нарезать на восемь кусочков.» | Input completed; question ACK44, terminal edit sent, result ACK46 в обоих клиентах |
| Финальный Web multi, runtime source `90768f8b` | Рис/гречка, IDs выбрала модель; Telegram toggle + Web toggle + Web Submit | Input `input-dbf16a22386b50d34636ff588d4cd7f0` completed; один вопрос и result «Принято: рис и гречка…», ACK47/48/49; оба клиента показывают результат ровно один раз |

Первое окно пережило same-profile App restart на открытом вопросе: questions=1,
model attempts=1, те же три outbox parts, без новой генерации. Pending part
закончила обычную доставку; старые ACK не были переотправлены. Перед Submit в
обоих multi окнах draft v3 содержал оба выбора, responses=0, model attempts=1.
На финальном build отдельно сохранены owning admission (`response_committed`),
completed Input, canonical ответ, publications, terminal projection edits и
independent delivery states. Приватная semantic self-review сопоставила текст
с фактическим ответом; `TestNativeCommunicationRetainedContentReviews` прошёл
на этих реальных kernel records без нового inference. В первых двух окнах
сохранены completed Inputs/results/delivery, но отдельный admission-row snapshot
не входит в старый native capture: он не выдаётся за измерение этой строки.

В начале Telegram-проверки клиент удерживал старую цитату вопроса из #104:
неподтверждённый reply target не был принят как новый model Input. После явного
снятия цитаты обычный запрос принят один раз. Это отдельная транспортная попытка,
не успешное исполнение запроса и не скрытый inference retry.

Все 3 admitted окна (два build-stage окна, одно финальное), без удаления расходов:
**9 model attempts, 9 observed, 0 unobserved**. Provider input/output/total:
**27,404 / 951 / 40,633**. Provider total взят из provider usage, не из суммы
input+output. Нативные budgets оставались конечными; allowance одной obligation
не пополнялся. Положительные результаты — bounded development evidence;
они не доказывают отсутствие ошибок модели на любых запросах и не являются
independent holdout или общим quality gate #76.

## Воспроизведение, проверки и rollout

- Linux `focused-v5.jsonl`: 312 passes (включая subtests), 0 failures/skips; vet.
  Финальные дополнительные Catalog/control cases: 7 passes, 0 failures/skips.
  Эти 7 включают 5 уже пройденных случаев, их нельзя суммировать как уникальные.
- Новый actual Core.Execute regression до fix воспроизвёл потерю ответа в обоих
  порядках answer/resume; после fix и Catalog/deletion проверки прошли.
- Первый focused запуск содержал ошибки новых fixtures: substring detector
  считал fixed instruction ответом, media-only planner применялся к message.
  Исходный красный лог сохранён; это не product regression и не pass. После
  исправления fixtures выполнялись свежие проверки, а не переименовывались логи.
- `scripts/dev/communication-core-tests.sh --regression` — обязательный PG/race,
  crash и regression gate CI; `--converter` — отдельная regression suite.
- `internal/acceptance` содержит конечные правила behavioral review с negative
  promise/false-result/duplicate/repeated-question/unknown-usage controls.

Private evidence: `/srv/storage/butler/agents/communication-105-20261022`:
source archives и binary SHA-256, `reports/focused-v5.jsonl`,
`owner-resume-before.jsonl`, `owner-resume-final.jsonl`,
`native-window1.json`, `native-final-summary.json`, `native-content-review.jsonl`,
исходные failed logs и оба native transcripts/ledgers. Финальные native binaries
собраны из `source-v6` (`90768f8b`); изменения итогового отчёта не меняют runtime code.

Рабочие owner profile/history/limits не менялись этой реализацией. Полная
готовность, диагностика/историческая лента и production rollout — #106–#107.
