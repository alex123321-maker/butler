# Receipt preservation and final-turn guards — 2026-10-12

The existing [High report](../../deploy/delegation/native-scenario-high-20261012.json)
remains immutable: SHA256
`f4649a539eed17bf01db034209a757bbfdb268759b2446c852f2260009bba0d2`.
Its parent receipt-projection gate failed after two real admissions and two correct
child extractions. The offending original parent text/field was not exported.
This work does not assign an exact cause to that historical failure or relax its
objective checker. Issues16/33 native acceptance remain separate.

## Verified production path and an independent guard repair

Actual private Broker, HTTP admission, ordinary core/input workers and PostgreSQL
preserve both committed receipt IDs through live continuation and durable
transcript. The normalized live envelope is `{status,result,error?}`; the child
task ID is `result.task_id`. A queued receipt contains its actual input ID, while
a waiting receipt has an empty input ID. Neither `completed` dispatch status nor
receipt publication proves completed child work or owner-verified task success.

Tool-phase text deltas do not accumulate into the final body. A positive
two-admission test with intermediate commentary already passed before any repair.
A synthetic three-request native Go High test additionally checks both original
call IDs and opaque signatures, exact `functionResponse.response.result` receipts,
waiting input ID and a separate terminal JSON final. Its HTTP backend and auth
are fixtures; it proves serialization without Google, credentials or model usage.
Recovery keeps both saved receipt bodies as private ToolOutput DATA, under the
extra wrapper `data.result.result.task_id`, without replaying old native calls or
discarding earlier receipt identity.

The ordinary core did have an independent boundary defect when a transport
emitted an apparent final before another tool proposal. A later incomplete turn
could reuse that stale final, and an earlier partial finish reason could classify
a fresh complete answer as partial. Deliberately scripted model events reproduced
both failures through the actual Broker/core/PG path. The High adapter does not
emit such a pre-tool final, so this is not evidence of the old native cause.

Core now clears the candidate answer and classification when a tool is proposed.
Each new final supplies its own finish reason. An incomplete or nil continuation
cannot save the earlier answer; independently confirmed admissions/results and
spent reservations remain durable. The missing-final PG test retains two actual
confirmed admission effects and creates no final row. Exact model final content,
receipt structure, Broker authorization, source guards and native serialization
are unchanged. Production does not construct a replacement parent answer.

## Zero-inference laba checks

Only `/srv/storage/butler/agents/delegation-receipt-path` was used, from base `739d55c`.
No shared deployment/current pointer, working account/profile/credential material,
native runner/window, signed corpus or carried historical report was changed.
The private PG/network/processes are disposable; reports and PG files remain.
All model events are scripted or served by a synthetic HTTP fixture. This test
workflow makes 0 real model calls and cannot prove new model quality/acceptance.

Sync the checkout to a new isolated agent root with `scripts/dev/lab.ps1`, then:

```sh
bash <root>/current/scripts/dev/delegation-receipts-lab.sh <root> narrow
bash <root>/current/scripts/dev/delegation-receipts-lab.sh <root> full
```

Narrow checks run final-turn/recovery units, the actual Broker/core/PG cases,
synthetic High protocol and affected vet. Full checks additionally run core and
transport suites plus existing actual Broker admission regressions. Both use the
race detector and repo-local caches. Shell syntax passed on laba.

| Evidence | Snapshot | Immutable events SHA256 |
| --- | --- | --- |
| Controlled pre-fix Broker/core failures; healthy narration case passed |20261012-110238-465|`09f0617d03350433f4e5534fe5cf4b4c05cd55f405b4f74f532f8522bff8db90`|
| Final-code narrow: 11 passed names, 0 failures/skips |20261012-111240-994|`9812873d3578df129d0384c2d4defc5b3539008f24f2c52be4b122afb2f61160`|
| Final-code expanded: 203 passed names, 105 top-level, 0 failures, 2 skips |20261012-111240-994|`677b1c342fb79f08731d924ef1a9e9c807aaf9d67a783860b9d3f2cc4329191a`|

Events are under this agent root's `reports/delegation-receipts-.../tests.jsonl`.
The expanded skips are `TestExecuteIntegrationPersistsRunAndTranscript` and
`TestDurableTaskExecutionIntegration`, which require their separate PostgreSQL/
Redis environment. They are not passed evidence; the selected actual PG/core/
Broker receipt cases all executed with 0 skips. This verification does not replace
the owner's separately bounded native single/parallel/recovery/stop acceptance.
