# Delegated context acceptance

Component evidence for issue16 and the source/privacy boundaries of issue31.
Model-selectable admission, objective report collection, production resource
routing and a shared-model single/parallel experiment remain pending. No issue
closure or production subagent deployment is asserted here.

Actual laba snapshot `20261011-080835-208`, Go1.27.2, isolated PostgreSQL/pgvector
with MaxConns2, actual deletion anchor and ordinary owned run pipeline:

`/srv/storage/butler/agents/delegation-resources/reports/race-20261011T010847Z.log`

Catalog and orchestrator app suites passed with `-race`. The context probe uses
scripted inference to inspect actual production preparation/publication, not a
native-model inference or external effect. Child receipt admission is a trusted
kernel fixture; it does not claim model-driven Broker delegation. Parent process
absence is represented by its completed durable run and expired execution lease;
this scenario does not claim an actual SIGKILL.

| Boundary | Observed behavior |
| --- | --- |
| Actual identity | Explicit task pin precedes selected context read; child has a separate session/run/lease and current task revision |
| Context | Selected contract/source roots plus own child history; parent transcript, unselected original bodies and ambient retrieval are absent |
| Authority | Candidate contract text remains DATA, not owner input/policy or kernel instruction; model final leaves task active |
| Provenance | Exact transitive original graph per selected source, not sibling roots invented as ancestors; original dependencies remain run privacy pins |
| Recovery | Selected body projection is refs-only; a new live reader restores current DB bodies and rejects altered selection/revoked permission |
| Deletion | Original source deletion before execution prevents the first model start; no ambient fallback occurs |
| Bounds | 64KiB selected raw bodies, bounded item envelope; stopped/expired worker and owner/job-role widening rejected |

Core preparation/projection/recovery and actual resource packages passed in
snapshot `20261011-080054-734`; the independent current-parent proposal grant
check passed Catalog `-race` in snapshot `20261011-080330-895`. Actual combined
workspace/resource tests passed snapshot `20261011-080623-908`, including the
timed-out run denial. Their production Broker/runtime composition remains pending.
