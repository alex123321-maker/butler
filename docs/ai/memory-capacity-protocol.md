# Prospective self-hosted memory capacity screen

Issue91 measures synthetic data on the laba hardware, separately from answer
quality. The initial supported target is10000 live768-dimensional chunks in one
scope, two concurrent readers, one background catalog writer, PostgreSQL limited
to one CPU/1GiB, and a client limited to one CPU/2GiB. Warm current-source-guarded
Store.Search p95 must be≤300ms, no unbounded queueing or disclosure, and every
request must finish or explicitly fail within2s (100ms observer/scheduler
tolerance). The warm phase has readers only. The mixed phase has100 reads with
two deletions, two corrections and one new source/fragment ingestion; a barrier
starts the writer with the first read and saved operation/read intervals must
prove at least10 overlapping reads. Mixed p95 must also be≤300ms; typed currentness
denials are explicit limitations, unexpected backend errors/deadlines fail the
target. Twenty warm-up queries precede
100 measured queries. The first query is reported separately; it is process-first
with an unknown OS cache state, never described as a machine-cold measurement.

The fixed stretch screens are100000 chunks in one scope and100000 across64
scopes. They do not silently enlarge the supported target. Seed91, source text,
vectors and query order are deterministic. All original source/fragments use the
real Catalog; vectors are synthetic bulk fixture rows, not embedding inference
throughput. Current hash/revision/privacy guards remain real. Report ingestion,
bulk-vector load, Verify/Activate and exact vector reuse separately. Fake vectors
cannot establish semantic recall or native generation quality.

Measure latency samples and errors rather than discarding failures. Retain
EXPLAIN(ANALYZE,BUFFERS,FORMAT JSON), database/relation bytes, query counts and
bounded Docker CPU/memory/I/O samples. Mixed catalog ingestion, correction and
deletion are a separate phase, including explicit stale/deleted read failures.
Independent summary/consolidation/lesson worker composition and local model queue
measurements are additional required gates; the Store screen alone cannot close
issue91 or establish foreground end-to-end latency.

The1M screen is initially omitted: it exceeds the prospective one-scope100000
coverage cap and the selected1GiB PostgreSQL budget. Record it as untested. No
global cache flush, host restart, or production data mutation is permitted for a
cold-cache experiment. If the10000 target fails, compare native pgvector indexes
on an isolated copy with scope-filtered recall/currentness before proposing any
production change. A failing stretch size alone does not justify an ANN migration.
No separate vector database or durable Redis state is introduced.

The additional prospective actual-local contention slice fixes100 initial and
100 new fragments in one owner scope, one index job and one ordinary foreground
question. The existing reviewed retrieval profile is reused, without a new
semantic quality claim. Start the question when the second generation enters
actual local indexing IPC; no observational wrapper delays/replaces responses.
Require a completed real query/index request interval overlap and the ordinary
foreground input to contain actual retrieved bytes plus its current typed UNKNOWN.
The predeclared single-query wall bound is5000ms; one query cannot establish p95.
The foreground answer provider is a labeled deterministic control, so cloud
inference is zero and this does not measure a complete native-model response.
Retain every local request interval, queue/execution/peak observation, both job
outcomes and full durable reservation/scope/model ledgers, including failures.
Shared summary/consolidation/experience worker policy composition remains separate.
