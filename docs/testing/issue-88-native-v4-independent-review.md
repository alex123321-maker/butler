# Independent review of temporal native v4

Verdict: **NO_GO; qualification remains false**. The root reviewer read all twelve
exact answers, frozen rubrics and actual Broker result witnesses in
[the immutable review JSON](fixtures/issue88/native-v4-review.json), SHA256
`503df55274a9479aee339e660e5b2ac390dec8b02404ec310d0b460d083e6d34`.
This review was performed on2026-10-18 after execution. It changes neither the
questions, original responses, source receipts nor the predeclared criteria.

| Original | Judgment | Evidence and limit |
| --- | --- | --- |
|t01_valid_february|Pass|Selects Кедр for February and keeps the later Пихта interval distinct. Historical expiry does not erase the historical answer.|
|t02_current_head|Fail|Correctly selects Пихта, but says freshness is unconfirmed because current time is unknown. Both actual facts/open results already contain `freshness.status=fresh`, `current_usable=true`, `requires_revalidation=false`. The generic source revision marker cannot override that separate assessment. The empty current-head result for the old source also does not establish that it was never accepted.|
|t03_known_before_second|Pass|Selects Кедр at the recorded-knowledge cutoff and explicitly places the Пихта source and recording after it. The heading loosely calls validity an event date; this does not alter the cutoff result.|
|t04_not_yet_recorded|Pass|Refuses a concrete value at the cutoff and distinguishes later recording from earlier claimed applicability. The opening absence statement is read within the selected-source scope established by the question and answer.|
|t05_open_and_unknown|Pass|Separates the explicitly open Лён interval from unknown validity. Its additional relative wording and metadata are supported by the actual subsequent original open.|
|t06_overlapping_periods|Fail|The first sentence declares бирюзовый confirmed at the historical instant using the current `claim_status`. Both actual accepted intervals cover that instant and both have `conflict=true`. The later conflict discussion never withdraws the single-value conclusion; current owner-head status does not resolve historical truth.|
|t07_exception_expired|Pass|Preserves the permanent Russian preference after the session exception. The English exception and its exact end are supported by its actual original open. The answer's source-currency caveat should be clearer about the separate stable preference assessment.|
|t08_stale_technical|Fail|Correctly retains4.2 as the last accepted value and requires revalidation, but additionally says it is not currently applicable. Stale verification establishes uncertainty, not that the stored value stopped applying. No external verification was performed.|
|t09_event_time_disjoint|Pass|Selects Ива by its February validity and keeps the September source EventTime distinct. It separately reports owner verification and the absence of independent external confirmation.|
|t10_current_correction_denies_old|Pass|Does not disclose the withdrawn old value; both actual reads return stale/empty. It does not invent the hidden reason.|
|t11_revoked_original|Pass|Does not restore denied content through a historical cutoff and keeps the denial reason unknown.|
|t12_completed_episode|Pass|Reports the retained historical completion claim, explicitly lacks independent effect proof, and grants no authority to repeat the action.|

Nine originals satisfy their rubric and calibration checks; three fail. The
failed answers are not repaired or replaced in this window. The two clear core
interpretation failures in t02/t06 independently suffice to reject qualification;
t08 adds an unsupported inference from stale verification to inapplicability.
Mechanical12/12 completion, known45/45 usage and zero content leakage do not
override these answer-quality failures. Full old costs and NULL observations
remain as recorded in the v4 evidence document.

The prospective implementation clarification exposes the actual verification
basis and explains source currency versus accepted freshness, current decision
status versus historical conflict, and stale versus false. It adds no benchmark
answers or source values. Its effect on native quality remains unmeasured until
a fresh complete, independently reviewed cohort runs with frozen source,
contracts, questions, policy and full usage accounting.
