# Prospective memory utility evaluation

New suites use V3 sufficient-support scoring: additional exact, opened, currently
allowed citations no longer invalidate a complete support alternative. V1/V2 keep
the old exact-set metric; V2 current-artifact semantics remain unchanged. See the
[model-free repair protocol](../../docs/testing/issue115-utility-repair-2026-10-30.md).

For the separate actual Core/Broker72-slot development collector, preparation
evidence and frozen native journal, see the [2026-10-30 protocol](../../docs/testing/issue115-utility-native-2026-10-30.md).
This command remains an offline scorer; the integration harness owns inference.

Offline, Go-only measurement tooling for #115, separate from frozen v6 and from
the independently reviewed `memory-eval` suites. Reuses acceptance's strict JSON
decoder, canonical digest and source representation. No additional dependency,
model transport, database, service, credential read or execution permission.

```sh
go run ./cmd/memory-utility-eval -init-dir NEW_PRIVATE_DIRECTORY
go run ./cmd/memory-utility-eval \
  -suite NEW_PRIVATE_DIRECTORY/suite.private.json \
  -observation observer-owned.json -out NEW_REPORT.json
```

Initialization creates a private, exclusive directory and 24 prospective cases.
`suite.private.json` contains the verifier oracle and synthetic history: **never
send it to an answering model**. `inputs.json` is the complete current-task
projection. Past source documents belong to a separate source preparation stage,
not current input. `preparation` retains the pre-deletion originals separately from
the current `sources` tombstones/status, so a future lifecycle collector can write
then revoke them instead of pretending they never existed. `manifest.json` commits
the suite and projected inputs.
Cryptographic random values distinguish newly generated suites. Published templates
are development material, not a blind holdout; no independent review is claimed.

Four families × five conditions cover recall, preference exceptions, task resume
and reusable procedures with useful, superseded, deleted, foreign-scope distractor
and current-sufficient evidence. Three current documents let A rederive a procedure;
the history note lets B/C cite its prior verified recipe. Thus the baseline is not
artificially prevented from solving a procedural task. Current-sufficient controls
do not require memory access. Four extra cases cover inspection, UNKNOWN, a typed
verified rejection and a checked current artifact.

`UtilityObservation` is **observer-owned**, not model output. It binds suite and
input hashes and requires complete observation, current authority and unchanged
original state. Source reads have exact identity/revision/digest; candidate discovery
does not establish reading. A cannot read history, B/C can read the same permitted
history, and `D_empty_memory` is an explicitly diagnostic empty-history arm.
The evaluator does not itself execute or enforce these permissions in Butler.

The model final has only typed outcome/value/citations/effect/artifact/check fields.
Contradictory prose, duplicate keys, unknown fields, trailing text, false checks,
uncited guesses, stale/foreign reads and unmatched receipts fail. Typed absence
alone cannot prove `unsupported_ready_value`; the observer must also capture that
exact rejection reason. Arbitrary natural-language semantics are **not** evaluated.
Old prose outputs are not converted/re-scored. Native integration must correlate
observer rows with actual source grants, Broker receipts, exact task/run/revision,
recipient and transport journal before any acceptance claim.

Reports always set `native_quality_qualified=false`. A successful synthetic witness
only tests the scorer. Source storage/indexing, skill promotion/loading, actual
retrieval, restart behavior and generated responses are **not implemented by this
command**. Candidate/opened/use diagnostics consume captured rows; they are not
measurements of a running memory pipeline. `SumMemoryUtilityCosts` retains failed
attempts and nullable totals and rejects duplicate IDs/negative totals/overflow;
correlation with the original ledger and full stage coverage remain collector work.

Validate without inference:

```powershell
& ./scripts/dev/test-go.ps1 -Packages @('./internal/acceptance','./cmd/memory-utility-eval') -Count1
```

On laba, `scripts/dev/memory-utility-offline-lab.sh ISOLATED_ROOT` tests the two
packages' memory/CLI tests with race detection and vets both complete packages in a pinned network-none container, builds
the CLI and generates one new private suite. Old raw archives and costs are never
opened for writing. See the [prospective protocol](../../docs/testing/issue115-memory-utility-v1.md).

The separate [Core/Broker observer](../../internal/acceptance/utilitytrace/README.md)
connects actual persisted traces to these observations. Its isolated controlled
integration is `scripts/dev/memory-utility-trace-lab.sh FRESH_ROOT`; it does not
call a native model. Newly generated v2 suites use a current verified-receipt
marker for dynamic artifact IDs; existing v1 suites retain fixed-ID semantics.
