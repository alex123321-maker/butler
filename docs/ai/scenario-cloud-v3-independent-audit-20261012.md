# Independent actual V3 shadow audit — 2026-10-12

Status: **NO_GO**. The single actual window closed failed after 18 charged
observations (ordinals 249–266). There are 17 completed observations and one
failed observation; no tuning, holdout or single/multi comparison was reached.
This report preserves the original failure and does not authorize a retry,
refund, output repair, resampling or execution.

The independent auditor ran offline on a separate source snapshot at
`ea570f09831b5d1acb8f57551dd7838ec8f1c585`, with binary SHA256
`e65aa6a051b259ce7fd7660134e98add3a355625fb297a973cf0e852b07632f1`.
It read only the exact authorized report, SQL witness, hardware metadata and
named public golden artifacts. Network was disabled; it mounted no import,
profile, authentication store or protected review private key. Native calls and
actions by the auditor: **0**.

The actual collector remains the separately prepared source:

- Code: `b4f594899f8780a7f20b8a9983ef53f5221ff96d`.
- Source tree: `ffde6d8f42f6972020391a30b0284583ed17a65415172c2a682043a4b34d4b2e`.
- Runtime: `f022b2b9a6bc135915ba5015a75bcc8741fb950aa67ea92df736a66b53ed017f`.
- Hardware: `d3632dff9b14bb6cb710b9f909dfaa8fd7ea6ff313babd28db392ecaa7731c3a`.
- Historical fold: `9eae522a5dd3cd727d7e8a57f01cdd7958d08431d9d0d1ea484086f4225e01b2`.
- Original private report: `2e35fa1600744b3686024f5173c20afececb5b23629dc1499e8028084d14726f`.
- Independent read-only SQL witness: `01bbc2c1472998c967339f251a0df1bd2853568583bf14ba0f1dfb5985736843`.

The SQL witness matches all 18 ordered journal rows exactly, including nullable
parent contexts, timestamps, input/output hashes, outcomes and usage. Correlation
alone claims neither quality nor inference nor execution authorization. The
safe projections preserve the raw report hashes and prepared pins:

- [Window audit](../../deploy/acceptance/cloud-shadow-v3-high-20261012.json),
  exact SHA256 `2561b8486c40ee3d2d23d01709b5b06962f3407165c54133e04ab5bd29cd9a16`.
- [PG correlation](../../deploy/acceptance/cloud-shadow-v3-pg-correlation-20261012.json),
  exact SHA256 `805a168c42e8531a0965994720a776aa505fb7d917e1f908b7b66338dfa9b5e2`.

All 18 new input/output/total token triples and observed model versions are known:
9,799 / 377 / 21,381 tokens; summed attempt time 135,942 ms. All observed versions
are `gemini-3.8-flash-n`; the requested profile remains
`gemini-3.8-flash-high`. This proves the observed family and requested profile,
not server effort, weight bytes, remote model RAM or invoice delta. Client
process peak RSS is 89,296,896 bytes, shared client telemetry only.

The cumulative history is **266 attempts**, 240 known input/output pairs and
26 NULL pairs; known input/output subtotals are **154,345 / 5,914**. The known
total-token subtotal is **271,108**, with **46 unknown total-token attempts**
carried forward from prior windows. These are subtotals, not complete historical
cost. No old counter, deadline, signature, corpus or NO_GO verdict is changed.

The original-witness recheck finds 17 correct completed development cases and
one failed case. Development still lacks 78 eligible cases, tuning lacks 64 and
holdout lacks 76. This prefix has no witness, authority or tool-proposal
violations; the failed observation counts as one schema error. No full quality
threshold, semantic generalization or comparison claim follows from this prefix.
Empty comparison cost totals do not establish a completed comparison.

The separately read content-free last diagnostic records `transport_error`,
`output_encoding_contract`, `response_stream`, observed finish `STOP` and
`internal_transport_error`. Ordinal 266 used 526 / 9 / 807 tokens in 4,224 ms.
The rejected content is not retained in the original report. Its precise JSON
syntax, object shape, duplicate-key or encoding failure therefore remains
unavailable; Markdown, capacity, quota and timeout are not established causes.

Proof: `/srv/storage/butler/agents/scenario-cloud-observe/reports/actual-v3-audit-20261012T161728Z-2292275`.
The original actual proof and durable PostgreSQL data remain under
`/srv/storage/butler/agents/scenario-shadow-v3-high-20261012/reports/shadow-execute-20261012T160856Z-2264794`.
