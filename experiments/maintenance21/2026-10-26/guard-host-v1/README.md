# Actual trusted-guard preparation: bounded component evidence

This component publishes no launch marker and cannot execute a candidate or
authorize an update. The guard image is5a7e4124…21062165, guard ELFc62270e6…6add1c9,
and cold candidate ELFa8ce3e6c…9800296. Exact full identities appear in logs/pins.
Only the trusted guard runs; main Butler, lab and production are untouched.

Final v6 Linux Go1.27.2/race checks pass7top-level tests and6subcases in8.134s,
with0failures/0skips and vet0. Two actual guard instances have observed kernel
identity/isolation evidence and complete known-object kill/wait/delete. The
normal witness records PID19134,128MiB,32PIDs,oneCPU,64open-files,effective zero
swap, private namespaces, root/sys/proc read-only bounds, loopback-only network,
declared environment and FD identities. No marker/candidate/model/claim exists.
The actual concurrent close won; the simultaneous measurement stayed unqualified.

Cold ownership is not qualified runtime: never-started preparation needs explicit
created/zero-times/restart0, while cleanup can delete a known created/exited
object. The unchanged running inspector still refuses missing physical network
identity. Lifecycle operations serialize; PID/start/runtime/control identity
cannot be silently rebased. Terminal faults and close requests cannot qualify a
later measurement. Close's5scontext includes lock waiting; one no-container case
observes5.00s and keeps UNKNOWN without retry. No hidden cases are involved.

History is retained without reclassification:

- Manual kernel v1 failed its two-variable environment check; its extra values
  were not captured. V2 explicitly declared HOME/HOSTNAME as well and passed.
- Go v1 failed before start because the full runtime inspector rejected a cold
  container's empty network ID; automatic cleanup also failed. Exact ownership,
  created/PID0/never-started metadata was checked before manual deletion.
- Go v2/v3 rejected a descriptor. The later metadata diagnostic observed FD3 as
  an ordinary file. Pinned Go runtime source shows its lifetime cpu.max FD opened
  O_RDONLY/O_CLOEXEC; v4 verifies only that exact guest kernel-file identity.
  The root-path hypothesis was not established: the later successful witness
  reports no root-path projection difference.
- V4's real component PASS did not prove complete admission safety. Independent
  review found cold prepare also allowed exited objects and identified missing
  lifecycle serialization/error handling. V5 fixes these; v6 retains terminal
  faults and includes mutex wait in the close budget. Failed versions, diagnostic
  logs and exact source snapshots remain separate.

No actual os.Root.Close fault was reproduced: this Go1.27/Linux implementation
returns nil. The synthetic sticky-state regression is not such a reproduction.
The generic error path now still attempts bounded termination and cannot erase
its first failure. Missing cleanup proof remains unqualified.

This evidence does not establish an independent check-plan admission, live signed
maintenance dispatch, candidate I/O, functional original task, active approval
continuation, trial/promotion/rollback or updater readiness. Those remain required
before whole issue21 closure. The public files contain synthetic component code,
operational IDs/hashes and fixed resource metadata; no auth, key, PG dump, private
hidden controls or grading answers are included.
