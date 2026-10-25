# Independent maintenance and rollback

Status: proposed implementation contract for issue21. This document does not
enable updates or establish release acceptance. Ordinary skills and tools retain
their existing admission contracts; diagnostics, a generated patch, a green
health endpoint and a model verdict grant no deployment authority.

Recovered from design commit e28a53ba for current-main local work on2026-10-25.
Implementation and independent acceptance remain incomplete; no production
updates are established by this local implementation/acceptance task.

Implemented so far: conservative inventory comparison, separate signed-review
integrity/qualification, immutable PostgreSQL evidence, profile/stage storage,
owner stop epochs and signed node-observation storage. These are component
boundaries only. Independent build/checker and controller/node processes,
effect admission, physical trial/promotion/rollback and active-state preservation
acceptance remain pending. No deployment profile is enabled.

The initial classifier treats existing apps/cmd/internal and deployment/checker
paths as protected. Ordinary automatic updates require explicitly pinned isolated
module capsules; no existing monolithic service is reclassified as ordinary by a
path boundary. Kernel permission remains exact and separate from ordinary grants.

## Owners and scope

The author prepares a bounded separate source/artifact version. The independent
checker owns fixed regressions, hidden cases, thresholds and compatibility checks.
The maintenance controller owns exact owner maintenance permission and durable
release state. A narrow node executor owns deployment of a fixed operator target.
The controller and executor run independently of the model and core process.
They expose no general shell, Docker API, repository path or credential lookup to
the author. PostgreSQL remains the durable source of maintenance decisions.

Changes are classified from the independently measured old/new source inventory,
not an author-supplied label. Classes are skills, ordinary tools, model
configuration, application modules and protected kernel. Unknown paths or mixed
changes inherit the more restrictive class. Policy, approval, owner controls,
credential mediation, journals, fencing, revocation, sandbox/egress, checker and
updater are protected. Updating the maintenance controller itself is an external
operator operation, outside its ordinary deployment targets.

An operator profile pins the supported module, its current immutable artifact,
allowed path/class boundaries, checker and regression identities, protocol/state
compatibility, fixed deployment topology and finite trial limits. Candidate data
cannot select another profile, checker, executable, network, mount or secret.
Protection is checked against the actual diff and built artifact, including
deployment/configuration changes. An ordinary repair permission cannot approve
changes to its own enforcement.

## Versioned records

The local implementation uses migration105, after current-main104; recheck that
number before merge. The former draft's059 reservation is obsolete.
Immutable records bind owner,
target/profile revision, exact base and candidate artifact digests, source/diff
digests, changed inventory, schema/protocol identities and measured classification.
An independently issued report binds that entire candidate, the fixed checker
binary/corpus and criteria, separately recording fixed regressions, hidden cases,
state compatibility and functional smoke verdicts. Author tests are additive.
The author cannot rewrite reports or replace failing cases with easier cases.

A separate owner maintenance mandate selects the fixed target/profile, permitted
class and source/version boundaries, criteria, finite budget and expiry. Within a
preapproved ordinary module, the controller may derive a one-use exact deployment
grant after independent checks, without another owner message. That grant binds
the measured known base and candidate, exact changes/report, trial deadline and
retained rollback artifact. A changed base must be checked again; a tag is not a
version boundary. Protected-kernel changes require explicit owner permission for
the measured protected diff, candidate and report, rather than a grant inferred
from an ordinary mandate. A general repair request or tool-installation mandate
is insufficient. Artifact tags and labels do not replace measured identities.

State separates staged, checked, trial_pending, trial_running, ready,
promotion_pending, promoting, enabled, rollback_pending, rolling_back,
rolled_back, blocked, unknown and stopped. A successful process start is not
checked, ready or enabled. Each effect attempt has a stable ID and monotonically
increasing fence; a transport timeout after dispatch becomes unknown and is never
automatically replayed. Node reconciliation must independently observe the exact
current artifact and deployment attempt before further action.

## Independent checks and state preservation

The checker executes the immutable candidate in a bounded isolated environment
with a synthetic database and runtime credentials. Owner credentials, live
transcripts, memory, arbitrary host files and the Docker socket are absent.
The checker binary, hidden cases and thresholds are operator-owned, mounted read
only and outside the author's workspace. The candidate does not publish its own
passing result. A failure to run or inspect a required check is a failure.

Checks exercise actual functions through their service boundaries: ordered input
acceptance, lease fencing, current authorization, approval suspension/recovery,
tool/result privacy, durable task/result and channel delivery as applicable.
Alive, ready and functionally working are distinct observations. Provider-dependent
checks report unavailable when unavailable; they cannot become zero-cost success.
Quality/error/cost comparisons use the same held-out cases and disclose unknown
usage rather than replacing it with zero.

Before a trial/promotion, the controller asks the owning modules for current
contract/state compatibility of active tasks, accepted inputs, approvals, grants
and effect/checkpoint continuations. It pins the current schema and protocol
identities and rejects incompatible active continuations before deployment.
Conservative blocking is explicit; incompatible state is never silently resumed
under a new contract. Compatible updates retain stable IDs, revisions, ordering,
source privacy, owner controls and current approval decisions.

The retained backup includes the known executable/configuration and a bounded
state-recovery record. It is not authority to overwrite live PostgreSQL. In
particular, rollback must retain newer owner stops, deletion/tombstone anchors,
accepted work, effect evidence and approval decisions. It must not undo external
effects or replay unknown ones. Secrets are retained only by their existing
credential subsystem, never copied into maintenance artifacts or model context.

Automatic application-module updates require unchanged state schema. A migration,
including an allegedly reversible one, requires a separately reviewed exact plan
and explicit owner permission. Irreversible conversion requires its own recovery
procedure and cannot be called automatically reversible. Executable rollback must
be compatible with the current schema; inability to establish that blocks rollback
and identifies the remaining operator action without restoring an old database.

## Trial, promotion and stop

The trial uses a fixed isolated target, synthetic inputs and finite time/call/
resource limits. It grants no live work or new permission. The independent checker
records actual functional outcomes, retained artifact identity and the trial's
measured costs/errors. Promotion needs successful current checks, current owner
permission, an unchanged known base, preserved compatibility, a valid rollback
artifact and current finite budget.

The controller commits a one-use deployment claim and exact effect identity before
contacting the node executor. No database transaction holds locks over network or
Docker operations. The executor checks the current claim/fence immediately before
the fixed target change, then reports measured artifact identity and outcome.
If stop races an already admitted effect, the actual outcome remains completed or
unknown; stop is never rewritten as proof that the effect did not occur.

Owner stop uses an independent authenticated controller path and durable epoch,
available while the main agent is down. It prevents new claims and promotions;
loss of controller/database authority denies new changes. Recovery does not
clear the stop or manufacture permission. A separately authorized exact rollback
may remain available to return to the retained compatible executable; it cannot
deploy an arbitrary artifact or lower protection. The author/model cannot unstop.

Initial Docker Compose deployment must keep this controller optional and default
disabled. Main services and author runtimes receive no additional Docker or root
capability. The fixed node-executor exception is justified only by deployment and
does not extend restart-helper's existing restart-only responsibility.

## Required isolated acceptance

Acceptance requires actual separate checker/controller/executor processes and
retained PostgreSQL history, not interfaces backed only by fixtures. Prove a good
candidate, broken release, live-but-functionally-broken release, incompatible
schema/continuation, author-modified tests, attempted protected-policy change,
concurrent version change, owner stop before/during a deployment, lost response
without replay and return to a compatible working version. Keep active tasks and
approvals across a compatible process update and explicitly deny incompatible
continuation. Report the original task's independently checked benefit and measured
error/quality/cost change. A component proof alone does not close issue21.

The current owner instruction confines execution to this PC. Use isolated local
processes, storage and synthetic effects. No SSH, remote test runner or update of
the working Butler deployment is part of this task. Keep exact source/artifact
versions and failed/unknown observations; local acceptance never implies rollout.
