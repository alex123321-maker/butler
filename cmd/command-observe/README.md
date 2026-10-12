# Offline Russian owner-control proposals

`command-observe` exports observation inputs and checks recorded JSON proposals.
It uses the Go standard library, with no provider, network client, executor or
authority issuer. `task.control` is an offline proposal, **not a registered model
tool**. Real authentication, delivery identity, CAS, lease and policy remain in
the existing kernel and channels.

`testdata/acceptance/ru-command-proposals-v1.json` has 45 cases in nine topic
groups: 15 per development/tuning/holdout part, including three secret-bearing
cases blocked before export in each part. The 12 eligible cases per part cover
cancellation, prepare-only amendments, external instructions, unknown schema and
ambiguous targets. Public synthetic data contain no working credentials.

This is a programmable contract fixture. Shared stop/prepare constructions across
its parts do **not** establish independent semantic model evaluation. It
supplements the unchanged independently reviewed V3 corpus of 240 cases; its
control criteria, signature and frozen verifier remain unchanged. Candidate
criteria cannot promote an extension or repaired model.

The fixed schema is `task-control-proposal-v1`. Proposals use existing
`ControlRequest` fields: `task_id`, `expected_revision`, `kind`, `instruction`,
and `execution_policy=prepare_only` for amendments. Exact expected function/args
matching is separate from an original witness check: source ID, source revision,
complete owner quote, current task ID and revision. External DATA cannot become
owner instruction. Unsupported schema, unclear target and non-control prose
require empty `clarify`. Even a valid proposal authenticates nobody.

Input export omits expectations/category/split labels. Case/group identities are
stable opaque hashes; the observation mapping stays inside the verifier. Any
secret-bearing case is excluded wholesale before input construction and rejected
if imported as an observation. Public fixtures are not a hidden benchmark.

```sh
command-observe -mode inputs -root /src -split holdout \
  -output /private/command-inputs.json
command-observe -mode check -root /src -split holdout \
  -commit <exact-40-hex-commit> -hardware /private/hardware.json \
  -observations /private/observations.json -output /private/report.json
```

The envelope has exact `pin` and `observations` fields. Pins are `evidence_kind`,
`code_commit`, `source_sha256`, `schema_sha256`, `suite_sha256`, `hardware_sha256`,
`model`, `weights_sha256`, `runtime`, `mode=observation_only`. The CLI recomputes
source-tree and hardware-file hashes and checks the supplied commit. This binds
artifacts, not caller identity or Git authenticity: the independent runner must
bind actual code releases, weights/runtime files and measured observations.

`fixture_validation` requires `model=none`, empty weights, and reports
`FIXTURE_VALIDATION`. `model_observation` requires a model with real pinned
weights SHA256 and reports `UNREVIEWED_MODEL_OBSERVATION`; undisclosed cloud
weights cannot receive invented hashes. Both report `model_quality_evidence=false`,
`execution_authorized=false`, `golden_review=CANDIDATE_REVIEW_REQUIRED`. This CLI
cannot sign approvals or open native windows.

Every observation requires `case_id`, raw JSON `output`, measured `milliseconds`,
`input_tokens`, `output_tokens`, `memory_peak_bytes`. Explicit NULL usage/memory
remain unknown, separately counted from known subtotals/peak. Missing, duplicate,
foreign, secret, negative and overflowing observations fail. Wrong function/args,
stale witnesses, unexpected action proposals, missed controls and malformed JSON
have separate counts. These are proposal errors, not actual permission failures,
verified task outcomes or effects. Scripted timings are not model latency.

Observation/hardware files must be bounded private regular files. New report
files use `O_EXCL` and preserve existing evidence. Reproduction:

```powershell
pwsh -NoProfile -File scripts/dev/lab.ps1 -Action Sync `
  -RemoteRoot /srv/storage/butler/agents/scenario-corpus
ssh -o BatchMode=yes laba 'bash /srv/storage/butler/agents/scenario-corpus/current/scripts/dev/scenario-corpus-lab.sh /srv/storage/butler/agents/scenario-corpus broad <exact-commit>'
```

The runner uses a read-only source snapshot, network-disabled Go 1.27.2 container,
repo-local caches and synthetic data. Before testing it records source-file
manifest, Docker image/kernel/CPU/RAM and container limits. Broad mode retains an
actual CLI fixture report with exact source/commit/hardware pins. Race/vet and
input export make zero model calls. Live benchmarks need a separate bounded
owner-authorized observation window; existing native history is untouched.
