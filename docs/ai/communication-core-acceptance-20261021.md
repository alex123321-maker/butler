# Communication backend foundation — 2026-10-21

Scope: kernel portions of #99–#100, then backend waiting from #103. Based on
`origin/main ab28dd886f76313f05246b714a845244eaf857a5` plus the reviewed communication
specification branch. This is a dormant backend change, not production acceptance.
The full [product contract](../architecture/user-communication-spec.md) and
[remaining roadmap](../planning/user-communication-roadmap.md) still apply.

## Boundaries

No Web/Telegram adapter, public owner API, prompt/catalog registration, Bootstrap
composition, Compose/env activation or deployment changed. No native model or
actual channel recipient was used. No new dependencies, runtime or container is
required by the product. Temporary local Linux Go/PostgreSQL containers are test
infrastructure only. The primary checkout and its pre-existing untracked
`prototypes/` directory are preserved; work is in `codex/communication-core`.

Available code: strict versioned message/photo/report/widget contracts;
message/widget canonical transactions; exact private dispatched-control verifier;
independent binding snapshots and delivery outcomes; typed attempt final and
memory provenance; durable response obligation/correction; first-valid-response
waiting, fresh ownership and real Task owner-control hooks. Photo/report are
contracts only and cannot pass readiness. All hooks remain uncomposed.

Both Catalog currentness and per-target `ChannelGuard` are mandatory. Tests use
controlled channel guards, not a production privacy/artifact export policy.
Current Catalog is used independently in actual private-control and physical
deletion tests. A model schema, private token or preference is never authority.

## Reproducible checks

Linux, Go1.27.2, PostgreSQL17 with pgvector, disposable `*_test` database. Caches
stay in repository `.cache/`. From the repository root, with an isolated test DB:

```sh
BUTLER_TEST_COMMUNICATION_POSTGRES_URL='postgres://postgres@127.0.0.1:5432/butler_communication_linux_test?sslmode=disable' \
  bash scripts/dev/communication-core-tests.sh --regression
```

For a Docker host the project can be mounted at `/src` in `golang:1.27.2` using
the test PostgreSQL container's network namespace. The script never starts the
application, admits a real owner, calls a model or connects a channel sender.

Required communication cases run with `-race -count=1 -run '^TestCommunication'`
across contracts, budget/authorization, Broker, owning app control, kernel,
Orchestrator and memory consumers. A skipped required case fails the script.
`go vet` covers those packages plus Input, Completion, Task, Run, outbox and
transcript. `--regression` also runs existing affected package tests, the changed
shared private-control fixture with race and no skips, and builds both services.
Unrelated native, backup and server-browser tests are excluded from that broad
regression selection; any other skipped regression is recorded, not called a pass.
`verify-changed.ps1` supplies the subsystem/spec mapping; it is not itself a test.

Reports remain local in `.cache/communication-core/{targeted,regression,control-regression}.jsonl`.

Current results: **29 distinct top-level communication checks passed with race,
zero failures and zero skips**. The final-source reruns in `core-final.jsonl`
(17 kernel checks) and `memory-final.jsonl` (2 compaction checks) include the last
control/privacy and actual PostgreSQL typed-compaction fixes. These overlap the
main targeted report; counts are deduplicated rather than added.

The wider existing-code selection passed **379 top-level tests**, with no
failures and **8 skips**. The skipped cases are not acceptance evidence:
`TestLegacyTaskMigrationIntegration` (exclusive migration DB),
`TestExecuteIntegrationPersistsRunAndTranscript` and `TestPipelineWorkerIntegration`
(Redis fixture), `TestBrokerBrowserSequenceE2E`,
`TestResourceBrokerActualChromiumPageCASIntegration`,
`TestWebToolsBrokerSixDefectsIntegration` (separate browser/runtime binaries),
`TestDelegationBrokerHarness` (subprocess-only helper), and
`TestActualPostgresSummarySnapshotDumpRestoreDeletionIntegration` (backup DB).
The shared delegation control selection is rerun separately with its compiled
Broker harness: **12 top-level checks passed with race, zero failures/skips**,
including `TestDelegationAdmissionActualBrokerProcessIntegration`. Both actual
service binaries (`./apps/orchestrator`, `./apps/tool-broker`) built successfully.
Final-source `go vet` passed for the last changed kernel/compaction packages;
the preceding scoped vet covered the remaining affected packages. Formatting
and `git diff --check` passed; diff against main contains no Web, existing
channel adapter, Bootstrap, go.mod or go.sum changes.

Physical FileManifest deletion checks ran on Linux, matching Docker deployment.
The Windows host's directory-sync restriction is not reported as a passing
native-Windows privacy test.

## Evidence by boundary

| Boundary | Actual check | Limit of the evidence |
| --- | --- | --- |
| Schema/identity | Duplicate/unknown/authority fields, false/null wait, typed answer cardinality, fixed authenticated HTTP | No native model format-quality acceptance |
| Broker ownership | Actual PostgreSQL grant/dispatch/private endpoint for message and widget; token alone denied; saved-receipt lookup | Broker executor lost-ACK routing uses a controlled service; no full external process-kill matrix |
| Atomic publication | PostgreSQL transaction rollback, exact replay/conflicting body, two publications + different final, two independent destination snapshots | No real channel transport |
| Currentness | Wrong owner, changed destination digest, binding CAS, missing/export-revoked channel policy, source guard refusal | Production artifact/per-channel policy must be composed later |
| Waiting | Both response/Finish orders, simultaneous Web/Telegram actor proofs, first winner, one canonical response, taskless conversation | Actors are verified fixture identities, not real callbacks |
| Durable resume | Actual Orchestrator + PostgreSQL + fresh claim/lease/context; preference is DATA; no old tool call; interrupted activation also resumes | Provider is a deterministic fixture, not cloud/native behavior |
| Owner controls | Actual Task Controller pause/answer/resume, unanswered wait, amend/stop, fresh Input and old Input cancellation | No owner UI wiring |
| Lifetime | Old Input TTL moved beyond 25h while question remains answerable; DB-clock expiry/review is terminal/idempotent | Simulated clock/deadline, not a 25h soak |
| Obligations | Single correction, restart-spend preservation, fixed safe failure, quiet/verified_task, empty typed terminal, exhausted quota/no destination | No semantic proof that model text satisfies a user's request |
| Delivery | Existing worker/UNKNOWN policy with two targets; unknown does not resend; revoked pre-send export fails | No actual Web/TG ACK or real network outage |
| Privacy | Real Catalog/FileManifest deletion of publication and original ancestor erases question/response/transcript/outbox copies; cached send denied; historical sending uncertainty retained | Already exported remote content cannot be erased by this backend |
| Mixed version/rollback | Epoch filter plus raw old SQL claim denied; ordered 088->089->090->089->088 twice; down refuses accepted history | No live mixed-binary deployment or backup/restore rehearsal |
| Memory/final | Publication vs internal final provenance in extraction/compaction; separate final selector and quiet final | Full history/feed UI migration deferred |

## Remaining acceptance

Keep #99, #100 and #103 open: their complete criteria include wider routing,
fault injection and/or channel behavior beyond this authorized backend slice.
No generic cross-channel reply resolver/ambiguity UI or owner binding/read API is
installed. New renderers must validate artifact ownership/export and implement
real ACK mapping, capability/version negotiation, expiry presentation, bounded
owner/global quotas and diagnosis. Review/controls/epoch support must be composed
consistently before any future profile admission. Full AC01–AC22, real channels,
native behavioral evaluation, process-kill/ACK-persistence matrix, operational
rollback and production enable remain separate gates. There is no enable switch
in this delivery.
