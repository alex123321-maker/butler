# Issue #147: current request and historical answers

The change adds an explicit model-visible boundary between an admitted current
owner request, an accepted answer to its exact question, and historical messages.
It extends PR #150; it does not replace its recent-exchange selection, recovery,
publication correction or stopped-attempt handling.

## Scope and implementation

- Base: PR #150 `8aa2c0450ee1d200093c691ece6a7670c2143cb5`.
  The branch was updated from the earlier `3cd09d7e` base before final acceptance.
- `current_request` DATA identifies the owning run, original input event and
  `owner_request` / `answered_question` phase. Only canonical response validation
  attaches the exact interaction ref. Payload-supplied identity/phase is ignored.
- Historical messages retain sanitized content and provenance, with original
  run, role and message kind in their DATA envelope. Those fields survive actual
  provider serialization, which does not carry arbitrary InputItem metadata.
- Fixed guidance distinguishes an explicitly requested new question from an
  answer to the same request. Answers and history cannot grant tool authority.
- Preparation persists in the existing checkpoint. Fresh provider reconstruction
  refreshes tools and the scope marker without duplicating it. Scheduled,
  initiative, delegated and converter work do not become new owner requests;
  credential waits do not become preference answers.

No transcript/session is cleared, query substituted, canonical answer rewritten,
budget reset, authority bypassed or uncertain action replayed by this change.

## Verification design

The deterministic regression feeds an identical old/new request, an old answer,
an unrelated topic and forged payload refs through actual preparation. It fails
on the latest PR #150 (`marker=0 old=false current=1`) and passes with this change.
Additional tests cover persisted answer scope, repeated refresh, background
exclusion, provenance, complete historical text and the actual native HTTP body.

The isolated lab uses PostgreSQL 17/pgvector, real durable Input/Session/Core,
canonical communication store, Catalog pinning and a separate Broker process.
The zero-inference integration test checks all four request boundaries and all
three accepted interaction refs through fresh Core instances.

Native arms use the existing encrypted provider reference through its ordinary
auth manager and proxy. Configured route: `antigravity:gemini-3.8-flash-high`;
reported version: `gemini-3.8-flash-n`. No credentials are copied into prompts or
reports. Both arms receive identical frozen Russian requests and synthetic owner
answers in one retained conversation per arm:

1. Ask a new multi-choice fruit question; accept apples and plums.
2. Repeat the exact same request; require another question and accept pears.
3. Ask a text question about a tablecloth; accept the white linen cloth with a
   green border. Require only that answer, without old fruit details.
4. Explicitly recall the latest tablecloth answer without another question.

Every wait resumes through a new Core service and fresh provider context using
the saved checkpoint. Source references and publication transactions are real;
answers are submitted by the test through the canonical owner response API.
This is live-model integration, not a human Telegram/Web UI acceptance or a
deployment of the candidate to the shared owner profile. Channel export policy
is a synthetic owner allow-rule and Web publication is stored as result-ready;
no external Telegram delivery or client-read receipt is claimed.

Both arms have a finite 16-call allowance, full provider token reservations and
one conversation ledger across the four requests. The direct Core harness has
no worker lease heartbeat, so it uses a finite 300-second lease. Ordinary
production policy values are untouched.

## Results

Final native results, safe request/interaction refs, source/binary hashes and
raw-evidence digests are in
[acceptance.json](../evidence/issue147-current-request-20261025/acceptance.json).

| Qualified arm | Scenarios | Model calls | Input / output tokens | Provider total tokens | Unknown usage pairs |
| --- | --- | --- | --- | --- | --- |
| PR #150 baseline | 4/4 | 13 | 81,443 / 884 | 112,260 | 0 |
| Candidate | 4/4 | 11 | 64,525 / 855 | 81,997 | 0 |

Both published exactly `Яблоки, сливы`, then `Груши`, then the accepted tablecloth
answer twice. The candidate's seven initial/resumed provider requests contain
four `owner_request` and three `answered_question` markers; the latter name the
canonical resolved interactions. Its 36 historical item appearances carry run
labels, with full text retained. The baseline has the same selected-history
mechanism but no new typed envelopes. No missing history is inferred from its
zero envelope count. Baseline needed two existing missing-publication corrections;
candidate needed none in this sample. These counts are not a performance claim.

Validation passed: affected Windows unit suite (139 top-level passes, four
infrastructure-gated skips), targeted native-wire tests, narrow `-race`, and
orchestrator/content/transport/app vet. Isolated Linux/PostgreSQL checks passed
141 top-level cases, including source revocation, ownership fencing, canonical
answer recovery and wrong Input/revision denials. Two older tests requiring the
separate general PostgreSQL/Redis integration environment stayed skipped; they
are not included as passing. The separate new Core/Broker integration passed
all four requests and seven current-request boundary assertions.

The broader Antigravity suite has one inherited failure:
`TestCatalogSchemasAreSupported` rejects the existing `web.search` schema with
`unsupported schema keyword`. This was reproduced on the baseline; the other
transport tests passed when that named unrelated case was excluded. This PR
does not silently repair or claim to pass that existing schema check.

The baseline already passed all four scenarios; therefore this experiment cannot
establish a native failure-to-success improvement or universal elimination of
the intermittent issue. The added boundary is established by the deterministic
red/green regression and inspected provider context, with native compatibility
checked separately. Issue #147 remains open for review and further observation.

## Setup failures retained separately

Earlier native windows on `3cd09d7e` are excluded from semantic comparison:

- Initial window lacked `communication.widget` in the test Broker catalog:
  3 measured calls, 41,979 provider total tokens; it did not test the requested
  question/answer chain.
- `baseline-v2` and `baseline-v3` reached the canonical question/answer but lacked
  `communication.Store.Sources` composition. A correct model-supplied answer
  source reference was therefore not pinned for publication. Both stopped with
  `effect_unknown`; neither was replayed or counted as success. Respectively:
  2 calls / 10,961 total tokens and 2 calls / 10,266 total tokens.
- Zero-inference qualification then exercised publication with those exact
  canonical source refs after composing Catalog. All four cases passed.
- An initial candidate attempt exhausted the 768 MiB temporary PostgreSQL
  volume during migration, before inference. The dedicated disposable test DB
  was recreated with a 2 GiB volume and bounded WAL retention. No shared DB,
  owner history, runtime or budget configuration was changed. The failed window
  and zero-inference permission/DSN/storage preflights remain in local raw logs.

Unknown accounting is not converted to zero. Scripted fixture attempts have no
provider usage events and are reported separately from measured native calls.
Raw synthetic evidence/logs are retained under `.cache/issue147/native/`; the
checked-in summary omits provider credentials and private working-profile data.
Across all native windows, including excluded setup failures: 31 measured calls
and 257,463 provider total tokens. Provider total includes additional accounting
beyond the separately reported input/output counters; they are not equated.

## Reproduction

`pwsh -NoProfile -File scripts/dev/issue147-check.ps1 -Action Test` runs the
affected unit suite, native-wire regression and vet with repository-local caches.
`-Action Build` exports the pinned baseline and builds the Linux Core/Broker test
binaries; `Rebuild` reuses that export. The baseline receives only the same
test-only driver, not the candidate production change.

For an authorized lab, upload binaries and migrations to the dedicated path in
`scripts/dev/issue147-native-lab.sh`, run `setup`, then `plumbing`, `baseline` and
`candidate` in separate evidence windows. A started native window cannot be
overwritten. Native runs take the existing provider-qualification lock and stop
on unknown effects; they do not deploy over the shared service.

`scripts/dev/issue147-regression-lab.sh` runs the PostgreSQL regressions against
that same dedicated test database without provider credentials or inference.
