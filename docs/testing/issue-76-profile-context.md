# Issue 76: preference context preservation

## Scope and development protocol (2026-10-23)

Baseline: `b01d441ed7933d3a1d6ff43923858d002f1cc275` (`origin/main`).
Subsystems: memory bundle formatting and Orchestrator prompt assembly.
This is a synthetic development regression screen, not an independent holdout,
native model comparison, deployment acceptance or completion of issue76.

Source inspection found two losses between authorized profile selection and
model input: `scope_type`/`scope_id` disappear during text formatting, and the
2500-character section limit can remove a trailing exception or provenance.
The current selector is already `memory-context-v3`; the older `applyBudget`
description in issue76 does not describe this baseline.

Candidate: retain explicit profile scope and source handles in both bundle and
Orchestrator formatting; preserve each selected preference in full through
section assembly. The existing complete-context budget removes optional records
whole. Ranking, eligibility, accepted claims and source-release guards stay with
their existing owners. A scope label is descriptive data, not a grant or an
automatic conflict resolver.

Fixed development cases before candidate implementation:

| Case | Required observation |
| --- | --- |
| Equal keys in user/session scope | Both scopes, values and original source handles survive formatting. |
| Long preference with a trailing exception | Complete exception and provenance reach the model section. |
| Actual Orchestrator input | The same complete record is DATA, never system/owner authority. |
| Context overflow plus current owner exception | Remove the oversized optional record whole; keep a fitting record, current input and task prohibition. |

Run the named regressions on unchanged baseline, then candidate, followed by the
complete affected service/prompt suites and relevant Orchestrator context tests.
Any loss of an exception, scope, provenance or mandatory constraint, authority
promotion, or context overflow is a failure. No model calls or external effects
are part of this screen; no token-quality, latency or cost improvement is claimed.
Unknown native-model quality remains unknown. Do not use this development set as
an independent control set in a later behavioral experiment.

## Remaining work

The subsequent [paired context/preference experiment](issue-76-context-results-20261023.md)
records native observations with this preservation fix shared by both arms. It
does not isolate the formatting fix's effect or establish production acceptance.

The selector can still omit optional preferences under count/size pressure;
preserving text does not prove semantic applicability or correct conflict
resolution by a model. This screen does not evaluate acceptance, revocation,
deletion/restart, inferred preferences or a new context-ranking policy. Those
require separately frozen scenarios, actual catalog/state evidence, and an
independent behavioral comparison before broad conclusions or issue closure.

## Local observations

On the unchanged baseline plus the new regression tests, both formatter cases
failed: all scope labels were missing, and the long rule lost its exception and
source in the model section. The actual Orchestrator-input case also failed.
The pre-existing whole-record overflow behavior already passed its new case.
These failures were observed before editing either production formatter.

After the candidate change, the complete `internal/memory/service`,
`apps/orchestrator/internal/prompt` and `apps/orchestrator/internal/orchestrator`
package suites passed locally on Windows with Go 1.27.2, as did `go vet` for those
three packages and `git diff --check`. The synthetic JSON round-trip check covers retained
source handles and quoted scope metadata; it is not a PostgreSQL restart test.

Reproduce using repository-local caches:

```powershell
. ./scripts/dev/common.ps1
$state = Initialize-ButlerAgentEnvironment
go test -count=1 ./internal/memory/service ./apps/orchestrator/internal/prompt ./apps/orchestrator/internal/orchestrator
go vet ./internal/memory/service ./apps/orchestrator/internal/prompt ./apps/orchestrator/internal/orchestrator
```

The complete-input estimator includes the retained text and metadata. Some long
preferences that previously fit only because they were cut will now be omitted
whole under pressure. This is an explicit tradeoff, not a recall improvement.
The combined `FinalPrompt` preview keeps its display limit; model delivery uses
the separately admitted `Sections`, covered by the Orchestrator-input test.
