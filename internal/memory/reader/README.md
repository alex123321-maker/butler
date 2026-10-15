# Catalog DATA reader

This package implements bounded source/fragment reads for the owning
Orchestrator's private Broker endpoint. It never authenticates an owner from model
arguments, executes a tool, changes permissions, accepts a fact or contacts an
external backend. The caller supplies its dispatch transaction and repeats
current guards before committing. Catalog AccessTx remains the source/ancestor
authorization boundary.

See [read contracts](../readcontrol/README.md) for limits and current search scope.

The [mandatory lifecycle gate](../../../docs/ai/memory-lifecycle-acceptance.md)
is extended by `TestMemoryReadLifecycleAttackCorpusIntegration`: all eight
synthetic attacks pass through real PG discovery/open as unverified DATA with
exact run/source pins. Correction denies old handles, deletion denies/purges them
after Store reconstruction, and reads create no claims or action permissions.
The read-only consumer publishes no durable memory, cache or index generation;
those publication/restore boundaries remain catalog/jobs/semantic gate cases.

The prospective #87 consumer extends discovery with a reported-EventTime UTC
filter and server-derived semantic candidates. EventTime is not historical
validity. Candidate source/current-chunk refs are reauthorized through Catalog
in the owning read transaction; semantic IPC is outside it. Search remains
discovery and receives no opened read_ref. Production qualification remains off.
