# Sealed actual V5 High release chain: NO_GO

The actual once-only chain is **NO_GO**. It admitted two requests, ordinals536–537,
from the independently accepted535-attempt quality/comparison baseline. The first
request completed with one authorized `converters.discover` tool packet. The
second request timed out without complete usage or model-version metadata. The
collector stopped before the remaining four scenarios; no child, adapter,
original result, project patch, notice or channel request exists.

PR74 remains a draft and issue33 remains open. The independently accepted
[V5 quality and four paired comparisons](scenario-cloud-v5-independent-audit-20261013.md)
remain unchanged. Neither those results nor this run's preparation/fault checks
turn the failed chain into acceptance.

## Exact source and preparation

Native code commit: `afa50b225b1341d2ff8a553188a69f6b6b2ac633`.
Source snapshot: `20261013-143513-149`,2119 files, TreeHash
`862fafb7a38a372a288a363fd73260b1efa851837080bdafecf90545d75cf26b`.
Complete source-manifest file SHA:
`ac9eb5ed617f28421582e605b2e260c5f6dd2eb78adc2c18d72539e877076277`.
The independently verified12 CRLF checkout transforms are declared by
`.gitattributes`; all actual snapshot bytes are retained separately.

| Prepared executable | SHA256 |
| --- | --- |
| Core/app race test binary | `383729b5493e2038f95fa23924feac880552531537d06f072c3eba8f77768b00` |
| Tool Broker | `36cfde3139d843673f9df60ba1848dcd58eee5a6fff5b16d66da463541cbfb83` |
| Converter runtime | `8fb12abff9627f13f8d306cf85e138ed0c741559fb5eb31280f8b30f74a00e4e` |

Preparation proof:
`/srv/storage/butler/agents/scenario-release-chain-v5-high-20261013/reports/chain-v5-high-prepare-20261013T073529Z-887300`.
It passed137 exact app/PostgreSQL names,67 independent checker names and4 Core
authorization names, all without failure/skip, with race and vet. The separately
retained [independent admission](../../deploy/acceptance/release-chain-v5-admission-20261013.json)
is9426 bytes, SHA
`ea80b5f64b5111397b123f9201667425107c4b0b3f5aca1f2fee2a039b8cb60e`.
Admission authorized only this fixed once-only scope, never overall readiness.

Actual immutable container images:

- Go: `sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61`.
- PostgreSQL: `sha256:ac08538c6f8b9904c33c8224c5e5706dbe760aca29db1d096972b4052c22a75d`.
- Converter base: `sha256:ce64758a109eb420d874a118f87920e625e12d3634e03b4a5573fd9f6e5d3507`.

## Actual request and product evidence

Sealed execution proof:
`/srv/storage/butler/agents/scenario-release-chain-v5-high-20261013/reports/release-chain-process-20261013T075645Z-968057`.
Native Docker ran2026-10-13T07:56:51.4227747Z–07:59:28.107949206Z, exit1,
OOMKilled=false, client memory limit1610612736 bytes. The global deadline
2026-10-13T08:56:45Z did not expire.

| Ordinal | Entry | Outcome | Input / output / total tokens | Elapsed |
| --- | --- | --- | --- | --- |
| 536 | normal_sent/start | completed tool packet | 7106 /155 /8462 | 13487ms |
| 537 | normal_sent/submit | timed_out, provider_timeout | NULL /NULL /NULL | 119977ms |

Ordinal536 reports `gemini-3.8-flash-n`;537 reports no model version. Neither
entry produced an assistant final. For537, no original provider terminal was
observed (`timed_out_count=0`); the timeout classification came from the deadline.
These are different facts. The recorded `failure_stage=stream` identifies the
collector's stream loop, not an HTTP/SSE internal stage. Authentication, HTTP
round trip and response-body failure cannot be distinguished retrospectively;
the internal cloud cause remains `UNKNOWN`. No original bytes or cost are
fabricated from a quote, previous response or unobserved model execution.

The safe owning PostgreSQL product confirms one authorized/confirmed discover
effect, one blocked root and zero live leases. It confirms zero delegated child,
converter version/check/trial/use, original result, prepared patch, owner notice
and channel request. `restart_sent`, `explicit_absence`, `lost_ack` and
`owner_stop` remain unattempted.

| Exact safe artifact | SHA256 |
| --- | --- |
| [Scope](../../deploy/acceptance/release-chain-v5-high-20261013.json) | `e1101e9f1b8b435e9f420aeea7d1507326cd0f0ee3bc80311c417792c899f709` |
| [Owning requests](../../deploy/acceptance/release-chain-v5-pg-ledger-20261013.json) | `778b8cd9e204ba074b98ac677d3867522728d25e3f7566473adff6f312160c05` |
| [Initial state](../../deploy/acceptance/release-chain-v5-pg-initial-20261013.json) | `31a5e87d90569a8a950e76206b1825c818e9ebddec9be2c54e71111da05f78a0` |
| [Product](../../deploy/acceptance/release-chain-v5-pg-product-20261013.json) | `82a3608fa9e86014140da6ad2e5bb769cfc1747cdab2c98027ccfdfb64557181` |
| [Actual Docker state](../../deploy/acceptance/release-chain-v5-runtime-state-20261013.json) | `4fb2e8ba23eaaf64be7b9967d265fddf2e0ccea07363bbfecfc38d5163716aca` |
| [Frozen independent checker](../../deploy/acceptance/release-chain-v5-final-audit-20261013.json) | `592d27ee34178a9b336eb0f9cce4784fac1ba214b22f05e1ad0b247a48f97c04` |
| [Independent failure correlation](../../deploy/acceptance/release-chain-v5-failure-correlation-20261013.json) | `fb8a430c327c4abec5984cad6fec6e79aa57d36866500dd588979722f282a424` |

The frozen checker retained both costs before returningNO_GO for actual Docker
exit1 under `runtime_or_global_deadline`. The separate5622-byte independent
correlation verified Scope↔PG rows/window, canonical initial/product hashes,
runtime identity, the absent product stages and the more precise failure facts.
It performed zero inference and mounted no credential import or private runtime
directory. Failed artifacts remain immutable.

## Accounting and final code checks

The two new admissions remain spent. Measured new cost is a subtotal of
7106 input/155 visible output/8462 total tokens, plus one unknown dimension in
each column. Cumulative history is now **537**:294598 known input/11827 known
output/572459 known total,510 known visible pairs and27 unknown pairs,47 unknown
total-token dimensions. Historical266's cause remains `UNKNOWN`. This is not
complete historical or complete new-chain measured cost.

Since the user's initial268-attempt boundary,269 new attempts have been admitted:
139175 known input/5884 known output/299387 known total, with537's expense unknown.
The user subsequently explicitly permitted `UNKNOWN` only for this closed537
failure, requiring complete measured expense in the next run. The pinned
[human criterion exception](../../deploy/acceptance/release-chain-v6-user-expense-exception-20261013.json)
records that trusted reply without claiming a signature. It preserves this
NO_GO and its NULLs; no future unknown attempt is covered by the exception.

On the same source, [all mandatory faults](../../deploy/acceptance/scenario-fault-v5-final-20261013.json)
passed321 named records,35 groups/eight scenarios, zero fail/skip. Safe report
SHA `c548bfe9d560f89ea824646ad861a524b43370b5273255a7f9ca1eb01f39d1d0`,
41952 bytes. Actual Chromium, PostgreSQL/Redis and dump/restore ran; the old sealed
physical Qwen LOAD/OOM witness was explicitly reused without new model loading.

Broad Linux Go verification passed2664 named tests/108 packages;1134 explicitly
gated integration cases skipped in that unit-only environment. Their required
current subsystem/fault counterparts ran separately without skips. JSON SHA
`04d8187e8ece527c33d8ca9a95661417df6ccb44f0879521fee33bad1186b1b8`;
additional `cmd/...` tests passed33 names/3 packages without skips, SHA
`8ae54a979b0f2574e8577e14c7b64747e575f0fc4a802af3db1073b50d594480`.
Both complete vet logs are empty. All ran with network-none and zero inference.

The earlier broad Windows result is retained as an unsuccessful platform check:
2627 named passes,58 named failures/12 failed packages,1111 skips, JSON SHA
`830d01f12869fa6299606b70672607f6539e4aa1ac3e9af030248ad2f637efc2`.
Directory fsync/private POSIX mode tests and canonical POSIX path validation
fail there; the same source's Linux suite passes. No test was weakened to convert
this Windows result intoPASS.

## Reproduction and next gate

Never Sync or execute the sealed V5 chain root or marker again. Offline audit
uses its frozen snapshot/binaries and only individually named safe0600 leaves,
as specified in the [checker README](../../scripts/dev/release-chain-v5-final-audit/README.md).
Root reproduced the2634-byte checker output exactly with
`scripts/dev/release-chain-v5-final-audit-reproduce.sh NEW_EXCLUSIVE_JSON_PATH`;
the successful reproduction remains reproduction ofNO_GO, with zero inference.
Any future scope needs a separately reviewed537-attempt history, code/protocol,
explicit request-stage observability and new independent admission. Within the
original80-admission chain allowance only78 remain;615 remains the absolute
ceiling. The failed two-entry window and its unknown cost cannot disappear,
receive a refund or becomePASS through a future report.

Only actual completion of all five scenarios with complete new measurements,
independent final correlation and issue-wide acceptance can permit PR readiness,
merge or issue closure. Cloud weights, remote equipment and invoice remain
unavailable; owner setup, model-free detection, finite-IR adapter development,
prepared/unapplied project and loopback recipients remain explicit boundaries.
