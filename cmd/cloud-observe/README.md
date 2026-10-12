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

Zero-inference checks use `scripts/dev/cloud-observation-lab.sh ROOT narrow|broad`
after isolated `lab.ps1 -Action Sync`. Go 1.27.2, network-disabled containers,
read-only source and repo-local caches run meaningful synthetic race/vet checks.
Native execution and auth remain exclusively with the separately authorized
root-owned bounded runner. Old sealed windows/usage/data are untouched.
