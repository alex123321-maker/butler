# Android companion acceptance, 2026-10-18

Implementation and quick-test evidence for [issue57](https://github.com/alex123321-maker/butler/issues/57).
The original issue remains open: the owner deferred the 24-hour battery run and
asked to use Wi-Fi only for this stage. A configured CI job is not a passing job.
Long offline/Doze/phone reboot and the external mobile route are not accepted by
the short failure test below. Production on `ssh laba` was inspected, not changed.

## Reviewed sources and implementation

Butler baseline: `1f64fd7847dffaca3d374ac5ed29f8efb57bfff4` (origin/main).
The source checkout was preserved; work used an isolated worktree. Companion
source came from `alex123321-maker/ButlerCompaniion` commit `01fc8bf` (the actual
repository spelling has two i's). Integrated source is `mobile/android`.
Its original app remains installed separately on the phone.

Affected subsystems: Android capture/queue/transport, authenticated HTTP ingress,
PostgreSQL source catalog and owner Web controls. See the
[versioned protocol and threat boundaries](../architecture/android-companion.md).
The original ingress acceptance below covers automation-disabled quarantine and
zero model invocations. The subsequently owner-requested standing permission,
20-message/hour wakeups, automatic notes, archive search and reversible noise
rules are covered by the [attention acceptance](issue-57-attention.md). Ingress
still records immutable private DATA with `private_observation_only` significance;
the separately admitted ordinary task supplies current authority and budgets.

## Environment

| Component | Actual stand |
|---|---|
| Host | Windows, Docker Desktop Linux engine |
| Phone | Samsung vendor model SM-G973F (Galaxy S10); ROM general property reports SM-S731B; Android16/API36 |
| Android build | JDK21.0.9, Gradle9.3.1 with pinned wrapper SHA256, AGP9.1.1; compile36.1/target36/min26 |
| PostgreSQL | Disposable local pgvector/pgvector:pg17; separate `mobile_test` database and deletion-anchor volume |
| Go | Container golang:1.27.2, repo-local module/build caches |
| Receiver | Actual `api.MobileServer` and all migrations, test-only `apps/orchestrator/cmd/mobile-lab`; separate owner and device ports |
| Phone data | Only synthetic notifications emitted by `dev.butler.companion.debug`; ONLY_INCLUDED filter excludes every other package |

The owner approved using the attached phone. USB supplied power, so battery cost
cannot be inferred. Tests granted/revoked only the debug app's notification
listener and POST_NOTIFICATIONS permission. No personal notifications were read
for acceptance and no root was used. A temporary VPN route prevented LAN access;
after the owner disabled VPN, the same direct Wi-Fi test passed. No VPN settings
were edited by the test. Original app/data and unrelated listeners were preserved.
The service now checks current consent/pause/package filters before extracting
notification extras, including its initial active snapshot.

## Executed checks

| Check | Evidence and scope |
|---|---|
| Go validation and HTTP contract | `TestValidateBoundedExternalData`, `TestReplayHashIncludesPayloadAndMetadata`, `TestMobileStrictBoundaryRejectsAuthorityAndDuplicateKeys`, `TestMobileOwnerAndDeviceProofsAreDistinct`: bounds, future timestamps, unknown/duplicate authority fields, oversized bodies and separate proofs |
| Durable PostgreSQL ingestion | `TestConcurrentBatchSingleLogicalCommitIntegration`, `TestDurableReceiptsPrivacyDeletionAndRevocationIntegration`, `TestPairingOneUseScopedCredentialAndRotationIntegration`: concurrent retry, source/receipt atomicity, fresh Store/lost ACK, whole-body conflict, private reveal, foreign read denial, deletion, rotation and revocation |
| Shared lifecycle gate | `TestMobileLifecycleAttackCorpusIntegration`: all eight shared attacks become quarantined DATA; zero tasks/runs/claims/grants, selective physical text deletion and independent source preservation |
| Synthetic load | `TestMobileThousandEventBurstIntegration`: 1,000 accepted plus 1,000 replayed as duplicates, 1,000 unique receipts, zero model runs. First measured run: 21.603s through Docker Desktop's host-published PG port. This is bounded ingestion/load evidence, not a sustained mobile/battery benchmark. |
| Existing Orchestrator API | Final full `go test ./apps/orchestrator/internal/api -count=1 -parallel=1 -v` passed in 35.542s against actual PG over a direct container network; app bootstrap compile passed. Optional external-driver skips are recorded in the receipt, not counted as feature acceptance. |
| Android original JVM suite | Eight tests passed for normalization, MessagingStyle/Unicode/progress, changes/removal/hash, privacy/transport, Room encryption, per-ID ACK/conflict/revocation and exact overflow accounting |
| Extended Android client contract | Seven additional tests cover 200/202 and partial/invalid ACKs, timeout with identical-event retry, 400/423/429/5xx backoff, 401/403 revocation, 409 terminal conflict, permission/pause/unknown-app allowlist gates, server binding, concurrent callback deduplication, post-after-remove and in-flight send/disconnect serialization. HTTP failures are deterministic client fixtures, not claims of real mobile network failure coverage. |
| Current retention and security policy | Real PG tests `TestPausedRateExpiredPairAndDeviceTakeoverDeniedIntegration` and `TestRetentionErasesOriginalAndPreservesReceiptIntegration` pass: pause/rate/expired code/device spoofing denied, unexpired source protected, expired text erased, tombstone replay remains duplicate. An initial retention failure exposed a rejected background actor; the fix uses an explicit catalog retention seam with current locked policy instead. |
| Real phone, Go and PG | `MobileLiveAcceptanceTest`: posted → updated → removed share a card and have distinct stable event UUIDs, every per-ID ACK DELIVERED, Room payload encrypted, permission withdrawal prevents capture. Passed through ADB reverse and direct Wi-Fi (final natural background lifecycle test 3.703s). These durations cover the test procedure; they are not per-notification delivery percentiles. |
| Receiver unavailable + process termination | `MobileOfflineAcceptanceTest` seed passed in 9.056s with the receiver container stopped: a newly emitted low-importance notification persisted encrypted and PENDING. App was force-stopped, receiver restarted, app relaunched; drain passed without a direct sync call (0.347s after launch), all saved card events DELIVERED. This is short offline/background recovery, not several-hour offline or a phone reboot. |
| Android Compose | `MobileSettingsUiTest` passed on the phone (4.857s): pairing URL, filters, queue and dashboard reachable. The final explicit disconnect button test passed in 3.188s: local token/key/queue cleared and separate owner GET devices confirmed REVOKED. |
| Owner Web UI | Playwright `web/tests/e2e/mobile.spec.ts` passed: sensitive text hidden until reveal; proof change erases devices/text. This uses controlled HTTP fixtures. Vue typecheck, targeted ESLint and stylelint passed. Actual owner HTTP pairing/search/deletion are separately exercised by phone/backup tests. |
| Actual backup/restore | `scripts/dev/mobile-backup-acceptance.ps1`: two committed sources → real `pg_dump -Fc` → selective deletion → fresh DB/`pg_restore` → stale restore denied → latest external manifest reconciliation → direct SQL confirms source text physically erased, independent source retained; replay gives 2 duplicates/0 new accepts |

An early full API run through `host.docker.internal:15457` exceeded existing
candidate-worker two-second deadlines. Other local full runs also timed out in
existing candidate cases while builds were active. Focused comparison passed on
the untouched main snapshot (8.877s) and this branch (11.911s); the final isolated
full API run passed in 35.542s without changing candidate code or its deadlines.
The timing failure is retained as a host-load-sensitive test limitation; it is
not claimed to be a proven universal root cause or a candidate-service fix. Native
Windows catalog tests cannot fsync the manifest directory; the accepted PG and
backup runs used the real Linux file manifest, not an in-memory replacement.

The first offline test had a repeat-run fixture defect: it selected an older
already delivered notification. The test now records prior IDs and waits for a
new ID before checking offline durability; both seed and drain then passed.

Final real Wi-Fi delivery was also measured without calling direct sync from the
live test: three background deliveries took 420, 667 and 1084ms in the phone's
own clock domain. This is a three-event sample with an active USB-powered phone,
not a p95/SLA or evidence of Doze delivery.

The instrumented load follow-up accepted 1,000 events and replayed 1,000 in
9.128s with 1,000 logical receipts and zero model runs. Fourteen resource samples
observed peaks of 96.12MiB/341.56% CPU for the Go race-test runner (includes setup)
and 403.3MiB/26.57% CPU for the whole disposable PG instance (includes its earlier
test footprint). CPU percentages can exceed 100% across cores. These are sampled
stand observations, not incremental per-event cost or production capacity limits.

## Reproduction

All caches, APKs and private operator state stay under ignored `.cache/` or
Android `build/`. Never commit owner proofs, device tokens or signing keys.

```powershell
pwsh -NoProfile -File scripts/dev/mobile-build.ps1 -Test
# JDK21 and ANDROID_HOME must point to installed SDK platform36.1.
# Signing helper generates/reuses a private local key; password is protected by Windows DPAPI.
pwsh -NoProfile -File scripts/dev/mobile-release-local.ps1
```

For Linux PG tests set `BUTLER_TEST_MEMORY_POSTGRES_URL` to a disposable PG17
database with pgvector. Run these on Linux so the deletion manifest uses real
directory fsync. Each fixture creates its own schema, runs all migrations and
uses MaxConns2; missing PG configuration causes explicit skips, not acceptance.

```sh
go test -race ./internal/mobile -count=1 -v
go test ./apps/orchestrator/internal/api -count=1 -v
go test ./apps/orchestrator/internal/app -run '^$'
```

The test-only receiver requires `MOBILE_LAB_POSTGRES_URL` ending in a `*_test`
database, `MOBILE_LAB_MANIFEST` pointing to an independent persistent Linux file
and `MOBILE_LAB_OWNER_TOKEN_FILE` pointing to a privately generated owner proof.
Build `go build ./apps/orchestrator/cmd/mobile-lab`; start it with `-initialize`
only for an empty database/new manifest, otherwise without that flag. It exposes
device ingress on 8080 and owner controls on 8081. Publish owner controls to
localhost only. The acceptance stand mapped these to 18057 and 18058.

Create a one-use pairing code through owner POST `/api/v1/mobile-owner/pairing-code`.
Keep the result in a shell variable; pass it privately as instrumentation argument
`pairing_code`, with `mobile_server` set to the reachable test URL:

```sh
adb shell am instrument -w -r -e class com.example.MobileLiveAcceptanceTest \
  -e mobile_server "$MOBILE_TEST_URL" -e pairing_code "$ONE_USE_TEST_CODE" \
  dev.butler.companion.debug.test/androidx.test.runner.AndroidJUnitRunner
```

The live test deliberately clears only the debug app's credentials/queue before
pairing and configures the synthetic-only filter. It must never target a user's
daily Companion package. Install both the debug APK and its androidTest APK first.
For failure testing, stop only the disposable receiver, run the same instrument
command with class `com.example.MobileOfflineAcceptanceTest` and `mobile_phase=seed`.
Force-stop the debug package, restart receiver, relaunch the debug app, then run
`mobile_phase=drain`. No direct repository sync is invoked by the drain test.
After testing restore temporary power settings and revoke debug listener access.

The backup script expects the named disposable Docker stand
`butler-issue57-postgres`, DB `mobile_test`, receiver ports18057/18058, a Linux
receiver binary at `.cache/issue57/mobile-lab-new`, and independent volume
`butler-issue57-state:/state`. It creates another `mobile_restore_*_test` DB and
receiver on localhost18059/18060. It never switches the production DB or stack.

```powershell
pwsh -NoProfile -File scripts/dev/mobile-backup-acceptance.ps1 `
  -OwnerTokenFile .cache/issue57/owner.token
```

## Artifacts and remaining gates

Installable debug and operator-signed release APKs are built locally. SHA256 and
signer fingerprints are recorded in the content-free
[evidence receipt](fixtures/issue57/acceptance.json) and
[APK SHA256SUMS](fixtures/issue57/APK-SHA256SUMS). Private
release keystore/password are ignored and must be backed up privately for future
updates. CI builds an unsigned release unless operator signing is configured.
See [installation and data deletion instructions](../../mobile/android/README.md).

The [final CI run](https://github.com/alex123321-maker/butler/actions/runs/39125342137)
passed all six jobs on code commit `85e424e1344be8a7d17edc66ef52caf53d2df274`:
Go, Android build/lint/15 JVM tests, and emulator API31/33/35/36. Every emulator
executed two context/Compose smoke tests; three external-fixture tests explicitly
skipped (live ingestion, offline procedure, paired disconnect). Those three ran
separately on the real phone. Emulator UI compatibility is not full notification
capture/OEM/power-mode acceptance for all four OS versions. The first CI attempt
failed because setup-android's default obsolete `tools` package was unavailable;
explicit `platform-tools` fixed the setup without bypassing SDK or lint checks.

Before closing the original issue:

- Extend full notification lifecycle/failure compatibility across the OS matrix;
  the current emulator smoke tests and separately executed phone e2e have the
  explicit scope above.
- Agree and perform 24-hour battery/queue/data-volume/loss measurement unplugged.
- Perform several-hour offline, phone reboot/unlock and Doze/OEM power-mode tests.
- When requested, validate mobile data through HTTPS/Tailscale. A temporary HTTPS
  tunnel attempt timed out and is not accepted. Wi-Fi is the only accepted route
  for this stage, at the owner's request.
- Autonomous message attention is evaluated separately in the
  [attention report](issue-57-attention.md); this earlier zero-model ingress run
  alone is not evidence for generative assistance.

No always-real-time delivery guarantee is made under Android power/network
restrictions. No production rollout or original-issue closure is inferred from a
passing build, an APK installation, or a merged PR.
