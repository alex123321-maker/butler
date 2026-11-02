# Native SSO follow-up failures, 2026-11-02

These attempts are retained, not replayed to reconstruct metrics. All native
durations below include model deliberation, scheduling, failed actions and
waits. They end at failed Input finalization and are **not login latency**.

| Attempt | Ingress UTC | Failed Input UTC | Inclusive elapsed | Model attempts | Observed input/output tokens |
| --- | --- | --- | ---: | ---: | ---: |
| Synthetic native05 | 14:23:06.279059 | 14:30:45.033940 | 458.754881 s | 23 | 458944 / 1525 |
| Google native01 | 14:53:00.561516 | 14:55:31.113235 | 150.551719 s | 8 | 67356 / 849 |

Synthetic05 omitted selectors and page IDs from its submitted goal. The model
opened RP/IdP pages but exhausted its mandate before requesting SSO consent.
There was no accepted IdP POST. This fails complete independent page-selection
acceptance; the shared conversation also retained older fixture metadata, so
it cannot establish uncontaminated selector-free qualification. Its stopped
classification was known/no_uncertainty.

Google01 selected the configured RP, verification endpoint and correct
external-login capability, then failed during Broker metadata preparation.
The native logical Task slot had a permanent effect ID distinct from its
Run/call execution intent. The new handler incorrectly treated these as one
ID. Retained request count and exchange-claim count were both zero; no Google
authorization, consent or token exchange occurred. Its stopped classification
was uncertain/journal_uncertain. An initial operator CLI argument error
(`12m` exceeded its observer limit) happened before submission; the already
created owner Task was reused for this single admitted native attempt.

The fix passes trusted dispatch identity separately and validates both IDs
against the exact journal row. A composed real Broker/PG test uses a logical
slot to ensure IDs actually differ. A separate pinned terminal owning absence
review can reconcile only metadata creation after terminal fences; it retains
UNKNOWN grants/dispatches, spent budgets and the failed historical attempt.
This document does not claim that review has been applied or that a subsequent
Google login/delivery has passed.

Private lab evidence remains under
`/home/laba/butler-sso-20261102/reports/native-sso-05` and
`/home/laba/butler-sso-20261102/reports/native-google-01`. No secret, provider
query, browser cookie or private reference is copied into this report.
