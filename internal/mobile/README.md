# Android observation ingress

`Store` owns device credentials, pairing and content-free delivery receipts.
PostgreSQL is authoritative. Devices initiate HTTP; there is no model, Redis
queue, tool invocation, owner-command conversion or per-notification run here.
The Orchestrator mounts the handlers and runs bounded retention in maintenance.

Device tokens are independent random 256-bit bearer capabilities stored as
SHA-256 hashes on the server. They permit status, writing this device's bounded
notification batches, rotation and self-disconnection only. Owner identity comes
from the locked device row. Owner controls require existing OwnerControlAuth.
An owner-entered phone verification code approves that pending request; the
separate random session capability, held on the phone, retrieves its token once.
An interrupted exchange requires a fresh pairing, never recovery with a public
device ID. Disconnect generates a fresh local device identity.

The device row serializes batches, revocation and rotation. A single transaction
creates immutable event receipts and private catalog sources; ACK
is emitted after Commit. The server hashes the entire normalized event, rather
than trusting the phone's content_hash. Same ID/different body conflicts. Batch
identity also binds its body. Receipts survive content deletion and retries.

Sources use existing memory deletion/restore guards. Each event is an immutable
source revision; notification_hash and previous_device_event_id link card
updates/removals without overwriting earlier observations. Only the catalog
holds text. Owner deletion and deterministic retention use its fsynced external
anchor and retain tombstones. Explicit forgetting erases all privacy descendants; archive_expire preserves selected notes and their exact retained evidence. Missing/rolled-back anchors
deny receipt creation and private reads. Retention is 1–365 days, default360,
bounded to 100 sources per sweep. Content-bearing backups retain the existing
30-day maximum and require the latest independent deletion anchor on restore.
Retention calls the catalog's narrow expired-mobile-source operation, which
rechecks expiry under the current device-policy lock. It does not fabricate an
authenticated owner request or allow deletion of arbitrary sources.

Ingress remains `private_observation_only` and invokes no models. The owner may
enable the installed `mobile_inbox` initiative once; this activates mobile DATA
reads and starts the existing scheduler's20-message/hour processing path.
Without that standing permission originals remain quarantined. Notes/rules use
Catalog derivatives and current Broker/task/mandate authority, not device auth.
`messages.go` projects distinct message identities without copying text; bounded
backfill supports already saved events. The owning Orchestrator composes the
observer and Tool Broker controls. Noise suppresses attention, retains search,
and can be undone via the owner attention view. Explicit source deletion erases dependent notes/rules and retained evidence; automatic expiry preserves retained memory. No content is logged; diagnostics contain counters.

Run `go test ./internal/mobile` on Linux with
`BUTLER_TEST_MEMORY_POSTGRES_URL=postgres://.../mobile_test`. Every integration
fixture uses a fresh schema, all migrations, MaxConns2 and a real deletion anchor.
Without that variable integration cases explicitly skip, which is not acceptance.
See [protocol and operations](../../docs/architecture/android-companion.md).

Optional SMS login relay uses a separate `/api/v1/mobile/otp/login` endpoint.
It authenticates and locks the paired device, applies a device rate limit, then
delegates matching/encryption to the credential subsystem in the same transaction.
It never creates a catalog source, accepted Input or model message. Account
route permissions plus an unambiguous approved login challenge are required;
device authentication alone is insufficient. The endpoint returns only accepted
or declined, never the code or matched account. See the credential vault spec.
