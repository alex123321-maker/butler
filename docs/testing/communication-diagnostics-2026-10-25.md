# Communication C08 local acceptance — 2026-10-25

Scope: [issue106](https://github.com/alex123321-maker/butler/issues/106), based on
`fdc573f245e54693d38ea1d2652b61c39413aae2`. The change implements operator diagnostics,
finite owner questions, migration compatibility and a safe drain runbook. It does
not establish C09/native model quality, full V1, a deployment or live delivery.
The [open issue audit](issue-audit-2026-10-25.md) records the separate remaining gates.

## Local environment and boundaries

All execution took place on the Windows PC. Docker Desktop's engine was
unavailable, so tests used an isolated Alpine3.22 user-space root in the PC's
existing WSL, without changing Docker's OS/packages. PostgreSQL17.11 and
pgvector0.6.2 used a separate `communication_diagnostics_test` database on
127.0.0.1:55446; every Go fixture created/dropped its own schema. Go1.27.2,
Linux race detection/vet, Windows Nuxt and Chromium used repo-local caches.
No SSH, production DB/profile, real bot/phone, native inference, deferred model
experiment, token store or external recipient was used.

The Alpine minirootfs archive SHA256 is
`412454ed98025ad9cd910d13f3d448b184e81502baa83180e89f98d0f13674be`;
Linux Go1.27.2 archive SHA256 is
`ecbadb99091a3f46e31f5f934b068b1864eafa7995211b39eaddf76996045fe5`.
These are test tools, not new product dependencies or containers.

The supplied test driver is `scripts/dev/communication-diagnostics-tests.sh`.
Give it an isolated PostgreSQL+pgvector `*_test` DSN through
`BUTLER_TEST_COMMUNICATION_POSTGRES_URL`. It runs required targeted and regression
selections with `-race -count=1 -json`, rejects every skip and executes scoped vet.
Web validation uses `npm run typecheck`, `npm run lint`, and
`npx playwright test tests/e2e/communication-diagnostics.spec.ts tests/e2e/communication-feed.spec.ts`.
`verify-changed.ps1`, whitespace and documentation-link checks accompany the source.

## Criterion evidence

| Issue criterion | Proof and limit |
| --- | --- |
| Independent Web/Telegram status; accepted is not sent/read | Real PostgreSQL metadata/HTTP report tests induce Telegram UNKNOWN beside Web result_ready, without content/ACK disclosure. Chromium verifies separate states, drain, unavailable observation and the visible publication warning before expanding delivery details. Browser data is explicitly a fixture, not a live Telegram ACK. |
| Legacy results/pending delivery survive without historical replay | Additive089–095/100 up/down/up retains the original legacy result/notification IDs, content, pending state and zero attempts. No explicit publication is backfilled. Existing media/history and rebound-binding tests remain required. |
| Old workers/rollback retain explicit checkpoints and waits | Epoch SQL/trigger tests deny legacy claims; current compatible drain rejects fresh ingress while preserving accepted work and owner reads. No-expiry/early answer/restart/Core-resume, cancellation and bounded correction tests preserve the existing identities and spending. Time-boundary fixtures simulate an expired Input more than25hours old; this is not a25hour live soak. |
| Readiness is more than a flag | The report checks schema, epoch, binding versions/capabilities, guarded source/export, Web reader, planner, widget projections and actual fixed Broker contract hashes. A real local gRPC Broker test changes the report contract target and proves mismatch; cancellation/unavailable Broker remains unknown. `full_v1_qualified` always remains false. |
| Secret/body/ref isolation | Unauthorized/query-selected owner reads are rejected. Marker errors/ACKs/recipient and publication text never enter the metadata report or fixed-label metrics. Failure returns only an observation-unavailable enum; current owner access remains necessary. |
| Finite owner limits across versions | Normal publication races for one remaining slot preserve a single question and atomic rollback. Migration100 adds a versioned PostgreSQL policy and a separate owner-row lock/count trigger. Earlier SQL writers without the Go/advisory check wait for an uncommitted slot and are denied using the post-wait count. Unsupported snapshot isolation denies; policy mutation through an owner endpoint identity and destructive down with retained policy/history also deny. |

Migration100 has no dependency on097–099; the existing migrator records each
filename rather than treating the greatest number as coverage of missing files.
An existing owner with more open questions than the new ceiling retains them;
new admissions wait for capacity. Exact accepted receipt replay remains unchanged.
Only trusted startup configures the ceiling, default16 or1..64. Changing it
increments the durable policy version without rewriting admissions or budgets.

## Operational result

The [runbook](../operations/communication-diagnostics.md) covers current observation,
UNKNOWN reconciliation, immutable admissions, exact owner failure acknowledgement,
no-expiry waits and compatible drain. Schema downgrade refuses accepted history
and policy instead of deleting it. Disabling new conversations does not create
model/tool replays or claim successful delivery/task completion.

The [checked-in receipt](communication-diagnostics-2026-10-25.json) records exact source hashes, test names/counts and JSONL
hashes. Full raw local evidence remains under repo `.cache/communication-diagnostics/`.
Initial compiler/fixture setup failures are retained separately and are not passes.
The final acceptance counts exclude repeated development runs and subtest inflation.
Web dependency deprecation/Node warnings were present; typecheck/lint/tests passed.

Final required gates: 14 targeted and21 regression top-level Go tests passed with
race detection and zero failures/skips; scoped vet passed. All14 Chromium cases,
typecheck and lint passed. Subtests and earlier repeats are excluded from these
counts. GitHub Actions are deliberately not run for this local-only acceptance.

This accepts C08's local implementation and operational contract. Issue107 still
owns AC01–AC22 native working-profile/release acceptance and a separately authorized
production enable. Nothing in this report qualifies that stage or issue147's
current-request semantic behavior.
