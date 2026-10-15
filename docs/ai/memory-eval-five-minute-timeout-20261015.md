# Five-minute model turns: implementation and laba setting

The owner's explicit five-minute request is implemented prospectively. New
memory-eval turns set both provider timeout and owning model_millis to300000ms.
The shared schema-dialect helper retains120 seconds for legacy diagnostic
callers; memory-eval supplies its own five-minute duration. Earlier parent,
mandate, source/ownership and cancellation conditions still apply.

On ssh laba, the existing deployment was configured through lab.env with:

- BUTLER_OPENAI_TIMEOUT_SECONDS=300. Despite its legacy name, the Orchestrator
  passes this value to the selected Antigravity provider too.
- model_millis=300000 inside BUTLER_EXECUTION_BUDGET_POLICY_JSON. This was
  previously absent, inheriting the120000ms default.

The operator change preserved every other environment value, source/deployment
pointer and image ID. The only live environment changes were the transport
timeout and model-time budget; the Broker received the same updated budget JSON.
Before restart, no running model attempt had been updated in the preceding ten
minutes. Orchestrator and Tool Broker were recreated from the same images and
both became healthy. No inference, image build, pull or database migration was
performed. Main remains on the existing3e26cd8 deployment; PR code was not deployed.

The private exact lab.env backup, candidate, dry configuration comparison and
safe applied receipt are retained at:
`/srv/storage/butler/reports/model-timeout-300s-20261015T084428Z`.
Private backup/config material is deliberately not included in this repository.

## Verification

Candidate implementation commit prefix0b5fc07, exact clean source snapshot
`20261015-154141-911`, was checked on laba. An isolated fresh PostgreSQL and
fake model verified actual model admission of approximately300 seconds;
invalid factory timeouts were rejected before private credential access.
Memory-eval, shared factory validation, earlier authority/scope deadline and
offline CLI regression tests: **32 PASS,0 FAIL,0 SKIP under race**. Vet passed.

Proof:
`/srv/storage/butler/agents/memory-eval-78-20261014/reports/memory78-timeout-20261015T084207Z-1876756`.
All model calls in these checks were synthetic; no Gemini request was made.

The previous a3d66736 experiment remains an immutable historical comparison
with its120-second timeout and original60-second p95 score threshold. It has
not been rerun or reclassified. A longer timeout allows waiting; it does not
establish whether an unanswered request was thinking, queued or delayed by the
network/proxy. The [h11 case analysis](memory-eval-v2-h11-case-analysis-20261015.md)
separately explains a completed but strictly rejected answer.
