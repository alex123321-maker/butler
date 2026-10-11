# First shared local runner observation — 2026-10-11

Status: transport and runner observations available; semantic check FAILED.
This is not completed delegation or a model-quality/release admission.

Source snapshot `20261011-132444-142` ran five fresh contexts against one
immutable CPU Ollama runner: one context with two sources, two sequential
single-source contexts, and two concurrently submitted single-source contexts.
All five generated actual finals with complete usage; there was no retry. The
fixed in-process admission and server OLLAMA_NUM_PARALLEL=1 serialize inference.
The independent server `/api/ps` observation found exactly one loaded model,
size1,018,534,295 bytes and size_vram0. One loaded weight service serves all
contexts. No main service/model or cloud credential was used or changed.

The model returned incorrect source IDs/shape in every case. Its first output
contained duplicated `source` keys; the deterministic duplicate-key scorer
rejected that output rather than silently selecting the last source. All five
observations are kept in [the original report](shared-contexts-first.json).
Its SHA256 is
`b45ed541437ea24620740d26821fbb9b7b6b559687c531fecf2e7dccac4a7825`.
[The actual loaded runner](loaded-runner-first.json) has SHA256
`358a641817a9b902ae18cf9e8dd21123f9b8d2691ddb12b7af6948e2f0a7ba51`.

All five known usage pairs total608 input and150 output tokens. The cold single
context took3365ms; sequential contexts took776/736ms; concurrently submitted
contexts took771/1513ms including the private queue. Cold/warm differences and
missing planning/verification overhead forbid a claim that delegation is faster
or cheaper. These are small public development observations, not the signed
Russian holdout, a production-controller baseline or durable task acceptance.

External proof:
`/srv/storage/butler/agents/local-generative/reports/shared-contexts-20261011T062541Z-4041957`.
The preceding v1 window failed preflight, before the driver or any generation;
that failure is retained at `windows/shared-contexts-v1`. The five-attempt v2
window is sealed at `windows/shared-contexts-v2`. Neither window is reset.
Preparation first verifies the public manifest, then disables download/egress,
mounts weights read-only, forces Docker runtime runc and num_gpu0, and uses
UID10001, 2CPUs, 2GiB, an internal network and no published host port. Go/race/vet
unit tests use only synthetic servers. The metadata-validation Python snippet
uses the lab's existing standard library solely to check a public JSON file;
it is not a service, model runtime or new dependency.

See [adapter](../../internal/transport/ollama/README.md) and
[preparation script](../../scripts/dev/local-generative-lab.sh). A later semantic
experiment needs a separately recorded prompt/profile/window; these five failed
results cannot be promoted by changing their scorer or rerunning silently.

Final synthetic verification on snapshot `20261011-133247-001` passed race tests
for ollama, transport and content, and affected vet. Proof:
`/srv/storage/butler/agents/local-generative/reports/transport-final-20261011T063303Z-4066343`;
tests SHA256 `f5fe2a7c171e4ce3ed39077f2a340e7491dd5082bdbe995b4e1f006a3880ae71`.
It added zero actual model requests. New usage-bound tests preserve explicit
zeros/unknown counts, reject negative partial measurements and verify a redirect
cannot reach another endpoint.
