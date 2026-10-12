# Same-fixture delegation control acceptance without inference

Baseline: `bdd85cdc752a739e0a4fa9be4789bf2f8fc7132d`. Only existing preflight and
scripted observability tests are strengthened; no production fence, transport,
provider, scheduling or native window is changed.

`TestFullHighDelegationIndependentDecisionsWithoutInferenceIntegration` now uses
one PG17/MaxConns2 fixture with the same immutable originals, source grants,
goal/mandate, shared day budget and permanent journal across widths1 then2.
Each actual private Broker composition uses its real admission width. Ordinary
Input Service/core produce four scripted source-pinned, unverified child finals
and current structured Catalog reports. Scripted model turns are explicitly
labelled; no operator import or native provider is constructed.

Width1 verifies owning report/final/source pins, saves a separate owner-verified
result, and sends actual authenticated HTTP accept/reject decisions. Original
result/report references and unverified finals remain. The predeclared selection
of original A does not turn correct extraction of conflicting B into false truth.
Width2 independently verifies the same owning report paths while keeping children
result-ready for current control baselines.

| Boundary | Positive baseline | Actual mutation and denial |
| --- | --- | --- |
| Revision | Both width2 `CollectGenerated` and recorded-source guards pass | Owner Amend succeeds; both old collections and source releases deny |
| Privacy | Authenticated owner reads both original pins | Actual Catalog Delete removes A; A owner read denies and unchanged B still reads |
| Stop | Same authenticated owner advances current first root | Owner Stop succeeds after deletion; the same owner advance denies |
| Fresh amendment input | Amendment queues a current continuation | Current owner Stop also cancels the second root; no queued/running input or live lease remains |
| Effects and verdicts | Two actual Broker admission effects per root | All four confirmed effects remain; parent finals remain unverified |

The safe `full-high-control-matrix.json` projection records only these measured
boundaries and a before-cleanup owning audit. It omits unmeasured native overlap,
latency and generation-quality fields. Native spending remains27; all ten model
turns in this fixture are scripted.

The existing `TestNativeDelegationScriptedOwnedObservabilityIntegration/width-two`
also proves two actual failed child core runs with no child final, then authenticated
owner Stop cancels both child tasks/inputs and prevents lifecycle advance. Its safe
audit retains two failed run outcomes, the original parent final and two confirmed
Broker effects. This is a controlled scripted failure, not replay or repair of a
failed native report. The immutable real remainder report's stopped processes,
blocked inputs and failed runs do not by themselves imply durable owner Stop;
its still-executing delegation metadata is retained as observed.

Isolated lab root: `/srv/storage/butler/agents/delegation-control-matrix`.
Final code snapshot: `20261012-131530-594`.
Real PG/race/vet report: `reports/broad-20261012T061534Z-239484`.
Six test pass events, zero fail/skip, zero inference: same-fixture controls,
parallel natural claim recovery, and existing width1/width2/provider-denial
observability. A previous compile error in the added test used an undeclared
error variable; its failed report `broad-20261012T061450Z-235845` is retained and
not counted as passing evidence.

Final `events.jsonl` SHA256:
`e9f8e43aa2935a88c23b3586c3913ef75439bf0dd7ce2abf799c99db929fa015`.
Empty `vet.log` SHA256:
`e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`.

```sh
go test -c -o /proof/broker ./apps/tool-broker/internal/broker
export BUTLER_DELEGATION_BROKER_BINARY=/proof/broker
go test -json -race -p 1 -count=1 -run '^Test(DelegationParallelRecovery|FullHighDelegationIndependentDecisions|NativeDelegationScriptedOwnedObservability)' ./apps/orchestrator/internal/app
go vet ./apps/orchestrator/internal/app ./apps/orchestrator/internal/input
```

Source mounts are read-only; the lab uses isolated agent-local caches, an internal
network and a disposable test database. No raw dump, original auth/environment or
credential material was read. These checks finish control implementation evidence
while actual High overlap remains a separately coordinated native gate.
