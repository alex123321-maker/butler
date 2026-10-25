# Local synthetic effort research

The completed [native pilot report](../../docs/testing/effort-research-results-2026-10-25.md)
retains all230 actual calls, failed attempts, unknown usage and source evidence.

The separate [18-call source-availability supplement](../../docs/testing/effort-evidence-protocol-2026-10-25.md)
shares an actual draft between reviews with/without a new T1 fixture observation.
It leaves the96-slot v2 corpus/producer unchanged and reports useful current
completion separately from correct acknowledgement of missing information.
Invoke it with `scripts/dev/effort-research.ps1 -Action Evidence -EnableNative`
after v2 ends, using new trial/output names. The same private auth store permits
only one refresh-capable process at a time.

This opt-in operator command studies #97 outside the working Butler deployment.
It uses the native Go Antigravity transport and an isolated loopback PostgreSQL
database named `effort97_test`. It has no real mail connector, external tool
executor, Broker bypass, publication, delivery or production constructor.
Fixture reads are **simulated source observations**, never actual tool effects.
The ordinary Core/Broker/native function loop is not claimed end-to-end tested.

The existing production loop already retains native function calls, signatures
and function responses. Its ordinary history accumulation is not a new feature.
This experiment adds an explicitly bounded, separate research driver with typed
goal versions, primary fixture observations, untrusted model candidates,
hypotheses, open checks, contradictions, read receipts and remaining calls.
The driver initiates all review phases; the snapshot is labelled harness data,
not an invented owner utterance. It stores state before the next model pass.
No working/profile memory or new production memory class is installed.

## Frozen v2 comparison

Twelve synthetic cases × two advertised profiles × four loop modes =96 slots,
one repetition. This is a descriptive pilot, not an equivalence trial. Case IDs
and off-model decision/value/required-source facets are fixed in
`internal/acceptance/effort/cases.go`; expected values never enter model input.
Native Google response schema, prompt, slots/order, model/profile, caps and
source identity are committed in a PG window and exported before inference.
Both profiles were separately qualified using a disjoint `ready` fixture.

- Baseline: normal proposal → fixture observation → next model pass until final.
- Repeat: one extra pass over the same evidence and previous candidate.
- Check same evidence: one checklist pass without additional source delivery.
- Checked: the same checklist may request new fixture evidence.

Modes rotate by case; Low/High order also alternates. Every slot gets six model
calls, a five-minute task deadline, 65,536 serialized input bytes per call and
8,192 generated tokens (thinking included). A model invocation has a two-minute
deadline; the entire window expires after three hours. These are finite process
guards, not a monetary limit requested by the owner. There is no automatic retry,
resume, model/endpoint fallback, or hidden adaptive switch.

The same source opportunities exist from the beginning. After its first final,
repeat/check-same cannot read new sources; checked may do so. Compare repeat vs
check-same for checklist effect and check-same vs checked for new-evidence
opportunity. Report actual changed answers/reads: an arm with no new read cannot
establish a causal fresh-evidence benefit. Randomness and one repetition limit
causal claims. Declared uncertainty is not evidence of universal model superiority.

Current-source quotes and citation visibility are checked by code. Hypotheses
never become source facts. An unresolved answer, stale goal, unsupported quote,
two repeated reads without new evidence, cancellation or exhausted budget stops
the slot. Clarification/absence/error are graded as correct responses where the
fixed fixture requires them; they are not successful real-world mail retrieval.
Free-text answers require separate review against the frozen primary sources.

The profile names are confirmed by authenticated subscription inventory.
V2 explicitly sends `thinkingLevel=LOW/HIGH`, the same Cloud Code route and
output/schema contract. Observed `modelVersion` is recorded; undisclosed weights
or exact internal allocation are not inferred from it. The earlier automatic
budget preflight and construction-rejected schema attempts remain separate,
including all unknown usage. No prompt/model tuning uses the quality sample.

## Private setup and invocation

Set repo-local `GOCACHE`/`GOMODCACHE`. Start a disposable PostgreSQL on this PC,
loopback port55449, database `effort97_test`; no pgvector extension is needed.
This research schema does not run production migrations. Generate a private
32+ character encryption key file with restricted Windows ACLs. Pass only file
references; never print secrets or load them into chat/model context.

```powershell
$env:BUTLER_EFFORT_LAB_ENABLE='synthetic-local-only'
$env:BUTLER_EFFORT_LAB_POSTGRES_URL='postgres://postgres@127.0.0.1:55449/effort97_test?sslmode=disable'
go build -o .cache/effort-eval.exe ./cmd/effort-eval
# Import an existing operator stream through the normal encrypted PG Manager:
.cache/effort-eval.exe -action prepare -key-file <private-key-file> -import-file <private-import-file>
# Or make a new local browser login with a privately supplied native app reference:
.cache/effort-eval.exe -action login -key-file <private-key-file> -oauth-client-file <private-client-file> -login-url-file <new-private-url-file>
.cache/effort-eval.exe -action inventory -key-file <private-key-file>
.cache/effort-eval.exe -action preflight -key-file <private-key-file> -trial <new-preflight-id> -source <code-identity> -out <new-directory>
.cache/effort-eval.exe -action run -key-file <private-key-file> -trial <new-trial-id> -source <frozen-commit-and-binary-identity> -out <new-directory>
.cache/effort-eval.exe -action export -key-file <private-key-file> -trial <existing-trial-id> -out <evidence-directory>
.cache/effort-eval.exe -action summarize -out <evidence-directory>
```

Browser login uses OAuth state and S256 PKCE with a local-only callback, fixed
Google endpoints, no redirect following during exchange, no raw error logging,
and the existing strict encrypted ImportAntigravity. The native app reference
is private operator material, not a product config/environment credential.
Only one refresh-capable process uses this isolated stored credential at a time.

## Evidence and validation

Admission atomically reserves a finite call before constructing/dispatching the
provider and inserts an UNKNOWN attempt. Process loss cannot turn it into zero
or replay it; reopening the same trial fails. Every terminal attempt retains
phase, safe error class/reason/status, latency, raw synthetic output, observed
version and nullable input/output/total/thinking counts. A missing thinking count
stays null. Invoice, quota depletion, cache allocation and currency cost remain
unknown. The full repeated context is included in each actual provider usage.
No actual inference occurs inside a PG transaction.

The summary preserves failed spend and unknown-count denominators, with nearest
rank p50/p95 over all observed slots. Correct refusals/clarifications remain
separate. It also writes a profile-blinded free-text review file and a separate
mapping; do not use Gemini to judge its own answers. There is no human gold claim.
`summarize` reads only safe exported files and requires no auth or database.

On Windows, prefer `scripts/dev/effort-research.ps1`: Validate/Build are offline;
native Inventory/Login/Preflight/Run require explicit `-EnableNative` and private
file references. Run/Preflight require a committed clean source and automatically
record its commit and rebuilt binary hash. The Linux race gate is
`scripts/dev/effort-research-tests.sh`; its two unselected historical memory
fixtures are explicitly documented. The separate actual adapter/memory PG test
can run in an independent PG+pgvector schema without inference.

Tests cover stop, deadlines, no progress, contradictions, source/claim separation,
goal amendment, evidence ablations, malformed/duplicate JSON, equal frozen slots,
native profile/schema construction, secure OAuth callback and real concurrent PG
admission/restart/closure. Required PG validation sets the loopback DSN above;
ordinary test discovery otherwise skips the explicitly named PG case.

The experiment does not change the working service's model, settings, auth,
queue, memory, channel profile or deployment. Promotion into the production
Orchestrator and actual Broker/tool acceptance would be a separate implementation.
