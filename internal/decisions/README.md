# Local decision observations

Optional Go caller for source-bound shadow annotations. It exposes no tool,
task-control, resource-grant or completion-verdict interface. Every event is
preserved; owner/controller policy remains authoritative even when a model
predicts stop, new task or confirmed success.

`New` requires the private composed `local-models:9090` backend, operator-pinned
service identity, fixed reviewed CPU profile and explicit `local_no_billing`
quote. `Observe` requires a current finite owner-created extract-kind job with
the distinct `decision_shadow` purpose and exact source revision/access/hash.
Secret data is denied before IPC. Every actual RPC receives a fresh durable
job/root-task/mandate/owner-day reservation. Bounded cancellation and current
source/job authority are checked after the call. Fabricated, uncorrelated,
partial or stale responses fail closed.

The nine-label taxonomy is fixed. Scores are uncalibrated. Score below 0.65,
unknown margin, margin below 0.15, empty output or insufficient data escalates.
Selected-only GLiNER profiles return one score and always escalate. A claimed
distribution must contain every category; partial alternatives fail closed.
The caller also pins preprocessing, FP32 precision and the reserved context cap,
and rejects selected-only replies from a
full-score profile. Reported score margins never grant execution authority.

`FullScoreProfiles()` selects separately versioned experimental GLiNER profiles
using the existing SDK's `Classifier(model).score` on the same resident weights.
All softmax scores are returned without another checkpoint load. The existing
profiles retain their selected-only output for exact comparison. These new
profiles are present only in `models.decisions-fullscores-evaluation.json`.

There is no automatic production consumer/scheduler. The separate decision
profiles remain disabled in the ordinary catalog. The explicit public lab
experiment records all cases, labels and failures in checked-in artifacts.
Research jobs use the extract lane only for this separate purpose; a future
worker must select `ClaimKind("extract")` and reject unrelated purposes. It
cannot resurrect a completed run lease or accept an owner permission.

See [the contract](../../docs/architecture/local-decision-signals.md) and
[acceptance matrix](../../deploy/local-models/acceptance-extraction-decisions.md).
Replay `deploy/local-models/verify-decisions-lab.sh` only after coordinating the
persistent service and operator evaluation catalog. Actual PostgreSQL/race tests
cover ambiguity, malformed scores, Secret/forged jobs, finite reservations,
late source changes and cancellation. A passing quality runner means a complete
experiment was recorded, not that any selection gate passed.

`verify-decisions-isolated-lab.sh <snapshot-root> <model-storage-root> selected|full`
instead creates one temporary internal service and PostgreSQL fixture for two
passes, using existing pinned dependencies/weights read-only. Its 8 GiB cgroup,
6 GiB RSS policy and reserve match the ordinary service. Both passes share that
service across idle unload, retaining cache pressure; deployed catalogs/services
are untouched. It preserves reports and removes only its owned containers/network.
The 2026-10-14 replay records all 480 attempts in
`deploy/local-models/decision-replay-20261014/`. No model passes screening. The
previously observed corpus is a regression fixture, not fresh held-out evidence.
Unavailable calls are counted as availability failures rather than class errors;
zero useful non-escalated signals cannot qualify as controller-call savings.
