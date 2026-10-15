# Isolated issue82 acceptance stand

This is an opt-in test composition, not the main laba deployment. PostgreSQL
uses a private internal network with test-only trust authentication and no host
port. Keep its data directory and credentials outside source/evidence exports.

`scripts/dev/memory-jobs-verify-lab.sh ROOT FRESH_LABEL` runs narrow PostgreSQL/race
tests first, then service checks, the actual Doctor/Broker test, vet and builds.
Storage, legacy public-schema fixtures and native inference use separate databases.

The native overlay composes the same current configured Antigravity provider,
plus isolated Orchestrator/Broker/Redis/proxy processes. It mounts only private
operator references; never put secrets into CLI arguments or public logs. The
prepare/start helpers are one-use setup. Before any new execution, freeze reviewed
source/binary hashes, current provider/model, finite shared budget and hard stop.

The recorded native-v1 and native-v2 windows are **consumed**. Their script is
the frozen reproduction recipe, not permission to replay them or reset counters.
Both failed; no native publication/quality acceptance is established. A future
window needs a new prospective specification and must retain prior costs. Do not
remove the legacy pipeline guard or change providers as a fallback.

`scripts/dev/memory-jobs-audit-lab.sh ROOT audit-FRESH_LABEL` records a read-only,
content-free accounting projection and the deployed provider selection. It makes
no model call and cannot expose auth row contents. See the
[acceptance report](../../docs/testing/issue-82-memory-jobs.md) for exact limits,
results, skips, blocker and migration/application rollback instructions.
