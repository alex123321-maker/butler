# Gemini ACP on primary laba: bounded acceptance

Date:2026-11-16, Asia/Tomsk. Issue:[200](https://github.com/alex123321-maker/butler/issues/200).
The owner explicitly selected opaque subscription budgets and primary laba.

## Deployed scope

The main Butler agent uses `antigravity-acp`, exact `gemini-3.8-flash`, through
the pinned official Linux CLI1.5.9 in a private non-root PID1 container. Core owns
context, authority, budgets, tools and recovery. The CLI returns decisions only;
native tools, MCP, plugins, skills, subagents and overage credits are disabled.
The Tool Broker, credential mediation, memory authorities and channel delivery
retain their existing contracts. No new Go dependency was added.

The foreground provider timeout follows the existing primary300-second setting,
with300-second protocol/process and305-second finite relay bounds. Earlier
scope/authority/caller deadlines still win. Logical input/output limits are2MiB
and64KiB. Each prompt spends a permanent reservation; native token observations
remain NULL. Opaque prompt limits remain strict even when the old native
conversation-turn opt-out is enabled. No paid/native foreground fallback exists.

Background memory remains explicitly configured on its previously qualified
native Flash High32768/300s profile through `BUTLER_MEMORY_MODEL_PROVIDER`.
Candidate/summary/consolidation/temporal capabilities must not silently disappear
when foreground changes. ACP does not inherit their native quality qualification.
This is an independent configured backend, not inference retry/fallback.

## Source and artifacts

Source worktree: `D:/Repository/butler/.cache/worktrees/acp-200`.
Base: `9ed43643e1f13654956bbb1ba79fe8be0e578fb6`; feature changes are uncommitted.
The primary checkout and its unrelated `prototypes/` were preserved. No PR,
commit or merge was created by this work.

Primary release: `/srv/storage/butler/main/releases/gemini-acp-20261116-200`.
Final effective configuration is its private `candidate.v6.compose.json`.
The operator pointer is `/srv/storage/butler/main/acp-active.compose.path`.
`source-final.tar.gz` is the final source snapshot; its digest is stored in the
private acceptance receipt. The older extracted `source/` directory belongs to
migration staging and must not be used as the final build source.
The original normalized configuration is retained as private `before.compose.json`;
the original native release and its credentials/profile remain available.
Application rollback retains migration112 and all attempts/spending/UNKNOWN.

| Artifact | SHA-256 / image identity |
|---|---|
| Core `core-budget-final/orchestrator` | `4dbadec5599a61383835384ea35b34545f1ac2bd319d67197e9c968710441293` |
| Broker `broker-final/broker` | `0d8da4cae05a2bb82c5452ccd4fd50a9c1d7c2b32c917f0a360c823cde4ffef7` |
| Isolated runtime image | `sha256:ee2b5f0d0317c478a4159e769816db2945fc252ef33ff97b1ae6b4a916c40dde` |
| Web output archive | `7dab20ce23f5873851d9306b32ff073e9ce165ac1bc031951dd12987b3c06b18` |

Both candidate directories preserve the original `tools.json` bytes. Core/Broker
communication-profile arguments and Telegram decoration flags remain intact;
the mounted profile digest did not change. Unrelated services/configuration were
compared before staging. Only Core, Broker, Web and the new ACP services changed.

## Executed receipts

| Case | Original identity | Observed result |
|---|---|---|
| Isolated real Butler text | `input-7b32a8632c9abcc0e99462e1dc28bb4d` | Completed; actual saved answer `ACP_BUTLER_OK: 42` |
| Isolated real doctor/approval/restart | `input-f6beb47a21aa876ac85d0e696c9b56c5`, `run-1794773639976597817-793d6b0003b7792a` | Core restarted at owner approval; actual Broker/doctor completed once, effect confirmed and dispatch completed; two opaque ACP turns; honest degraded report |
| Final local image and Core | `input-85e729cea52309ca7622e4808625cf24` | Completed through the ordinary kernel after final-image containment changes |
| Primary text/publication | `input-9ccc85de1a6a10bf0bce9ecf71d77a4c`, `run-1794776414397795534-96c224d6eb8b4f28` | Two real ACP turns, completed `communication.message`; `ACP_LABA_OK: 42`; Telegram `sent` with ACK; protected Web feed actually returned the marker |
| Primary deferred public Web | `input-bc1b022dbc6f44d69c1ce2f1413f3dea`, `run-1794777000857675390-516f0643492f2940` | Six real ACP turns; `tools.search`, `tools.load`, `web.fetch`, another discovery and `communication.message` completed; Example Domain HTTP200; Telegram ACK and Web result |
| Final Core/Broker after strict opaque-limit update | `input-cb523a9e9a8d2785d68cc62369867d37`, `run-1794777516852645796-4e3f923723c3c35a` | Completed; two ACP attempts with NULL native tokens; final publication `ACP_FINAL_LABA_OK: 42`; Telegram ACK and Web result |

The Web-tool case also searched for `memory.search`. Broker returned an empty
catalog because this was a taskless conversation; current memory tools require
Task/read authority. The model correctly reported that it did not execute that
read. This is retained policy enforcement, not a passed memory search or a reason
to widen authority. All-Task memory execution remains a separate live case.

The deployed ACP settings JS bundle was fetched from the actual primary Web
server and its digest matched the staged artifact. This establishes deployment,
not visual interaction QA. ESLint, Vue typecheck and Nuxt build passed locally.

## Required checks executed

- Real PostgreSQL opaque reservation/restart/unknown-usage/rollback tests.
- Real PostgreSQL strict ACP prompt limit with native conversation opt-out,
  across a new Controller; denied requests created no extra attempt. The old
  native opt-out regression still passed. Budget race checks passed.
- Real PostgreSQL owner-budget API test reports the independent opaque prompt
  limit. Provider API refuses unsupported Web OAuth and never invents connection.
- ACP/provider/config/factory and targeted Core composition/admission tests;
  canonical static catalog and memory read/temporal schemas; configured time bounds.
- Offline Linux subprocess/provider/config checks, ACP/provider race x2 and vet.
- Actual non-root Docker PID1 fixture killed/reaped a detached descendant.
- Actual Google TLS preflight from laba through the fixed allowlist/public-IP
  proxy, including OAuth, AICode, Cloud Code and account-image hosts.
- Official login on the new laba profile completed; a fresh process negotiated
  the exact model without dispatch before primary activation.

Final primary API checks returned `opaque_prompt_limit_enabled:true` and read
`ACP_FINAL_LABA_OK: 42` through the protected Web feed. Core, Broker and Web were
healthy. The isolated local stack and test databases were stopped after saving
the complete synthetic smoke database as the private ignored
`.cache/acp-release/isolated-smoke-evidence.dump`; Google profile volumes remain
private and were not copied to laba.

The full app suite is not claimed green: unrelated private-key/disposable-DB and
frozen native-history prerequisites reported earlier were not replaced.

## Retained failures and limits

- Earlier login/runtime windows, spent/unknown attempts and queued local cases
  remain in the evidence; failed windows were not relabeled successful.
- Direct AICode access on laba timed out. Only the Go egress sidecar now uses
  the existing operator HTTP proxy; Google destinations are checked public IPs,
  TLS stays end to end and the CLI remains network-none. No proxy config changed.
- The first cutover's readiness script could not read a root-owned host token
  and automatically restored the previous Core/Broker. Readiness was corrected;
  migration112 and history were retained. This exercised the actual rollback.
- The first primary test hit a missing `tools.json` staging file and failed
  before model admission. Original catalogs were then restored byte-for-byte.
- A broader30-second request timed out before tools. It retained its attempt;
  the next test used a new identity under the corrected300-second bound.
- Automatic review rejected deletion of the local T3 protected-code file with
  `blocked by policy`. The one-use code was consumed successfully; temporary
  copies on laba were removed. No code/token/raw CLI log was printed.
- No primary live browser/credential/schedule/delegation/revocation matrix is
  inferred from message/Web success. Vision, streaming and tool batches are not
  advertised. Background generation through ACP still needs its own quality gate.
- Google entitlement/quota and consumer-account terms remain separate from
  transport success; no account rotation, identity disguise or ban guarantee.
- Disk headroom on laba is critically low: approximately250MiB on `/` and534MiB
  on storage at the final observation. See operational follow-up
  [201](https://github.com/alex123321-maker/butler/issues/201); no unrelated data
  or Docker volumes were pruned.

## Extended acceptance on 2026-11-16

The old `butler-eval.report-template.ru` recipient no longer exists. No owner
account or replacement public host was required: an isolated HTTPS recipient,
private synthetic account and test-only resolver/TLS roots were used locally.
Production private-egress rules and Google profiles were not changed/copied.

- Primary public browser: input `input-2d27fdde84c5c73b54e53746036ae19d`,
  run `run-1794807477138444551-63ee239d0b113961`. Actual deferred search/load,
  `browser.read` and `communication.message` completed. Web retained
  `ACP_BROWSER_LABA_001: Example Domain`; Telegram is `sent` with an ACK.
  Five ACP attempts retain NULL native tokens.
- Primary schedules: creation input `input-e2fb6fdeb0b6c1e853e0052ffe459986`.
  `sch_cb53192982b8344c8813a948a6fdc531` was cancelled at revision2 with zero
  occurrences. `sch_7f333ce5241fc81e6436eae496392236` admitted exactly one
  ordinary Task/Input. Its first run failed *before inference or tools* because
  unrelated shared-chat history exceeded the context allowance (224527/61440).
  Scheduled task history is now scoped to its own attempts/interactions, with
  the same source/privacy/context guards. The regression uses a huge unrelated
  conversation and verifies retained UNKNOWN and exact interaction data.
- This fix was deployed as Core SHA256
  `a319c5956533b3a77a6191e3b91310a65f7130b31dd1f2efc091e8501dc016b6`,
  effective `private/candidate.v7.compose.json`. Broker, runtime, communication
  profile, proxy and existing budgets were preserved. The previous v6 artifacts
  remain the rollback target. No migration or scope reset was performed.
- After independently checking zero earlier model/tool attempts, the owner
  control resumed the **same** scheduled task. Continuation
  `input-918843e1bf4fc39499b5e24540a1641c`, run
  `run-1794809657981868968-60f79b1e1f4962de`, completed two ACP turns and
  `communication.message`. `ACP_SCHEDULE_FIRED_001: 56` is Web `result_ready`
  and Telegram `sent` with ACK. The original failed Input remains failed;
  no additional schedule occurrence or automatic effect replay was created.
- Composed isolated HTTPS login: run
  `run-1794809430908195214-65c477a4a1bd2d4c` used real Core/Broker/vault/Chromium
  and ACP, including the protected exact-form owner decision. Independent
  recipient counts: one accepted POST, two authenticated reads, one anonymous
  denial. `browser.login` completed once with `browser_login_v1` and journal
  `effect_confirmed`. The final included the random account fact that was never
  placed in the prompt. A private audit found neither username nor password in
  stored messages/tool arguments/results. All seven isolated attempts, including
  an earlier old-binary timeout, retain NULL native tokens. Earlier missing-anchor
  permissions and stale30s-binary failures remain in that database.
- Eleven actual Chromium login controls passed, including unsupported/ambiguous
  forms, foreign targets, stale observations, wrong password and public landing.
  These are zero-model mechanical checks, separate from the composed ACP login.
- Candidate extraction: 32 fixed Russian cases scored, citations32/32,
  core semantics23/24, holdout8/8, unsafe promotions0, confirmed facts/tasks/
  mandates0. The opaque accounting exception is explicit; unknown usage was
  never relabelled measured. Window v2 stopped after30 scored cases when an
  additional component suite filled the local256MiB PostgreSQL tmpfs. A backup
  was saved and that mount expanded in place; no database/history was reset.
  Separate bounded remainder windows carried hash-pinned original scores and
  checked that each remaining case had no earlier model attempt. v3 failed
  fixture setup without inference; v4 scored one case, then hit a one-poll
  scanner assumption. The harness now waits for ordinary bounded polling.
  v5 scored only the last unattempted case and passed the combined fixed gate.
- ACP consolidation preserved complete duplicate/uncertain/UNKNOWN evidence.
  Live dispatch cancellation and policy revocation each stopped without
  publication and retained one permanent reservation with NULL usage.
- The first ACP summary was published through sequence8. Its second request
  timed out at300s; the prior valid checkpoint, raw transcript and conservative
  spending were retained. That window is **failed**, not qualified. A separately
  capped recovery experiment is tracked independently. Primary background memory
  still explicitly uses native Antigravity until its own qualification completes.

Targeted PostgreSQL regression checks for shared final budget, pause, priority,
publication rollback, migration-history retention, repeated compaction, late
correction/deletion/UNKNOWN and consolidation correction/CAS/retirement passed
on a separate persistent local test database after the tmpfs incident. Their
earlier storage failures are not counted as successful runs. Targeted Core task
history and native-memory composition tests also passed. Full-suite success is
not claimed. Positive bounded delegation evidence is recorded below; primary's
pre-existing zero child allowance has not been bypassed.

The separately frozen summary recovery passed in111.37s: sequence17 is now
covered, four primary messages remain in the raw tail, and exact owner
conditions1/16 survive. It used a new one-call owner policy and revoked the spent
old policy; the original timeout/window/reservations were retained. The local
memory cohort finished at39 actual attempts within its unchanged40-prompt UTC-day
scope (35 publications, four failed/withdrawn attempts, all unknown native usage).
These compatibility checks back separate exact ACP profile gates; they do not
relabel the failed latency window or inherit native cost/latency conclusions.

Positive delegation also completed on the isolated full stack: parent task
`task_cedf3e0a7f85eb0a49632d7734768f6d`, child
`task_5778103190126b10be70164754919558`, independent session
`delegation:_0cc0327e1fe998ac2c0230aa8ddbcdab`, child run
`run-1794811272074699392-4aec246b53eb0bfe`. The child had no tool permissions,
no descendants, one ACP prompt, zero token allowance (not applicable),300s,
8MiB input and64KiB output reservation. It returned the exact selected source
identity/revision/hash and value17 with `uncertainty:source_only`. The durable
child state is `result_ready`, its Input completed and result stays `unverified`.
Model output was not promoted into verified task completion. The primary zero
child policy remains unchanged.

## Primary ACP background activation

The independently checked profile gates and explicit operator configuration were
then deployed on primary laba: both foreground and memory backend are now
`antigravity-acp`, exact `gemini-3.8-flash`. Effective configuration is
`private/candidate.v8.compose.json`, Core SHA256
`e0238e3cb724ac0ca07b0edb45b53a569b454dfcb6b7b744a30a2f8cbd5fffc8`.
The v7 native-memory configuration remains the immediate rollback target.
Broker/runtime/proxy/communication profile were unchanged. A quiescent before/
after checksum verified identical pre-existing budget limits, deadlines and
spending. There were zero existing model-memory policies to reinterpret.

One new owner-authenticated synthetic document and explicit one-prompt opaque
policy `primary-acp-memory-001` exercised the actual primary background worker.
Job `model_e0dc9f74164b8c6d9a99c9dbde43cdb37433d420d166eb52763ff826e92156a6`
finished with published source `source_de4a7850527b0cd5fe503e274cb755d73caddd9e`.
Its sole attempt reports ACP/Flash, NULL usage, one prompt,300000ms,8MiB input,
64KiB output and zero applicable token allowance. The owner read model explicitly
reports `opaque_subscription`. The one-use test policy was then revoked through
the protected owner API; publication and spending were retained.

Targeted Linux race tests for opaque jobs/budgets, actual memory profile selection
and scheduled/owner task history passed, followed by vet of the affected Core,
configuration, budget and memory packages. An earlier race *build* ran out of its
local256MiB `/tmp`; it ran successfully with2GiB temporary build space. This was
separate from PostgreSQL and from the live Google windows.

The obsolete public HTTPS recipient and SSH alias are not required for this
acceptance. The local recipient/credentials were synthetic; primary public
browsing, scheduled delivery and background generation were separately observed.
Positive primary child execution is still prohibited by the existing zero child
budget. The isolated positive delegation does not bypass or alter that policy.
