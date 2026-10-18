# Independent review of temporal native v5

Verdict: **NO_GO; qualification remains false**. The root reviewer read all
twelve unchanged questions, frozen rubrics, answers and actual result witnesses
in the [immutable review JSON](fixtures/issue88/native-v5-review.json), SHA256
`b05089d6c911f1b20b10501b3e8f4b8b931541bb331f33923ccd9745afb66b88`.
Eleven originals pass; one contains an unsupported acceptance-state assertion.
This review changes no response, receipt, source, denominator or charge.

| Original | Judgment | Evidence and limit |
| --- | --- | --- |
|t01_valid_february|Pass|Selects Кедр for February, keeps the March interval separate and preserves historical evidence despite expired current suitability.|
|t02_current_head|Fail|Selects Пихта and correctly reports fresh owner verification. It additionally describes the old Кедр source as not registered as an accepted temporal fact. The current-head query returns only Пихта; the old original open returns its body and source-currency marker, without an acceptance-state assertion. Neither observation proves that the old source was never accepted.|
|t03_known_before_second|Pass|Selects Кедр at the recorded-knowledge cutoff and keeps the later Пихта record outside that knowledge. Recording, validity and owner verification remain distinct.|
|t04_not_yet_recorded|Pass|Refuses a concrete value at the selected-source cutoff. Its additional later recorded_at timestamp is grounded in the actual Start profile context, even though the subsequent empty facts result does not repeat it.|
|t05_open_and_unknown|Pass|Separates the explicitly open Лён interval from unknown validity. Additional relative wording and temporal metadata are supported by its subsequent facts/original reads.|
|t06_overlapping_periods|Pass|Reports both overlapping alternatives as unresolved and explicitly refuses to use current confirmed/superseded state or newer recording as a historical winner. This corrects the earlier v4 failure.|
|t07_exception_expired|Pass|Preserves Russian after the temporary session exception. The actual original open supports the English exception and its exact end; the permanent preference remains stable.|
|t08_stale_technical|Pass|Retains4.2 as the last accepted value, requires revalidation and performs no external read. One parenthetical loosely joins verified/applicable, but the answer explicitly states that needs_revalidation does not mean4.2 is false or stopped applying. Its claim of current verification remains withheld. Null interval bounds are described imprecisely as unknown; this does not establish or change a date.|
|t09_event_time_disjoint|Pass|Selects Ива by the explicit February validity, keeping the September document EventTime separate. Its event date and owner verification are supported by returned DATA.|
|t10_current_correction_denies_old|Pass|Returns no withdrawn old value and reports stale/empty reads without guessing the hidden reason.|
|t11_revoked_original|Pass|Does not restore denied content through a historical cutoff and leaves the denial reason unknown.|
|t12_completed_episode|Pass|Reports the retained completion claim, distinguishes owner acceptance from independent effect proof and grants no authority to repeat the action. Its loose null-bound wording does not invent a date or a current mandate.|

## Checking the disputed additional statements

The second reviewer extracted only actual model-visible InputItems and tool
results for t02/t04 from original entries004–007 and011–012. Events, hidden
reasoning, authentication and transport material are excluded. The projection is
retained at
`/srv/storage/butler/agents/issue89-20261016/reports/native88-visible-context-review-v1/visible-contexts.json`,
SHA256 `f5768713802d67d17ccb9260fe50d68fda89e5dfa666bf0544f1ba32787e5423`.
Its individual original-entry hashes match the immutable review manifest.

For t04, entry011 Start InputItems[2] supplies recorded_at and verified_at
`2026-10-18T03:55:31.687396Z`, with the same source/accepted timestamps reported
by the answer. That date is grounded; inspecting only the later tool result
would have incorrectly classified it as invented.

For t02, entry004 contains current Пихта and language preferences, without the
old Кедр claim or its acceptance history. Entry005 returns current Пихта;
entry007 opens the old Кедр original without temporal acceptance metadata.
The statement that the old source was not registered as an accepted temporal
fact therefore exceeds the available evidence. The primary selected value is
correct, but the answer fails calibration and citation entailment for that
additional assertion. Empty filtered results and missing annotations are not
proof of rejection, absence of historical acceptance or a particular hidden
reason.

The v5 cohort mechanically completed12/12, with36/36 native observations known,
248587 full native tokens and no product NULL usage. All charges and the earlier
v4 NO_GO remain retained. Mechanical completion and improvement from9 to11
passing originals do not override the remaining semantic failure. A prospective
general contract clarification needs its own complete source/model/contract/
history/capacity freeze and independent full-cohort review before qualification.
