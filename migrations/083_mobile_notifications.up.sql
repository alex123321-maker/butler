-- Mobile receipts contain identifiers/hashes only. Content belongs to the
-- versioned source catalog and its deletion/restore fence.
CREATE TABLE mobile_devices (
 device_id TEXT PRIMARY KEY, owner_id TEXT NOT NULL, device_name TEXT NOT NULL,
 token_hash TEXT NOT NULL UNIQUE, status TEXT NOT NULL DEFAULT 'ACTIVE'
 CHECK(status IN ('ACTIVE','PAUSED','REVOKED')),
 android_sdk INTEGER NOT NULL, app_version TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(), last_seen_at TIMESTAMPTZ,
 retention_days INTEGER NOT NULL DEFAULT 30 CHECK(retention_days BETWEEN 1 AND 365),
 request_window TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(), request_count INTEGER NOT NULL DEFAULT 0,
 accepted_count BIGINT NOT NULL DEFAULT 0, duplicate_count BIGINT NOT NULL DEFAULT 0,
 rejected_count BIGINT NOT NULL DEFAULT 0
);
CREATE INDEX mobile_devices_owner ON mobile_devices(owner_id,created_at);
CREATE TABLE mobile_pairings (
 session_hash TEXT PRIMARY KEY, code_hash TEXT NOT NULL UNIQUE, owner_id TEXT,
 device_id TEXT NOT NULL DEFAULT '', device_name TEXT NOT NULL DEFAULT '',
 android_sdk INTEGER NOT NULL DEFAULT 0, app_version TEXT NOT NULL DEFAULT '',
 status TEXT NOT NULL CHECK(status IN ('PENDING','APPROVED','CONSUMED','REJECTED')),
 expires_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()+interval '5 minutes'
);
CREATE TABLE mobile_events (
 device_id TEXT NOT NULL REFERENCES mobile_devices(device_id),
 device_event_id TEXT NOT NULL, event_hash TEXT NOT NULL,
 notification_hash TEXT NOT NULL, package_name TEXT NOT NULL,
 previous_device_event_id TEXT, change_type TEXT NOT NULL,
 observed_at TIMESTAMPTZ NOT NULL, posted_at TIMESTAMPTZ NOT NULL,
 received_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 source_id TEXT NOT NULL REFERENCES memory_sources(source_id),
 significance TEXT NOT NULL DEFAULT 'private_observation_only'
 CHECK(significance='private_observation_only'),
 PRIMARY KEY(device_id,device_event_id)
);
CREATE INDEX mobile_events_recent ON mobile_events(device_id,received_at DESC);
CREATE INDEX mobile_events_card ON mobile_events(device_id,notification_hash,observed_at);
CREATE TABLE mobile_batches (
 device_id TEXT NOT NULL REFERENCES mobile_devices(device_id),
 batch_id TEXT NOT NULL, body_hash TEXT NOT NULL,
 PRIMARY KEY(device_id,batch_id)
);
CREATE TABLE mobile_request_limits (
 key_hash TEXT PRIMARY KEY, window_start TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 request_count INTEGER NOT NULL DEFAULT 1
);
