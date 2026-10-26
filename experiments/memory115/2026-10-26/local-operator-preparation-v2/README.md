# Final local operator preparation: database-exit fence

This adds a missing distinction between operating-system process exit and a
PostgreSQL backend that may still finish an already submitted write. Before the
final encrypted credential snapshot, the operator requires all other client
backends in the native credential database to exit under a bounded deadline.
The preflight target pool is closed during native collection; recovery uses one
fresh connection. Failure to prove backend exit prohibits auth handback.

Six top-level race tests pass, including14 actual isolated PostgreSQL cases.
The added case opens a second backend with an uncommitted synthetic refresh:
the final read is denied, then observes the latest durable value only after
commit and backend exit. All synthetic tables/transactions are cleaned up.
Vet and static build pass. The unchanged original prepared-collector check and
actual child-cgroup containment were rerun with auth/native=0 on this exact ELF;
the empty cgroup was removed. Original source/preseal/five-binary/history pins,
finite cohort and deadline remain unchanged.

Actual authenticated execution, provider refresh/handback, native comparison,
legacy lab-main closure and release qualification are not proved by these gates.
Earlier preparation evidence remains immutable in the sibling v1 folder.