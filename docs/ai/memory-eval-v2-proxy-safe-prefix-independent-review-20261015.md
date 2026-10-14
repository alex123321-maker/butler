# Independent proxy-safe actual prefix review, 2026-10-15

Decision: **PASS_CORRELATED_RETAINED_PREFIX_ONLY; NO_GO_COMPLETE_COMPARISON**.
The collector retained 54 completed native QA originals and one bounded timeout;
the original scored matrix still lacks six eligible observations and three
planned skip observations. This document grants no further model entry, repeat,
repair, fallback, source change, tuning, budget/deadline reset or promotion.

Reviewer: codex-independent-memory-review-20261014, separately appointed
/root/memory_independent_review. The reviewer authored no candidate/control/
rubric, performed no authentication/Infer or PG mutation, read no credential/
import/key contents and wrote only independent review documents. Checks used
safe synthetic table reads and explicit read-only UID10001/network-none leaf
or whitelisted tar mounts. Role independence shares the host.

## Exact source, runtime and original pins

Root: /srv/storage/butler/agents/memory-eval-v2-proxy-safe-final-20261015.
Prepared release10853346d9c527e09c82558104359b65b84166f7, exact snapshot
20261015-023224-339 / source
b8e806ae23d66b5da1a1afefd0ac0de061b34eea08edb3983b4e8eacff2ac720.
Collector0333191d0a98158b011f02c6c50a51dd002efbbfc9e6f96ab4e7292ac4ddd68d;
audit CLIa3569612179830e497fd0c2e5ec0b4b179d054d6ccb3687f986ec7f32a9ba235;
pins7fa930f54927c9350f53193ae1250649dd597ec3d8a174828c4b4ca312d043eb.
No model input, schema, expected answer, criterion or memory configuration was
changed during this window. Later documentation is not relabelled as model
snapshot bytes.

The reviewer independently read actual execution proof
f4b276cab2d7a282ec9c1f97a24457ed56450964873740efa1ee963d41b468ca:
exact collector command/image, UID10001, CPU2, RAM1610612736, PIDs128,
read-only rootfs, source/runtime/metadata/review/prior/auth mounts read-only,
private output writable, exact three networks, ProxyConfiguredtrue and
LocalBypasstrue. Infer-disabled local Describe preflight was PASS with SHA
c39bfcb07bff3ae680e461cf5b8343126bc8e81ffd5c57cb4bbcd852b7acdfdb.
No disposable native OAuth preflight helper exists in the prepared source.

| Retained leaf | SHA256 |
| --- | --- |
| ledger.json | abb9ce3840e3ec994e024180017ba684b999066a2541f5c4b039700ec6725307 |
| frozen-before-native.json, 69-input schedule | 927ab6dd08663cf3968f32292386806ee40462139cad27aedde44040a61f5204 |
| measurements.json | a6ba530510103e4183fd70489b448d408da57b7d213454e170cd8c943a18602c |
| comparison-measurements.json | 29413d4fff88424de8b7f113e41a9db33d327b37608f8ac92e16c1f2b76e7af8 |
| comparison-frozen.json, original72 | 4d7c5197bda48c7629ec3b7eafcd3a6482efc28454189328f4eaada0d419d8cd |
| captures.json | df70ccdd58e9841db936e659a42fe138f0c2f7b70dead156797fabc813767b0b |
| report.json | 508d3483ef7deabdbcb51e8c9067340fcd9b6e54066dfa2e03765b966416046c |
| collection-status.json | ce0743a56515b9a9f25ef70265e2791c1e7f78bed8f3a36191224a659e20e07b |

The scored freeze remains
93dc4e25a835caeca81c7098aa5f23144ee21489eca3b2729da33aa0d11e1e9f.
Golden raw5571d474..., suite7c74abe1... and criteria9be3b4b5... remain
independently approved and unchanged. The 69-input schedule excludes only the
three original paid failures; it never changes the original72 denominator.

## Typed failure and limits of diagnosis

The new failure is v2-h09 retrieval,
case memory:32f7f5b380825072177b869a45041f6e,
run memory-run-54e8165270d1a3828513a51d9f873f6a,
operation operation_ce22ad0ffe8e0a0246b3fe8417f909d592c7383d871a7369.
Its exact frozen input digest is
d07dfcd8b4a064ddf86250647c89f72249b07825ff9e30665bbdf1f638febed8;
retained input file6245bytes SHA
82440fb3f8d036c59cc2f9074d173c4215670a9e21392b6d327a8974d5e8b352.

The original safe event array711bytes SHA
39a100228c22f4ba608e264b05ca1574a690ea88e4d7499ca95b26a9ecedbfa3
contains only run_started at19:53:53.037012101Z and transport_error at
19:55:53.029185917Z, error_code provider_timeout. No HTTP status or finer
reason was observed. Total elapsed120014ms matches the bounded request
timeout. Raw final is zero bytes (SHAe3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855);
model version is empty and input/output/total usage and invoice are null.

This establishes the timeout class and absence of a retained final. It does
not establish whether the upstream executed inference, its hidden usage, load,
network cause or the cause of any earlier direct-route HTTP400. Safe projections
must not reconstruct unobserved provider bodies or retrospectively assign causes.
The failed generation's spend is retained; original-write outcome is success
for this failure. The historical first original-write failure stays unchanged.

## Independent correlation

At19:58:21 UTC the reviewer independently compared all19 whitelisted durable
table arrays with live PG: exact equality after sorting rows. There are55
native admissions/reservations/attempts, zero auxiliary admissions/jobs,54 QA
rows and165 immutable guard receipts (110 dispatch,55 final). All55 attempts
are matched to exact original input/final hashes, provider/model/version/
usage, operations, reservations and conservative wire allowances. New allowance
sum2507177 plus660396 carried units equals3167573.

All runs, owning sessions/tasks/revisions/constraints, current lease/fence/
expiry and exact ordered source references match their originals and all
three guard phases. Current durable task revision is max(1, model task revision);
that distinction is explicit rather than invented by the model. All current
guards were checked before the original absolute expiry.

The closed window is111calls /3167573units with prior56 /660396, original
expiry2026-10-14T20:16:58.345536Z. No auxiliary inference was repeated. All57
source/revision records,69 parser chunks and69 object links match the retained
parent deterministic content/identity after excluding fresh storage timestamps.
All66 bindings/bundle/search original files are byte-identical to the parent;
all22 capture records match the original full freeze.

The reviewer read every one of54 raw completed JSON answers and independently
reimplemented the11 assessment flags in Python. All594 flags match saved QA.
Raw hashes, complete final/run-completed events, unique event IDs, exact pure
model projection/instructions, actual version gemini-3.8-flash-n and all three
usage dimensions match. Input+output is no greater than reported total:
provider total can include thinking and is not replaced with their sum.
No tool_proposed event occurred.

The composite retains all236 prior expenses plus620 current rows, exactly856
unique IDs, and every prior failed observation unchanged. Current154 logical
stage calls plus prior191 equals345, already above frozenMaxCalls280.
Model admissions111 are a different counter. All54 native input/output/total
known usage sums are34177/3052/164775. Four failed native generations and the
original failed write yield five failed expense rows. Unknown invoice,
embedding tokenizer, hardware/physical PG cost and original retrieval pipeline
latency stay unknown. All broader task successes remain null.

The composite contains54completed,4failed and5planned-skipped observations;
nine original slots are unobserved (six eligible native and three h10 planned
secret skips). No paid failure is converted to a skip, no completed output is
replaced and no denominator is reduced.

## Frozen rubric interpretation

| Strict canonical/rubric count | no_ltm | full_context | retrieval |
| --- | ---: | ---: | ---: |
| Completed originals | 19 | 17 | 18 |
| Context answer correct | 15 | 6 | 7 |
| Canonical truth correct | 5 | 6 | 6 |
| Applicable evidence | 5 | 6 | 6 |
| Citation contract correct | 19 | 12 | 13 |
| Task code correct | 18 | 16 | 17 |
| Answer/instruction consistent | 15 | 14 | 14 |
| Strict false-positive marker | 4 | 11 | 11 |
| False abstention marker | 0 | 0 | 0 |

These are the unchanged finite canonical/rubric counts, not a semantic human
error rate. Concrete semantic limitations observed after freezing include:

- d02 full_context says "На русском языке"; d11 says "Нет, ...не оплачен".
  They preserve the actual meaning and have valid anchored quotes, yet fail
  lowercasing/whitespace-only exact alternatives.
- h01 full_context says "шкаф 7, код П-44" with both correct witnesses; h05
  says "Нет, не запрещено"; h07 retrieval correctly states revocation and
  notification only on completion. Word order, punctuation and extra phrasing
  cause canonical rejection.
- d03/d12/h04 give supported negative statements instead of the required
  empty-answer abstention. This is an output-contract rejection; these outputs
  do not invent the unavailable confirmed value.
- d05 correctly refuses restart with wait_for_check, but phrasing and extra
  historical citation fail the current-instruction golden. d08/h02 correctly
  continue the retained goal and prohibit publication/upload, yet fail lexical
  answer equality. Rubric stop_proposal_violation generated from those flags
  does not prove an actual stop or permission-kernel violation.
- h06 describes checksum comparison with anchored relevant originals, but its
  longer wording, an additional applicability citation and in full_context an
  abbreviated required span fail the frozen answer/citation contract.
- h08 proposes checking quantities in the existing file without recreating it,
  using check_inventory instead of the frozen verify_file in all three arms.
  This is an actual action-code mismatch under the finite enum. The natural
  language and enum meanings overlap; it does not propose forbidden recreation.

No official result is recalculated with post-observation synonyms or corrected
goldens. Further model/rubric tuning on these exposed controls would require a
fresh independently reviewed control freeze. Safely absent-arm abstention has
context credit without actual-fact credit; e.g. h01 retrieval lacks sufficient
joined evidence and declines. Proposed action prose and guard observations
remain distinct from executed external effects.

The official negative report is retained with candidate_thresholds_passedfalse,
baseline_comparablefalse and promotion_allowedfalse. The standing scope in
versioned-memory-acceptance.md allows a *complete correlated negative*
comparison to accept a harness, but this prefix has six unattempted eligible
slots. Therefore full harness comparison acceptance is withheld here.

## Conditions for considering a prospective remainder

There is no statistical requirement to discard the untouched six fixed slots
merely because transport stopped, provided no input/model/schema/rubric/config
or selection depends on answer quality. Any new prospective protocol needs
its own reviewed source, tests, exact prepared pins and finite admission gate.
All58 admitted native slots and all63 retained observations must be excluded
from execution: the latter includes five earlier planned skips. A valid
remainder is the fixed original suffix of nine observations: six eligible
h11/h12 requests and three h10 secret skips.

The immutable carries are111calls/3167573units,856expenses,54completed/
4failed/5skipped, original72freeze and original absolute expiry. No fifth
unreviewed chain, repeated capture/embedding, paid resampling, removed failed
cost, new deadline or larger budget is implied. All old evidence stays sealed;
this document is not entry authorization.
