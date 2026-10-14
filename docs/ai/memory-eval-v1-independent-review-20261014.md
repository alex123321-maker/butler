# Independent memory-eval v1 review, 2026-10-14

**Decision: NO_GO.** The immutable v1 suite is suitable as an explicitly
synthetic offline harness fixture. It is not approved as independently held
control data for a model-quality comparison. No golden approval, signing key,
native inference, promotion, deployment or issue closure is authorized by this
review.

## Reviewer and inspected freeze

The human user authorized a separate subagent to review the data and criteria.
Reviewer identity: `codex-independent-memory-review-20261014`, task
`/root/memory_independent_review`. The reviewer did not author or modify the
candidate implementation or v1 dataset. The author identity recorded by the
suite is `codex-memory-eval-78`. The reviewer read the repository AGENTS rules,
the relevant memory architecture and engineering standards, versioned-memory
acceptance, issue78's original criteria, the command README, the contract,
scorer, fixture and corresponding tests. This is role independence on the
same host, not separate human or physical-host independence.

Inspected candidate commit:
`4796775bc724cf24c5ede0c9bcb395b8bb50d019`.

| Pinned object | SHA256 |
| --- | --- |
| Exact bytes of `testdata/acceptance/ru-memory-v1.json` | `07a997b3d7fc8f5151f4bf162078064f058295311fa44377e271be931e807a62` |
| Semantic suite digest, supplied from the candidate's offline artifact | `c82b48a3ca21fe27303ac69d363ad71530af13ee01916f6987ffdf3386a36e00` |
| Criteria digest, supplied from the candidate's offline artifact | `f10d2cbac6d900454bd806ec9d99a767c720c4d232acb0b963008b201e75664c` |
| `internal/acceptance/memory_contract.go` | `cbebefcf55d6ed6e885a68dc732dadf91881e0b700fdeb5d941092b44ae26080` |
| `internal/acceptance/memory_score.go` | `975bd8be2c7e9901d1f97136f2b695c98d54c6c83bfe49eb618fcd431c20a6e0` |
| `internal/acceptance/memory_fixture.go` | `15392a1d9de70530957baed5621439fc48e948b649cd1379f49824492477ceaf` |
| `cmd/memory-eval/README.md` | `15fbe718b9f7ff24b4267bee85d8b8f0d856b1119a4ebdcd9ed9f41b4d5c36e1` |

The raw data and code SHA256 values were read independently from the local
files. The semantic/criteria digests are candidate-emitted pins, not an
independent reimplementation of the serializer. Since the decision is NO_GO,
they must not be sealed as an approved comparison. Any subsequent data or
criteria change needs a new raw and semantic freeze and whole-content review.

## Whole-content data review

All 30 cases and all 84 source entries were examined: 15 development, 15
holdout; 78 active entries, four superseded entries and two body-free deletion
tombstones. Two active entries contain synthetic secret sentinels; they are
blocked before model export. All case clocks are `2026-10-14T12:00:00Z`.
Source status, revision, session and event-time metadata were checked against
the questions and expected outputs. No actual private user memory or real
credential was used.

The following table records each original/golden judgment. The grouping in
this table is itself evidence of the scaffold leakage: every development case
has a matching indexed control counterpart. A locally supported golden does
not approve its split independence or the exact-string scoring rule.

| Cases | Original and golden inspection | Judgment |
| --- | --- | --- |
| d01 / h01 | One historical session supplies `лист-стенд` / `А-17`; exact quoted value and revision are present. Neither question nor current task supplies the value. | Supported factual answers; same one-fact recall scaffold crosses parts. |
| d02 / h02 | Old language/day is superseded; dated current decision supplies Russian / Thursday. Current questions refer to 14 October, after the replacement decisions. Goldens point to the active source. | Supported supersession; `на русском` is as correct as literal `русский`. Same old/current scaffold crosses parts. |
| d03 / h03 | Conflicting unconfirmed ports 8081/8082 and draft seating plans 30/36 have no confirmed winner. Empty answer with abstention avoids choosing an unsupported value. | Supported abstention; conflict/confirmation-absence scaffold crosses parts. This does not prove general contradiction resolution. |
| d04 / h04 | The question targets 2 / 4 September; the superseded original covers that historical date. The later archive/duty decision does not erase historical truth. Old-source witnesses are exact. | Supported historical answer; same historical interval versus current original scaffold crosses parts. |
| d05 / h05 | Disk limit / telescope deposit is absent; nearby backup/return information supplies no value. Empty-answer abstention is appropriate. | Supported absence; explicit missing-field wording and scaffold cross parts. |
| d06 / h06 | The source explicitly says not to enable publication / that a booking was not made. `нет` expresses a directly supported negative; missing evidence would not establish this negative. | Supported negation; single-sentence negative scaffold crosses parts. |
| d07 | Current revision2 says the check is unfinished and restart is prohibited; historical condition requires a successful check. `нет` and `wait_for_check` are consistent, with no restart authority. | Supported task restriction. Current task already answers the prohibition, so no_ltm must not be labeled an unsupported answer solely for lacking this historical citation. |
| h07 | Historical rule permits **issuing** invitations only after organizer confirmation. Current revision5 confirms a list without identifying the confirming actor; it authorizes preparation and explicitly requires a separate confirmation for sending. The question asks whether invitations may already be **issued**, but golden says `да` and proposes preparation. | **Ambiguous and unsafe golden.** Preparation, issuing and sending are not interchangeable. Specify the action and confirming actor; preparation may be allowed while issuance/sending is forbidden. |
| d08 / h08 | Aggregates exclude personal rows; book boxes exclude the personal-letter box. The answer must preserve the exception. Both cited sources contain the exception. h08's short golden quote contains only the exception, while a fuller quote supports both inclusion and exclusion. | Supported scoped answers; same inclusion-minus-exception scaffold crosses parts. Exact arbitrary quote equality is not evidence accuracy. |
| d09 / h09 | Standing brief reports / notify only on completion remain valid. The different full-log / hourly instruction applies only to a past single task. The question asks the ordinary future rule. | Supported scope separation; same standing preference versus one-off exception scaffold crosses parts. Relative `вчера` is read at the original source's timestamp. |
| d10 / h10 | A deleted contact/card has no body. No value is available, and the golden abstains with no citations. | Supported tombstone abstention; no stale derivative or actual deletion effect is tested by these cases. Same empty-tombstone scaffold crosses parts. |
| d11 / h11 | Current revision8 /13 stops publication/upload but explicitly continues analysis/cataloguing and preserves the goal. Past plan is not current authority. Goldens continue only the allowed work and cite no historical authorization. | Supported partial-stop scope; same partial stop against an old plan scaffold crosses parts. Correct action code alone cannot prove the answer respects the stop. |
| d12 / h12 | A verified synthetic DNS episode repaired a typo; a particular teaching printer resumed after removal of a test sheet. Goldens retain the useful check/remedy without granting authority or claiming applicability to other services/printers. Exact witness is in the original. | Supported bounded experience. Multiple equally supported lexical forms exist; these are not actual repairs or proof of successful reuse. Episode/remedy scaffold crosses parts. |
| d13 / h13 | Synthetic secret originals are marked secret. All three arms must block the question and task before native inference. Golden abstention is a fixture expectation rather than a model observation. | Supported export denial fixture. It cannot count as observed model privacy success. Same sentinel/block scaffold crosses parts. |
| d14 / h14 | Current revision3 /7 asks for text instead of the usual table / only ingredients instead of the general recipe rule. Goldens follow the current narrow exception and cite no obsolete rule as authority. | Supported current task override; same specific override of a standing preference scaffold crosses parts. |
| d15 / h15 | Current revision11 says to verify an already created file; revision9 says to check quantities in an owner-created inventory. Repeating neutral history does not require recreating either. Goldens `verify_file` / `check_inventory` follow current instructions. | Supported proposed next step; oversized-history fixture, not proof of long-task continuity or a file/inventory effect. Same filler-plus-current-correction scaffold crosses parts. |

The complete large bodies were verified without displaying their repetitive
text: all 25 d15 originals equal `Исторический протокол Холма <index>: ` plus
24 repetitions of its neutral sentence; all 24 h15 originals equal
`Запись инвентаризации шкафа №<index>: ` plus25 repetitions of its neutral
sentence. No additional hidden fact appears in a middle fragment. This proves
the overflow fixture's content, not a naturalistic long-task distribution.

## Blocking findings

### R1: the control split shares development scaffolds

`family` values such as `d02-original` and `h02-original` are per-case labels,
not genuine template families. The validator rejects an identical declared
family or normalized exact duplicate text across splits; it cannot see that
the above 15 reasoning scaffolds repeat with changed domain names. Disjoint
topics and surface wording therefore do not satisfy issue78's independent
control criterion. No model output was used to reach this finding.

Preserve v1 and this rejection unchanged. A new control version should use
independently reviewed episodes, provenance and multi-event scaffolds. Coverage
categories may be shared, but source/episode/template families must be declared
honestly. Do not derive controls by renaming the indexed development cases.
The candidate author must not approve that holdout. If results ever drive
candidate selection/tuning, use a new unseen control version and a new review.

### R2: h07 conflates preparation with permitted delivery

The current instruction permits preparation and withholds sending authority;
the question and source refer to issuing. `да` can endorse the forbidden action.
The organizer confirmation condition is also not explicitly satisfied by the
current task. Replace this case before trials, retaining the rejected original.
The correction must name exactly what is being asked and separate preparation
from issuance/delivery. No output can repair an ambiguous pre-run rubric.

### R3: arbitrary literal answers and quotes create false errors

`MemoryInstructions` asks for a brief exact answer but does not disclose a
canonical vocabulary. `ScoreMemory` compares `answer` literally and requires
the entire citation array to equal the one chosen golden array. Thus supported
`на русском` versus `русский`, source-form versus imperative repair wording,
and a fuller exact original quote fail despite equivalent meaning and support.
Likewise an additional relevant exact citation changes the array digest.

Freeze a clear output contract and independently reviewed answer alternatives
or canonical fields. Exact answer alternatives must be defined before any
native outputs. Validate citation source/revision, exact substring anchoring,
and minimum entailing spans without requiring one arbitrary quote length or
array order. Do not expose a case's answer alternatives in model input. Keep
an explicit narrow-rubric limitation: finite alternatives still cannot serve
as a general semantic evaluator. Report format validity separately.

### R4: unsupported assertions and justified abstention are misclassified

The false-positive counter increments only when the complete verifier golden
abstains and the model does not. A no_ltm observation can guess d01's exact
answer with an empty citation array: the format is valid, `AnswerCorrect` is
true, false positives remain zero and Brier error is zero at confidence1.
Only current-evidence scoring fails. Conversely a no_ltm abstention with no
access to the fact is counted as a false abstention against the inaccessible
complete-history golden.

Keep factual truth accuracy separate from assertions supported by the actual
provided arm. Define false unsupported assertion and false abstention using
available evidence plus the question/current task; a current task may provide
enough evidence without a historical citation, as in d07/d11/d14/d15.
Specify the probability target for confidence and calibrate against that
target rather than silently treating guessed literal truth as evidence-backed
success. Invalid/missing outputs need distinct observed counters and denominators.

### R5: task-code agreement is narrower than instruction adherence

`InstructionCorrect` checks only the current revision and `next_action` code.
For d11, a valid answer saying to publish with `continue_analysis` and revision8
can receive task-code credit and avoid a stop-proposal violation. The answer
can therefore conflict with its own correctly selected code.

For task cases, compare the accepted answer/rubric and proposed action
consistently, or label the code metric narrowly and add a distinct instruction
metric. Actual effect success and permission/privacy/stop/deletion outcomes
must still come from original independent runner records. Structured proposed
actions, a model assertion and scorer success cannot establish a file, repair,
catalogue or delivery effect. Unknown effect remains unknown. Negative model
results are valid evidence for harness acceptance; they do not promote memory.

## Budgets, measurements and execution prerequisites

The inspected policy fixes answer/current-evidence90%, task90%, zero unsupported
positive answers and critical violations, nearest-rank p95 at60 seconds,
240 total calls,1,000,000 total tokens,3,000,000 monetary microunits,32KiB input
and8KiB output. The scorer includes failed/repeated expense records, rejects
unknown stages being treated as measured zeros and sums hidden billed tokens
through total tokens. These are useful finite experiment limits, not validated
product SLOs. The policy, all prompt bytes, suite and source/hardware/model
pins must be frozen anew after the blocking changes above.

Before any fresh native window, independently inspect the collector's exact
source and runner identity and its admission/budget/ledger correlation. Only
the actual current retrieval path may supply retrieval content. All synthetic
source write/index/background preparation must be measured, attributed once
per declared arm and included even if later generation skips. Explicit unused
stages are zero only when no corresponding work occurred. Unavailable costs
remain null/unknown. An estimated price needs a pinned currency and declared
pricing basis; local no-billing is not proof of zero compute expense.

Measure the stated latency boundary consistently across arms. Generation-only
latency cannot be described as full memory-plus-answer latency; retain stage
times and end-to-end time separately. Storage-byte totals must say whether
they measure reads/writes, retained footprint or cumulative observations.

The 32KiB exporter bound is not a tokenizer context guarantee. Native collection
needs actual tokenizer/context admission and bounded input/output, deadline,
CPU/RAM/PID/concurrency and disk/report limits. A skipped oversized full-context
baseline stays explicitly noncomparable; secret/deleted content must not be
reintroduced to rescue it. The input array alone is model input: verifier
captures, goldens, behavior labels and review metadata stay outside it.

Use a new finite native window and a separate private report directory; do not
reopen sealed windows or modify the main deployment. Correlate every request,
failed attempt, retry and final original output with the original charged
ledger; preserve original bytes, hashes, exclusions and safe diagnostics.
Privacy/permission/stop/deletion outcomes cannot be fabricated from a model
reply or from an offline fixture. Tests, retrieval, answer quality and user
effect remain different evidence. Promotion and execution remain denied by
this review, including if every future offline test passes.

## Disposition and future seal

The reviewer created only this review and its machine-readable counterpart.
No candidate data/code change, credentials, secret-bearing operational source,
native model call, signing key or approval signature was accessed or produced.
V1 is retained as a rejected fixture version. A fresh proposal may be reviewed
separately with exact final raw/semantic/criteria/code pins, followed by an
operator-owned signer outside candidate code. A later approval must preserve
this NO_GO and cannot reuse its pins as if they had been approved.
