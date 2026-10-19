# PR #119: merge scope and verification

This PR adds operator opt-in tool discovery. Both service flags remain false by
default. Merging makes the implementation available; general model-quality
qualification remains open. The owner-requested laba deployment and its exact
artifact versions remain recorded in the separate deployment manifest.

## Review correction

A contract could change between Core's loaded-selection lookup and Broker
preparation. Previously Core accepted the newly prepared runtime/schema even
though it differed from the selected contract. A regression test reproduced
one authorization and one runtime dispatch before the fix.

Core now captures the selected contract's execution hash before preparation and
compares it with Broker's returned binding before approval or authorization.
The same regression now rejects the replacement with zero authorization and
zero dispatch. Broker's existing exact checks cover later contract changes.
Unloaded and revoked proposals fail before preparation. Another regression
fills all sixteen method slots, rejects a seventeenth and preserves the earlier
selection. The successful scripted loop uses real contract hashes; the actual
Core/Broker/PostgreSQL process tests exercise the production binding too.

## Reproducible validation

`pwsh -NoProfile -File scripts/dev/tool-discovery-local.ps1` passed on the
corrected source: **24 top-level tests, 29 including subtests, zero failures and
zero skips**, followed by successful `go vet`. It uses disposable PostgreSQL17,
Go1.27.2, race instrumentation, compiled Broker and ordinary Core/Input/runtime
integration both with and without skills. The original JSONL SHA-256 is
`95b057fec639aa4e1534a046555d303fe001f3987ef411d24de16503f5870f67`.

The new `Tool discovery` GitHub Actions workflow runs that same no-skip suite
against its own PostgreSQL, verifies modules, checks candidate control with
Linux/race and uploads the exact JSONL. The runner resolves the repository from
its script location, so CI and the local Docker wrapper share one entry point.
CI status and logs are attached to the PR's current commit; this report does
not replace the checks for that commit.

Complete Windows package suites passed for all14 targeted packages: toolcatalog,
transport and its six adapters, actionauth, config, candidatecontrol, Broker
core/app, Orchestrator core and input. These are ordinary package runs; unlike
the mandatory Linux suite, database-dependent tests may skip without their
test database configuration. PowerShell parsing, changed Bash parsing, evidence
JSON parsing and `git diff --check` also passed. No dependencies or migrations
were added. The primary checkout stayed at the unchanged base
`405a096b8eecfc3ad2fe1dce100c7e8a3ab9d312`.

The earlier [implementation report](tool-discovery-acceptance-20261019.md) and
[post-rollout manifest](../../deploy/tool-discovery/post-rollout-verification-20261019.json)
retain their original source versions and counts. Historical Windows
sealed-fixture prerequisites described there were not rewritten.

## Remaining qualification boundary

The [sealed native comparison](tool-discovery-native-20261019.md) remains valid
for its recorded source and scope: search -> load -> actual Doctor execution
was proved; the full-usage pair used15,074 versus12,732 native tokens and took
12.38 versus26.50 seconds. Preliminary quality was4/4 versus3/4, including an
unresolved `invalid_tool_request`. Quality noninferiority and complete monetary
cost are unknown. No new inference was performed for merge preparation, and no
sealed window or budget was reopened. These observations do not qualify the
feature as a default or close an implementation/quality issue.
