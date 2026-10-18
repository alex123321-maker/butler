# Ordinary foreground during memory model consumers

This prospective controlled experiment exercises two actual production consumers,
task compaction and owner-prepared consolidation, separately. Each uses real
PostgreSQL, source/owner policy admission, the actual model worker and its
periodic priority/source guards. After its first inference has been durably
admitted, a new ordinary InputService→Flow→Worker attempt starts. A controlled
provider holds that foreground until the background worker has actually yielded;
the test creates no synthetic Session lease and does not cancel the job itself.

The predeclared bound is5000ms for each complete foreground attempt. This is one
controlled observation per consumer, not a p95, native quality, shared device
throughput or proof of the complete92 workload. No inference service, cloud
authentication or internet request is used.

The foreground must complete with its current typed goal and unknown-effect
state. The real background worker must return waiting/pending with one original
charged attempt, NULL usage, unchanged quote/reservation and no new candidate.
After the foreground lease ends, the existing finite policy admits a separately
charged retry. The ordinary compaction/restart or consolidation/read/delete
scenario then continues through its existing assertions. No reservation is
refunded, no NULL is converted to zero and no external effect is inferred.

The driver `scripts/dev/memory-capacity-background-lab.sh` creates an isolated
PostgreSQL1CPU/1GiB container and runs cached Go1.27.2 with race/vet at1CPU/2GiB.
It saves actual model requests, owning foreground input, retained background
attempt and exact charged quote. It requires both named tests, no skips/failures,
both observation files and the continuing coherent timeline before writing PASS.

`background-v2`, source `8934b67a`, passed both mandatory tests with no failures
or skips, race detection and vet on ssh laba. The compaction foreground completed
in257ms and the consolidation foreground in273ms, below each predeclared5000ms
bound. Each background job subsequently completed with two separately charged
attempts: the original NULL-usage cancellation and one known-usage retry. The
original attempt row remained byte-identical after recovery. Compaction retained
its1-turn/1081344-token/300000ms/8388608-byte quote; consolidation retained its
1-turn/68000-token/10000ms/8388608-byte quote.

Both finite policies included their two-call capacity before any admission.
The earlier `background-v1` retained a consolidation fixture with only one
predeclared call. Its foreground preemption passed, but recovery correctly denied
the second quote. That failed window was not reopened or enlarged. V2 is a new
complete controlled window with its capacity declared from the start.

Reports: `/srv/storage/butler/agents/issue92-20261018/reports/background-v2`.
The events SHA256 is
`1c3e6b5903c486e1b09c1af96568072739f2961581b45c6b835ff6fa9cffec07`;
the `evidence.sha256` file hash is
`ed308f17d5517b55aff3b2d9b2aa6b93f7ba065a4dee1c85f9a189023c0665b9`.
This proves the two described controlled overlaps and charged recovery paths.
Native latency, p95 and the complete92 workload remain separate gates.
