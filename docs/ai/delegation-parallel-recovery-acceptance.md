# Parallel recovery without native inference

Baseline: `b2100bd2bcd4147b1168a252cd7369c3415c5140`.
Production Input Service already has two workers. No scheduler, worker count,
provider, ownership, source guard or native window changes are made here.

`TestDelegationParallelRecoveryWaitsNaturalClaimExpiryWithoutInferenceIntegration`
uses actual PG17/MaxConns2, Catalog, budgets, completion, ordinary Input Service,
core, separate child sessions and a real private Broker process. Its parent is
scripted, with actual Broker effects and two independently owned admissions.
Public Input Store claims with equal three-second TTLs start one second apart.
The test observes the first claim expired while the second remains live, then
waits for the PostgreSQL clock to pass both recorded expiries. No timestamp is
rewritten. This models lost claim holders; it does not claim an OS process crash.

A shared scripted provider holds both actual core streams open. Both current
claim IDs, attempts, task revisions, lease IDs/epochs and source guards must be
valid concurrently. Original accepted events/task/session refs remain unchanged.
The two current source-pinned observations become ordinary unverified finals and
result-ready reports. Parent effects persist and native spending remains27.
Scripted observations and timing barriers are implementation proof, never High
generation or quality evidence.

Isolated lab root: `/srv/storage/butler/agents/delegation-bounded-workers`.
Final focused source snapshot: `20261012-122046-128`.
Report: `reports/narrow-20261012T052049Z-60929`.
Actual PG/race/vet: one passing test, zero failed or skipped tests, zero inference.
`events.jsonl` SHA256:
`0f79512cbf10b3f4f77352349f75cff982e7974d5f6638b1625cf825fe43c671`.
Empty `vet.log` SHA256:
`e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`.

```sh
go test -c -o /proof/broker ./apps/tool-broker/internal/broker
export BUTLER_DELEGATION_BROKER_BINARY=/proof/broker
go test -json -race -p 1 -count=1 -run '^TestDelegationParallelRecoveryWaitsNaturalClaimExpiryWithoutInferenceIntegration$' ./apps/orchestrator/internal/app
go vet ./apps/orchestrator/internal/app ./apps/orchestrator/internal/input
```

The focused lab container uses an internal network, read-only source mount,
agent-local Go caches and an isolated disposable test database. It mounts no
operator provider import. Actual High overlap and the remaining native acceptance
controls must still pass their separately coordinated, immutable window.
