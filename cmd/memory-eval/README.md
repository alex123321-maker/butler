# Memory evaluation

`memory-eval` extends issue33's `internal/acceptance` contour. The command freezes,
scores and audits offline. The opt-in orchestrator test collector composes current
Catalog, structured Parser, owned Index Runtime, HybridReader, BuildBundle,
ownership, PostgreSQL budgets and native transport. It adds no service or executor.

## Independent data review

V1 remains an immutable offline fixture with independent NO_GO: its controls
mirrored development templates. Two rejected pre-observation V2 proposals remain
in `testdata/acceptance/reviews`. Accepted V2 has24 synthetic cases,12 development
and12 controls,60 globally distinct originals. A separate user-authorized reviewer
inspected all originals, clocks, alternatives and citation spans and independently
reproduced suite/criteria digests. This is shared-host role independence, public
controls and a conservative lexical rubric, not blind/statistical readiness.
Do not tune/select against these controls; subsequent selection requires a fresh
control version and review. Real cases require separate human-checked annotation.
`behavior_refs` link issue76 categories instead of creating another error catalog.

## Zero-inference checks

`make test-memory-eval` is the small mandatory contract/CLI set. On laba:

```powershell
pwsh -NoProfile -File scripts/dev/lab.ps1 -Action Sync `
  -RemoteRoot /srv/storage/butler/agents/memory-eval-78-20261014
ssh laba 'bash /srv/storage/butler/agents/memory-eval-78-20261014/current/scripts/dev/memory-eval-verify-lab.sh /srv/storage/butler/agents/memory-eval-78-20261014'
```

The isolated PG/race script uses fake inference for complete three-arm collection,
actual parser/index/bundle/budgets, malformed originals, supported refusal versus
guessing, failed spend, task/lease/source races and original ledger correlation.
It mounts no credentials and sends no local/native inference requests.
Its PostgreSQL data is bound below the isolated proof directory on storage,
rather than an anonymous Docker volume on the system partition.

## One-use model comparison

Human authorization of a fresh window and final independent runner review are
required. Prepare pins a clean source commit, binary, exact image IDs, hardware,
data/criteria, runtime and resources. An external operator seal accepts only
independently approved bytes and discards its temporary signing key. Then:

```sh
SRC=/srv/storage/butler/agents/memory-eval-78-20261014
RUN=/srv/storage/butler/agents/memory-eval-v2-native-20261015
# COMMIT is the exact clean40-character candidate commit; RUN must be new.
bash "$SRC/current/scripts/dev/memory-eval-model-lab.sh" "$SRC" "$RUN" prepare COMMIT
bash "$SRC/current/scripts/dev/memory-eval-model-lab.sh" "$SRC" "$RUN" execute COMMIT
bash "$SRC/current/scripts/dev/memory-eval-model-lab.sh" "$SRC" "$RUN" audit COMMIT
```

An atomic entry marker, fresh PG and monotonic closed-window row deny reopening.
Actual command/UID/read-only mounts/CPU/memory/PID/image pins are checked before
credentials. Secrets remain mounted private references resolved by existing
credential code. Main database, deployment pointers and credential files stay
outside this experiment. Timeout stops the runner, preserving its DB/artifacts.
The SSH operator may remain UID1000 without sudo. Named output hashes/readback
and initial permissions use confined network-free Docker helpers; credential
readability is checked by UID10001 without reading credential contents.
Before the entry marker, a compiled metadata-only entry authenticates Describe
on `butler-lab_local-inference` and checks the exact frozen profile; Infer is
disabled. The actual runner joins the fresh PG network, `butler-lab_backend`
for native transport, and `butler-lab_local-inference` for the local worker.
All three exact network names and the fresh root-derived window ID are audited.

Frozen limits: High `gemini-3.8-flash-high`, required observed version
`gemini-3.8-flash-n`;280 new inference admissions;4,000,000 conservative shared
units; at most66 native samples before planned skips;3 hours;2 CPUs,1536MiB,
128PIDs;4096 safe events per entry and128MiB reports. Every arm has32KiB complete
serialized input cap,32768 generated tokens including thinking,8KiB final bytes.
Reservation covers JSON escaping/schema/framing and is enforced by the same
provider payload cap. Actual usage exceeding allowance fails closed. No retry,
resampling, repair, schema fallback or continuation. Failure preserves the prefix.

Order is fixed case then no_ltm/full_context/retrieval, not randomized. All current
retrieval captures freeze before native entry. no_ltm receives question/current
task only; full_context includes every allowed undeleted original, including
superseded historical DATA, or is explicitly skipped in full when oversized.
Secret cases block all arms before export. Retrieval receives the exact captured
prompt/source metadata without appended complete original bodies.

The bounded configuration uses current user-scoped semantic chunks, Gemma2,
structured-go-v1 fragments, chunk limit2, bundle budget12 and HybridReader's6144
byte context boundary. Profile/episodic/working/summary stores are not configured;
reranking/extraction/summary/consolidation are off. This is current local retrieval
composition, not full production bundle coverage. Future classes must retain
every backing source/revision/claim; retrieval DATA grants no owner authority.

## Original evidence, expenses and scoring

Each sample retains exact input/instructions/native schema, raw UTF-8 final with
explicit encoding tag, ordered safe events, actual version/input/output/total
usage, unique native/auxiliary reservations and current dispatch/final receipts.
Valid JSON strings and malformed outputs remain distinct negative originals.
Current source/task/lease checks run before authentication/HTTP, during inference
and before acceptance. Spend persists before retention/source errors. A safe SQL
allowlist excludes settings; raw HTTP, reasoning, auth and provider error bodies
never enter reports. The offline auditor checks originals, freeze, all owning
rows, source/alias/owner bindings, revisions/constraints/leases and runtime pins.

All nine stages are measured, unknown or not_used, including failed prefixes.
Source write/index/read setup is attributed once to that case's retrieval arm;
native input reading and artifact retention belong to each arm. Session/task/
guard/ledger overhead not separately observed is unknown, not not_used. Off
workers and zero retries are explicit not_used. Generation total includes hidden
thinking tokens. The worker's actual embedding tokenizer counts and subscription
invoice remain null; a zero admission quote is not a measured zero invoice.
Local invoice has explicit no-billing basis. Host energy and physical PG/WAL/
backup footprint are unknown. Known storage is a scoped source/chunk/index-row
and artifact-byte subtotal. Unknown measurements deny complete-cost claims.

Per-sample p50/p95 cover owned input/admission through native completion; the60s
gate applies there. Diagnostic pipeline p50/p95 add one-time write/index/query
capture only to retrieval, without hypothetical amortization. Stage intervals
can overlap and must not be summed as elapsed or billable duration.

Alternatives normalize lowercase/whitespace only, frozen before outputs. Exact
quotes must anchor both original and actual prompt and include reviewed entailing
spans; larger quotes/order are accepted, with the reviewed minimum source set.
This is a conservative finite rubric, not a general semantic paraphrase judge.
Truth and context-supported correctness are separate. Unsupported exact guesses
are false assertions. Safe missing-context refusal is correct but cannot recover
an answerable fact; an empty retriever cannot pass90% actual-fact usefulness.
Task prose and action code must agree with current instructions. Malformed output,
calibrated confidence/Brier, false refusal and recall/MRR are separate metrics.

Frozen policy:90% answer/current-fact and90% instruction, zero unsupported
assertions/critical violations,60s p95,280 calls,4,000,000 units and3,000,000
monetary microunits. Unknown invoice/effects deny promotion. QA/proposed actions
do not prove file/repair/delivery or long-lived task completion; actual outcomes
remain null without independent genuine effect evidence. Negative observations
are preserved. Harness acceptance, model-quality pass, complete cost, actual user
outcome and promotion are distinct. Offline scoring cannot prove native execution;
read-only audit can certify correlated observations but cannot execute or promote.

The retained90ed model window completed53auxiliary calls and then failed its
first native entry. Pending-only continuation is separately specified in
`docs/ai/memory-eval-v2-pending-protocol-20261015.md`: pass the exact original root
as the preparation script's optional fifth argument and use a different new
root. It reuses original captures without inference, carries spent counters
and the original absolute expiry, and audits with `pending-audit`. The original
failed slot remains in the scoring denominator. Never repeat an eligible paid
slot or treat its unknown usage/cause as zero.

That pending root also retained a failed full_context admission. A prospective
residual62 is finite, separately reviewed and preserves both eligible failures,
all183existing expenses,55/614740 and the original expiry. Its filtered parent
snapshot is limited to two failed prefixes; a deeper chain is rejected. It uses
the original pending root as the fifth argument and a new residual root, with
window-scoped setup expense IDs. HTTP rejection diagnostics may retain numeric
status and fixed protocol reason only. No earlier unknown is inferred from them.

After the retained third failure, a separate prospective route correction is in
`docs/ai/memory-eval-v2-proxy-correction-protocol-20261015.md`. It follows the
established native laba proxy route, requires safe runtime proxy/bypass proof and
the native runtime's strict encrypted auth store. No refresh-capable disposable
metadata probe runs before entry. It carries56/660396, all236expenses
and3failedslots into a three-level parent snapshot, admits only61unpaid entries,
and retains the original expiry. Fourth-depth lineage is rejected. This needs
new independent source/runtime review; old gates do not grant this entry.

The closed proxy-safe root subsequently retained54complete answers and one
120-second provider timeout. The separately reviewed final-six protocol in
`docs/ai/memory-eval-v2-final-six-protocol-20261015.md` supersedes its parent-depth
limit for one new6-slot root. All58paid inputs and63consumed observations,
including5planned skips, remain immutable; only6unpaid inputs plus3remaining
skips execute. Four closed parent levels are supported and a fifth is denied.
Aggregate111/3167573 carries the original absolute expiry; final planned
117/3442427 is unchanged. The separate negative cost score becomes401/280.

The final-six window subsequently sealed after h11 no_ltm returned only
provider_unavailable in10036ms, without observed HTTP status/cause/version/usage
or final. Its final counter is112/3212748; no further entry is granted. The
retained comparison has54complete answers,5paid failures,8original planned skips
and5unobserved eligible inputs, across the original72-input matrix. All937expense
records and391stage calls remain, including the failed admissions. The public
[partial score](../../testdata/acceptance/results/ru-memory-v2-partial-20261015.json)
is NO_GO/model_quality_evidence=false; it cannot establish full PR acceptance.


## Renewed five-input window

The explicit owner-authorized renewal binds a new30-minute deadline, exact five
unpaid inputs, parent pins and accumulated spend. See
[the prospective protocol](../../docs/ai/memory-eval-v2-renewed-five-plan-20261015.md).
After prepare and independent source/prepared review, auth-preflight performs
zero model calls in a separate fresh encrypted PG auth database on the new
window server. Execute reuses its persisted refresh rotation. Its latency/probe
counts are included as background expense with unknown monetary cost. Execute
is one-use and never resamples a spent input. Offline pending-audit requires all
five attempts, including safely retained provider failures; numeric NO_GO and
promotion refusal remain possible after complete harness acceptance.
