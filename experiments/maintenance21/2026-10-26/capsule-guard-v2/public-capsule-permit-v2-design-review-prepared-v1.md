# Capsule permit v2: checker versus maintenance effect — PREPARED_ONLY

Static design review only. No source edits, formal freeze, corpus authorship, process/Go/PG/Docker/model execution, key reads, check admission or maintenance permission is issued here. Prior v1 guard/package artifacts and evidence remain immutable.

**Conclusion:** the proposed two-kind v2 marker is preferable to calling a checker manifest a maintenance claim. It resolves the ordering problem without creating deployment authority, provided the host owns the two admission paths independently. A candidate must be runnable in a confined independent check before a qualifying review of that candidate can exist. That check is authorized by the separately established finite local synthetic-check scope, not by a nonexistent positive maintenance review. An actual fixed-target maintenance effect still needs its exact original signed/live one-use claim.

## Marker and host bindings

The proposed canonical bounded marker can contain only:

```
version:          butler.maintenance.capsule-launch.v2
admission_kind:   independent_check | maintenance_effect
admission_sha256: exact digest defined for that kind
candidate_sha256: measured candidate ELF
expires_at:       original finite marker deadline, never later than parent bounds
```

No commands, paths, target selector, permission flags, resource values, requested checker or signing request enter the marker. Keep the old strict canonical/duplicate/unknown-field rejection and byte bound, and accept only the exact two-kind enum under v2. The guard verifies representation, finite expiry and actual candidate hash, then execs the fixed candidate; it still cannot authenticate a plan, verify a controller signature, grant permissions or certify a review. The enum names the host's already-validated basis. It is not a request to grant that basis.

The private host must retain a full evidence join from marker digest to its exact original admission object, plan/profile/runtime identity, prepared process, candidate/image/guard/package hashes, invocation/case ID and observed output. A digest with no recoverable matching admission evidence is not qualified. The host must not accept caller-provided marker bytes or derive authorization merely because the marker parses.

## Two distinct code-owned paths

| Kind | Admission basis | What it may do | What it cannot do |
|---|---|---|---|
| independent_check | Operator-pinned frozen check plan and an already authorized finite local synthetic-check scope, with exact run/case/artifact/runtime identities and remaining allowance | Start one measured isolated candidate invocation, supplying only that case's bounded synthetic data; record checker-owned functional/negative/unavailable evidence | Change the deployed fixed target, create or consume a maintenance dispatch, issue owner permission, treat stdout as a review, sign author-supplied reports or expand the check plan |
| maintenance_effect | Exact original controller-signed claim verified against the fixed profile plus successful current one-use NodeControl admission, original stop/fence/base/expiry/permission/review checks | Perform only that exact declared node effect within its finite lifetime and report independently measured settlement | Fall back to independent_check when permission/review/authority is missing, choose another target or artifact, refund/replay a spent dispatch or erase UNKNOWN |

Prefer separate private constructors/entrypoints for a checker runner and maintenance effect executor. They may share a low-level fixed marker writer, but the kind is selected by trusted code, never an author argument or a public `kind=` launch API. A marker valid under one path must not qualify the other. No failed maintenance dispatch may be relabelled as a check to continue the same operation.

For `maintenance_effect`, preserve the existing meaning: `admission_sha256 = SHA256(exact original canonical EffectClaim raw bytes)`, matching the signed claim and root evidence. Do not substitute a trial manifest or resign it as a claim. For `independent_check`, use a separate versioned canonical check-admission type/domain that binds the frozen plan/scope, run, case/invocation, candidate ELF/image/guard/package/runtime and finite expiry. Its hash must be reproducible from retained checker-owned evidence. It is a check record, not a new ordinary maintenance permission class or a positive review.

The checker scope must already be explicit in trusted startup/operator configuration under the authorized local task. It must not be inferred from candidate code, author tests, a model's statement, successful compilation, an empty permission record or this note. Use finite plan/attempt identities, serialize launch consumption and retain spent/failed/unavailable attempts across the run; a crash or missing output cannot silently recreate a fresh allowance or select replacement cases. Final qualifying Review still comes only from the separate checker after actual fixed/hidden/compatibility/functional observations, with its existing pinned signer and Store qualification.

Call checker execution `independent_check` rather than deployment trial state. It leaves the installed target unchanged and cannot populate maintenance `trial_running`, `ready`, `enabled` or settled deployment state. A deployment trial/promotion/rollback remains a separate actual lifecycle; the already implemented claim-purpose set must not be silently broadened by this marker revision.

## The selected initial host profile

The proposed128MiB/32PIDs/oneCPU/effective-zero-swap, network-none/read-only root,8192-byte input/stdout and4096-byte stderr,15s prepare/measure,5s runtime and5s cleanup are a new fixed prospective capsule profile. They are not proof from earlier compiler limits and must be measured on the actual guard/candidate runtime. They grant no networking, mount or deployment rights. Observe the candidate hash/guard memory within that cgroup; OOM/output overflow/unavailable measurements stay explicit failures.

For the proposed root-owned0755 control tmpfs, the marker must also be root-owned and readable by uid65532 but not writable by it (for example0444 or0644 with no group/other write). Root0600 would deny guard reads; uid65532-owned writable marker would violate host-only publication. No preexisting marker reuse is permitted, and candidate/guard/ancestor paths must have no writable mounts. Host publication is a fixed exclusive atomic operation. Candidate reading the nonsensitive marker is acceptable; candidate writing or replacing it is not.

Actual process/container identity, PID start time, cgroup membership, privileges, read-only mounts, allowed FDs and IPv4/IPv6/network/socket boundaries remain required from the prior host-interface proposal. Guard wait expiry and parent prepare/runtime deadlines are conjunctive: v2 must not widen the existing30s marker bound or turn a failed5s check into another mode. The effect path retains one-use NodeControl and original currentness checks immediately before the declared effect. Races after admission retain actual completed/UNKNOWN outcomes rather than manufacture a no-effect guarantee.

## Needed public proof before freezing/using v2

The eventual implementation should make the following ordinary invariants observable in its existing public checks: the independent-check path succeeds without a qualifying deployment review while touching only its isolated synthetic runtime; marker/admission/ELF identities match; neither path can consume the other's evidence; invalid/unknown enum or missing plan/claim fails; candidate cannot write the marker; maintenance mode does not bypass NodeControl; finite buffers/deadlines and full runtime join are retained. These are proposed checks for a later coordinated implementation, not authorization here to write or run fault-injection tests.

V2 changes a trusted guard/host enforcement component, not candidate application behavior. Pin the new guard ELF, permit/host contract, package recipe/image and check profile separately. Keep prior v1 sources/digests/evidence, and do not call old v1 measurements proof of v2 isolation or authorization. No hidden authorship starts until the new public source/checker/target contract is frozen. Full issue21 acceptance remains the separate fixed/hidden functional, real task/approval compatibility, independent signer and actual trial/promotion/rollback evidence.
