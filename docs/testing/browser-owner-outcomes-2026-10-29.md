# Browser outcome review and native continuation

Implementation slice of #163, stacked on #176. General browser acceptance is
still NO_GO; native POST/login qualification is recorded separately below.

## Implemented boundary

The kernel can suspend the same Input after an opaque click/fill/type and a
separately confirmed, digest-correlated `browser.freeze_snapshot`. The protected
owner API accepts a factual outcome report, separate from action approval. Known
reports require checked recipient, settled processing and a receipt reference;
only digests are stored. `owner_outcome_attestation_v1` remains explicitly owner
assisted. No DOM acknowledgement, LLM verdict or fixture oracle automatically
reconciles an effect. The original grant, UNKNOWN tool transcript and spending
remain; unknown/expiry/cancellation/version changes cannot resume inference.
Known canonical evidence can resume only the same current Input/run, with fresh
provider context and ordinary claim/lease/source/task/budget guards. No action
replay or automatic task completion is introduced.

Historical review additionally requires a terminal run, a correlated locally
acknowledged action and an expired recorded browser context. It appends factual
evidence without reopening the old Input/run. The UI exposes an explicit outcome
choice and separate factual checks; drafts clear on owner changes and ambiguous
POST acknowledgements never cause automatic resubmission.

## Verification

- Full source: **124 Go test/subtest pass events, zero failures/skips**, isolated
  pgvector17, Go1.27.2, race detector; vet passed. This includes actual Core
  recovery with real PostgreSQL/claims/leases and a provider boundary that stops
  before inference, immutable original transcript/grants, foreign/stale/forged
  evidence denial, unknown/expired/cancelled/changed-task denial and existing
  stopped-attempt/approval/restart regressions. It is not a native model test.
- Web: typecheck, TypeScript/CSS lint and three real Chromium UI scenarios pass:
  known factual report, unknown after a drafted known choice, lost report ACK
  without automatic POST retry. API responses are stubbed in these UI checks;
  actual owner API/PostgreSQL behavior is verified separately.
- Reconstructed minimal Core: **105 Go test/subtest pass events, no skips or
  failures**, same PostgreSQL/race/vet checks and required test-name assertions.
  Core, Broker and browser compiled. The browser delta is #176's frozen reader.

Raw proof roots on `laba`:

- `/srv/storage/butler/agents/server-browser-owners-20261029/reports/20261028T183630Z-2555201`
  (minimal Core);
- `/srv/storage/butler/agents/server-browser-owners-20261029/reports/20261028T184356Z-2591362`
  (full source plus historical GET compatibility);
- `reports/web-20261028T183354Z` (typecheck/lint/three Chromium cases).

Earlier failures remain retained: an initial historical-test fixture used a
parameterized multi-command SQL statement; a full-schema fixture omitted required
completion criteria; offline npm installation lacked a cached tarball; a broad
regression omitted its memory-test DSN and skipped one required case. All were
corrected before acceptance. One first reconstruction script masked patch errors
inside a Bash function and built the unchanged Core. That build and its empty
owner-test selection are **invalid**, despite their old success text, and were
never deployed. The builder now returns on each failed reconstruction command;
validation requires actual named owner tests. Successful proof roots above are
different, fresh runs. No failed native trial was replaced to improve metrics.

## Lab deployment and provenance

The existing lab Core source (47f9429c plus the separately recorded GET-evidence
and authority deltas) was reconstructed and given only this outcome slice.
Recovery's added branches were adapted to that older Core's existing terminal
write interface; its unrelated newer refactor was not deployed. The browser and
Broker use their actual retained GET source plus the frozen-reader delta.
The [source manifest](browser-baseline/owner-outcomes/source-manifest.json) and
[binary hashes](browser-baseline/owner-outcomes/binaries.sha256) identify the
actual backport; they do not claim that the lab runs the complete PR HEAD.

Migration105 was installed with the existing Go migrator. Exact binary hashes,
health and unchanged private environment were checked after replacing Core,
Broker and browser. PostgreSQL, Redis and Web containers were unchanged. The new
owner-review UI is implemented/tested in this PR but is not part of this backend
lab rollout. Protected API tests use the real owner-control path. Previous
binaries/configuration and additive migration/evidence remain available for
rollback. No budget, provider/model selection or permission policy was reset.

## Historical blockers resolved without replay

nav-02's original locally acknowledged fill remains in its original UNKNOWN tool
transcript. Its recorded context expired, the recipient process is stopped, its
fsynced journal is empty, and its pinned public catalog has no input handlers or
write route on GET. An explicitly counted test operator used the new protected
owner API to attest absence of an **external** effect. This does not deny that
the local input was filled and is not an autonomous machine receipt. Journal
evidence now records `effect_absent`; Input/run remain failed and the stopped
classification became known. See the
[content-free owner report](browser-baseline/owner-outcomes/historical-nav02-owner-assisted.json).

Two revision1 GET compiler failures (control04/05) also retained stopped write
isolation. The exact historical contract is reproduced from 1e24a910 and pinned
as `504555e5109e2f7478d87fc21a5598c28f1f27b3aaa58830851675cab9bff90f`.
Existing kernel-only stopped review now supports this additional no-write proof.
A real rollback dry run preceded exact application to those two Inputs. Their
effect/dispatch stay UNKNOWN, Input/run stay failed, no read success is invented
and no action or model is replayed. Immutable no-write evidence releases only
their stopped write isolation. The unrelated owner's other-session stop was not
modified. See the retained [dry run](browser-baseline/owner-outcomes/historical-get-read-dry.jsonl)
and [application](browser-baseline/owner-outcomes/historical-get-read-apply.jsonl).

Before the new POST window, ordinary queue preflight found zero active owner
Inputs and zero uncertain stops in the original browser-baseline session.

## Native POST/login window

The original unused matrix slots are used; old trials are never replayed for
missing timing data. Prompts, provider/model and ordinary discovery/Broker paths
remain unchanged. Each trial receives a fresh recipient/config/journal. Any
exact action approval or factual test-operator report is counted as assistance,
separately from autonomous completion. Protected credential consent remains
subject to the owner's real vault PIN; no PIN reset, guessing or grant bypass
is permitted.

Prospective timings use persisted Input acceptance through terminal observation
and delivery, plus complete inter-tool intervals. Thus model deliberation,
context/queue/owner waits and tools all remain inside elapsed time. Tool-only
durations are secondary. Costs or unavailable usage remain unknown. Existing
20-trial timing exports are unchanged.

## Native failures, assisted completion and inclusive timing

These are the original matrix slots, not retries replacing failures:

| Trial | Input elapsed, seconds | Result |
| --- | ---: | --- |
| form-01 | 155.236 | FAIL: revision2 stale preparation became UNKNOWN before POST; recipient empty. |
| form-02 | 120.259 | FAIL: generic budget guard still retained form-01's independently reviewed no-write uncertainty; no POST. |
| form-03 | 501.850 | Assisted PASS: exactly one accepted POST, quantity3; same Input/run resumed after factual owner outcome report; final Web result and Telegram delivery verified. |
| login-01 | 47.381 | FAIL: invalid native tool request before browser/vault access. Exact rejected provider payload was not retained; the validation rule was not weakened. |
| login-02 | 1310.170 | FAIL: two access requests expired during owner/network waiting; a third attempt had UNKNOWN dispatch and no owning request row. No credential grant or login. |
| login-03 | 117.727 | FAIL: the third login-02 request's retained uncertainty blocked its access request before dispatch. |
| consent-01 | 718.842 | FAIL for complete task/delivery: real owner consent, exactly one accepted login and authenticated account read; correct private reference in unpublished draft, but publication refused an unselected Catalog source ref. |
| consent-02 | 734.176 | FAIL: execution_stopped after owner approval and snapshot, before credential preparation or POST; request expiry/cleanup is included. Receiver journal empty. |
| consent-03 | 91.662 | FAIL: invalid_tool_request after three discovery searches, before browser/vault access. Receiver journal empty. |

The original 3×5 matrix has now attempted all 15 slots: three read completions,
one assisted form completion, and 11 failed complete-task slots. consent-01's
successful login/private-data read is a partial result, not another completion.
The nine newly attempted form/login/consent slots and 79 tool spans are retained
in [inclusive final timings](browser-baseline/owner-outcomes/timings-inclusive-original-final/README.md).
The earlier six read/navigation slots and their existing measurements are not
replayed or replaced. These heterogeneous observations are not an autonomous
success rate or a latency SLO.

consent-02 retains the coarse execution_stopped error; the exact failing branch
was not established and is not inferred from its last checkpoint. consent-03's
rejected provider payload was not retained. Neither failure justifies relaxing
tool validation or credential policy. Real owner consent in consent-02 did not
produce any credential preparation, browser POST, publication or delivery.

form-03 required four exact action approvals and one factual test-operator
report. Its receiver journal contains exactly one accepted form event at
02:32:44.543267232Z. Publication
`publication:61c356287c88f415db52eb7ac0e4bd49c709d7bd15be6915357950db33a6e1a8`
has a Web result and sent Telegram outbox record. Private verification checked
title/quantity/saved wording and absence of credential canaries without exporting
answer/fixture values. This is owner-assisted completion, not autonomous browser
qualification. Tools consumed3.058s of its501.850s Input interval; inter-tool
decision/context/queue/owner waits consumed493.917s and the terminal tail4.875s.
These intervals do not isolate pure model thinking CPU time.

login-02's full elapsed time includes the network failure, owner setup and all
model/owner waits. It was not rerun after fixing the Windows route. Clash's
current-profile private-address DIRECT overlay restored Chrome access; no Butler
authentication, PIN or permission was disabled. These host routing changes are
not part of the product patch or a browser-acceptance result.

## Follow-up fixes and owning credential review

Preparation revision3 adds a closed pre-input rejection protocol: only correlated
guards before input dispatch can prove local absence. Revision2 lost ACK remains
unknown. The exact historical ordinary-text revision2 preparation can separately
prove no external write without claiming local field success. The budget loop
independently rechecks that immutable proof for every scoped uncertainty. Both
Core and Broker need its reader. PG/race/vet checks recorded47 and96 passing
test/subtest events respectively, zero failures/skips. Real runtime Chromium and
Broker/PostgreSQL/Chromium sequences passed; initial native failures remain.

Credential local preparation now accepts revisions2/3 with identical current
owner-request/record/origin/run/Input-claim/revocation boundaries. Owner consent
does not authorize submission. The current minimal Core additionally needed the
same revision3 local-success policy/ACK compatibility; its exact narrow backport
was tested before deployment.

The pinned historical `credentials.request_access` contract owns one atomic
request/audit transaction. Under its original terminal session/run/Input locks,
absence of the exact call's request proves local ABSENT: the creator takes those
same session/run locks and requires a nonterminal run. An expired, never approved
or notified existing request can prove creation only after exact metadata/hash/
audit checks. A pruned browser context requires the separate confirmed original
open and a digest-matching expired saved result. This is a kernel operator review,
not an owner decision, secret grant, login receipt or automatic fixture oracle.

Actual login-02 review found two different expired request calls and no record
for its third UNKNOWN attempt. A real rollback dry run predicted known; exact
application appended `stopped_access_request_absence_v1`. Input/run stay failed,
the two existing requests stay expired, consumed UNKNOWN grant/dispatch and all
charges remain. Its stopped classification is now known. Other calls' rows were
not borrowed as receipts. See the retained
[dry run](browser-baseline/owner-outcomes/login02-expired-access-drained-v6-dry.jsonl)
and [application](browser-baseline/owner-outcomes/login02-expired-access-drained-v6-apply.jsonl).

Final PG/race/vet qualification:151 pass events, zero failures/skips, including
terminal/foreign/stale/malformed/approved/notified/released denial, pruned-context
digest checks, real Vault creation without granting an expired request, and
existing budget uncertainty regressions. Proof root:
`/srv/storage/butler/agents/server-browser-owners-20261029/reports/expired-access-20261029T034040Z-718205`.
An earlier147-event window also passed the real Broker/Vault control integration.
Minimal deployed Core and Broker separately passed revision2/3 owner-grant,
claim replacement, revocation and submission-boundary tests and vet/build.

Failed qualification attempts are retained: a no-test package was initially
treated as a skip; a minimal Core test found its remaining revision2-only success
verifier; later compilation hit the existing per-user tmpfs quota. No failed
native result was replaced, and no global quota or journal was cleared. The final
isolated build used finite CPU/memory, no network and read-only source mounts.
Initial operator reviews refused lock contention or insufficient context proof.
A short no-active-Input drain avoided background session-row contention. One
candidate rollout correctly restored the previous binaries after an unresolved
dry run. Only a later known rollback dry run was applied.

Current narrow backend release:
`/srv/storage/butler/main/releases/browser-credential-revision3-20261029-65b3d7230556-v6`.
Core/Broker health and binary identity were checked; private environment and
Browser/PostgreSQL/Redis/Web containers are unchanged. No migrations, PIN reset,
budget reset or wider permission were introduced. The historical credential
review ran through the one-shot full-source kernel operator; this does not claim
that its writer or the entire PR HEAD is deployed in the minimal Core.

consent-01 used the real owner PIN/decision, two successful deferred secret
preparations and a revision3 pre-input rejection/reassessment between them.
The recipient recorded one accepted login and one authenticated account read.
An operator factual report continued the same Input after the opaque click;
the original UNKNOWN tool transcript remains. Private comparison confirmed the
unpublished draft contains the exact requested private reference and no username/
password canary. No publication or delivery exists, so the complete slot fails.
This used three assisted decisions: owner consent, exact submit approval and
factual outcome report. Its correct draft is not counted as a delivered answer.

The publication carried an unselected `message:` source ref. The exact owning
revision2 message contract was reproduced and its atomic fenced publisher
reviewed. A rollback dry run followed by exact application proved only local
publication ABSENT. The failed Input, original UNKNOWN tool/grant/dispatch,
charges and successful receiver login remain. No answer or login was replayed.
The schema description now explicitly distinguishes selected Catalog source refs
from event/turn IDs and browser observations; no source/privacy gate was relaxed.

Final additional qualification (including both pinned message revisions and
wrong-revision/owner/receipt/claim/lease denials):189 PG/race pass events, no
failure/skip; vet passed. Proof root:
`/srv/storage/butler/agents/server-browser-owners-20261029/reports/expired-access-20261029T040542Z-827701`.
An earlier publication fixture paired the revision2 hash with revision1; actual
dry-run review refused it. The pairing and negative test were corrected before
application. The first source-hint rollout restored the previous binaries after
that unresolved dry run. Current release is
`/srv/storage/butler/main/releases/browser-source-hint-20261029-v8`; it contains
the tested narrow Core/Broker guard changes and source-ref description only.
The operator contains the fuller historical review. Browser/PG/Redis/Web and
private environment remain unchanged. Immutable candidate binaries were linked
between release folders to preserve backups without duplicating large files.

All 15 original matrix slots are retained. General acceptance remains NO_GO
pending a complete delivered login/private-data result and the remaining #163
criteria. Additional fault-injection observations are recorded separately below;
they cannot replace failed matrix slots.

## Additional lost-response trial (outside the matrix)

`post-loss-01` used one ordinary native Input, accepted at
04:25:42.804159Z, and the same provider/model and Broker paths. The synthetic
HTTPS recipient exposes a real GET form at `/lost-response`; GET applies no
effect. A valid POST fsyncs its journal before closing the response connection.
Every duplicate would create another journal event. No receipt/config/reset
endpoint, browser script injection or HTTP substitute is supplied to the agent.

The GET form addition passed the recipient package on Windows (5.136s) and Linux
(1.581s), plus Linux vet/build. Its HTTP unit deliberately sends two POSTs to
verify that duplicates remain visible; these are unit checks, separate from the
native trial. Only the new exact systemd instance uses recipient binary
`ec12a4169819040d0afc86e388d8fe8a1026e6c761e9040aa13ac74b8cdfbce3`.
The prior global unit/binary and every earlier journal/config were preserved.

The native receiver recorded exactly one accepted form, quantity 3, at
04:32:09.681500897Z. The agent's single click retained `effect_unknown`, then a
confirmed frozen snapshot suspended the same Input for factual review. After
independently checking the receiver journal and synchronous handler, the test
operator reported `effect_confirmed` through the protected owner API. This is
explicit assistance, not an automatic oracle or machine receipt. The original
UNKNOWN click remains in the final transcript, and no POST was repeated.

The same Input resumed, closed the browser, corrected one locally rejected
communication draft and published
`publication:da349ff9a858871913ab9b49c5aa61515ff9303a0b43e333e2896654dd9f9267`.
Web has `result_ready` and Telegram has a persisted `sent` acknowledgement.
A protected owner GET verified the publication; private checks found the public
title/quantity and saved wording, with no credential canary. The answer also
contains uncertainty wording: these bounded text checks do not prove that its
explanation is perfectly unambiguous. A delivery API acknowledgement does not
prove that the human read it.

This qualifies the assisted no-duplicate/factual-review/continuation/delivery
path for this one fault injection. It does not qualify autonomous outcome
resolution, a delivered credential/private-data task, CAPTCHA/SSO, or all of
#163. Assistance: four exact action approvals (including a failed pre-input
preparation) and one factual report. Input wall time is **573.790s**, including
model deliberation, context/queue/operator waits and tool work: 544.486s of
inter-tool gaps, 11.763s of tool spans and 17.541s of finalization. Telegram sent
was observed at 558.600s from acceptance and overlaps that interval.

See [native metadata](browser-baseline/owner-outcomes/post-loss-01-metadata-final.json),
[receiver](browser-baseline/owner-outcomes/post-loss-01-recipient-final.json),
[factual report](browser-baseline/owner-outcomes/post-loss-01-owner-outcome.json),
[private comparison](browser-baseline/owner-outcomes/post-loss-01-verification.json)
and [timings including the extra case](browser-baseline/owner-outcomes/timings-inclusive-with-extra-fault/README.md).
That second timing export contains 10 trials and 94 tool spans; only nine trials
belong to the original matrix. The original-only export remains separate.

Remaining blockers are the native invalid-tool-request failures, the unexplained
consent-02 execution stop, reliably publishing/delivering a complete protected
login result, and the remaining restart/cancellation/revocation/native criteria
of #163. Existing component checks do not close those acceptance requirements.

Later prospective qualification is recorded in
[login fixes](browser-login-fixes-2026-10-29.md): a new diagnostic trial identified
an invisible native array-bound constraint, its presentation was fixed without
widening validation, and a separate assisted login/private-data request completed
with exact answer verification and Web/Telegram delivery. Those additional trials
do not replace the original failed slots or rewrite the earlier blocker snapshot.
