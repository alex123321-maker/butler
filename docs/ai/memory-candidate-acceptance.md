# Candidate memory publication —2026-10-11

Related #17. A catalog source or extraction-deduplication row wait can outlive a
run lease or an independent auxiliary claim. Catalog now repeats time-sensitive
authority in the same transaction after these waits. It denies an expired
worker even when an exact retry's candidate already exists. Run state, current
task/assignment and delegated deadlines remain authoritative. Source/authority
locks remain held; pool2 needs no third connection.

Actual PostgreSQL regressions observe `pg_stat_activity` to establish that the
worker is blocked, wait for PostgreSQL's clock to cross the original deadline,
then release the blocker. They cover first publication, exact retry, source-body
access, and an independent `extract` job. Expired calls publish/return nothing;
fresh session ownership can publish or recover the original neutral candidate.
Confidence1 is still a candidate. Existing concurrent owner head-CAS tests pass;
these publications cannot overwrite an accepted preference or create a mandate.

Final source snapshot:
`/srv/storage/butler/agents/memory-candidate-admission/snapshots/20261011-174349-893`.
Proof:
`/srv/storage/butler/agents/memory-candidate-admission/reports/memory-candidate-20261011T104359Z-696625/tests.jsonl`.
SHA256 `662357cbf52bdbbf669b025fce383089bac60b70661af980f280feef5e4f5634`.
47 distinct test/subtest names passed across catalog and jobs, race detector and
vet passed. One separate opt-in `TestActualPostgresDumpRestoreDeletionIntegration`
was skipped; no dump/restore run is claimed by this proof. Zero model calls.
The disposable private PG17 database/network were removed by the harness.

Earlier passing narrow proof and pre-delegation-finalizer full proof are retained
at `memory-candidate-20261011T104019Z-684316` and
`memory-candidate-20261011T104205Z-690101`. Their hashes are
`7e06622498d4ff505b41d59da6c64fac1cc8d3c3b8704aa582a52029da3aafe5`
and `e87023f598218a8762aa70467866a239a5376654becbd50945f0aafcf74fc909`.

This repair does not add a production agent proposal tool, enable the disabled
legacy extraction worker, prove local-model semantic quality, or close #17's
remaining workspace/shared-memory composition gates. Main laba deployment and
its optional feature flags were unchanged.
