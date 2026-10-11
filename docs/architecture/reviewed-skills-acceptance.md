# Reviewed skills acceptance

Checks run on the isolated laba agent snapshot and test databases, with no main
deployment, private account reads or real external writes. Go 1.27.2 is used.
Migration 051 has not been deployed by this agent.

| Requirement | Evidence |
| --- | --- |
| Independent source proof | Manifest/unit tests reject model-only verdicts; the actual private task adapter verifies owner criteria, immutable final run/message and recipient-confirmed historical contracts. |
| No new authority | Procedures/descriptions are private Document DATA; strict parsing rejects unknown/duplicate authority fields. Exact tool/task/approval/budget/source guards remain responsible for execution. |
| Progressive bounded loading | Real Broker scenario observes description before selected `skills.load`, checks its immutable digest and restores procedure DATA after each approval; at most 12 descriptions, 16 KiB per body, four run pins. |
| Dependencies, failures and inapplicability | Manifest carries exact tool hashes/revisions, applicability and explicit failure/unknown stop rules. Fixed validation rejects unsafe replay recommendations and requires three scenario classes. |
| Updates preserve accepted behavior | Actual PG tests reject changed interfaces and invalid proof; working version remains active until accepted CAS. Concurrent usage pins at MaxConns=2, rollback and permanent revoke are checked. |
| Bounded correction | Actual PG test admits only descriptive correction under exact parent grant plus current mandate/revision/purpose; changed inputs, expired/revoked grants and revoked mandate cannot promote. Database time is rechecked after waits. |
| Outcomes and resource measurements | Actual baseline/result and skill-use/result remain distinct; quality reports verified and unverified attempts, technical tool failures, durable model attempts/token allowances and nullable observed usage. It never treats a new skill as a completed task or missing usage as zero cost. |
| Approval continuation and retries | Production Broker plus separate reviewed runtime and PG perform inspect → exact approved prepared patch → objective stored-artifact check. Fresh provider/flow instances resume the same run at three approval boundaries. One recipient POST occurs per task; original project stays unapplied. Failed objective check produces no authoritative final or owner verification. |
| Source deletion and backup restore | Actual PG tests physically redact derived manifest after deleting its parent; an old snapshot is denied before reconcile, then physically redacted. Cross-scope citation alone cannot publish. Missing anchor denies access. Digest/lifecycle audit survives body removal. |
| Owner interface | Chromium verifies exact lifecycle generations, evidence display, usage/outcome separation, rollback, correction grant/revoke, deleted-source history, escaped hostile description and no token persistence. TypeScript and lint pass. |

The end-to-end test uses scripted inference proposals and actual owned execution
components. Fixed validator scenarios are independently enforced checks, not
execution of arbitrary natural-language test descriptions. Baseline and loaded
procedure consume respectively four and at least five admitted model starts;
the fixture proves truthful measurement, not a cost reduction. Model-generated
quality improvements must be established with later independently verified
outcomes. Owner verification remains a separate authenticated decision.

Run actual PG tests with `BUTLER_TEST_MEMORY_POSTGRES_URL` pointing to an isolated
`*_test` database, then repeat `-race` for `./internal/skills` and
`./internal/memory/catalog`. The production process test uses
`scripts/dev/lab-extensions.sh` with
`BUTLER_EXTENSION_E2E_TEST_REGEX=^TestReviewedSkillRealBrokerDurableApprovalE2E$`
and an isolated `BUTLER_TEST_ACTIONAUTH_POSTGRES_URL`. The fixture narrows both
its Orchestrator and Broker pools to MaxConns=2. Run the Chromium test
`web/tests/e2e/reviewed-skills.spec.ts` alongside typecheck/lint.
