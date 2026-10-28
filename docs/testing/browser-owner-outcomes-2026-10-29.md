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

Native observations are in progress; this document will retain every accepted
trial, including failures and waiting states, before reporting qualification.
