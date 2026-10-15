# Owning source guard: bounded observation

Native temporal v3 (`d31d039843587974e26432b74adbe6c791ace71e`) remains a
NO_GO. One original completed, the second failed with durable
`memory_source_check_timeout`, ten were not executed. All six actual native
entries reached HTTP/response and have known usage:36,054full tokens
(29,873input,1720visible output,4461additional native tokens),52.547provider
seconds and36,261serialized request bytes. The closed ledger and all original
outcomes remain under `reports/native-v3` on the isolated issue88 server root.
The precise historical source-check phase was not instrumented and remains
unexplained; neither native transport failure nor a specific disk/load cause is
inferred from this diagnostic.

The content-free diagnostic (`6a46a187`, `reports/core-v20`) retained fixed phase
durations, pool statistics, query class/hash and PostgreSQL wait names only. The
no-auth actual Core/Broker facts/open/open/final strategy passed with instant and
5second-per-entry controlled streams. The slow stream executed89 current source
checks: one observer COMMIT took70ms, and independent PG wait samples observed
IO/WalSync. No pool starvation, lock failure or200ms timeout was reproduced.
The read-only replay of retained t02's current terminal state correctly denied
50checks at the lease phase, preserving exact business/spend hash. This does not
recreate the earlier live state or prove the earlier timeout's cause.

The owning token-only standalone `CheckRunSources` proof has no selected source
refs and no owner actor or auxiliary checker. Its exact helper chain is:

| Helper | Effects inside this observation |
|---|---|
| `AccessTx(nil)` | Manifest reads, current lease/run/task/session SELECTs; no selected refs or checker pins, therefore the source-binding INSERT branch is unreachable. |
| `CheckRestoreTx` / `checkManifestRows` | Private manifest Load and durable watermark/deletion SELECTs; no append, reconciliation or audit write. |
| `CheckTaskAuthorityTx` / `CheckAssignmentTx` | Delegation, task assignment, mandate, goal and purpose policy SELECTs with current row locks/time predicates; no budget use or authority mutation. |
| `delegation.CheckTaskTx` / `LoadTx` | SELECTs and migration057 `butler_task_check_delegation`: recursive SELECT/row locks and stale exception only; no session binding, admission or delegation spend. |
| `CheckDelegationSourcesTx` | Existing contract, parent authority, exact grants, original graph/source and deadline SELECTs; no body grant or writes. |
| `CanonicalSourcesTx` / `finalizeAccessTx` | Recursive SELECT and repeated current lease/task/authority checks. Empty roots do not bind any source. |
| `CheckRunSourcesTx` | Repeats restore/run/delegation and migration048 current transcript/source/watermark SELECTs; checks072 summary originals and075 temporal/source/run pins using SELECT/row locks. |

The token-only observation now ends with explicit bounded `Rollback(ctx)` and
propagates its error. The same rows are locked through the same final proof;
rollback releases them at the same boundary as the former commit. There is no
observer business mutation requiring a durable commit or WAL-sync wait. The
existing200ms bound is unchanged. Trusted compound owner/checker contexts retain
their previous commit semantics because their independently supplied composition
is outside this narrow read-only audit. Owning HTTP dispatch/publication callers
of `CheckRunSourcesTx` keep their existing full transactions and commits.

The regression hashes source/claim/head/temporal/run/task/deletion/budget state
before and after20current checks and records no mutating statement, no COMMIT and
20ROLLBACKs. A separately armed client-side300ms COMMIT delay fails the retained
compound-owner commit path at200ms while token-only observation succeeds. This
counterfactual control proves avoidance of unnecessary commit dependency; it is
not a historical disk-fault reproduction. Current temporal CAS still rejects
the next check and late SQL publication. Existing correction/deletion/authority/
delegation tests remain required, as does the complete12-case controlled flow.

Cumulative85+88 v1/v2/v3 conservative use is92model turns,98,254,848quoted tokens,
17,820,000ms,771,751,936input bytes,6external reads and25,165,824egress bytes,
with three retained NULL usages. This latest vector already includes the old
82/83/86-turn carries once. Known full native tokens are a228,628lower bound,
not total invoice cost. Other issue77 windows have separate ledgers and must be
combined once by the root report. Qualification remains false; no historical
window is reopened, refunded or selectively rerun.
