# Terminal supervision and persistence regression

When a failed run blocked its own task, the still-running authority watcher
could observe that new task revision as external revocation and cancel the
provider. Core now joins the watcher before terminal task mutations and atomic
completion, using the original execution context and database write fences.
Real in-flight observation failures remain authoritative. Terminal persistence
errors retain their safe stage and inspectable original causes instead of being
lost as warnings or nil-error blocked results.

This small change extracts the production repair from research PR156 without
its candidate instructions, fixture stop policies, transport changes or archives.
The original repair is commit 4bbc9ec2bea3e7d258500b6e4b8fcd1cfc9f521f.

## Local validation, 2026-10-28

Base: 7537509d38f6ef21ae7d2641fa37ca8a2f27d2e6. Go1.27.2, Linux/amd64,
race detector, isolated PostgreSQL17 on this PC. No inference or real credentials.

- Full orchestrator package: 144 top-level PASS, zero FAIL/race reports,
  15.383s. Three unconfigured integrations skipped: the Redis composition and
  two memory-source cases needing their separate database variable.
- Those two memory-source cases then ran separately with the dedicated memory
  database variable: both PASS, zero skips, 11.250s. This covers slow current
  checks, actual revocation and refusal to publish after source deletion.
- Targeted terminal plus durable-task execution: seven top-level PASS,
  zero skips, 9.731s. The new standalone PostgreSQL regression first observes a
  genuine live authority check, commits the failed outcome through the real
  Store and holds the former race window open for600ms. It confirms the old
  authority is now invalid, no provider cancellation occurs, the original
  transport failure survives and task/run failure states are durable.
- Unit regressions retain in-flight lease/task failure, parent cancellation,
  timeout, wrong lease, idempotent join and safe terminal-write error propagation.
- `go vet ./apps/orchestrator/internal/orchestrator`: exit0.
- `scripts/dev/verify-changed.ps1` and `git diff --check`: exit0.

Reproduce with an isolated migrated test database using:

```sh
BUTLER_TEST_POSTGRES_URL=... go test -race -count=1 ./apps/orchestrator/internal/orchestrator
BUTLER_TEST_MEMORY_POSTGRES_URL=... go test -race -count=1 -run='^Test(OwningSourceVerificationBoundSlowCurrentAndRevocationIntegration|IdleProviderCannotPublishAfterSourceDeletionIntegration)$' ./apps/orchestrator/internal/orchestrator
go vet ./apps/orchestrator/internal/orchestrator
```

The Redis-dependent full-service test was not run. This evidence establishes
the bounded Core repair, not model quality, delivery acceptance or deployment.
The initial local runner stopped before Go because persisted Linux mounts were
absent after restart; reconnecting the existing image resolved that environment
failure. No closed research result was reclassified.
