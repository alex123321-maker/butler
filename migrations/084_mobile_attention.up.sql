-- No notification text is duplicated outside the versioned catalog.
CREATE UNIQUE INDEX mobile_one_source ON initiative_sources(owner_id) WHERE source_json->>'observer_kind'='mobile_inbox';
CREATE TABLE mobile_agent_access (
 owner_id TEXT PRIMARY KEY, enabled BOOLEAN NOT NULL, revision BIGINT NOT NULL DEFAULT 1
);
ALTER TABLE mobile_events ADD COLUMN messages_indexed BOOLEAN NOT NULL DEFAULT false;
CREATE TABLE mobile_attention_batches (
 batch_id TEXT PRIMARY KEY, owner_id TEXT NOT NULL, source_id TEXT NOT NULL REFERENCES initiative_sources(source_id),
 source_revision BIGINT NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 finished_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX mobile_one_attention_batch ON mobile_attention_batches(owner_id) WHERE finished_at IS NULL;
CREATE TABLE mobile_messages (
 message_id TEXT PRIMARY KEY, owner_id TEXT NOT NULL, device_id TEXT NOT NULL REFERENCES mobile_devices(device_id),
 source_id TEXT NOT NULL REFERENCES memory_sources(source_id), part INTEGER NOT NULL,
 received_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 batch_id TEXT REFERENCES mobile_attention_batches(batch_id),
 state TEXT NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','remembered','ignored','noise')),
 result_source_id TEXT REFERENCES memory_sources(source_id)
);
CREATE INDEX mobile_pending_messages ON mobile_messages(owner_id,received_at,message_id) WHERE state='pending';
CREATE TABLE mobile_noise_rules (
 rule_id TEXT PRIMARY KEY REFERENCES memory_sources(source_id), owner_id TEXT NOT NULL,
 enabled BOOLEAN NOT NULL DEFAULT true, created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
