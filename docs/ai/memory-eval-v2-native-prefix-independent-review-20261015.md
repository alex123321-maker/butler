# Independent actual prefix review, memory-eval v2

Decisions: **PASS_RETAINED_PREFIX_CORRELATION_ONLY** and
**NO_GO_COMPLETE_MODEL_COMPARISON**. The corrected one-use window retained
53 successful auxiliary embedding RPCs and one failed native admission. It
produced zero QA results and cannot establish a complete comparison, model
quality, complete expense, candidate promotion or issue78 acceptance. The
native failure's underlying cause is **UNKNOWN_FROM_RETAINED_ORIGINALS**;
neither an external transient fault nor a schema/collector bug is proved.
This review grants no new Infer call or reopening of either spent window.

Reviewer: `codex-independent-memory-review-20261014`, independently appointed
task `/root/memory_independent_review`. The reviewer authored no candidate or
data, made no authentication/Infer call, and did not read credential imports,
settings/key material or provider headers. Read-only SQL, safe Docker metadata
and individually mounted named synthetic-original files supplied the evidence.
All original experiment files, SQL rows and containers were left unchanged.

## Exact spent window

Root: `/srv/storage/butler/agents/memory-eval-v2-native-20261015`.
Release: `90ed0f0b081dedff7f8d951db6457ee97bc2eb5f`.
Source snapshot:
`memory-eval-78-20261014/snapshots/20261015-001230-992`.
Its pre-entry independently reproduced source SHA is
`2a6a6c00f4d70c3e027a2c837f441771eba4f606aa0ec339e5cbef86048deeb3`.
Collector binary SHA:
`0eff2b2a02b63cab3d47d1fe28f5992f5232e23308ce76a40cb7bf458ed76b39`.
Audit CLI SHA:
`9372749995d07a952b8a0c7b848c7b34c156145dc4b510b0f16f490d33e2796a`.
Hardware SHA:
`695f43959492d165436b41a36f745319780cf53695c05222529f3d547963a138`.
The separately reviewed resource, source, binary, three-network and WindowID
pins passed before entry. The compiled metadata/profile preflight succeeded
without inference; its retained log SHA is
`315a3d9c96134ed1283fcb8572b1cfb8578d986f2b15bd5f37b04cb2afcdb5f0`.

Docker final state is exited/exit1/OOMKilled=false, started
2026-10-14T17:16:57.761468Z and finished17:17:19.385382242Z.
PostgreSQL window is closed=true, calls=54, reserved_units=568935; its plan hash
is `f92df80a4b1dca501c3094445d92b24946e3e015dab46dc873d9b116c1c8917d`.
There are54 distinct owning reservations,53 auxiliary IPC receipts, one native
model attempt, three native guard receipts and zero QA/result rows. Reservation
policy charges sum to54 model turns and339982 conservative tokens. That policy
subtotal and the separate stricter wire/framing window counter568935 are
different accounting views; neither is measured tokenizer usage or invoice.
The stricter original window counter must not be refunded or replaced by the
smaller policy subtotal in any future aggregate accounting.

The original v2 golden semantic suite/criteria digests are unchanged:
`7c74abe1234987883bd252126a8cade19b51ffb4e89d1b0df6bbc25447be413d`
and `9be3b4b51b5832bffc6349915cb61db4761e4b54ee95097b43677a28b9997e68`.
No QA answer observations exist from either spent window for model tuning.

## Independent correlation of all53 auxiliary RPCs

The reviewer read every original aux file, extracted its original Request and
Response serializations and independently compacted JSON without re-encoding
floating-point values. Both SHA hashes match the exact IPC ledger. Request ID,
profile and revision match their response; all outcomes are success. Each
receipt has one distinct owning reservation with the approved case owner and
one model turn, and one attributed embedding expense. No reservation is reused.
All53 local tokenizer totals remain null; local-no-billing basis does not prove
zero host/energy/full process cost.

There are31 document embedding RPCs and22 query embedding RPCs. The former
contain69 exact owned catalog chunk texts/titles; the latter contain the22
reviewed questions. All request fields match the pinned profile, task kind and
dimensions768. All91 returned vectors have the requested IDs, finite768-value
shape and matching pinned model revision. All53 responses report device=gpu.
This proves retained protocol/identity correlation, not embedding quality or
independently measured physical GPU expense.

All55 active source bodies/events correspond to their reviewed goldens; two
deleted sources remain deleted. All69 derived chunk bodies are anchored in
those approved originals. Every source alias/revision/access/content hash,
owner and selected hybrid chunk binding was checked. All22 capture prompts,
ordered unique selected IDs and SHA of original compacted bundle bytes match
the original freeze. The reviewer reconstructed its typed outer serialization
and reproduced frozen digest
`93dc4e25a835caeca81c7098aa5f23144ee21489eca3b2729da33aa0d11e1e9f`.
The before-native and final frozen files are byte-identical. All72 original
case/arm identities and allowed question/current-task projections match the
approved cases, including the secret-source withholding and original skip
reasons. No golden answers, rubric, split or citation rules enter the actual
native model projection.

Aux-file aggregate SHA is
`5846f09322506b9504255489c486e64d6bd5a571ca921ca62042a35d482a6632`;
capture aliases/bundles/search aggregate SHA is
`02a1083f5e6492dbe8edb223a88a1273bf741619b4989483105edc8ead7e61be`.
Each aggregate hashes sorted `basename + NUL + individual-file-SHA + newline`
records; the auxiliary names are their request IDs, and capture names are the
bindings/bundle/search basename without .json. This separately reimplemented
prefix check does not replace the complete runner auditor's failure.

## Native admitted failure and diagnosis limits

The only native case/arm is
`memory:b19116fc74b63e9cae77cc8658d2cb87/no_ltm`.
Operation `operation_bbac90d37c366701b9977c41ad4ae9cf21c1d53def2a816a`
is linked to run `memory-run-9894c6c8e81ee972b0d87f36f1db3abb` and reservation
`a4ccc69da59bb84850511b2eaecb6890191de7519188f126ed6f6e6a6a745e51`.
Admission is antigravity/gemini-3.8-flash-high, input allowance12302 and output
allowance32768. The reviewer independently reproduced original input digest
`38a1ac84eb86db9b315a79d15559c5ee5a9749260ef30cb209adabab566f13d6`,
verified exact instructions/projection, and compared the actual native schema
to the previously reviewed producer. No input, schema or prompt substitution
is evidenced. Constructor/schema agreement does not prove remote API acceptance.

Two dispatch guard receipts and one final guard bind this operation, run,
case/arm, current task/constraints revision1, empty no_ltm source list and the
live owning lease. Check times precede lease expiry and failed-run completion.
The second dispatch check is the reviewed exact HTTP-dispatch boundary. It
establishes that the call passed that guard, not that the remote server accepted
or billed inference. The authoritative run finished failed.

Elapsed native/pipeline observation is2718ms. Retained safe events are only
run_started and transport_error, with no model version, usage or AssistantFinal.
The raw final is zero bytes. The attempt is transport_error, the measurement is
failed/transport_lifecycle_or_metadata_incomplete, generation expense is
unknown/failed/calls1, and input/output/total tokens and invoice are null.
Unknown broader task success remains null; no action effect is asserted.

All129 expense records match measurements and original ledger by ID, including
the charged failed native prefix. The retained original-write record has
measured/failed,7ms,673bytes and zero inference calls. Its outcome inherits the
native failure even though the zero-byte final and673-byte safe event file were
successfully retained; it is not evidence of a filesystem retention failure.
Unused-stage records do not become extra embedding calls.

TransportError.Code/Details/HTTP status were not included in the safe event
export. Provider headers, body and private authentication material were not
retained for reviewer use. Therefore HTTP401/400/429/5xx, network failure,
quota/tier rejection, schema incompatibility or remote transient failure cannot
be selected as the proven cause. Later static code analysis or a control-plane
health signal cannot retroactively supply this missing actual error receipt.
Future diagnostics may retain an allowlisted typed code/status prospectively;
raw provider error prose or credentials must remain excluded.

## Original artifact pins and acceptance boundary

| Safe original | SHA256 |
| --- | --- |
| ledger.json | `264c928b0c245961e588b2a1d7efc267d205f21e7f68e2e3a4b9ab3601cb665b` |
| measurements.json | `8ec7959fe11d2adf98bcdd3eeb58c00105bbc72d1ae6a5972a2aead7d9ea2959` |
| frozen-before-native.json and frozen.json | `4d7c5197bda48c7629ec3b7eafcd3a6482efc28454189328f4eaada0d419d8cd` |
| captures.json | `df70ccdd58e9841db936e659a42fe138f0c2f7b70dead156797fabc813767b0b` |
| partial-measurements.json | `88a83e19898ed1591adf43bb2d7738aa5ffe412c32923198a0a19b401e55b381` |
| input-b19116fc74b63e9cae77cc8658d2cb87-no_ltm.json | `b05c27f7c621e547871829ea3d91962b2a5b9e2c3de5938a5ee6c15993a5956e` |
| final-b19116fc74b63e9cae77cc8658d2cb87-no_ltm.txt | `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855` |
| events-b19116fc74b63e9cae77cc8658d2cb87-no_ltm.json | `2cca9450b54128b165c2605bf2935ca4187c57be4e03015d5f4b4622b34ad681` |
| report.json | `5ec4776f971f8dd6da750c4fa9fda02fdfecde6c4312069b647e4ac85d143ec6` |
| collection-status.json | `ce0743a56515b9a9f25ef70265e2791c1e7f78bed8f3a36191224a659e20e07b` |
| plan.json | `aa23b8548a6a6a5eeccfe66f265e868c3c44c58201fc465beb628196f395fccd` |
| entered/final-state.json | `2e5dda0926e8a6ff9e586ed824050d48e394d84d10eb124b30737a5c964f36e8` |

All private entries above are relative to the exact experiment root's private
directory, except entered/final-state.json. Original collection status is
Complete=false/collection_failed_no_retry. Report is NO_GO with
model_quality_evidence=false, promotion_allowed=false and
baseline_comparable=false. Complete runner correlation appropriately cannot
pass because63 eligible native outcomes are absent and the paid first slot is
failed. A closed admission window and53 valid embedding responses do not
establish the user's requested complete model comparison.

## Prospective pending-only continuation assessment

A separately reviewed pending-only continuation can be designed without
automatically replacing the frozen controls: there are no native QA answers,
semantic-error-based selection or tuning. This is a reviewability assessment,
not entry approval. Both spent roots stay sealed. No53 auxiliary RPC or paid
native slot may be repeated; all prior original expenses/unknowns remain visible.

The original64-eligible/72-total matrix, input SHAs, skip reasons, data,
schema/prompts and criteria must remain exact. The paid failed slot may be
excluded from a residual admission schedule only; it must stay eligible and
failed/unknown in the composite score, never become a new planned skip or
disappear from a denominator. Full-quality/baseline claims remain subject to
the unchanged rules and may remain NO_GO even after every unpaid slot finishes.

Any rebased catalog must independently validate all exact old source aliases,
bodies/revisions/access hashes and chunk bindings, plus current owning task,
constraints and lease authority for new calls. Reused22 captures must be
explicitly attributed to the original90ed runtime and originals; they are not
newly observed retrieval execution. Do not clone prior receipts as fresh work.
Use typed immutable lineage pins to original source/binary/frozen/ledger and
paid54/568935 offsets, correlate only63 new unpaid native admissions, and
enforce aggregate280/4,000,000 and the original absolute expiry without refund
or clock reset. Offline positive/negative cross-root correlation and exact
fresh prepared pins require a new finite independent review before any Infer.
Prospective typed diagnostics cannot rewrite this window's UNKNOWN cause.
