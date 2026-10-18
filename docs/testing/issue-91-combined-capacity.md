# Issue91 combined capacity collection

The2-CPU client/2-CPU PostgreSQL deployment-build controlled mixed profile passed
its frozen1500/5000 ms and availability gates in v6; its separate private-PG
shared-buffer restart slice also passed. The closed race windows
below retain real foreground latency and availability failures; passing v6 does
not retroactively qualify them. Native cloud p95 remains unmeasured by controls.
Main deployment was unchanged.
The [prospective protocol](../ai/memory-capacity-protocol.md) fixes every resource,
build, denominator and permitted privacy denial before its corresponding window.
The [operations runbook](../operations/memory-capacity.md) describes finite
support, rebuild/retirement, backup/restore, disk reserve and worker/budget control.

## Scope and hardware

The shared laba host has12 logical CPUs (AMD Ryzen5 5600H) and an RTX3060 Laptop
GPU with6 GiB device memory. Cached Go1.27.2, PostgreSQL17.11 and pgvector0.8.7
are used. Private PostgreSQL has1 GiB,64 MiB shared buffers and32 server
connections; the application fixture pool maximum is2. The Go client has2 GiB.
CPU quotas vary explicitly in the table. The existing shared local-models
service and its pinned authenticated IPC are observed without changing limits.
Other root acceptance containers shared the host during later windows; no
per-request contention cause is inferred from that fact. Duration uses monotonic
time.Since; wall timestamps correlate intervals. Main clock/services were never
changed. Existing main uptime reporting anomalies were not treated as restarts.

Every whole window uses10000 current Catalog chunks,768-dimensional deterministic
seed91 vectors in one user scope, plus100 initial and100 new actual indexed
fragments in a separate project scope. Synthetic vector loading and the labeled
fixture activation proof establish execution/resource behavior, not semantic
quality. The current Catalog guard remains active on every actual retrieval.
The ordinary default scope list also includes the request session and global
scope, with no active generations in this fixture. Consequently the aggregate
diagnostics include semantic_index_unavailable while the selected user generation
still runs actual query IPC, Store.Search and returns four current hits. The
diagnostic is retained, not reinterpreted as complete indexing of all scopes.
Six actual setup turns precede the real61-second quiet-policy wait and corpus
preparation. One first retrieval,20 warmups,100 warm and100 mixed requests follow,
with two readers. The first retrieval is after seed/verification/initial local
IPC and has unknown OS cache state; it is not machine/model cold.

The full controlled foreground p95 ceiling is1500 ms, comprising the separate
300 ms Store envelope plus1200 ms for durable input claim, current authority,
actual local query IPC, transcript and atomic finalization. This prospective
engineering target does not redefine300 ms Store performance or native-model
response latency. Every scheduled request stays in its full phase denominator
and must finish or fail within5000 ms. Warm requires100 completions; mixed
requires at least96. At most four mixed denials may be explicit typed Catalog/
Task currentness denials overlapping an actual deletion interval plus100 ms.
Generic cancellation and unrelated authority errors remain availability failures.

## Closed race windows

Times are milliseconds; p95 is nearest-rank over all100 scheduled requests,
including denial/failure. Each row is a new whole window, never a selective retry.

| Window / source | Client + PG CPUs | Warm p50 / p95 | Warm completed | Mixed p50 / p95 | Mixed completed | Outcome |
| --- | --- | ---: | ---: | ---: | ---: | --- |
|v2 /a8dc3307|1+1, race|717.164 /1587.255|100/100|670.492 /1253.597|0/100|NO_GO; warm miss and unusable mixed fixture|
|v3 /3716cb2d|1+1, race|701.745 /1588.446|100/100|1396.765 /1755.918|98/100|NO_GO; both p95 misses, one unexpected cancellation|
|v4 /38eb5fc8|2+1, race|767.418 /2138.730|98/100|1737.890 /2619.542|96/100|NO_GO; p95 and unexpected availability errors|
|v5 /69e27563|2+2, race|756.931 /1522.640|100/100|1433.225 /2187.786|95/100|NO_GO; p95 and availability floor|
|v6 /58165262|2+2, no race|599.432 /1305.977|100/100|981.917 /1413.040|97/100|PASS; three bounded typed privacy denials, zero unexpected|

V1 failed before retrieval because the consolidation history was too recent;
its six setup turns and zero local/cloud calls remain retained. The correction
waits61 real seconds instead of rewriting timestamps. V2's strict scratch-only
marker expectation conflicted with real same-owner working-memory privacy erasure;
its omitted correction revision also correctly denied. These fixture faults do
not erase its valid warm latency miss. V3 measured all221 retrieval attempts but
then stopped on an ambiguous observer SQL operation_id column. V4 stopped after
its old summary correctly returned a typed source denial that its observer had
accepted only as jobs.ErrClaim. Subsequent collectors fix those observer defects;
the original windows and all measured failures remain immutable.

V4 incorporates53d68390's production foreground source-proof wait of2 seconds;
therefore its comparison with earlier sources is not CPU alone. The v4 client
retained22417 throttled microseconds/4 periods, PostgreSQL22786769/400 periods.
V5's2-CPU PG retained zero throttling; its client retained1272 microseconds/1
period. This justified a prospective resource candidate and does not prove the
cause of an individual failure. V5 mixed requests39/40 had unproven cancellation;
request47 retained the exact private Core enum execution_authority_check_failed.
Those requests did not overlap deletion. The two typed restore denials at mixed
requests0/1 did overlap deletion and remain explicit unavailable answers.

## Full background and privacy mechanics in v5

V5 reached phase closed in268.37 seconds, go vet passed and no race report was
emitted, while the test correctly failed its SLO/availability assertions.
The100 mixed requests took73.124 seconds. Two corrections used exact expected
source revisions and withdrew old references. Two real deletions denied old
references and left zero copied body bytes/chunks. They operated outside the
stable retrieval generation. Same-owner working state was deliberately erased;
current typed goal and constraints survived. The provider accepted either the
original synthetic marker or actual content-free privacy erasure. A scratch
marker is not evidence of an actual durable UNKNOWN external effect.

The second actual index job reused100 vectors and embedded100 new fragments in
18.737 seconds. Fifty actual four-fragment index IPCs plus221 actual query IPCs
were retained.36 foreground intervals overlapped maintenance and14 actual query/
index IPC interval pairs overlapped. Foreign-owner access denied and the stable
source remained current. The exact scan still enforces current scope/source
access; no ANN or post-disclosure authorization was added.

Both real admitted model jobs yielded to interactive work and returned waiting;
this is bounded background degradation, not concurrent native inference
throughput. Their original rows retained NULL usage and exactly their original
1-turn/98304-token/300000-active-ms/8388608-input-byte quotes. The old summary
policy subsequently terminated authority_or_source_denied with only its original
quote: Catalog deletion globally withdraws transcript revisions, which invalidates
the pinned summary snapshot. A separately predeclared fresh owner one-call policy
used the current snapshot. Consolidation used its original separately charged
retry. Recovery took3144 ms, published a summary through sequence8 and activated
the owner's consolidation. All four background attempts remained: two original
NULL rows, byte-identical after recovery, and two known synthetic400-token rows.
No reservation refund, reopened policy, budget reset or uncharged retry occurred.

V5 retained498 reservations and452 scopes. The owner-day conservative ledger used
498 turns,23732224 tokens,84360000 active milliseconds and1904263850 provider-input
bytes. These are admission charges, not measured tokenizer/electricity/invoice
usage. There were223 observed foreground model attempts; the controlled provider
reports80 input/30 output tokens per observed success. Native cloud/auth calls
were zero. A configured zero monetary price never converts NULL usage into zero
physical cost. Database size was100054707 bytes; vector relation size46366720.
Local IPC reported peak1769 MiB RAM and719 MiB device memory, maximum queue126 ms
and summed execution33114 ms; these service peaks are not incremental task RAM.
Actual CPU/memory/I/O and cgroup evidence remain in each retained resource log.

## Deployment build and PostgreSQL buffer restart

Fresh v6 and its separate cold-v3 slice were frozen in58165262 before collection.
They directly measure a no-race deployment build at2+2 CPUs with the same limits
and all original gates; estimated race overhead is never subtracted. Actual
lease/task authority observations preserve original deadlines and calls, fixed
error types and cumulative pool counters. Private PostgreSQL state/wait-event/
blocker observations omit all query bodies. Sparse sampling alone cannot prove
that no short wait occurred.

V6 passed in189.52 seconds,1 test/0 failures/0 skips, go vet0 and driver complete.
The first retrieval completed in597.674 ms;20/20 warmups completed with
p50/p95536.867/644.546 ms. Warm100 p50/p95 is599.432/1305.977 ms,
maximum1386.570 ms. Mixed97 completed and three denied with actual
typed catalog_restore errors at indices0/1/3 during deletion; all100 p95 was
1413.040 ms, maximum1480.613 ms, total mixed window50.128 seconds. No cancellation
was relabeled as an allowed denial. There were38 foreground/maintenance overlaps
and12 actual query/index IPC pairs. Both exact-revision corrections, both physical
deletions, foreign denial, stable current source, exact100-vector reuse and new
100-fragment indexing completed. The second index job took14.863 seconds. Both
model jobs yielded, original NULL rows stayed byte-identical and separately
charged recovery completed in1880 ms with four total calls and summary through8.

All221 scheduled retrieval-path requests remain;220 reached actual query IPC,
50 actual index IPCs covered200 newly embedded fragments, and224 foreground
provider attempts were observed including the six setup turns. The three denied
requests did not all reach IPC/provider.498 reservations/450 scopes remained;
owner-day charged498 turns,23826432 tokens,84600000 active milliseconds and
1912652284 input bytes. The originally fixed owner-day allowance was1024 turns,
1000000000 tokens,400000000 active ms and8589934592 bytes. Both original unknown
background usages remain NULL with their98304-token/300000-ms quotes; observed
controlled recoveries report400 synthetic tokens each, without invoice claims.

The corpus text SHA256 is
64b422659f79762c6487c37c225a9337f8b0e7fee2c22a9e4f87f8f0ed7ac84c.
Preparation took16.407 seconds. Its actual EXPLAIN(ANALYZE,BUFFERS) retained a
10000-row exact sequential scan and top-N heapsort,61.953 ms execution,30316 shared
hit blocks, zero shared reads/temp blocks; this prepared-plan sample is not cold
or full foreground p95. V6 database bytes102545075/vector relation46366720.
Final client/PG cgroup CPU usage was61.169223/116.639760 seconds, throttling
65239/121581 microseconds; peak cgroup memory1082621952/446103552 bytes and all
OOM/max events zero. These counters include compilation/test/vet/PG startup.
There were62 Docker resource snapshots: sampled CPU maxima201.39% client and
133.16% PG; the last cumulative private block-I/O samples were453 MB read/244 MB
written client and3.68 MB/1.02 GB PG. Local IPC reported maximum1779 MiB RAM,
719 MiB device,85 ms queue and31846 ms summed execution. Shared model-service
counters cannot be attributed exclusively to this profile.

Observed PG groups were73 ClientRead,26 without a wait event,6 WalSync and1
VacuumDelay, with no sampled blocker. Two observed task-authority conflicts
overlapped actual deletion and had typed restore-denied final receipts. Six
observed context-cancelled task checks belonged to successfully completed inputs;
one lease-ID cancellation has no direct run mapping in this artifact. Neither
these observer samples nor the passing new build establish the cause of v5's
three unexpected errors, which overlapped neither correction nor deletion.

The cold slice restarts only its exact verified private PG container after actual
initial indexing IPC, while the input worker is stopped. Database/schema/new
postmaster identity,10000 chunks, unchanged reservations and current Catalog must
be proven before one first query,20 warmups, one matched serial warm query and100
two-reader warm queries. All122 outcomes remain; first/serial wall≤5000 ms and
warm100 p95≤1500 ms with100 completions. Only PG shared-buffer restart is claimed:
mandatory metadata recovery may warm pages, OS cache is uncontrolled and the
model is already warm. No machine/model cold p95 is inferred from one observation.

Cold-v3, the same58165262 build, passed in146.80 seconds,1 test/0 failures/0 skips,
go vet0 and driver complete. The exact owned PG postmaster changed from
2026-10-18T06:31:20.268020Z to06:33:05.821999Z. Database/schema/10000 chunks,
reservation digest and current Catalog all matched before the worker restarted.
Its first query completed in757.873 ms: retrieval337.883 ms, four hits, actual
local query IPC82 ms with queue0, provider entry681.453 ms after input start.
These intervals are nested and cannot be summed. The matched serial warm request
took444.042 ms;20 warmups p50/p95520.652/705.420 ms;100 warm p50/p95
616.449/1337.124 ms. All122 completed. A single first/serial comparison is not a
general cache-speedup estimate or cold p95.

The retained PG cumulative read/hit counters changed from2226/1459096 before the
first request to10606/6274368 after all122. They persist across a clean PG restart
and do not prove physical disk reads; temp bytes stayed27770880 and deadlocks0.
There were25 actual initial index calls,122 actual query calls and128 controlled
foreground attempts including setup,275 reservations/247 scopes, zero background
model attempts and zero native cloud/auth calls. Owner-day charges remained275
turns,13338624 tokens,47220000 active ms and1073768942 provider-input bytes.
Local IPC reported peak1752 MiB RAM/719 MiB device, maximum queue76 ms and summed
execution21251 ms. Client cgroup peak204128256 bytes and CPU26.652959 seconds;
final post-restart PG segment peak190681088 bytes and CPU38.468720 seconds.
The latter is a new cgroup segment, not total PG CPU over the entire fixture.

## Issue91 criterion mapping

| Criterion | Evidence and boundary |
| --- | --- |
|Reproducible sizes/hardware/plans/results|Original10k/100k/64-scope report plus v6/cold-v3 protocol, exact source/seed/hash/plan/resources;1M explicitly untested.|
|Known supported size/load and overflow|Runtime10000/generation; two readers and finite2+2 controlled deployment workload pass. Over-limit actual regression denies before inference/activation; larger single scope and race SLO remain unqualified.|
|Interactive SLO or bounded degradation with jobs|V6 all100 warm/all100 mixed p95 passes;97 completed/3 typed deletion denials, no unexpected. Both admitted background jobs yield waiting and recover separately charged; original NULL charges survive. No simultaneous native model throughput promise.|
|Scope/revocation/deletion|Exact current Catalog/foreign-denial controls, two correction CAS/two physical deletes while queries run; stable generation survives. Deleting that generation can deny answers until rebuilt. No ANN/post-disclosure filter.|
|Comparative recall/latency/cost and rebuild/rollback|Measured exact10k/100k baseline and full known/NULL charges; synthetic vectors establish no semantic quality. Selected300-ms Store gate passes, so ANN comparison is not triggered. Existing staged reuse/review/activation CAS/rollback and retirement remain; native recall/QA retains separate evidence.|
|No unjustified service/database|No new service/dependency, vector DB or durable Redis state; existing pinned local service observed only.|
|Operational recovery|Runbook covers independent deletion anchor+DB/assets backup, isolated restore/currentness, finite rebuild/CAS, retirement,20-GiB reserve, ENOSPC worker/policy pause and unchanged budget history. Actual full restore/lifecycle remains parent acceptance evidence.|

This qualifies the explicitly bounded controlled capacity profile and operational
recipe, not an unbounded lifetime, native-model p95/QA or final issue77 production
readiness. Passing a new build does not retroactively qualify a failed profile.

## Acceptance boundaries and evidence

The existing [component report](issue-91-memory-capacity.md) retains10k/100k/64-scope
plans, seed, sizes and metrics.10k warm Store p95174.937 ms meets300 ms;100k in
one scope p951281.698 ms misses.100k across64 scopes p95121.953 ms queries only
one selected scope at a time. The runtime hard cap is10000 per generation and
over-limit admission denies before inference/activation.1M is explicitly untested.
Those passing Store measurements do not qualify the failed full foreground path.
An ANN migration is conditional on the selected Store target missing; it has not
missed, and an unrelated full foreground miss does not establish ANN benefit.
No ANN recall/cost improvement is claimed on synthetic vectors. Exact baseline,
scope/currentness checks, staging/review/activation CAS and rollback/rebuild remain.

The [retirement evidence](issue-91-index-retirement.md) separately proves owner
CAS retirement/activation races without deleting source or spend history. The
[priority evidence](issue-91-background-interactive.md) proves two single-query
controlled overlaps and charged recovery;257/273 ms are not repeated p95.
Native cloud response p95 and semantic answer quality remain unmeasured by this
collector. Full lifecycle/restore and lesson-worker composition are separate
parent acceptance evidence. No separate vector database, durable Redis memory,
new mandatory GPU service or infrastructure dependency was introduced.

Private evidence root:
`/srv/storage/butler/agents/issue91-mixed-20261018/reports/`.
Each window retains source/protocol checksums, actual JSON events/ledgers, query
plan/preparation, resource/cgroup samples, fixed limits/versions and its terminal
state. Failed fixtures remain; only owned containers/networks were removed.

| combined-mixed.json | SHA256 |
| --- | --- |
|mixed-v2|53c9b58b15f8513d79eb0b6ff1d8279d6d1f80cb8c54c11bdb4a62b043973ee4|
|mixed-v3|6031875111b266ffc09b8a551c8982d57d3791a89bcbd438a23443906a8aef81|
|mixed-v4|cb338c27437346947dcc39af9bb7ae55fc48a7a82ef9251a94524f93b105f8fb|
|mixed-v5|2a93179702fb6c2ee7ceaede17f1d194d57e93f35db4fe8c2a54937816eb8fcb|
|mixed-v6|97885469782d02f61c9bc69c02e04f388ff90a1140e8a11c75db185484c4ac65|
|cold-v3|c95b4b610d883b5ccda3651023f7b2e684dcad604d879db1fa5d62a83f3db229|

V6 evidence.sha256 has SHA256
61ea510c9abb68dd69ab80b79bb81bc4ac5ef564bb75fa49846d2236dd111a3a;
synthetic-preparation.json is
44d5669911309334ac208d5294080ba05f16d11a16277c8a90ef62a317040bc6.
Cold-v3 evidence.sha256 is
61ff42af17a1b38fda9ce1032c1c4a9f0bab65bb2746738773f9ca9c5f0d0d09.

Reproduce only with a fresh label and isolated immutable snapshot:

```sh
BUTLER_CAPACITY_LOCAL_MODEL_IP=172.28.0.3 \
  bash "$ROOT/current/scripts/dev/memory-capacity-mixed-lab.sh" "$ROOT" FRESH_LABEL mixed
# After its owned containers/network have been cleaned up:
BUTLER_CAPACITY_LOCAL_MODEL_IP=172.28.0.3 \
  bash "$ROOT/current/scripts/dev/memory-capacity-mixed-lab.sh" "$ROOT" ANOTHER_FRESH_LABEL cold
```

ROOT must match the agent directory guard. The runner requires20 GiB available
on that storage filesystem, cached images/modules and the existing private local
token file; it mounts no cloud auth and downloads nothing. No shared cleanup,
main restart, host cache flush or clock change is permitted.
