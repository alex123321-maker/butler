# Independent finite corrective entry review, memory-eval v2

Decision: **PASS_CORRECTIVE_BOUNDED_ENTRY_ONLY**, conditional on fresh prepared
pins and a successful zero-inference metadata/profile preflight. This is a
separate finite review for the new root
`/srv/storage/butler/agents/memory-eval-v2-native-20261015` and window ID
`memory-eval-v2-native-20261015`. It grants no re-entry to the old window and
certifies neither actual model quality, complete costs, external effects,
promotion, merge, deployment nor issue78 closure. A fresh post-run original
trace/ledger assessment remains required, including negative results.

Reviewer: `codex-independent-memory-review-20261014`, task
`/root/memory_independent_review`, independently appointed by the human user.
The reviewer authored neither candidate implementation nor golden data. This
review read code, safe SQL/Docker metadata, synthetic artifacts and test proof;
it accessed no private credential/import/signing material and made no Infer
call. Independence is between roles on a shared host.

## Corrected bytes and proof

Reviewed clean commit: `4e9d8694b014265234438941ea28ccfb50e38111`.
The functional correction is commit
`5ad6e0368265728f4fac19afc0bcd80bce494248`; the final amendment adds only an
early spent-root denial to the script. Compared with previously reviewed
`1aa037fc91e41a7e3279596760bf76a3cfe5fee9`, executable changes are limited to
the runtime network/window contract, its test, the actual collector window ID,
audit CLI window correlation, metadata-only preflight and operational script.
README and the independent aborted-window assessment were also added/updated.
Native instructions/schema, golden/criteria, source authority, model event
collection, original retention and expense correlation are unchanged.

| Corrected executable object | SHA256 |
| --- | --- |
| apps/orchestrator/internal/app/memory_eval_native_test.go | `ec01c910acab8c3a256b8533a143fdd8f0a05175fa7268ac4d147a549f5d4931` |
| apps/orchestrator/internal/app/memory_eval_preflight_test.go | `6ee924ad2b60b3a82ba263bd9870ab522b02148a5a99a1e25d5bff8c42a263bd` |
| internal/acceptance/memory_runtime_pins.go | `344bcf85ff32ae58bc54a4617d1e2f53b462f5bb1422b86f81deb0cb5d89e494` |
| internal/acceptance/memory_runtime_pins_test.go | `0b8f91b98857c31ca576a93a2552204d4ab36998d1be22dbbd770f872dd6df91` |
| cmd/memory-eval/main.go | `7dfa9ba903078ad2d38e2217f9d21107692f9f82590266e43f89644e83743dca` |
| scripts/dev/memory-eval-model-lab.sh | `baf1593e966ad2b6dcf8742388732512d65558ae2402c52f0fb2d0e13b9ebdb5` |

All other executable bytes retain the pins in the prior independent readiness
declaration, SHA
`7f55aabce61f9625c9df3b3a6a523ebf5e024493f0834edac372c42f8493b654`.
That original one-window admission was spent; its surviving code findings do
not authorize reopening it.

The reviewer independently walked and rehashed 2,557 files of exact tested
snapshot
`/srv/storage/butler/agents/memory-eval-78-20261014/snapshots/20261015-000351-891`,
reproducing TreeHash
`99b9d2a228aa2233ad9e7279aaa08e325e43a59b53eaf1ac29e352ea51628281`.
That snapshot predates appending the independent aborted assessment to the
source tree, and predates the final one-line spent-root guard. It is claimed
only for its exact tested executable bytes, not as the future whole-tree hash.

The reviewer read proof directory
`reports/memory78-fixture-20261014T170437Z-3023431` directly. Narrow race proof
SHA `5f7943e4d4d73dcc6dddb38715b7883f161f1f9c65208bc06bdd2d1800952c13`
contains17 named passes, zero failures/skips, including missing/substituted/
duplicate network rejection, original64-observation fake correlation, current
authority and retained failure cases. CLI race proof
`cli-race.jsonl`, SHA
`3ae3f65aef4e9e66b005fedb742c025f2333082718134510460927e3940a4319`,
contains3 named passes and zero failures/skips. Runner/diagnostic files are
empty; the implementing root reports vet exit0. These are fake/component
checks, not actual inference. Post-run read-only source manifest SHA
`d26e4e3d9b273a269234df6a35e0cf207bd35934a6b6f627740c08d31389c30f`
honestly names the tested snapshot and is not pre-run instrumentation.

The final one-line guard was inspected separately. The reviewer independently
hashed its exact script and ran bash-n (exit0, empty diagnostics) on snapshot
`/srv/storage/butler/agents/memory-eval-78-20261014/snapshots/20261015-000856-349`.
The author reports the rejection probe returned the fixed spent-window denial
before the metadata branch. The reviewer independently confirmed the old
entry marker still exists, no local-metadata-preflight.log was created there,
and its ledger still has SHA
`6cb3a3ae66e3c198cf32e862a3428fe1fa194ace391865b9d4931b7c68289540`.

## Operational correction

The actual runner now joins the fresh experiment network, `butler-lab_backend`
and `butler-lab_local-inference`. ValidateMemoryExecution requires all three
exact names, rejects duplicates/substitutions/missing/extra networks, and binds
the experiment network to the syntax-checked pinned WindowID. Prepare derives
WindowID from the new root basename. The actual plan uses that pin; audit CLI
requires it to equal the original frozen plan's ID. This fixes both the proven
missing worker network and the old hardcoded plan-window identity.

Before the new entry marker, a compiled metadata-only test uses the production
AuthenticatedBackend.Describe on local-inference and Adapter.Verify against
the exact frozen embedding profile/space. Its wrapper Infer method always
returns a fixed error without invoking the underlying backend. Describe does
not admit or dispatch a model job. The test has a10s context and20s process
timeout, logs only fixed messages or safe gRPC status code, mounts only the
compiled runtime and private local-token reference read-only, and has neither
native import nor database mount. Resources remain UID10001, read-only root,
no capabilities,2CPU/1536MiB/128PIDs. Credential resolution stays inside the
existing trusted production client; raw bytes never enter model/tool arguments
or diagnostic output. The reviewer does not read that private file.

An early entered-marker denial now precedes all metadata/auth preflight and
log writes on execute. The atomic mkdir remains the final one-use entry gate.
Thus a spent-root retry cannot overwrite its preflight log or authenticate
metadata before rejection. Fresh preparation, output whitelists, isolated PG,
source/binary/data/criteria guards, exact execution proof, hard stop, finite
event/report retention and recoverable failure artifacts remain in force.

## Finite admission and required fresh pins

This approval is for one fresh public synthetic24-case/three-arm comparison,
64 currently eligible native samples (metadata ceiling66), requested
gemini-3.8-flash-high with required observed gemini-3.8-flash-n, fixed arm/case
order, no fallback/retry/repair/resampling. Shared new auxiliary+native
inference admissions stay at most280 and4,000,000 conservative units, over
3h,2CPU/1536MiB/128PIDs,4,096 safe events per entry and128MiB reports. Metadata
preflight is bounded non-inference work; its existence does not make overhead
or subscription invoice known. The old sealed window admitted zero inference,
so no model samples from it are repeated or used for tuning. Its source-write
and failed-background expense prefix must remain preserved separately.

Golden raw remains
`5571d474ec54a8a29765e055cc0475e73653d679d5f250fa074dcea1d9d6b1a5`,
semantic suite
`7c74abe1234987883bd252126a8cade19b51ffb4e89d1b0df6bbc25447be413d`
and criteria
`9be3b4b51b5832bffc6349915cb61db4761e4b54ee95097b43677a28b9997e68`.
Independent golden review document SHA remains
`39ff6b07131243044f7e5bc52433ac98489e5e72f2182614dd01a4bccbb3d153`;
its machine-readable declaration SHA remains
`b533980ec505bc0797092d86945dd68032d39fb04a71efb9bb054e9adbcb9337`.

After committing this review document, prepare must bind the new exact source
tree/release, newly compiled collector/audit CLI, hardware/config, image IDs,
WindowID and unchanged external golden approval/public key. The reviewer must
independently compare those safe prepared pins before native entry. Successful
actual metadata/profile preflight must be retained before the one-use marker;
failure grants no inference. No as-yet-unbuilt binary hash is invented here.
Unexpected executable/config/data/schema changes require review. Any failed
or spent new window stays sealed; later entry needs separate finite review.

The original v1 NO_GO, rejected v2 proposals, initial runner NO_GO, spent prior
readiness and independent aborted-window assessment remain preserved. Finite
lexical/synthetic/nonblind controls, absent production memory stores, unknown
local tokenizer/invoice/host/PG costs and null broader task effects retain the
same limitations. Actual QA or latency failures are valid negative evidence;
they must not trigger control-driven tuning or additional model calls. This
approval supports collecting originals and a subsequent independent audit,
not a claim that candidate promotion or full quality acceptance has passed.
