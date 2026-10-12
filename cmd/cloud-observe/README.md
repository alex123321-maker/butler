# Cloud shadow observation importer

This Go standard-library command exports frozen Russian inputs, records the fixed
acceptance policy and imports quality/comparison observations. It has no provider,
auth importer, network client, Tool Broker, owner-control endpoint or executor.
`Score`/`Compare` and their real-local-weights requirements remain unchanged.

The data are the unchanged independently signed V3 corpus: 240 examples / 30
source groups, 96 development, 64 tuning, 80 holdout. Four secret-bearing holdout
examples are blocked before export and scoring, leaving 236 observations:
96/64/76. Case IDs are opaque; expected answers, classification suffixes, group
labels and control gold are excluded from model inputs. The public synthetic
corpus is not a blind benchmark. The separate 45-case command supplement remains
a contract fixture, not this quality holdout.

The independent root accepted these thresholds **before live observations**:
one attempt per eligible case, no retries/resampling/holdout retuning; each split
at least 90% overall and 85% per kind, at most one missed-important and one
false-important classification per split; zero schema/source-fabrication/unsafe
authority/tool-proposal/transport failures; p95 at most 60 seconds and each attempt
at most 120 seconds. `FrozenCloudPolicy()` and `CloudPolicyDigest()` provide the
fixed policy. There is no threshold override. Missing cases yield NO_GO.

Fixed comparison originals are `d01-search`, `d03-fact`, `d05-important`,
`d07-control`, selected before quality results. Each arm has the same four
original inputs, profile, code, hardware and client allowance hash, with a cap of
four calls per task. Single uses one fresh context. Multi uses two independent
child observations and one synthesis: three calls, all counted. Synthesis receives
the original and both child answers as DATA; model agreement supplies no truth or
verification. Exact original-source/golden checks judge the final. These are
shadow contexts; actual kernel delegation is separate product evidence.
Child JSON is serialized canonically as DATA, preserving every value and source
quote while keeping its input digest stable across recorded JSON envelopes.
Malformed child JSON is rejected before constructing a synthesis request.

`CloudPin` binds provider `antigravity`, requested `gemini-3.8-flash-high`, nullable
reported provider version, runtime and runtime SHA, code commit, source SHA,
hardware manifest, corpus/schema/policy/client-resources hashes, window ID and
`mode=shadow_observation`. Proprietary `weights_status=unavailable` requires
`weights_sha256=null`: no fabricated weight hash. Reported versions must match a
known pin and each other; missing versions remain separately counted unknown.
The requested profile and client hardware are not byte-identical weights or
identical proprietary server hardware.

Each attempt records unique attempt/context/case IDs, fixed role, nullable parent
context correlation, start/end UTC, elapsed milliseconds, exact logical model
input digest, raw final JSON or NULL, finite outcome, tool-proposal flag and
explicitly nullable input/output/**total** tokens and client process peak RSS.
Thinking tokens may make total exceed input+output; total is never derived.
Missing fields, duplicate/aliased metadata, invalid timing, overflow, changed
inputs, secret/foreign cases, retries and inconsistent versions are rejected.
Programmatic source checks bind refs/quotes to original documents; unsafe
authority, command-intent mismatches and missed/false classification are distinct.

Known usage subtotals and unknown attempt counts stay separate. Any NULL input,
output or total denies full token cost. Comparison aggregates all charged attempts
before evaluating failed children/finals, preserving their spend. Client RSS is
shared process telemetry, not per-child model memory. Remote model memory and
incremental subscription invoice are unobserved NULL; subscription membership
does not establish a per-request invoice delta. Complete token accounting is
separate from unavailable money/remote-memory measurement.

Perfect fixture outputs report `SYNTHETIC_THRESHOLD_FIXTURE` or
`SYNTHETIC_COMPARISON_FIXTURE`, never actual model quality. Native claims report
`CLOUD_SHADOW_THRESHOLDS_PASS` / `CLOUD_SHADOW_COMPARISON_PASS` only with complete
source/schema/quality/token gates. These pure functions cannot authenticate a
collector's inference claim: independent audit must correlate every attempt with
the charged owning ledger and immutable native report. All reports keep
`execution_authorized=false`; no model release follows automatically.

```sh
cloud-observe -mode policy -root /src -output /private/frozen-policy.json
cloud-observe -mode inputs -root /src -output /private/cloud-inputs.json
cloud-observe -mode quality -root /src -commit <exact-commit> \
  -hardware /private/hardware.json -observations /private/quality.json \
  -golden-public-key /review/public-key.hex -golden-approval /review/approval.json \
  -output /private/quality-report.json
```

Quality envelope: `{pin: CloudPin, attempts: [CloudAttempt]}`. Comparison envelope:
`{single: CloudComparisonRun, multi: CloudComparisonRun}`. Comparison tasks include
the original input SHA, task wall interval and every attempt; attempt time sums
remain distinct from wall time. Library helpers are `ExportCloudInputs`,
`CloudComparisonInputs`, `MakeCloudModelInput`, `CloudModelInputDigest`,
`VerifyCloudResponse`, `ScoreCloud` and `CompareCloud`.

The CLI recomputes actual source-tree/hardware hashes and requires bounded private
regular input files. Independent key/approval files must be outside candidate
source with exact V3 review byte hashes; `DecodeCloudGoldenArtifacts` then checks
their original signature. It cannot create/sign an approval. Reports use O_EXCL
and preserve older evidence. Commit/runtime/model identity still require the
independent frozen runner's release and owning-ledger audit.

`-mode window-audit` independently imports the complete private collector report
from the fixed `issue33-high-shadow-20261012-baseline45` window and code commit
`f3e0cbc11dedc2fb8e547e61f0153bf78cde6f98`. It reads at most 2MiB, requires the
external signed V3 golden and independently supplied prepared source/history
hashes, and hashes the prepared hardware manifest. It reconstructs the original
fifteen-artifact hash chain and retains 45 historical attempts, 20 known pairs,
25 NULL pairs and the original known subtotal 74698/2210.

Every new ordinal must be contiguous from 46, unique and matched to exactly one
observed attempt with the same context, input hash and charge time inside its
recorded attempt. Missing children, extra/reused contexts, changed source/runtime
pins, widened resources and counters fail import. Sequential timing is checked
against the window's declared one active context. Quality and comparison are
recomputed from original witnesses; stored scorer verdicts are ignored. All new
attempt costs are aggregated before scoring, including failed children and NULLs.
A failed collector stays NO_GO even when its stored or recomputed metrics pass.
The requested High profile remains distinct from the bounded observed version:
a known non-3.8-Flash family yields NO_GO; absent versions remain unavailable.
The opaque server suffix does not prove server effort, hardware or weight bytes.

```sh
cloud-observe -mode window-audit -root /audit-source \
  -observations /private/observations.json -hardware /prepared/hardware.json \
  -expected-source-sha256 <independent-prepared-source-hash> \
  -expected-history-sha256 <independent-fifteen-artifact-baseline-hash> \
  -golden-public-key /review/public-key.hex -golden-approval /review/approval.json \
  -output /private/new-safe-window-audit.json
```

The exported audit contains only pins, counters, fixed scorer metrics and per-new
attempt ordinal/context/input digest/time/outcome/version/nullable usage/RSS. It
has no case input, documents, model output, source text or credential material.
It pins the complete original private artifact by SHA256. Valid partial/failed
observations export NO_GO with every spent ordinal for future carry-forward;
malformed correlation denies import. O_EXCL protects prior audit files. Audit
PASS validates a collector claim offline and grants no execution/model release:
the independent root must also verify the actual immutable runner and PostgreSQL
journal. Roles share an administrator-controlled host, not hardware isolation.

Zero-inference checks use `scripts/dev/cloud-observation-lab.sh ROOT narrow|broad`
after isolated `lab.ps1 -Action Sync`. Go 1.27.2, network-disabled containers,
read-only source and repo-local caches run meaningful synthetic race/vet checks.
Native execution and auth remain exclusively with the separately authorized
root-owned bounded runner. Old sealed windows/usage/data are untouched.
The additive `witness_diagnostics` in a window audit separates invalid/duplicate
source references, empty keys/quotes, quotes absent from originals and exact
value/quote mismatches. Anchored quotes with differing values are counted
separately. The frozen `source_fabrications` metric remains unchanged; its name
does not mean every contract violation is a literal hallucination. Diagnostics
export only counts and never values, quotes, source text or goldens.
Prospective `v2-inputs`, `v2-policy`, `v2-quality` and `v2-compare` use a separate
explicit `testdata/acceptance/ru-cloud-fresh-v2.json` corpus. V1 functions, hashes,
goldens and NO_GO audits remain unchanged. V2 native configuration is code-owned
High, application/json, a kind-generic JSON schema,32768 generated tokens including
thinking, and8192 visible JSON bytes. Unknown usage/version cannot become known
zero or the requested model identity. The same90/85/safety0/latency thresholds
remain fixed; the dataset and comparison cases require independent review before
new calls.

V2 imports require external new signed review artifacts plus trusted
`-expected-review-key-sha256` and `-expected-review-approval-sha256`, exact source
commit/tree and actual hardware. The native three-argument review helper pins
the two newly published external artifacts from the independent whole-corpus
review. An explicitly pinned fixture/import helper never writes or signs approval.
Pure offline success is not native provenance or execution permission. Existing
artifact output uses O_EXCL. See the
[prospective design](../../docs/ai/scenario-cloud-prospective-v2-design-20261012.md).
# Fixed V2 client allowances

The V2 resource pin binds the exact client allowance: four calls per comparison
arm, 65,536 input bytes, 32,768 generated tokens including thinking, 8,192 visible
JSON bytes, 120 seconds per attempt, 480 seconds per arm, one context at a time,
and a 1,536 MiB client process limit. A changed resource digest or excessive arm
wall time denies comparison readiness. The cost projection retains all recorded
calls before a wall-time rejection. These client limits do not observe cloud
weights, hardware, remote RAM or an incremental subscription invoice.

`v2-window-audit` imports a private collector report of at most 2 MiB offline.
Supply the external V2 review files and their trusted hashes, `-commit` for the
observed immutable release, `-expected-source-sha256` from preparation,
`-expected-history-sha256` for the 16-artifact/228-call baseline, and the actual
`-hardware` file. The auditor may be built after the observed release; its own
tree does not replace the prepared source hash.

The safe result contains no questions, documents, model outputs or approval
material. It correlates ordinal/context/phase/case/role/input hash/canonical
output hash/outcome/nullable usage and the actual immutable PG expiry. Canonical
JSON hashing changes only representation, never string contents or semantics.
All recorded costs precede re-scoring failures or incomplete comparisons. A
failed collector stays NO_GO; all 228 previous charges and 25 NULL input/output
pairs remain spent. Historical total cost is incomplete for the first 45 calls.
O_EXCL output cannot overwrite evidence.

`v2-pg-correlate -observations <private collector report> -pg-witness <separate
read-only SQL artifact>` compares the exact V2 PG witness version, window counter,
baseline/policy hashes, expiry and every ordered metadata/usage row. NULL and
literal zero remain different; no extra, missing or reordered rows are accepted.
The output contains only artifact hashes, counts and a matching verdict. It
does not score the corpus, infer, read authentication, or establish quality or
execution permission. A failed collector may match its genuine SQL journal;
its cost and NO_GO verdict remain owned by the separate window audit. The
immutable runner/source must establish that the SQL file came from the actual
owning database. Both inputs are bounded to 2 MiB; output cannot overwrite an
existing artifact.
