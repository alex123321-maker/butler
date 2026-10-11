# Guarded user-tab component acceptance

## Durable owner choice and fresh core recovery

The owning application now joins finite guarded choice publication to actual
input suspension and run/task waiting state in the same transaction. Missing or
lost input claims publish no choices. Protected owner select/revoke atomically
readies the same stored version1 selection continuation. Revocation while queued
and a choice arriving before the old worker finishes remain effective.

Core recovery acquires a new real Session fence and repeats current input claim,
source pins, task/constraints and exact guarded session/approval/deadline checks.
Current tool definitions reload before activation; source changes during that
reload deny before provider entry. Actual Catalog deletion cancels/redacts the
queued input and removes its cached continuation. Corrected sources, revoked or
expired selections, changed tasks, lost claims, missing composition and advanced
tool checkpoints deny. Restart after activation can revalidate the local choice
without extending expiry or replaying a browser action. A fresh provider context
receives the opaque local choice as private DATA; it supplies no browser authority.

Final narrow PG17/pool2/race and affected vet passed on immutable snapshot
`20261011-154444-349`,13 scenarios plus their parent test, zero skips:
`/srv/storage/butler/agents/guarded-tab-continuation/reports/task-observations-20261011T084447Z-301065`.
Events SHA256 `9b06f7c94d1fd91e320b6c6eb0cf9e205de18691c30bff12ca51135528e6501c`.
Earlier expanded owner/core/input checks passed177 test/subtest names in7 packages
on snapshot `20261011-153929-717`, proof
`/srv/storage/butler/agents/guarded-tab-continuation/reports/task-observations-20261011T083933Z-278676`,
events SHA256 `a302c97344cebb598482adc1f7e8363839ec515811e419928057ed52186caa06`.
Its29 explicit skips require the older separate service DSN; they are not passes.
Five critical actual generic approval/input/action-slot regressions were enabled
separately against disposable PG, passed without skips on snapshot
`20261011-154400-466`, proof
`/srv/storage/butler/agents/guarded-tab-continuation/reports/task-observations-20261011T084404Z-298295`,
events SHA256 `7c08e085abef6bc3062b8612bbd1c0394352272632c1ac8ceaa2230747c43aad`.

The independent resource/native-host/Broker regression passed72 PG/bridge
test/subtest names and10 real Chromium fault/positive scenarios on snapshot
`20261011-153943-743`, proof
`/srv/storage/butler/agents/resource-runtime-bridge/reports/single-tab-20261011T083947Z-279871`.
The PG phase explicitly skips Chrome, which executes in the later browser phase.
Hashes: PG `914a8dae764aa06ed4e92ece3d195e4084ec88bec423c7c5c9f17cfa253f4a2c`,
bridge `7f177517e296d2c87fc14ca022e54a022f97ca3934d16f7dd1970a4ca768c3e8`,
Chrome `0a515fd31f8b233cc5308d4272e84bdaa569e8f06b7ee288694be680bcb4b362`.
The final guarded recovery release uses its own fenced provider context and the
ordinary context for subsequent writes; that final change was checked narrowly.
Earlier failed compile, missing-source-pin and correction-CAS fixtures remain
in the lab report history. No checker was relaxed to accept them.

All model outcomes in these tests are scripted, with zero real inference calls.
Whole issue17 still needs ordinary worker request/delivery, private runtime
activation, workspace and shared memory/model composition. Main optional flags
and deployment are unchanged. Component proof does not close those gates.

Contributes to #17. The issue remains active and production activation remains
disabled. This report proves an optional mechanism on public synthetic data,
including actual Linux Chrome native-host installation and the authenticated
owning selection library. It does not claim human Web/Telegram acceptance.

## Reproduction and exact evidence

All execution used `ssh laba`, Go 1.27.2 and the isolated root
`/srv/storage/butler/agents/resource-runtime-bridge`. Tests use disposable internal
networks/PostgreSQL schemas, owning pools with MaxConns=2, storage caches/temp,
and read-only source snapshots. No main Compose/current/database/credential,
provider or GPU configuration changed. Model calls: zero.

```powershell
pwsh -NoProfile -File scripts/dev/lab.ps1 -Action Sync -RemoteRoot /srv/storage/butler/agents/resource-runtime-bridge
ssh laba 'bash /srv/storage/butler/agents/resource-runtime-bridge/current/scripts/dev/single-tab-resource-lab.sh /srv/storage/butler/agents/resource-runtime-bridge'
ssh laba 'bash /srv/storage/butler/agents/resource-runtime-bridge/current/scripts/dev/resource-runtime-lab.sh /srv/storage/butler/agents/resource-runtime-bridge'
```

Final executable/test snapshot: `20261011-105734-678`. The
[machine report](single-tab-resource-verification.json) records source-file and
raw-event SHA256 hashes, all skips and exact isolated report paths. Markdown and
this machine report were written after testing; executable sources did not
change. Earlier failed driver/fixture reports remain on the lab and do not
count as acceptance.

The machine report records exact package/top-level/subtest counts for the new
boundary, complete bridge and seven-package regression suites, all with `-race`.
Owning durable test pools use MaxConns=2. Chromium scenarios are skipped in
the Go-only containers and run separately with static non-race Go binaries.
Other pre-existing browser/egress scenarios listed as skipped are not claimed
by this report. The prior actual controlled-browser stale-page test also passes.

The legacy bridge test now owns a no-proxy HTTP transport, disables keepalive
and drains health responses. Its full regression had exposed an intermittent
shutdown timeout from fixture connection reuse; production legacy behavior and
the test's shutdown deadline are unchanged.

## Actual Chrome, extension, host and Broker results

The runner uses real Chromium/Xvfb, a temporary unpacked production extension,
a real guarded host subprocess with framed stdin/stdout, real private HTTP
authority, gRPC Router and Broker/PG action/budget/resource stores. A test-only
native-frame forwarder calls the statically imported production extension
exports. The fixture copy adds only driver references and a worker boot marker;
no such global is present in the installed production extension.

Nine fault cases use the native-frame forwarder. A separate positive case
installs an actual Linux native-host manifest for the exact extension origin
in a private temporary profile: Chrome `connectNative` launches the guarded
host subprocess and calls fixed metadata through it. It creates two actual tabs
of the same origin, distinguished by different opaque candidate tokens,
generated ordinals and exactly one active marker. The owning controller performs
authenticated Request/Select; no approved candidate is hand-seeded for this
Broker/Chrome flow. It then performs a real private Broker status/click dispatch.
Input/navigation come from Playwright; DOM mutation is independent public page
interference. The recipient is a loopback public fixture, without account or
credential material. The profile is synthetic, not the owner's live account.

| Scenario | Actual recipient calls | Durable reservations/bindings | Result |
| --- | ---: | ---: | --- |
| Valid exact status then click | 1 | 2 | Local receipt recorded; click still unverified/unknown |
| Manual input after observation | 0 | 2 | Old observation refused |
| Manual input, fresh observation, old plan | 0 | 3 | New ref and greater version observed; old ref refused |
| Independent DOM mutation | 0 | 2 | Stale plan refused |
| Same-origin document navigation | 0 | 2 | New document has no previous observation |
| Cross-origin navigation then fresh status | 0 | 2 | Fresh observation cannot widen selected origin |
| Different actual browser-instance identity | 0 | 2 | Host selection mismatch refused |
| Lost ACK, cold Chrome/profile restart, new real Broker ticket | 1 | 3 | Persisted quarantine survives; no remap or repeated effect |
| Two concurrent executors | 1 | 2 | Second admission rolled back before dispatch; one click |
| Installed Linux native host, two same-origin owner choices | 1 | 2 | Actual Chrome launches host; atomic owner selection feeds private Broker |

The cold restart changed the actual Chrome tab number. Therefore this proves
retention of the frozen old binding and refusal of automatic mapping to a new
tab, not transparent same-tab continuation. The persistent lane remains
`awaiting_ack`; both a driver-level successor and a real new Broker read
authorization/reservation/ticket are refused.

The Broker's ordinary effect journal remains authoritative. A successful local
resource receipt/host ACK is `dispatch_completed`, not recipient verification.
Clicks in this proof remain `effect_unknown`; unknown effects are neither
refunded nor automatically replayed. Private tickets/fences/native metadata and
browser field/document contents are absent from tool observations.

## Durable and private boundary checks

The later owner-waiting adapter passed actual disposable PG and race checks on
snapshot `20261011-121430-072`, proof
`/srv/storage/butler/agents/task-observations/reports/task-observations-20261011T051434Z-3803281`.
Eight cases cover actual Session lease release, successful/idempotent selection,
revocation, anonymous/foreign owners, changed revision, stopped task, a newly
acquired live executor and source staleness after metadata. No owner operation
creates/renews a lease; the stale worker proof remains denied. Events SHA256:
`74dd1c80fd537e05569de6813c2635bc68092d824bb435af9bfa5c3b2fd37999`.
This is a library proof, not composed Web/Telegram acceptance or activation.

- Owning Request/Select/Revoke requires authenticated owner plus current execution
  and source authority. Runtime metadata RPC is outside PG locks. Approval,
  candidate, opaque session and immutable guarded marker publish atomically.
  Legacy activation cannot manufacture the marker. Same-choice replay is current,
  not a prior decision shortcut; other choices deny.
- Actual two-connection concurrency tests pause inside current source authority,
  observe `pg_blocking_pids` and release the first operation. Claim-before-revoke,
  revoke-before-claim and selection-replay-before-claim finish in that serial
  order; revoke-first consumes no allowance/backend call. Timeouts are failures.
- Exact row barriers cross selection/approval/binding deadlines while waiting.
  Fresh final DB time after domain and authority waits denies missing/expired
  deadlines, shortened approval and expired exact-selection replay. DB clock
  query errors deny. Final validity cannot extend the earliest frozen deadline.
- Owner fence, amended task, source deletion, revoked/foreign/expired selection
  deny before concrete execution and consume no new allowance. Expiry after a
  committed claim denies the actual runtime while preserving its reservation.
- Trusted selected HTTP(S) origin strips query/path/fragment; userinfo and invalid
  origins deny. Stored dispatch origin is compared with current trusted metadata
  and actual Chrome tab/page origin. Origin and bind-session changes never
  partition the physical host/tab resource lane.
- Migration 063 freezes owner/run/task/revisions/host/deadline and exact selected
  candidate/tab/origin. Authority/mapping changes and history deletion deny.
  Empty downgrade is allowed; retained bindings/requests/selections deny rollback.
- Private auth-file symlink/public permissions/oversize and remote/wildcard/DNS
  gRPC bind addresses deny. Runtime service tokens remain host/Broker-only.
- Exact `guarded-1` contracts are required; pinning a legacy hash is insufficient.
  All actions other than status/click and all resolved credential inputs deny.

## Production gates still open

Migration064 and the guarded Approvals component now preserve and display original
same-origin choice ordinals and active-at-request markers. Protected GET requires
current waiting-owner/task/source authority; native host/tab refs stay private.
Original labels survive reload without reconstructing current Chrome order.
Choice rows are immutable and their retained history prevents downgrade. Ordinary
Approve/Reject cannot replace this guarded selection. Page-only owner proof fences
late responses; selecting and revoking send only their fixed decision bodies.

Actual waiting-owner HTTP/PG, schema mutation, core race and vet checks passed on
snapshot `20261011-125524-281`, proof
`/srv/storage/butler/agents/task-observations/reports/task-observations-20261011T055535Z-3943834`,
events SHA256 `19b411798c704f5df6c5bd6eb750cfc1ed791dce3012b5de4dce2a4a20a3c68b`.
Typecheck, lint and 13 actual Chromium UI fixtures passed on
`20261011-125736-048`, private log
`/srv/storage/butler/agents/task-observations/reports/task-view-20261011T055740Z.log`.
An earlier UI check rejected accidental double JSON encoding, and a second check
found that refresh discarded the decision receipt. Both failures remain retained;
the client now sends an object through the shared encoder and keeps the card while
refreshing its status.

The full private native-host/actual Chrome/Broker regression also passed with the
064 fixture applied, on snapshot `20261011-125623-054`:
`/srv/storage/butler/agents/resource-runtime-bridge/reports/single-tab-20261011T055703Z-3949941`.
PG/race event SHA256 `8805df44a16f9d8c748efe3e1878ecfaee532b4c56ac6c22df37b0de60af9f87`;
bridge/race `e3c2210b6c57145bba4a509a83641643c884a6894b4c5872f6417501b5247694`;
actual Chrome `018af7cf959ab15e4775253dd4e030ea6c341c6789e8c2a38ea8bb735f7b8394`.
No model call or main deployment occurred. These are component/channel-display
proofs; automatic worker request/resumption and executing production composition
remain pending.

The application now composes an operator-only, default-disabled waiting-owner
Web decision endpoint for existing guarded requests. On snapshot
`20261011-124302-890`, actual PostgreSQL/Session lease release followed by HTTP
select/revoke passed eight scenarios, including unauthenticated/foreign owner,
changed task, stopped task, resumed executor and source changes after metadata.
Same-choice HTTP retry returns the same opaque session and creates no worker
lease. Four actual schema mutations (disabled/dropped trigger, no-op function
and removed dispatch column) prevent readiness. Exact-case/duplicate/oversized
body and duplicate authentication tests, operator-only config validation,
complete Orchestrator core race tests and affected vet also passed.
Private proof:
`/srv/storage/butler/agents/task-observations/reports/task-observations-20261011T054314Z-3904037`.
Events SHA256:
`1a15c148a72c704087d570b31bbcd0ef173f818e477613388ac62da513a315db`.
There were zero model calls. API selection neither resumes execution nor confirms
a browser effect; UI/Telegram delivery, request creation and executing runtime
composition still require their own acceptance.

Full startup/contracts must expose only the guarded target, with current
authority/source/budget hooks and no raw alternative. The existing controlled
browser and user-tab constructors are distinct profiles; there is no implicit
fallback or grant of unconfigured protected tools. A remote Broker needs an
operator-authenticated tunnel/gateway to the host's loopback listener.

The original requesting/executing controller requires a live worker lease;
the separate waiting-owner adapter and Web decision route are measured above.
No stale lease shortcut is provided. Guarded-only production
startup/contracts/routing, actual complete channel
selection/manual acceptance, effect reconciliation, workspace/runtime callbacks,
shared local weights and memory-conflict admission, other observation-aware
actions and aggregate user-browser network/privacy policy remain pending.
Windows private-file ACL/native installation is unproven, a platform limitation
separate from the measured Linux manifest flow. Observation
covers bounded light DOM/form/focus/scroll in the main frame; it cannot certify
hidden JS/server/canvas/shadow-root state. No user cookie export or SecretStore
injection is provided. The user's page scripts may have their own network
effects, requiring independent exact owner policy before activation.

Persistent uncertain-lane cleanup is deliberately absent. Clock consistency
between PG host/native host/browser remains an operational prerequisite.
See [the contract](../architecture/single-tab-resource-boundary.md) for lock
order, deadline clamp, observation and ACK behavior, and
[production composition](../architecture/single-tab-production-composition.md)
for exact typed API/constructor/reference boundaries and unimplemented wiring.
