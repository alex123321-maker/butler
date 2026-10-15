# Synthetic memory capacity observations

This is component evidence for issue91, not production readiness or final77/92
acceptance. A12-thread AMD Ryzen5 5600H laba host ran PostgreSQL17.11/pgvector0.8.7
with one CPU/1GiB and a separate Go client with one CPU/2GiB. Docker CPU/memory/I/O
samples, hardware/version output and exact EXPLAIN(ANALYZE,BUFFERS) plans are
retained. No host cache flush or production restart occurred. Machine-cold cache
state is unknown; the first Search call is reported separately from warm queries.

The prospective protocol is [memory-capacity-protocol.md](../ai/memory-capacity-protocol.md).
Seed91 generated real synthetic Catalog originals/fragments and deterministic
768-dimensional vectors. Bulk vector fixture loading is explicitly not embedding
inference throughput or semantic quality. Coverage Verify/Activate used a labeled
fixture proof. Every timed Search used the actual current-source Catalog guard.
One hundred warm and100 mixed queries had two concurrent readers per size.

| Chunks / scopes | Warm p50 | Warm p95 | Mixed p95 | Mixed completed /100 |
| --- | ---: | ---: | ---: | ---: |
|10000 /1|120.158ms|174.937ms|152.706ms|24|
|100000 /1|1198.473ms|1281.698ms|1287.096ms|91|
|100000 /64|22.673ms|121.953ms|128.399ms|79|

Every warm query completed. Mixed workload contained two original deletions,
two corrections and one source/fragment ingestion. Recorded operation/read
intervals prove respectively10,14 and31 overlapping queries. Mixed failures were
explicit currentness denials:10000/1 had75stale+1denied;100000/1 had7stale+2denied;
100000/64 had2stale+19denied. These are unavailable answers, not successful reads
or hidden retries. Successful-only and full scheduled observations remain in the
raw report. Exact known-chunk owner-positive/foreign-denial controls and exact
withdrawn citation controls passed; a healthy unrelated source survived.

The64-scope cohort measures one selected scope per query, not one query searching
all64 or complete foreground latency. Its smaller per-generation scan explains
the lower latency. The supported prospective Store target remains10000 chunks
per generation, two readers and explicit bounded currentness failures under
mutation. The existing owned indexing runtime already caps a generation at10000;
100000 in one generation is an isolated Store stretch fixture, not a runtime
deployment configuration. No ANN migration is justified by the passing10000
target. The100000 single-scope stretch misses the300ms target. The1M cohort is
untested beyond the selected contract/resource budget and is not extrapolated.

| Chunks / scopes | Catalog fragments | Synthetic vector load | Verify + activation |
| --- | ---: | ---: | ---: |
|10000 /1|4.435s|5.237s|7.047s|
|100000 /1|43.670s|50.733s|67.288s|
|100000 /64|42.398s|51.535s|75.760s|

The standalone collection sourceefd8d7a4 passed all3cohorts/4test events,
0failures/0skips,503.125s. First fixture version failed before measurements because
its user scope did not match the authenticated owner; it is retained. The final
collection's report-directory permission prevented the shell checksum step;
only those three synthetic output directories were made readable, then checksums
were recorded. Measurements were not rerun or rewritten. Later code makes these
directories readable under the private operator-owned report root by construction.

## Actual local indexing and reuse

A separate pinned authenticated local-model slice exercised the existing owned
indexing runtime on100real synthetic fragments. It made25actual four-item local
embedding requests. Initial owned enqueue/rebuild took14.581s; a second exact-space
generation reused all100vectors in1.310s with zero additional model calls. Both
generations passed actual current coverage and owner-review activation CAS using
the existing accepted profile artifact. This does not independently requalify
answer quality on the synthetic resource corpus.

Service-reported observed queue maximum was0ms in this serial slice; summed model
execution was12.401s. Reported peaks were1685MiB RAM and707MiB device memory,
not incremental per-task allocations or a concurrency guarantee. Complete local
reservations/jobs/scopes are retained; there were no cloud calls. Native token
billing is not inferred from these local timings. This measured slice does not
diagnose the earlier87index rejection.

Sourcef49b019f passed the actual local slice in16.874s,1test/0failures/0skips.
The preceding compile-only field-width error is retained and made no model call.
The explicit `scope_chunk_limit` operational outcome was separately verified on
actual PG:4indexing regression tests passed,0failures/0skips, race and vet. An
oversized scope reached neither inference nor activation and spent no model quote.

## Evidence and remaining gates

Server root: `/srv/storage/butler/agents/issue91-20261016/reports/`.

| Artifact | SHA256 |
| --- | --- |
|capacity-v2/tests.jsonl|e4b6db9feb6a11b62782ab3092f5f907909656a7a9d472421a46ff34f0e8039f|
|capacity-v2/chunks10000_scopes1/results.json|3b85254e37c83bec1d0190b1cdb525160473fe1adaadcee06fe40493994101d2|
|capacity-v2/chunks100000_scopes1/results.json|205043d64fe633666d74cca536f427a74e5661771bbba637f5ee551b7dc3a686|
|capacity-v2/chunks100000_scopes64/results.json|964449009b412575cf1b1feb85644415ba7592c144abfecfedac3cda15408c8e|
|actual-local-v2/tests.jsonl|0368d9f0023a8deaf1d7f66b268d0f9af5c805e0134e9b859ef1b0ef541b3019|
|actual-local-v2/actual-local-index.json|0fbe0c2bd3008b603de8d0b2cfdfed8c412616997989840a705ef96fe485393a|
|limit-v1/tests.jsonl|7dfcfe7229d5e67bd8c842f5e925be419f1a32f831238d30462276f65d7b6d9d|

Independent summary/consolidation/lesson workers under an interactive workload,
model queue contention, maintenance composition and end-to-end foreground SLO
remain additional gates. The Store screen and serial local index slice cannot
close issue91. Capacity does not establish complete memory retrieval, truthful
answers, zero cost or readiness to activate new memory consumers.
