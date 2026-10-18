# Issue57: autonomous notification attention

This extends [ingress acceptance](issue-57-android.md) with the owner's standing
permission to use messages without individual requests/reviews. Selected policy:
20 distinct new messages or one hour since the oldest unprocessed receipt,
automatic source-backed notes, archive search and reversible noise suppression.
App collection filters remain in Companion. Previous originals/notes follow
server retention and explicit deletion, independently of future app collection.

## Executed local stand

All phone/model acceptance described here runs on the Windows laptop in Docker:
real PostgreSQL17, Redis, Orchestrator, separate production Tool Broker and native
Antigravity `gemini-3.8-flash-high`. Only synthetic notifications from
`dev.butler.companion.debug` are collected. The phone uses home Wi-Fi to the
laptop's ingress-only port18061; owner control is loopback18062. No ADB reverse,
SSH forwarding or remote Butler participates in this route. USB supplies ADB for
instrumentation. Model inference uses the laptop's Internet connection.

An initially prepared isolated `laba` stand was stopped at the owner's request;
it received no phone messages and ran no model tasks. Opaque provider state was
returned with compare-and-swap before shutdown. The local stand holds operator
credential references only in ignored `.cache/issue57/local-native`; reports do
not include bearer tokens, ciphertext, encryption keys or phone serials.

Provision a **fresh** stand after building the actual binaries:

```powershell
# Build Linux attention-orchestrator, attention-broker, attention-init and
# mobile-wifi-proxy-linux into .cache/issue57 using repo-local Go caches.
# Operator supplies private provider-auth.value and provider-encryption.env
# in .cache/issue57/local-native; never put their values in command arguments.
pwsh -NoProfile -File scripts/dev/mobile-attention-local.ps1
pwsh -NoProfile -File scripts/dev/mobile-attention-local.ps1 -Configure
```

The script pins a separate `_test` database, empty Broker registry plus owned
memory/message tools, source-deletion anchor, synthetic owner, explicit cloud
privacy, read/prepare-only mandate, finite48-turn task/96-turn mandate allowance,
native output8192 and300-second call ceiling. Other tools/credentials/external
effects are not granted. This is a disposable acceptance profile, not deployment.

## Executed evidence

| Check | Actual result |
|---|---|
| Real callbacks / Wi-Fi | `MobileAttentionAcceptanceTest`, phase `threshold`:20 messages durably ACKed, Android procedure3.449s;21 event receipts included an OS group summary,20 logical message parts |
| Initial native failure retained | First attempt made9 model turns but only read data: existing `prepare_only` excluded the prepare-effect `messages.process`. Pending decisions stayed pending. Fixed by normal task policy plus the exact standing read/prepare mandate; no global policy was weakened |
| Native autonomous processing | The same20 originals were admitted under corrected configuration. Actual Broker calls: inbox, four process decisions, inbox. Two searchable notes,17 noise decisions and one ignored injection; all20 terminal |
| Note quality | Confirmed dentist appointment27 Oct2026 11:30 Asia/Tomsk; friend's next-week invitation remained a proposal without agreed day/time. No external reply/booking/action |
| Durable completion | Task Service independently verified completed input/run plus terminal decisions. Saved task result uses `mobile_batch_v1`, not owner attribution. One useful-result Web receipt; no per-message approvals |
| Learned rule / new phone event | A new advertising notification matched the saved app/channel/literal rule and became noise without an additional autonomous task/model run;18 archived noise messages remained searchable |
| Native archive search | A separate ordinary owner task independently selected `messages.search` and `memory.search`, returned the hidden WIFI57 promo with citations, confirmed the dentist record and preserved invitation uncertainty |
| Owner controls | Actual owner HTTP disabled/re-enabled the noise rule. Attention view omitted rule text without explicit reveal |
| Timer | Actual PostgreSQL boundary test:19 fresh messages do not wake;20 wake once, including concurrent observers/replay. One message at59 minutes waits; at61 minutes it wakes. On the live laptop/native-model stand, the real phone's single barber message was held below the gate, then its test queue `received_at` alone was set to61 minutes old. The normal observer admitted it and the real model/Broker saved the confirmed22 Oct2026 15:00 Asia/Tomsk appointment; Task Service verified completion. Original source/phone timestamps were unchanged. This controlled aging is not an actual one-hour wait or Doze test |
| APK1.2.0 update / fixture repeat | Pairing, app filter and posting permission survived `adb install -r`. Two fresh advertising callbacks durably ACKed in2.533s and2.084s; another20-message phase ACKed in3.696s. Initial repeat timeouts were a fixture defect: fixed Android notification identity plus a13-digit timestamp redacted as a card number produced unchanged content, correctly deduplicated by the client. The fixture now uses a fresh notification tag and explicit stage/configuration assertions; privacy/deduplication remain unchanged |
| Restart / continuation | Batches and decisions remain in PostgreSQL through actual local application restarts. Integration test checks a single continuation receipt for incomplete completed work, under the same task; unfinished decisions/uncompleted inputs cannot verify completion |
| Privacy / deletion | Actual separate Broker process requires permanent dispatch/current claim/source pins. Remembered text is physically erased with its original. Shared original ingress attack corpus still calls no model when standing permission is absent |
| Web | Vue typecheck and targeted lint passed; two Playwright tests passed, covering explicit reveal, noise-rule undo and proof-change clearing. These are routed synthetic API fixtures, not a claim of browser/live-Go integration |

Current qualification is Wi-Fi only. The remaining original operational gates
(24-hour unplugged battery/traffic/loss, long offline/reboot/Doze and external
mobile route) remain as recorded in the ingress report and were not started.
The source/PR is reviewable; neither issue closure nor production rollout follows
from a successful test. Checksums and final CI/native receipts accompany the
acceptance snapshot.

Current Android artifacts are version1.2.0/code3. All15 JVM tests and lint passed;
debug and operator-signed release builds passed. Current
[SHA256 sums](fixtures/issue57/APK-SHA256SUMS) bind these artifacts. The original
ingress acceptance's1.1.0 artifact hashes remain historical evidence.
