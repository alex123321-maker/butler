# Catalog DATA reader

The optional owning `SourceLocator` callback runs only after full Catalog and
typed source eligibility. Experience discovery supplies canonical sanitized
`experience_evidence_v1` field-range pointers. Returned search text remains
discovery, and open still reads exact current original text with ordinary guards.
There is no accepted-fact cache or synthetic citation/read receipt.

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

The owning composition supplies `ExperienceEnvironment` before runtime reads.
Its content-free lesson/ancestor metadata filter runs before lexical or semantic
candidate ranking, pagination and Catalog run pinning. A configured empty value
excludes lessons when no experience consumer is installed; nil retains generic
reader behavior. Open/neighbors deny an incompatible lesson before acquiring its
pins. This prefilter never grants access: complete Catalog and owning experience
checks still run before release, and previously exposed pins are never removed.

The runtime additionally supplies `ExperienceDependencies` from the owning
experience `Prepared.DependenciesJSON()`: exact currently permitted Broker
tool names, hashes and revisions obtained outside SQL. All lesson ancestors must
match before candidate ranking or pins, including semantic candidates and exact
open/neighbors. An empty array excludes lesson lineage while retaining ordinary
DATA. Nil retains the lower-level environment-only composition; the Orchestrator
always supplies an explicit array. Later installed/current release proofs remain.
