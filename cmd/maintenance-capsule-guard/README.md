# Fixed private capsule launch barrier

Component for issue21, default absent from packaged artifacts and deployments.
This Linux PID1 command accepts no arguments, request stream, host paths or
environment configuration. It waits at most30seconds for one canonical finite
`/control/launch.json`, hashes the fixed regular `/candidate` ELF and replaces
itself with that exact path using only `GOMAXPROCS=2`. Other platforms deny exec.
An invalid, expired, oversized or noncanonical marker fails without a retry.

The current marker contains version `butler.maintenance.capsule-launch.v2`,
typed `admission_kind`, exact `admission_sha256`, measured ELF
`candidate_sha256` and `expires_at` no more than30seconds in the future.
`maintenance_effect` binds the original verified signed claim; only the node's
private current one-use dispatch path may publish it. `independent_check` binds
a separately frozen finite operator-authorized synthetic check plan/case and
measured artifact/runtime/bounds; it is not deployment permission. Separate
code-owned constructors must select these modes, without caller choice or
fallback. A checker can therefore inspect a candidate before issuing its review
without inventing a maintenance claim. The guard validates only marker shape,
expiry and candidate hash: it is not an owner permission or a signature.
The future fixed executor must pin the independently built
guard and candidate, validate actual no-network/no-privilege/kernel limits
before atomically publishing the marker, and obtain current one-use controller
dispatch immediately before launch. Only the private host can publish it while
this guard is running; candidate code must not run earlier. The root filesystem
must be read only, and candidate/guard paths must have no mutable mounts.
The marker remains root-owned and readable but nonwritable by UID65532 (for
example0444 in a root-owned0755 control directory). Historical v1 binaries,
markers and package evidence remain immutable and do not parse this v2 format.

`os.Root` confines file traversal but is not process isolation. This program
does not enforce runtime duration, resource limits, current authority or effect
settlement and cannot sign a result. The host must retain a finite deadline,
stop/join the container, measure outcome and preserve uncertainty. Successful
exec does not mean functional success. The optional `GuardedPackager` now binds
one privately pinned guard and independently built candidate into a distinct
scratch image, verifies actual file bytes/modes in a never-started container,
and retains the guard/package/image identities separately. The legacy direct-
entry package is unchanged. Actual guarded execution, independent checks and
rollback acceptance remain outstanding. No working Butler process uses this command.
