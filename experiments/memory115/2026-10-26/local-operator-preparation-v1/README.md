# Local #115 operator preparation, zero auth/native

The operator wrapper is separate from the original frozen collector. Its exact
static ELF, original preseal and finite gate evidence are recorded in receipt.json.
The original collector's existing prepared-whole-comparison check passed in
34.13 seconds. A real child entered the new kernel cgroup with CPU1, memory2GiB,
swap0 and pids256, exited, and its empty cgroup was removed. CPU/memory/pids
controllers were enabled for new child containment; no limit was applied to
an existing process. This occurred before any next #76 native admission.

Six top-level Go tests passed with race detection, including 13 actual isolated
PostgreSQL recovery/CAS cases and six strict encrypted-format cases. Vet and
static build passed. The dedicated memory115_operator_test database contains
only synthetic operator transactions, all rolled back. No real key/auth value,
provider discovery, native model, original journal or target native domain was
read or claimed by this preparation.

The implementation keeps original source/preseal/history/five ELF pins,
serializes the local auth owner, rejects live source windows/leases/claims,
uses a bounded private reference and finite child environment, waits for native
process exit and whole-cgroup emptiness before recovery, and CAS-protects the
source encrypted setting. Actual private-format validation reuses the existing
strict import parser without provider calls or authoritative storage. Failed
or ambiguous handback remains failure with private recovery material retained.

These gates do not prove authenticated execution, latest actual refresh
recovery, native comparison, legacy lab-main closure, full 57-stage acceptance,
release qualification or issue closure. Those original consumers and the
original deadline remain required. No native execute was run here.