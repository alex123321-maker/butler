# Android notification companion, protocol v1

This implementation adds Android ingress and owner controls to the existing Go
Orchestrator, plus a Kotlin application under `mobile/android`. It preserves
Session, Run, Broker and provider boundaries. Notifications are external DATA,
never owner messages, instructions, approvals, mandates or tool calls. Root,
Accessibility, SMS, contacts, location and notification-action execution are
absent. Android's own masking remains authoritative. NotificationListenerService
does not provide historical app messages or guaranteed full notification text.

## Pairing and authority

The owner authenticates the Web `/mobile` page with the existing owner-control
token, kept only in page memory. A phone can request pairing and display a
ten-character verification code. The owner enters this code on the Web page;
the phone then checks approval using a separate random 256-bit session capability.
Alternatively the owner creates a one-use 256-bit code and pastes it into the
phone (including supported `butler://pair?url=...&code=...` data).
Both flows expire after five minutes. Codes/session IDs are hashed in PostgreSQL;
the phone session capability and issued token are encrypted with Android Keystore.
Only one token response is issued. A lost exchange response requires fresh
pairing; a device ID or verification code cannot recover the token. Duplicate
device identities cannot transfer ownership. Disconnect creates a new identity.

Server device tokens are random 256-bit values, stored as hashes, with no owner
control scope. They may write observations for exactly one device, get its
status, rotate its token, or revoke themselves. Owner identity is read under a
device row lock. No JSON field or device header can supply owner authority.
Revocation/rotation and batch acceptance serialize on that lock. An already
committed request can precede revocation; no subsequent request is accepted.

## HTTP contract

All JSON is bounded to 256 KiB. Duplicate keys, unknown fields, trailing values,
unsupported versions, over 50 events, over 128 KiB per event and timestamps over
five minutes in the future (or before 2000) are rejected. Clients send at most
five events and 240 KiB per request. Strings/structured message counts are
bounded, with truncation recorded before client hashing. Rates are shared in
PostgreSQL: 60 pairing operations/minute globally, 120 batches/minute/device.
No notification text or server error details are placed in error responses.

Device routes (only this prefix may be exposed through a public proxy):

| Method/path under `/api/v1/mobile/` | Request/result |
|---|---|
| POST `pair/request` | device_id, device_name, android_sdk, app_version → pairing_session_id, user_verification_code, expires_in_sec |
| POST `pair/poll` | pairing_session_id, device_id → PENDING or APPROVED with one device_token |
| POST `pair/exchange` | pairing_code and device metadata → device_id, device_token, server_name, paired_at |
| GET `status` | device proof → ACTIVE/PAUSED status, server time, max_batch_size |
| POST `notifications/batch` | schema_version=1, batch_id, sent_at_epoch_ms, events → accepted_ids, duplicate_ids, conflict_ids, rejected, server_time_epoch_ms |
| POST `rotate` | current device proof → replacement device_token, old proof immediately invalid |
| POST `disconnect` | device_id, reason, current device proof → 204 |

Authenticated routes require `Authorization: Bearer <device token>` and
`X-Butler-Device-Id`; batch additionally requires `X-Butler-Schema-Version: 1`.
200 only acknowledges IDs after PostgreSQL Commit. An identical event replay
appears in duplicate_ids; a conflicting event ID in conflict_ids, without
overwriting its earlier content. A conflicting whole batch ID returns 409.
Omitted ACK IDs remain pending. Android validates that ACK IDs belong to its
actual sent batch and are not assigned multiple outcomes. Rejected codes are
typed, never echoed as arbitrary diagnostics. 400 is malformed input, 401 wrong
or revoked proof, 409 conflicting state, 429 retry with Retry-After,
423 is an owner-paused device (events remain pending), and 503 is unavailable
database or deletion reconciliation. No 2xx is a model result.

Owner routes under `/api/v1/mobile-owner/` use existing owner-control Bearer
proof: GET devices, GET events (`device_id`, `q`, explicit `reveal=true`), POST
pairing-code, POST approve (`code`), POST device (`device_id`, `status`,
`retention_days`), POST delete (`source_id`, `revision`). Search returns up to
100 recent matches; text is hidden unless explicitly revealed. Revocation does
not silently delete history; delete sources separately. Filters live on the
phone. Device tokens have no read access to this owner surface.

## Event v1

The matching typed contracts are `internal/mobile/contracts.go` and
`mobile/android/app/src/main/java/com/example/model/NotificationContractModels.kt`.
Required identity fields: schema_version, device_event_id (random stable UUID),
notification_key, package_name, change_type (POSTED/UPDATED/REMOVED),
posted_at_epoch_ms, observed_at_epoch_ms, content_hash (lowercase SHA-256).
Previous device event ID and channel/category/group identifiers are optional.
Boolean flags distinguish initial active snapshots, group summaries, ongoing/
clearable state, user redaction, metadata-only mode, reported system redaction
and truncation. Visibility is retained as reported metadata, never authority.
Payload contains available title/text/big_text/sub_text/summary_text/info_text,
conversation_title, bounded sender/text/time messages and progress values.
Absent content remains absent; no OTP recovery is attempted. Server reception
time is independent of both reported timestamps. Server replay identity hashes
the complete decoded event, including metadata and payload; content_hash alone
is not trusted. Android keeps the same event through all network retries.

## Local persistence and Android lifecycle

Android 8+ (minSdk26), targetSdk36, compile SDK36.1; JDK21, Gradle9.3.1,
AGP9.1.1. Kotlin, Room, Compose, Retrofit/Moshi were inherited from
ButlerCompaniion commit 01fc8bf; WorkManager is added because process callbacks
cannot provide reboot-safe retries. See mobile README for exact provenance.

The listener stores bounded normalized events in SQLite with AES-256-GCM text
encryption using Android Keystore. Production encryption fails closed if
Keystore is unavailable; software keys require an explicit unit-test injection.
Tokens use Keystore encryption too; Android backups are disabled. Queue defaults:
5,000 events, 25 MiB accounted encrypted payload/metadata, 14 days. Delivered
records are removed first. Undelivered overflow and expiry increment a visible
loss counter. Ordinary process/network interruption does not discard rows.

Collection requires pairing, current system notification permission and an
unpaused device. All available apps or only included apps are selectable;
sensitive sources support local masking. Capture is serialized. On connection,
only currently active notifications are snapshotted. Card changes receive fresh
event IDs with previous-event linkage; network retries retain IDs. Initial
callbacks and repeated unchanged content are deduplicated. The app cannot read
before the system grants access, and checks permission before new delivery.
Clearing credentials destroys the old queue key and queue; it cannot transmit
old data to a newly paired server. Changing server while paired requires an
explicit disconnect. HTTP redirects are disabled, so tokens cannot follow a
redirect. Server URL credentials/query/path/fragment are rejected.

Debounced event sends are supplemented by unique WorkManager work with network
constraints, bounded drain, exponential retry and periodic recovery. No infinite
foreground service is used. Samsung/Doze may delay delivery; no latency guarantee
is made. Network/TLS/auth failures are distinct fixed diagnostics without private
error strings. Invalid ACKs and unconfirmed events remain undelivered.

## Memory, deletion and significance

Each accepted event atomically becomes an immutable private quarantined catalog
source; source provenance records the trusted Android adapter and event package.
Receipts outside the catalog hold IDs, hashes and times, with no notification
body. Unreviewed sources are excluded from all model retrieval, extraction and
indexing. Significance is the fixed typed `private_observation_only`; no automatic
model calls, tasks or notices are admitted. Future significance adapters need
separate current goal, privacy, mandate and budget admission and new acceptance.
Explicit owner source review and purpose grants use existing memory controls;
model conclusions remain candidates. Notification text cannot select them.

Owner deletion/retention uses the existing fsynced external deletion manifest,
purges source revisions and derivatives, and retains content-free receipts.
Identical redelivery cannot recreate content. Replay after restoration must
reconcile the latest independent manifest before reads/ACK. Retention defaults
to 30 days per device (owner configurable 1–365). Sweeps are bounded to 100
sources. Existing backup retention/erasure rules apply; unknown offline copies
and already exported content cannot be recalled by this feature.
The background retention seam rechecks expiry and locks the current device policy
before deletion. It has no owner actor and cannot delete an unexpired or unrelated
catalog source; owner-initiated deletion continues to require owner proof.

## Routes and operational boundaries

Home Wi-Fi can use a private IP with explicitly enabled local debug HTTP.
Public domains/mobile networks require HTTPS; Tailscale can supply a protected
route, with HTTPS preferred. A private 192.168.x.x address does not become
reachable on LTE. The local HTTP switch defaults off. Normal system CAs are
required; there is no accept-all TLS or certificate-pin bypass. Loopback is a
debug-only simulator and is never evidence of Go/server delivery.

Proxy only `/api/v1/mobile/` to the private Orchestrator. Never publish `/api/`
or owner/settings/memory APIs to make phone ingress reachable. Disable body and
Authorization logging at the proxy, impose the same body limit and TLS, and
strip untrusted forwarding headers. The isolated test receiver exposes a
separate owner port and refuses non-`*_test` databases. It never starts models.

Use the [acceptance report](../testing/issue-57-android.md) for actual results;
this specification does not claim that an unexecuted test or CI job passed.
