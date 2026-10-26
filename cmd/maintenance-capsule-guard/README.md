# Fixed private capsule launch barrier

Component for issue21, default absent from packaged artifacts and deployments.
This Linux PID1 command accepts no arguments, request stream, host paths or
environment configuration. It waits at most30seconds for one canonical finite
`/control/launch.json`, hashes the fixed regular `/candidate` ELF and replaces
itself with that exact path using only `GOMAXPROCS=2`. Other platforms deny exec.
An invalid, expired, oversized or noncanonical marker fails without a retry.

The marker contains version `butler.maintenance.capsule-launch.v1`, exact
`claim_sha256`, measured ELF `candidate_sha256` and `expires_at` no more than
30seconds in the future. It is a private host barrier, not an owner permission
or a signature. The future fixed executor must pin the independently built
guard and candidate, validate actual no-network/no-privilege/kernel limits
before atomically publishing the marker, and obtain current one-use controller
dispatch immediately before launch. Only the private host can publish it while
this guard is running; candidate code must not run earlier. The root filesystem
must be read only, and candidate/guard paths must have no mutable mounts.

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
