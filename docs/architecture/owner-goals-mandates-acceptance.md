# Issue #14 acceptance evidence

These checks run on `ssh laba` using an isolated agent source snapshot and
`butler_owner_goals_test`, with a separate schema per case. Go is pinned to
1.27.2. Competing owner, executor and Broker processes each use PostgreSQL pools
with `MaxConns=2`; independent observer connections hold deterministic trigger
barriers and verify actual lock contention. Shared Compose, production state and
credentials are not changed by the acceptance suite.

| Criterion | Executable evidence |
|---|---|
| Author/source/version/scope/current state | Real owner API/proof/delivery test; immutable event snapshots and current/stale obligation view |
| Assumptions cannot grant authority | Fenced agent proposal rejected as mandate target; explicit owner confirmation creates a separate objective and source link |
| Broad initiative and selected extensions | Real HTTP recovery uses current assigned mandate without per-action approval; exact reviewed extension gate accepts permitted capability subset and denies changed manifest, added capabilities and revocation |
| Explicit ambiguity/conflict rules | Two matching grants require explicit selection; selected extension grant cannot override overlapping explicit deny; path/account scope escape denied |
| Amend/pause/revoke active branches | Actual PG ClaimGrant contention for revoke/pause/goal amendment; failed propagation fault plus fresh Manager restart; idle provider cancelled before sweep and late final ignored |
| Every initiative explains authority | Root/child persisted assignments retain goal/mandate/policy revisions, purpose, observation and reason; contended admission becomes stale after later owner amendment |
| Owner read/edit view | Real authenticated read/write API plus Chromium fixture covering assumptions, mandate amendment, current obligation reason, pending propagation and no token storage |
| Synthetic baseline | No seeded authority records or personal facts; fixtures use synthetic owner/service/account and reviewed synthetic extension |

`apps/orchestrator/internal/goal/authority_ordering_integration_test.go` proves:

- Authority mutation commits first: real ClaimGrant rejects; recipient sees no
  request, the grant remains unspent and no budget reservation is retained.
- Claim commits first: subsequent revoke retains a real recipient receipt or
  durable unknown outcome when acknowledgment is lost; re-claim cannot replay it.
- Atomic final Save commits first: revoke/branch sweep retain the unverified
  task result, final result and delivery outbox. Mutation-first Save is covered
  by the propagation fault test and creates none of those records.
- Admission commits first: later goal amendment denies that exact old assignment
  and eventually pauses its branch. It cannot adopt the new goal implicitly.

`scope_integration_test.go` denies a foreign account/service path and a second
recovery beyond the same mandate's external-call allowance. This tests quota
enforcement at real dispatch commit, not an optimistic precheck. The extension
test exercises the trusted same-transaction admission gate; it does not install
an extension or prove a third-party installer safe.

`execution_integration_test.go` uses the actual Orchestrator with a controllable
idle transport and real atomic completion. It inspects full owner-policy context
and revokes while no sweep worker runs, then injects a late provider final. No
authoritative result, outbox, task result or final transcript text is written.

The goal integration suite also proves that failed branch propagation does not
restore authority: the lease remains valid, but final Save/checkpoint already
fail; a trigger fault leaves an attempted pending job; a new service instance
retries and cancels root and child while preserving history. Current
purpose-policy changes invalidate old assignments and remain full text in fresh
context; an old propagation job leaves a fresh current task active.

Explicit manual admission proves owner authentication and current scoped
authority even with initiative disabled. The same mandate denies scheduler
admission; an unproved manual request or a revoked mandate is still rejected.

Trusted initiative admission can start a root as `prepare_only`; real PG child
admission inherits that restriction, refuses an override to normal and denies a
commit proposal despite the broad mandate. JSON cannot supply the private initial
policy. Both 038 down migrations refuse saved authority/bindings in real PG.

`deadline_integration_test.go` holds real budget-reservation INSERTs until the
database clock crosses a confirmed purpose deadline. Dispatch, model and child
admissions all roll back with no reservation, model attempt, dispatch journal or
child retained; the HTTP recipient sees no request and the grant stays unspent.
The model release deadline follows the earlier policy expiry while the durable
mandate budget period remains unchanged. Owner-shortened expiry also preserves
the original allowance and fences old branches. Unit tests cover DST gaps/folds and
calendar-day active hours; the Broker test checks that the runtime receives the
same earlier owner deadline even when no budget adapter is installed.

Recorded lab validation on 2026-10-11 (Tomsk), final Go source snapshot
`20261011-041738-551`: full `go test ./...` passed. Actual isolated-PG race checks
passed for goal/task, completion, input, approval, session, run, orchestrator,
API, actionauth, budget and Broker (12 packages) on `20261011-041115-651`;
the final manual-admission and purpose-row metadata changes passed the complete
goal/actionauth/budget/Broker PG race suites again (20 top-level goal cases).
Current Web checks passed on `20261011-041414-526` and are recorded in the agent reports
under `/srv/storage/butler/agents/durable-tasks/reports/`:
`goals-release-race-final.log`, `goals-shortened-deadline.log`,
`goals-final-manual-policy-race.log`, `goals-release-web.log` and
`go-20261010T211759Z.log`. Earlier combined PG/race
reports retain boundary evidence. The general unit suite intentionally omits database
URLs; the separate PG/race suites supply them explicitly. Existing generic Redis
and real Google transport checks are outside this authority acceptance run.

Run with an isolated `BUTLER_TEST_GOALS_POSTGRES_URL` ending in `_test`:

```sh
go test -count=1 ./apps/orchestrator/internal/goal
go test -race -p 1 -count=1 ./apps/orchestrator/internal/goal ./internal/actionauth ./apps/tool-broker/internal/broker
```

The actionauth/Broker packages need their own isolated
`BUTLER_TEST_ACTIONAUTH_POSTGRES_URL`. Omitting a URL skips its database cases;
a unit-only pass is not acceptance evidence. Narrow fixtures apply both 038
files after creating their production-shaped task tables. The Web checks are
`npm run typecheck`, `npm run lint`, and the Chromium `owner goals` fixture.

Practical boundaries: broad resource enforcement currently handles HTTP(S)
origin/path plus typed credential references; account selectors hidden only in
arbitrary request content need a dedicated contract. Goal policy provides
extension attachment permission, reporting preferences and intervention rules;
it does not implement extension installation, daily report scheduling or
initiative discovery. Revocation preserves already committed external evidence
and delivery intents. A model final remains an unverified result.
