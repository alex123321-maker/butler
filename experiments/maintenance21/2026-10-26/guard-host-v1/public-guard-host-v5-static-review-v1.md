# Guard-only host v5: independent static fix review

Scope is exactly the two held Go files with hashes89b1cb3d… andf2914eed…, their equal before/after source inventory, v5 script/log/exit and the preserved v4 sources. This is read-only inspection with own-cache receipts, not a new Go/PG/Docker/model run, formal freeze, hidden authorship or candidate/update admission.

## Fixes confirmed in source

- Initial `inspect(false)` now separately requires `capsuleNeverStarted`: explicit canonical unambiguous RestartCount0, state created and non-null explicitly zero StartedAt/FinishedAt. Missing/aliased fields or already used lifecycle fail. The owned cleanup helper still accepts exact created/exited objects without manufacturing a runtime identity. RestartPolicy is explicitly no with zero maximum retries. This addresses the v4 initial cold-state finding.
- `measure` and `close` acquire the same lifecycle mutex. Remeasurement first disposes its old control handle, then refuses a different prior PID/start tick/runtime or control directory inode rather than rebasing the original witness. It stores the witness only after the final currentness check. Future marker publication must still use the same lifecycle ownership and repeat its currentness checks; a saved witness is not a launch grant.
- Root, sys and selected proc read-only mount checks are strengthened; the `/control` tmpfs remains a separately measured root-owned0755 writable host surface. This is an explicit fixed point/flag allowlist, not a claim that all of proc is read only or a substitute for trusted daemon configuration.
- `close` no longer skips known-container kill/wait/identity/pids/delete when closing the control handle fails. It records a sticky private boolean and keeps the fixed overall error after successful physical cleanup; `cleanupConfirmed` separately reports physical container cleanup. A cached second close returns the original result without issuing another API. No raw path/daemon error string is returned to the public caller.

No real FD-close failure was demonstrated: parent explicitly reports that the relevant current Go1.27.2 root-close implementation returns nil, and the v5 log contains no injected/emulated FD-error PASS. The above error-branch improvement is structural source evidence only. It must not be presented as an actual tested FD-close failure or repair of the original v1-v3 failures.

## Two qualifications for the next marker/runtime implementation

1. `measureLocked` checks `closeAttempted` and context on entry, but not an already recorded `controlCloseFault`. After such a structural close fault it can theoretically return a later qualified measurement, although `close` will retain the fault. Before using measurement to authorize publication, a terminal host-handle fault must remain unqualified rather than be laundered by a later read. This is not an actual reproduction on the current nil-returning Go root-close implementation.
2. The five-second cleanup context is constructed after acquiring the lifecycle mutex. Thus it bounds cleanup APIs after lock acquisition, not total wall time including an in-flight measurement. Measurements use the caller's15s prepare bound. The actual race observed close winning, not a worst-case measurement-first cleanup latency guarantee. The future5s-runtime/cleanup supervisor must explicitly account for/cancel and join any in-flight measurement or otherwise implement its stated total bounds; do not silently advertise the current helper's API timer as a five-second wall bound.

These are finite lifecycle contract qualifications before candidate execution, not permission to add fault tests or weaken isolation. Current trusted-guard-only measurement still has no marker/authority/candidate I/O path.

## Actual retained v5 observations

The Linux/race log has five top-level PASS and six never-started subcase PASS, no FAIL/SKIP, package3.272s; sequential shell exit0 includes vet. The actual guard-kernel test is1.10s in this log, not the earlier v4's1.05s. It records PID18723, inner PID1, fixed image/guard/candidate hashes, six private namespace witness,128MiB/32PIDs/oneCPU/zero-swap, nofile64, one own-root FD and one exact read-only CLOEXEC guest cpu.max FD, fixed four-variable environment, root/control checks and absent marker. The second actual race reports PID18811, measure_qualified=false and cleanup_confirmed=true in1.14s. Known kill/wait/delete plus cached repeat close were observed. No candidate, admission or model was exercised.

The checker independently checked source equality and retained file hashes; it did not re-query the kernel or Docker daemon. Prior v1 cold-network/cleanup failure, v2/v3 descriptor failures and manual exact cleanup remain history. Neither v5 success nor this review proves functional checking, pending-task/approval survival, independent review signing, live deployment, trial or rollback.

## Publication scope

The accompanying JSON lists the exact two source files, v5 source inventories/script/log/exit and this own-cache review/receipt as the safe component publication scope. Inspection found no key, token, password, DSN, settings content, private case/golden or candidate input/output in those selected files. Log witnesses contain operational PID/container/hash/resource metadata; script paths and fixed image digests are nonsecret but intentionally public implementation detail. This is a selected-file content/provenance review, not a blanket approval to publish sibling .cache, private Docker configuration or runtime directories. Preserve published bytes, source pins, original v4 finding and prior failure history. The recorded guard/image hashes are evidence identities, not permission to execute them.
