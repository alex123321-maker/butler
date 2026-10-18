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
The current source needs its own immutable run; implementation alone is not
acceptance evidence.
