# Reviewed extension acceptance

Validation ran on the isolated lab on 2026-10-11 with Go1.27.2, separate
`butler_security*_test` databases and disposable processes/containers. The
production deployment, Google credentials, main PostgreSQL and shared Compose
stack were unchanged. All writes went to a synthetic one-file project recipient.

| Requirement | Implemented boundary and proof |
| --- | --- |
| Distinct states | PG staged/reviewed/configured-disabled/healthy/enabled/available are independent; UI shows missing origin and enablement grants no task tools |
| Compatibility before enablement | Strict schema/ABI/dependency/unknown-member/duplicate/bounds tests; owner-only fixed GET readiness reserves037 allowances before egress |
| Update invalidation | Real production Broker+PG approval test denies a previously approved manifest after v2 selection; old attempt ends, new run/lease needs current readiness and permission |
| Disable and rollback | Same actual Broker fixture returns to v1, needs fresh permission and reads the existing unapplied artifact; immutable versions, goals/task revisions and recipient/effect history remain |
| Scoped discovery | Current owned task pin precedes catalog; exact tool/resource/credential access or current14 source/hash/tool `ExtensionPermission` required; catalog failure makes zero inference/effects; repeat pin and stale lease/revision tests |
| Deterministic search | Current permission filtering precedes stable lexical matching/name order/finite limit; semantic selection is a future admitted ranker over this allowed set, never a grant source |
| External executor | Real production Broker binary, authenticated Go executor subprocess, actual PG approvals/leases/journal/budgets and recipient HTTP; optional production image/Compose overlay and owner UI/install guide |
| Secrets and privilege | Actual deferred synthetic secret needs exact approval and fixed declared sink, reflected value masked; authenticated subprocess denies private/prefix/traversal/duplicate/command/workspace/raw-secret proposals before any request |
| Sandbox | Actual production image healthy as UID10001 with read-only root/token, cap0, no-new-privileges, no Docker/hostFS/PG/model/core/owner auth; separate confinement run has network none |
| Dependencies and source | Measured fixture binary digest matches fixed operator profile; immutable raw manifest text/source/revision/dependency history; no fetch/install hook or executable package format |

The complete fixture path is install → review → missing required origin → save
configuration → fixed readiness → enable → exact task attachment → approved GET
failed check → approved ordinary POST commit preparing immutable patch → verified
recipient receipt → GET passing prepared check with `applied=false` → update →
denied old approved action → fresh run/lease/permission → v2 deferred credential
GET → rollback → fresh readiness/permission → read retained artifact → disable.

Both authority orders are tested with MaxConns2: disable before saved-grant claim
creates no dispatch; claim holds the current installation lock and a later disable
waits, preserving the truthful already-executing journal. Eight simultaneous
readiness requests admit one attempt/charge; interrupted validation becomes
unknown, publishes no health and is never replayed. An owner requests a new probe.
Exact manifest text survives the private JSON relay without duplicate-key collapse
or whitespace normalization. Database down refuses nonempty durable history.

The network fixture changes only test construction: a public synthetic hostname
resolves to a public fixture address and a fixed dial reaches its disposable HTTP
server. The production IP/DNS/redirect policy remains enabled. The functional
subprocess and the network-disabled production-image confinement are separate
proofs; the latter does not claim production outbound delivery or real repository
build quality. Recipient check fields remain DATA; cause and permission come from
trusted task/action/manifest history, never `produced_by_butler` in a response.

Run affected Go/PG suites and `-race`, then `scripts/dev/lab-extensions.sh` with
the isolated actionauth test DSN/read-only source snapshot. Optional
`BUTLER_EXTENSION_E2E_TEST_REGEX` selects additional owner/initiative tests;
`BUTLER_EXTENSION_E2E_OUTPUT_DIR` keeps binaries in the operator cache. Run the
separate `lab-extension-container.sh` only against its explicitly allowed agent
root. Web validation covers typecheck/lint/build and all12 Chromium cases including
the reviewed installation path and Linux visual baselines. The old Win32 images
were preserved; Windows rendering was not executed on this lab-only workflow.

The initial ABI is deliberately bounded HTTP-JSON. Marketplace delivery,
automatically installed arbitrary executables and filesystem/shell tools are not
provided. Metadata cannot attest an arbitrary third-party binary: the operator
must deploy/review/pin the actual image, and packages cannot alter its target or
privileges. Reads still need normal network-release authorization. Generic writes
retain unknown-effect semantics; only the existing verified recipient protocol
permits confirmation/reconciliation.

The reviewed release was subsequently deployed to the main lab. A private
pre-migration PostgreSQL dump preceded migration050. Startup now runs the current
migrator before Broker readiness recovery; the executor, Broker, Orchestrator and
Web report healthy. Live authenticated owner listing through the private Broker
relay, anonymous rejection and read-only Web/API browser checks all passed. They
are separate from the synthetic recipient fixture above and installed no owner
package or real project action on the main deployment.
