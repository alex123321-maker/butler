# Deferred tool schemas: implementation verification

Client date: 2026-10-19 (Asia/Tomsk).
Code under verification: `2c8987e3450194b77a76b7015fd402a3a1d42957`.
Base: `405a096b8eecfc3ad2fe1dce100c7e8a3ab9d312`.

The operator-only feature defaults to false on both services. Implementation and
process/wire verification passed. Native model quality, total cost and latency
comparison have not been performed; this is not production enablement evidence.

## Reproducible checks

Run `pwsh -NoProfile -File scripts/dev/tool-discovery-local.ps1`. The final run
passed 21 tests with zero failures/skips, then passed `go vet`. It used disposable
PostgreSQL 17 and Go 1.27.2 Linux containers, race-enabled Go tests and an actual
compiled Broker process. The containers were removed after completion. Exact
test names, elapsed times and the original JSONL SHA-256 are in
[the verification manifest](tool-discovery-verification-20261019.json).
The original local log is `.cache/tool-discovery/targeted.jsonl`.

| Criterion | Evidence |
|---|---|
| Initial context has only discovery schemas | Model-loop fixture and actual ordinary Core integration assert two definitions at model start. |
| Search covers methods beyond 64 | A 130-method search/page corpus reaches the last method; Broker scoped catalog retains more than 100 core contracts. |
| Bounded, useful discovery | Russian reminder/browser queries, exact-name rank, complete pagination and changed-result cursor rejection pass. |
| Loading exposes selected schemas on the next turn | Actual Core asserts two definitions after search and three after load, then executes the selected method. |
| Loading does not grant execution authority | Exact owning hash/resource/no-credentials checks reject forged controls; child search cannot disclose the parent tool set; actual Broker rejects an expired original input claim. |
| Durable selection and revocation | A fresh Service reconstructs refs from current-run receipts, ignores a foreign run, removes upgraded/revoked versions and denies unavailable catalog recovery. |
| Bounded context | Oversized definitions reject the load without changing earlier selection; repeated recovery replaces its own catalog context. |
| Transport updates | Antigravity tests actual serialized requests and signed alias continuity; Responses/Realtime, Codex collision aliases and Copilot wire replacement pass. Fixed discovery schemas fit the native Antigravity dialect. |
| Ordinary conversation without Task | Durable Input -> real Core -> Broker process -> search -> load -> synthetic gRPC read runtime -> final passes with exactly one runtime invocation and three durable completed tool calls. Final verdict remains unverified. |
| Existing action identity | Action-slot persistence/approval recovery and distinct intent/revision tests pass against PostgreSQL. Taskless slots remain per-run; identical references cannot link effects across runs. |

The runtime and model in the full Core chain are synthetic fixtures. The actual
Broker, PostgreSQL, authorization, input claims, dispatch, transcript, completion
and budget paths are exercised. Their success is not a native generation result
or verification of real system health.

The targeted full Windows package suites also pass for toolcatalog, transport and
all its adapters, actionauth, config, extensions, generatedtools, Broker app/core,
Orchestrator core and durable input. Compose configuration and `git diff --check`
pass.

## Broader checks and remaining release work

The broad Windows Orchestrator/application/registry/project-fixture run is not
green. It includes missing sealed native-history prerequisites, Windows private
file restrictions and a dedicated disposable-namespace requirement. Representative
failures were reproduced on untouched base `405a096b`:

- `TestNativeDelegationPrivateRuntimeFiles`: private key initialization failed.
- `TestReleaseChainV10ActualHistoryPreserves556`: sealed historical fixture admission denied.
- `TestFunctionCandidatePassesOrdinaryBrokerSchemaOnly`: function proposal unavailable.
- `TestPreparedArtifactIsDurableBoundedAndIdempotent`: fixture storage unavailable and Windows file cleanup failure.

No historical artifacts were rewritten to make these checks pass. The original
main checkout remained clean at its starting commit.

Before production enablement, complete the frozen real-provider comparison in
[the architecture contract](../architecture/tool-discovery.md): same task set,
model, instructions, budgets and independent task verification; record complete
usage/cost, latency, search/load turns and failures. Initial schema reduction alone
does not establish a cost or quality improvement.
