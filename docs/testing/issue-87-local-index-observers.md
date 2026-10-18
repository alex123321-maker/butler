# Local index observer transaction diagnosis

The generic historical `e04/no_ltm` rejection remains unexplained. Its retained
ledger SHA256 is
`ef3b23faaa5668a74a0aefc217c127a957aec71c3c491cd805b0793b260a8771`.
The job retained one local reservation of 6144 quoted tokens, 60000 milliseconds
and 124 input bytes. The 6144 figure is `2048 * (two items + 1)`, not the hybrid
retrieval byte limit. The reservation preceded the failed job receipt by 3.612
seconds. The retained PostgreSQL log contains no ERROR, statement timeout, user
cancellation or deadlock; neither it nor the collector retained a failing source
phase. These observations cannot identify a provider, source, database or guard
cause. The closed native window, its failure and its spending were not changed.

## Controlled reproduction

`memory-index-observer-lab.sh` uses an isolated real PostgreSQL 17/pgvector database,
the production indexing Runtime, ordinary job admission and a delayed test local
backend. PostgreSQL has one CPU and 512 MiB; the cached Go 1.27.2 race runner has
one CPU and 2 GiB. There are no cloud/local inference-service requests, credentials,
auth rotation or main database access.

The immutable baseline source `93f4d525` passed seven tests/subtests, zero failures
and zero skips, plus race and vet. A normal two-call slow index observed sixteen
bounded source proofs, sixteen COMMITs, zero rollbacks and twenty chunk checks.
Both reservations remained charged and all five vectors were persisted.

A separate deterministic client-side 300 ms COMMIT delay cancelled the first
local call under the unchanged 200 ms observer deadline. Both independent source
observers reached COMMIT. The failed job retained its one reservation and no
vector. This proves an avoidable observer-transaction failure under the injected
condition. It does not reproduce historical server WAL latency or establish the
cause of the earlier native rejection.

## Narrow correction and verification

Source `c2883a29` changes only the two private periodic local source/chunk
observation boundaries to bounded rollback. They still run the complete catalog
graph/body, exact source/chunk revision/hash and current claim proofs. The private
job checker cannot bind live-run pins or execute a caller-supplied access checker.
The audited helper chain only SELECTs/checks/locks; it has no business mutation to
publish. Full allowance admission, ordinary source reads, result acceptance and
publication keep their commits. General `Catalog.CheckObjects` remains unchanged.

The corrected cohort passed seven tests/subtests, zero failures and zero skips,
plus race and vet. The slow index retained sixteen proofs and twenty chunk checks,
now with zero COMMITs and sixteen rollbacks. A second test kept the 300 ms fault
armed on two direct calls to the unchanged general full-proof API: those controls
timed out, while twenty production periodic proofs rolled back and both actual
test-backend calls succeeded. Source data remained byte-identical and both
reservations remained charged.

Four separate in-flight changes—source correction, source revocation, claim
takeover and chunk revision—still cancelled the call. A deliberately uncooperative
backend returned valid late vectors in each case; none persisted or activated,
and the original reservation stayed charged. The 250 ms tick, 200 ms proof
deadline, full currentness proof and finite job policy were not relaxed.

Server evidence is retained at
`/srv/storage/butler/agents/issue87-observer-20261018/reports/`:

| Window | Source manifest SHA256 | Test JSONL SHA256 |
| --- | --- | --- |
| baseline-v2 | 9483505757ab9ffb0ecd812b7d0457bfb25337d49344c404ef672575a256b1e8 | f1ea7a13e42a8ee0198768a9cc78ac93790a0e1cc28ce321632043aca2ffa732 |
| fixed-v1 | b9fc50ab194d729174e2b3af8cce2db0fb425cbb3bf14ee8f6536eaf6c37208f | f23347e485a3acc59cd9d46bbe4d981d98f786ee91d48f01a2bb5b2c7737b24b |

Both vet logs are empty (SHA256
`e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`).
The earlier `baseline-v1` setup stopped before PostgreSQL or tests because its
requested network overlapped a retained network. That setup is retained; v2 used
a separately declared free subnet. No historical admission or ledger was reset.

These controls qualify the transaction correction only. They do not qualify the
adaptive memory consumer, native answer quality or issue 77 acceptance. A fresh
complete prospective native comparison remains necessary.
