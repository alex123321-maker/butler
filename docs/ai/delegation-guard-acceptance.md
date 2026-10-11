# Delegation guard component acceptance

This records component evidence for issue16. Production Broker admission,
accepted capability storage and actual single-versus-parallel model experiments
remain pending. These tests do not prove a completed delegation release.

The actual laba source snapshot was `20261011-081915-765`. A disposable,
resource-bounded PostgreSQL17/pgvector container on its own internal network
applied all migrations; each fixture had its own schema, initialized private
deletion anchor and `MaxConns=2`. Go1.27.2 ran the selected tests with `-race`.
No shared Compose service, main database, model service or credential was used.

Private evidence:
`/srv/storage/butler/agents/durable-tasks/reports/delegation-20261011T011922Z-2769663/tests.jsonl`.
SHA256: `395a55c3077fe79544acfec4d0b0d977836f90fe28e247ce77fd697b3083b2be`.
All eleven top-level guard groups passed, including their three blocked-attempt
subtests; no selected test skipped or failed. The ordinary four-package lab
build/test also passed; without its separate memory test DSN, that check alone
does not prove PostgreSQL behavior.

To repeat the database tests, provide a private PostgreSQL17/pgvector database
whose name ends in `_test` through `BUTLER_TEST_MEMORY_POSTGRES_URL`, then run on
the lab source snapshot:

```sh
go test -json -race -run TestDelegationGuards -count=1 -timeout=120s ./apps/orchestrator/internal/app
```

The database setup/confinement follows `scripts/dev/delegation-lab.sh`; this
guard run changes only its selected package/filter to the command above.
Absent that DSN the integration tests explicitly skip, which is not acceptance.

| Boundary | Actual component evidence | Remaining product evidence |
| --- | --- | --- |
| Exact subset | Different tool/hash/resource/effect/credential, wildcard, expanded budget and deadline refused before child publication; actual assigned goal/mandate also refuses widening | Private Broker current claimed delegate action and accepted capability snapshot, not the fixture permission table |
| Original sources | Real Catalog grant binding; copied child grant cannot survive original parent grant revocation; Secret and unselected/foreign report ancestors refused | Integrated model tool and scheduler release under these checks |
| Report observations | Real completion transaction creates final/message/artifact/result/outbox; real action journal and branch budget reservations independently compared with report IDs/hashes/counts/states/units | Actual model and recipient effects; fixture dispatch outcome is a synthetic independent runtime observation |
| Verification | Forged caller result, missing uncertainty and mismatched evidence denied; actual version/hash-pinned owner verification accepts only the exact final and criteria | Production authenticated verification path and conflicting real executor reports |
| Partial and unknown | Completed partial final retains required uncertainty/blocker enums; unknown effect cannot become accepted through report prose | Ordinary blocked/failed attempts need a separate historical observation collection contract, as described below |
| Stop/deadline | Real Controller stop and Collect compete under the task-tree/session order; immutable committed effect survives; late collection/admission denied; DB-clock expiry after a metadata wait denied | Integrated all-runtime interruption and actual process fault matrix |
| Durable phase | Admission passes empty child identity; lifecycle passes exact persisted child identity and succeeds after the actual parent lease is released | Production accepted-snapshot persistence/recovery and actual parent process kill/restart |
| Task meaning | Accept/reject preserves independent child/parent semantic task state; accepting a report never completes a task | Owner UI must compose current task state with historical delegation state |

The mandatory local `DelegationPermissionReader` fixture reads a separate
PostgreSQL capability row. It proves guard transaction/identity/subset behavior,
not the authority of a real registry, old approval or model-selected metadata.
The actual Catalog, owner control, manager, session lease, action journal,
completion and budget stores are used. No HTTP/RPC is performed inside a guard
transaction and no second pooled connection is borrowed there.

Budget `used` means conservative durable reservations over all eight dimensions,
not measured tokens, billed cost or a success assertion. Reservations are counted
once by reservation identity even when shared scopes charge the same operation.
The fixture covers a single exact child execution; multiple model-produced
research reports, objective conflict resolution and quality/resource comparison
on one shared loaded model remain untested.

A partial assistant final saved by ordinary finalization leaves the task active
and has a completed immutable final; it can be collected with its limitations.
By contrast, `BlockRun(effect_unknown)` leaves a blocked task, and ordinary
`SaveRunOutcome(failed/timed_out)` blocks it while advancing its revision and
retaining an unverified attempt note. These paths do not manufacture a completed
final. The regression uses these actual methods: late Collect is denied,
historical result/journal evidence remains, and no final is invented for failed
attempts. Current Collect/Decide and execution/source lifecycle checks remain
active-only. A future read-only failed/blocked observation bridge must pin the
original attempt revision and current parent/source rights separately; widening
worker or dispatch authority is not a solution. Stop, cancellation and source
deletion continue denying late collection.

An immutable final pins the journal state/evidence it observed. If later
reconciliation changes that permanent effect, an old report fails comparison.
A fresh explicit observation/report workflow is needed; this component does not
rewrite the immutable final or silently upgrade the old report to success.
Likewise, after a collected result is stopped, `result_ready` remains historical
metadata while the task is cancelled; it is not permission for further work.

Issue16 and issue33 stay open until the production admission/runtime and model
experiment criteria pass. No new migration or production enabling switch is
included in this component.
