CREATE TABLE extension_versions (
    manifest_hash text PRIMARY KEY CHECK (manifest_hash ~ '^[a-f0-9]{64}$'),
    extension_id text NOT NULL,
    version text NOT NULL,
    source_uri text NOT NULL,
    manifest_text text NOT NULL,
    installed_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    UNIQUE (extension_id, version)
);
CREATE TABLE extension_reviews (
    manifest_hash text PRIMARY KEY REFERENCES extension_versions(manifest_hash),
    owner_id text NOT NULL,
    profile_pin text NOT NULL,
    reviewed_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    source_json jsonb NOT NULL
);
CREATE TABLE extension_installations (
    extension_id text PRIMARY KEY,
    owner_id text NOT NULL,
    manifest_hash text NOT NULL REFERENCES extension_versions(manifest_hash),
    generation bigint NOT NULL CHECK (generation > 0),
    status text NOT NULL CHECK (status IN ('staged','disabled','enabled','quarantined')),
    config_json jsonb NOT NULL DEFAULT '{}',
    credential_refs_json jsonb NOT NULL DEFAULT '{}',
    allowed_tools_json jsonb NOT NULL DEFAULT '[]',
    health text NOT NULL DEFAULT 'unknown' CHECK (health IN ('unknown','healthy','failed')),
    health_reason text NOT NULL DEFAULT 'not_validated',
    healthy_until timestamptz,
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE extension_events (
    event_id bigserial PRIMARY KEY,
    extension_id text NOT NULL REFERENCES extension_installations(extension_id),
    manifest_hash text NOT NULL REFERENCES extension_versions(manifest_hash),
    generation bigint NOT NULL,
    kind text NOT NULL,
    source_json jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE extension_task_access (
    task_id text NOT NULL REFERENCES tasks(task_id),
    extension_id text NOT NULL REFERENCES extension_installations(extension_id),
    owner_id text NOT NULL,
    task_revision bigint NOT NULL,
    constraints_revision bigint NOT NULL,
    manifest_hash text NOT NULL REFERENCES extension_versions(manifest_hash),
    generation bigint NOT NULL,
    revision bigint NOT NULL CHECK (revision > 0),
    tools_json jsonb NOT NULL,
    resources_json jsonb NOT NULL,
    credential_refs_json jsonb NOT NULL,
    expires_at timestamptz NOT NULL,
    revoked boolean NOT NULL DEFAULT false,
    source_json jsonb NOT NULL,
    PRIMARY KEY(task_id,extension_id)
);
CREATE TABLE extension_probes (
    probe_id text PRIMARY KEY,
    extension_id text NOT NULL REFERENCES extension_installations(extension_id),
    manifest_hash text NOT NULL REFERENCES extension_versions(manifest_hash),
    generation bigint NOT NULL,
    config_hash text NOT NULL,
    state text NOT NULL CHECK (state IN ('executing','ready','failed','unknown')),
    result_json jsonb NOT NULL DEFAULT '{}',
    started_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    finished_at timestamptz
);
CREATE FUNCTION reject_extension_history_update() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'extension provenance and history are immutable'; END $$;
CREATE TRIGGER extension_version_immutable BEFORE UPDATE OR DELETE ON extension_versions FOR EACH ROW EXECUTE FUNCTION reject_extension_history_update();
CREATE TRIGGER extension_review_immutable BEFORE UPDATE OR DELETE ON extension_reviews FOR EACH ROW EXECUTE FUNCTION reject_extension_history_update();
CREATE TRIGGER extension_event_immutable BEFORE UPDATE OR DELETE ON extension_events FOR EACH ROW EXECUTE FUNCTION reject_extension_history_update();
