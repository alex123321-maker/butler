# Closed stream-v5: NO_GO at the owning budget-day boundary

One admitted collection ran from producer
`b9d9ea02f3275811513188f4701596b61562487c`, collector SHA256
`7127dd676ce944ecca85ab2aa7605c5ea64e27e05d66ecfce11f8df53db816ab`.
Window `issue76-fixture-stream-20261025T231554` started23:15:54UTC and stopped
on23rd arm at00:00UTC, exit1. It was never resumed or replayed.

There are23/48 indexed arms,22 complete prefix arms and11 paired episode slots.
The45 indexed owner outcomes are42 completed,1 failed,1 local UNKNOWN and1
unexecuted local tail. There are44 actual owner inputs and42 owner finals;
2 automatic finals are separate. Of96 planned opportunities,52 were not
executed (51 unindexed and1 local tail). Completion is not semantic success.
The full comparison remains incomplete, quality/mapping stayed hidden, the
candidate is unpromoted and issue76 remains open.

The101 native admissions have100 measured usage observations,748105 known
provider-total tokens and1 unavailable usage. Entry100 returned an actual tool
proposal; entry101 submitted its result and ended with run_started followed by
transport_error(provider_timeout), no HTTP status/answer/tool/final,19851ms and
NULL usage. It was not a no-inference/auth-only probe and cannot be retried or
refunded. Entry102 is absent. The original405 rows remain identical; inclusive
history is6 closed windows,506 admissions,500 measured responses,3832577 known
tokens and6 unavailable usage observations.

The durable checkpoint states `execution_budget_deadline` at
`model_budget_reserved`. Entry101's reservation includes the actual owner_day
scope ending2026-10-26T00:00:00Z. The owning Controller takes the minimum scope
deadline and Orchestrator applies it to the call context. The observed19851ms
matches the remaining time to that boundary. `provider_timeout` is the outer
transport diagnostic; an upstream fault,20s provider setting or750ms authority
latency is not established by it. The independent audit preserves the exact
durable/source proof and the limit that serialized context.Cause was not stored.

The independent reader qualified22 sealed streams. The final unsealed stream
was reconstructed for diagnosis only:11254 events/5627 pairs,2 segments plus
tail,zero operation errors/drop/open spans. Its quiescence failed and it is not
qualified. The new lossless recorder did not hit the old65536-event bound.

All1719 raw files (44812506 bytes) are readonly;1692 case inventory files were
checked independently. Every archive entry was byte-verified against the full
raw inventory. Archive SHA256:
`a472c9a61c38387f8372a3d2d6c11099c7db3012d1c7731ce61c0972d10d9618`.
Raw inventory SHA256:
`0f4acd68d3998e467e95424635fd9e13c7b7a69cf077161b3d6ab8edf7c8c176`.
The separate private full control-database dump has5981627 bytes and SHA256
`2d8a2bb1ca95bae14b38fbb1e3a623166e525cad82882c1a3d3dab26e7253687`.
pg_restore list verification passed; it is not a restore test. The dump is not
published. All historical raw files, protocols and judgments remain unchanged.

A future study needs an independently frozen operational admission check that
the original four-hour window fits inside the actual UTC owner budget day,
followed by fresh independent controls. It may not extend a scope deadline,
change the current stop policy or reinterpret this failed collection as a pass.
