# Initiative admission after budget contention

Validated on isolated laba on 2026-10-12 (Asia/Tomsk), starting from `e805a9c`.
The affected subsystem is initiative promotion in the Orchestrator. The change
adds no dependency, migration, provider request, permission or deployment.

Promotion already refreshed authority after shared budget waits, but reused its
earlier source-hours decision, observation-age check and UTC-day attention row.
An actual PostgreSQL regression demonstrated three invalid ordinary admissions:
the source's active hour had closed, the observation had passed its 24-hour
cutoff, and midnight had moved admission into a day with exhausted attention.
Each original case created a task and ordered input instead of deferring or
rejecting. The failing baseline is retained separately.

The final admission branch now repeats the deterministic source-hours decision
and freshness check with the refreshed PostgreSQL time. Midnight defers with
`admission_day_changed`; a subsequent normal pass checks the new day's model
preflight and attention counter. A reconstructed promoter retries the saved
candidate and correctly sees the exhausted new-day counter. Denied cases retain
zero tasks, ordered inputs, model reservations and old-day attention increments.

The regression uses only its disposable schema to shadow `clock_timestamp()`.
An independent connection holds the real mandate budget row, and
`pg_blocking_pids` confirms the promotion transaction is waiting on that exact
connection before the schema clock advances. The production clock has no test
seam. The fixture's two-connection service pool retains normal admission APIs;
the observer connection supplies the boundary fault only.

| Proof | Retained artifact |
| --- | --- |
| Baseline: all three cases fail | `reports/red-20261011T193926Z/events.jsonl`, SHA256 `f3ab4f2026671df2243c703f4740086f84063c02f385e96e6591445ad61cfc31` |
| Final: 55 test/subtest names pass, zero failures/skips | `reports/green-20261011T194045Z/events.jsonl`, SHA256 `295524ef64cdf8e59a1574cfa300b6f5a6e521d79a874393275544271a77cfbd` |
| Explicit `go vet` passes without diagnostics | `reports/green-20261011T194045Z/vet.log`, SHA256 `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855` |
| Final source snapshot | `snapshots/20261012-024033-722` |

The proof root is `/srv/storage/butler/agents/initiative-time-20261012`.
Go1.27.2 ran `go test -race -json -p 1 -count=1` and `go vet` for
`./apps/orchestrator/internal/initiative` against actual PostgreSQL with private
schemas in a disposable `_test` database. Reports pin the production change
(`promotion.go`, SHA256 `607bc7ca9ffe3429e854e9326b773f0c965ebbfa7903433b7db5bad50fa8d8e2`)
and regression source (`promotion_time_integration_test.go`, SHA256
`07af5ee0a4974c0e475fb24d873a7b4259e76f87d72b9e03edd3ea585d20e396`).
The later README and evidence updates change documentation only.

Issue #15 stays open. This component proof complements the existing
[actual Broker prepared-correction fixture](../architecture/initiative-acceptance.md).
It does not establish live generative execution, automated task verification or
actual channel delivery. Antigravity history, sealed windows, owner credentials,
main source pointer, deployed services and user changes remain untouched.
