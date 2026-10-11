# Guarded user-tab component acceptance

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

Full startup/contracts must expose only the guarded target, with current
authority/source/budget hooks and no raw alternative. The existing controlled
browser and user-tab constructors are distinct profiles; there is no implicit
fallback or grant of unconfigured protected tools. A remote Broker needs an
operator-authenticated tunnel/gateway to the host's loopback listener.

The measured owning controller is synchronous under a live worker lease.
Production Web/Telegram selection after releasing that lease needs a distinct
current-owner/suspended-run authority adapter; no stale lease shortcut is
provided. Guarded-only production startup/contracts/routing, actual channel
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
