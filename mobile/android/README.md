# Butler Companion

Android companion for [Butler issue57](https://github.com/alex123321-maker/butler/issues/57).
The starting Kotlin/Compose UI, Room queue, normalizer and client were imported
from [ButlerCompaniion](https://github.com/alex123321-maker/ButlerCompaniion),
commit `01fc8bf` (2026). The original separate app/package is preserved on test
phones: this project installs `dev.butler.companion.debug` or release
`dev.butler.companion`. The authoritative integrated source is now this folder.

Requirements: JDK21, Android SDK platform36.1/build-tools36.0.0, network access
for the pinned Gradle9.3.1/AGP9.1.1 dependencies. minSdk26, targetSdk36.

Windows: set ANDROID_HOME and run
`pwsh -NoProfile -File scripts/dev/mobile-build.ps1 -Test` from the repo root.
Linux: `cd mobile/android && GRADLE_USER_HOME="$PWD/../../.cache/gradle" ./gradlew
--no-configuration-cache testDebugUnitTest lintDebug assembleDebug`.
Use `connectedDebugAndroidTest` for a connected device/emulator. Tests named
MobileLiveAcceptanceTest require real receiver arguments; a skipped live case
does not prove ingestion. Ordinary UI/Keystore checks are separate evidence.
Debug key/cache files stay in repo-local `.cache/` and are not committed.

For a signed release, supply KEYSTORE_PATH, STORE_PASSWORD, KEY_PASSWORD and
optional KEY_ALIAS=upload through private operator configuration, then use
`mobile-build.ps1 -Release`. Without these, `assembleRelease` produces an
unsigned APK for later operator signing. Keep the private signing key and
passwords outside Git and back them up privately: Android updates require the
same signer. Debug and release have different package IDs and signatures.
APK paths: `app/build/outputs/apk/{debug,release}/`. Verify SHA256 before install;
Version1.2.0 additionally explains autonomous use on the app-filter screen.
Permitted messages can feed owner-enabled20-message/hour assistance on Butler.
App exclusions stop future collection; server history/notes have independent
retention/deletion controls. `MobileAttentionAcceptanceTest` seeds synthetic
notifications only, verifies real callbacks and durable Wi-Fi ACKs; the host
must separately verify native processing, search and noise decisions.
use `adb install -r <apk>`. Never uninstall/clear an existing instance merely to
resolve a signature mismatch; retain/export needed data and use its actual key.

First install: enter server URL, request a verification code, confirm it on the
Butler Web Android-devices page, then complete pairing on the phone. Read the
access explanation before opening Android notification-access Settings and
granting permission. Android13+ sideloads may require the app's system App Info
"Allow restricted settings" confirmation; only the device owner can authorize
that system step. Select included/excluded apps and test with a synthetic
notification. The dashboard shows permission, queue, last delivery and errors.
Pause stops new collection/transmission. Metadata-only mode stops future text
collection; clear the old content queue before syncing that mode. Revoking
system access stops collection/new sends. Disconnect clears credentials and
the now-unreadable old queue; server history is deleted separately via Web.

For Samsung, allow ordinary background work if prompt delivery is desired;
Doze/deep sleeping can defer WorkManager. No root, Accessibility or permanent
foreground service is required. History before installation is unavailable,
apart from currently active notifications. Android15+ may mask OTP content;
there is no workaround or promise of complete message history.

Use HTTPS/Tailscale when away from home. Plain LAN addresses do not work on
mobile data without a route. HTTP is a deliberate local debug option, off by
default. Red TLS errors must be fixed at the server/certificate, never bypassed.
See [protocol/retention/threat boundaries](../../docs/architecture/android-companion.md)
and [executed test report](../../docs/testing/issue-57-android.md).

Optional SMS login codes: enable the separate switch in App filters, with text
transmission and notification access enabled, then configure the account's
phone, actual default SMS package and exact sender in the unlocked Web vault.
Only new, fresh, non-summary SMS notifications with one login code are relayed.
The candidate stays in memory for bounded retries and is never written to Room
or diagnostics. SMS queue copies stay masked while relay is enabled. Payment,
transfer/reset messages and multiple candidates are refused. Pause, permission
withdrawal and disconnect stop new relay attempts. No READ_SMS/RECEIVE_SMS or
redaction bypass is used; Android-hidden codes and process death require manual
entry. Local extractor/wire tests and APK compilation do not prove real phone
delivery; see [vault acceptance](../../docs/testing/credential-vault-2026-10-24.md).


Login-code relay rejects password reset, change/update and recovery notifications
in addition to payment context, including generic verification/security wording.
These messages remain on the protected manual path.
