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

## Prospective combined controlled workload (mixed-v1)

Before its first measured request, the combined runner fixes PostgreSQL at
1 CPU/1 GiB and the Go race client at1 CPU/2 GiB on the same laba host. It retains
10000 synthetic768-dimensional chunks in one user scope, with real Catalog
sources and current-source guards. Vectors are a deterministic seed91 fixture;
actual query embeddings use the pinned local service. This combination measures
cost and execution, and establishes no semantic quality claim. The local service
is shared and its existing limits are observed, not changed by the runner.

The first ordinary InputService/Flow attempt is reported separately with unknown
OS/model cache state. Twenty warmups precede100 warm attempts and100 mixed
attempts, with two readers. Every scheduled attempt, including denial/failure,
stays in its phase denominator. The controlled full foreground p95 target is
1500 ms, allowing the existing300 ms Store target plus1200 ms for input claim,
current authority, actual local IPC, transcript and atomic final publication.
This is a prospective engineering envelope, not a derived native-model latency
target. Each attempt must finish or explicitly fail within5000 ms; unexpected
errors, timeouts, missing source bytes or lost typed UNKNOWN fail the window.
The300 ms Store target is retained separately and is not replaced by1500 ms.

The mixed phase starts with actual admitted compaction and consolidation model
attempts, then runs ordinary foreground requests while100 new fragments are
ingested and indexed through actual local IPC. Two independent maintenance
sources are corrected and two are deleted during this phase. They are outside
the selected stable retrieval generation: this supported profile does not claim
uninterrupted successful answers while that same generation is invalidated.
Old references must fail currentness, deleted copied bodies/fragments must be
physically erased, foreign-owner reads must deny and the stable generation must
remain readable. At least10 foreground intervals must overlap the ingestion/
index job interval, at least one actual query/index IPC interval must overlap,
and both admitted model consumers must overlap an ordinary foreground attempt.

The controlled background provider waits only for its actual cancellation. Real
interactive priority must return both jobs to bounded pending/waiting state.
This is explicit background degradation, not concurrent native model throughput.
Each owner policy declares two calls before admission: the original cancelled
attempt retains NULL usage and its complete quote, and recovery after the
foreground phase requires a separate charged attempt. The worker must complete
that recovery within60000 ms under its original deadline and finite allowance.
No refunded reservation, silent retry or enlarged policy is allowed.

The complete experiment has a12 minute test deadline, at most221 measured/warmup
foreground queries plus six setup attempts,50 four-fragment local index batches,
and four background model attempts. The private fixture budget is fixed before
any request at1024 model turns,1 billion tokens,400 million active ms and8 GiB
provider input per owner day; individual tasks/conversations retain finite
limits. These are conservative admission units, not measured tokens. No cloud
authentication is mounted and all provider responses are labeled controls.
All durable reservations, known/NULL usage, job states, request intervals,
query plans, relation/generation sizes, CPU/RAM/I/O and local queue/device peaks
are retained. Process-first is not machine-cold; no host cache/clock flush or
production restart is performed. Native answer quality and native cloud p95
remain separate acceptance gates.

### Fresh mixed-v3 correction of the fixture contract

The complete mixed-v2 window failed and remains NO_GO, including its1587.255 ms
warm p95 miss against1500 ms. Its scratch-only UNKNOWN expectation conflicted
with Catalog's deliberate same-owner working-memory erasure on deletion; the
stale summary snapshot correctly lost its old transcript-revision binding. Mixed-v3 is
a new whole window with the same sizes,1500/5000 ms bounds, two readers, warmup,
hardware and complete denominator. It does not retry selected old measurements.

The foreground provider checks the current typed goal/constraints and either
the original synthetic working marker or the actual content-free privacy-erased
working state. It never claims that a scratch marker is durable external-effect
evidence. At least96 of100 mixed requests must complete. At most four requests
may instead explicitly deny with a typed Catalog restore/currentness/access or
Task conflict error, and each must overlap an actual deletion interval (including
100 ms observation tolerance). Generic cancellation, a backend/deadline error,
an uncorrelated denial or a lost current goal/constraint fails the window.
Every request stays in full scheduled p50/p95; completion/denial/error counts
are separate. A window of blocked requests cannot qualify throughput.

Corrections use the ordinary exact-revision Catalog correction API. Both original
admitted model jobs must still yield with NULL usage and unchanged charges.
The old summary policy must then terminate as stale after the concurrent transcript revision withdrawal,
without a second model reservation. A separately predeclared fresh owner summary
policy allows exactly one call against the current snapshot. Consolidation uses
its original separately charged retry. Thus the whole window still admits four
background model calls, including both original NULL calls, and never repairs,
refunds, increases or reopens either consumed original policy.

### Fresh mixed-v4 resource candidate

The1-CPU race client is not supported at the1500 ms p95 target: complete warm
samples in v2 and v3 measured1587.255/1588.446 ms, and v3 mixed p95 was1755.918 ms.
These remain valid misses even though the whole windows also failed fixture
checks. V4 prospectively changes the client quota to2 CPUs/2 GiB while retaining
PostgreSQL1 CPU/1 GiB, the race build, all221 measured/warmup queries, two readers,
1500/5000 ms gates, the exact v3 privacy-denial limit and complete background
accounting/recovery gates. No no-race candidate runs concurrently or replaces
these results. The current foreground source-proof contract from53d68390 is
included: its bounded default wait is2 seconds and every release still requires
a fresh current proof. Therefore the older/newer timing comparison is not a
controlled estimate of CPU alone.

The runner retains actual cgroup cpu.stat, memory.events and memory.peak before
its containers are removed, plus periodic samples through each measured phase.
Older removed cgroups cannot establish a throttling diagnosis. It requires20 GiB
free on the intended storage filesystem before creating a new fixture. The
first measured retrieval follows seed/verification and initial real local IPC;
it is neither a cold process nor a cold machine/model observation. OS cache
state is not controlled, and no cache benefit is inferred from its difference
from the warm concurrent phase. Cold shared-buffer isolation, if later measured,
must be separately declared and may restart only the owned disposable PG.

### Separate shared-buffer-cold-v1 slice

Only after mixed-v4's owned containers/network have been cleaned up, a fresh
private fixture repeats the same10000 synthetic-vector/current-Catalog setup,
actual local initial100-fragment index and six controlled setup attempts. The
client remains2 CPUs/2 GiB with race detection and PostgreSQL1 CPU/1 GiB. No
background model job is dispatched in this cache slice. After all setup work
has finished, the client stops its idle input worker and writes a content-free
barrier containing its exact database/schema/postmaster identity. The driver
must verify the exact owned PG container ID, report data mount and resource
limits before restarting that container alone. It never restarts main, flushes
host caches or changes a clock. A second barrier and changed postmaster start
time prove the restart; connection-pool recovery and current Catalog checks
precede the next request.

The first actual query measures the ordinary input-to-terminal-receipt path,
retrieval interval, authenticated local query IPC/queue/execution/peaks and the
controlled provider's actual entry. PostgreSQL shared buffers were restarted;
mandatory metadata recovery may have warmed metadata pages. The OS page cache
is uncontrolled and the model is explicitly already warm from initial IPC.
This is not a machine-cold or model-cold result. Twenty warmup queries precede
one matched serial warm query and100 two-reader warm requests. All122 retrieval
requests and every outcome/charge remain; the cold and matched serial requests
must complete within5000 ms, and the100-request warm p95 must be≤1500 ms with
all100 completed. There is no general p95 claim from the single cold observation
and no cache-speedup inference from unlike concurrency. Database read/hit
counters, complete ledgers, resource samples and exact restart receipts survive
failure. This separate slice does not replace the mixed workload or any miss.

### Fresh mixed-v5 and shared-buffer-cold-v2 resource contracts

Mixed-v4 failed at its frozen target: warm p952138.730 ms with98/100 completions,
mixed p952619.542 ms with96/100 completions, one typed restore denial and three
unproven errors. Its actual client2-CPU cgroup retained only22417 throttled µs,
whereas the1-CPU PostgreSQL cgroup retained22786769 throttled µs/400 throttled
periods. This justifies testing a new resource profile; it does not prove the
cause of any individual cancelled request. Older clock/contention conditions
are not reconstructed from these later counters.

Before either new collection, PostgreSQL is now fixed at2 CPUs/1 GiB and the
client at2 CPUs/2 GiB, with race detection, the same1500/5000 ms targets,221
full-window queries, two readers and unchanged availability/typed-denial bounds.
The stale summary may return the actual typed Catalog denial rather than only
jobs.ErrClaim: its terminal authority/source-denied row and exactly one original
NULL reservation remain required before the fresh explicit one-call policy.
Fixed private Core cancellation enums and type/sentinel chains are retained
prospectively; arbitrary provider/storage error messages remain excluded.

The earlier unexecuted shared-buffer-cold-v1 recipe at2+1 CPUs is withdrawn.
The separate shared-buffer-cold-v2 slice uses this2+2 profile and its already
declared122-query/cold/warm bounds. It runs only after the complete mixed-v5
window has ended and cleaned up, without overlapping another owned capacity
profile. All1-CPU and2+1-CPU misses stay in the report. A failing new collection
is not repeated unchanged until it happens to pass.
