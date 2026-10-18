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
creates immutable event receipts and private quarantined catalog sources; ACK
is emitted after Commit. The server hashes the entire normalized event, rather
than trusting the phone's content_hash. Same ID/different body conflicts. Batch
identity also binds its body. Receipts survive content deletion and retries.

Sources use existing memory deletion/restore guards. Each event is an immutable
source revision; notification_hash and previous_device_event_id link card
updates/removals without overwriting earlier observations. Only the catalog
holds text. Owner deletion and deterministic retention use its fsynced external
anchor, erase derivatives and retain tombstones. Missing/rolled-back anchors
deny receipt creation and private reads. Retention is 1–365 days, default 30,
bounded to 100 sources per sweep. Content-bearing backups retain the existing
30-day maximum and require the latest independent deletion anchor on restore.
Retention calls the catalog's narrow expired-mobile-source operation, which
rechecks expiry under the current device-policy lock. It does not fabricate an
authenticated owner request or allow deletion of arbitrary sources.

Significance is typed `private_observation_only`: no rule is currently admitted
to invoke models, create tasks, send notices or extract claims. An owner can
explicitly review sources and use the existing versioned purpose-grant controls;
the phone cannot mint those decisions. Search/reveal are owner-only. No content
is logged; existing path/status/latency instrumentation and per-device counters
provide diagnostics without event text or credential labels in metrics.

Run `go test ./internal/mobile` on Linux with
`BUTLER_TEST_MEMORY_POSTGRES_URL=postgres://.../mobile_test`. Every integration
fixture uses a fresh schema, all migrations, MaxConns2 and a real deletion anchor.
Without that variable integration cases explicitly skip, which is not acceptance.
See [protocol and operations](../../docs/architecture/android-companion.md).
