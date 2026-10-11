# Controlled-browser resource admission acceptance

Contributes to #17. The issue remains active. This proof covers the optional
controlled-browser Broker/runtime boundary; production composition and the
user's Chrome `single_tab` protocol remain separate acceptance gates.

## Reproduction

All execution took place through `ssh laba` in
`/srv/storage/butler/agents/resource-runtime-bridge`. Source was read-only for
tests. A disposable internal network and PostgreSQL17 database were created
below that root; all owning stores used MaxConns=2. Go1.27.2 and its caches/temp
files were on storage. Main Compose, main PostgreSQL, production profiles,
credentials, model providers and GPU were unchanged. No model call was made.

```powershell
pwsh -NoProfile -File scripts/dev/lab.ps1 -Action Sync -RemoteRoot /srv/storage/butler/agents/resource-runtime-bridge
ssh laba 'bash /srv/storage/butler/agents/resource-runtime-bridge/current/scripts/dev/resource-runtime-lab.sh /srv/storage/butler/agents/resource-runtime-bridge'
```

The script first runs the narrow new boundary with `-race`, then the seven
affected package regressions with real isolated PG. Chromium is deliberately
skipped in the Go-only container and executed in a separate lane using the
existing browser image, static test binaries and a real browser subprocess.
That Chromium lane is not a race-instrumented binary. The browser harness's
loopback dial seam is test-only; production public-IP policy is unchanged.
No unexecuted browser scenario is counted as passed.

The final source snapshot and log hashes are recorded in
[the sanitized machine report](resource-runtime-verification.json). Raw events
remain on the isolated lab root. Reports contain no live account or credential
material. Earlier failed fixture/runner attempts were retained; they do not
count as acceptance.

## Boundary results

| Scenario | Actual mechanism | Result |
| --- | --- | --- |
| Exact action and finite admission | Existing Broker Prepare/Authorize/Verify/Claim/Outcome, PG journal/budget, private authority HTTP and gRPC Router | One reservation; one backend execution; completed local lane/version increment |
| Absent actual budget reservation | Deliberately missing Claim budget hook | Runtime0; grant stays authorized; no resource lease, dispatch or reservation |
| Competing writers | Same run/session, distinct grants, first actual runtime call paused | Second denies; allowance and grant/resource changes roll back; no RPC under PG locks |
| Spent action/fence | Repeat the actual authorization and private runtime attempt | No replay; prior runtime count unchanged |
| Foreign/fabricated browser session | Other live run or invented typed handle | Denied before budget and resource claim |
| Cancellation/source deletion after claim | Current PG task/source changed before delayed runtime release | Backend0; committed uncertainty and resource marker retained |
| Expired resource/recorded unknown journal | Private delayed ticket after expiry or original outcome recording | Backend0; no retry |
| Grant expires during resource row wait | Deterministic separate transaction barrier plus fresh PG clock | Whole admission rolled back; grant unspent |
| Lost receipt | Runtime responds once, authority Finish deliberately unavailable | Unknown result; durable inflight survives; successor and replay denied |
| Migration rollback | Empty062 rollback; browser-only history and populated dispatch history | Empty allowed; nonempty history retained and rollback refused |
| Actual Chromium stale observation | Real production revision2 open/click contracts, private authority and separate browser process; independent DOM mutation between open and click | Stale click unknown; recipient effect requests0; two conservative budget reservations; resource quarantined |
| Private boundary | Anonymous/extra policy fields/redirect/malformed service token/routing credentials | Denied; tokens/tickets absent from model results |

The mechanical backend tests exercise real PG/Broker/HTTP/gRPC with a small
synthetic page backend and a trusted already-created-context fixture. Only the
Chromium lane proves actual page behavior and creates its session mapping from
the real successful open receipt. Its timed DOM change is external page
interference, not a human using the legacy user's-tab extension.

`completed` in the resource mapping means a guarded local lane finished. It
does not prove recipient acceptance or turn an uncertain external tool result
into `effect_confirmed`. The ordinary action journal remains authoritative.

## Acceptance remaining for issue17

| Requirement | Status after this delta |
| --- | --- |
| Stable trusted resource identities, bounded leases/fence/version, sorted all-or-none foundation | Existing058 foundation plus this exact browser adapter |
| Real Broker grant/budget/resource same-TX composition and guarded runtime | Passed isolated optional adapter; production bootstrap/readiness still pending |
| One executor on user's real Chrome tab; manual owner interference advances observation | Pending legacy `single_tab` protocol and real user-tab acceptance |
| Two code fixes in separate copies and conflicting file writes | Existing isolated workspace/storage proof; production Broker/runtime callbacks pending |
| Mutual multi-resource wait and crashed-owner recovery | Existing resource foundation proofs; no complete production tree/runtime scenario claimed |
| Task-tree stop of workspaces/shared workers | Pending concrete production callbacks; this runtime propagates cancellation/deadline only |
| No secrets/new rights through temporary copies | Existing workspace containment and Broker credential boundaries retained; complete delegated runtime composition pending |
| Atomic task/mandate/day resource budget | Passed actual browser claim/rollback; shared-worker full scenario pending |
| Memory preference correction race avoids last-write-wins | Existing candidate/version rules unchanged; combined issue17 race remains pending |
| Independent effect reconciliation and known artifacts | Required constructor guard; fixture explicitly denies reconciliation; production owning adapter pending |

Default composition stays unenabled. There is no user/browser handoff, automatic
renewal, expiry cleanup of an inflight effect, unsupported exactly-once guarantee
or network/runtime bypass. The complete startup order is documented in
[resourceexec](../../internal/resourceexec/README.md).
