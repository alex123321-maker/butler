CREATE TABLE skill_versions (
 digest text PRIMARY KEY CHECK(digest ~ '^[a-f0-9]{64}$'), skill_id text NOT NULL,
 version text NOT NULL, owner_id text NOT NULL, source_ref_json jsonb NOT NULL,
 dependencies_json jsonb NOT NULL, correction_shape text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), UNIQUE(owner_id,skill_id,version)
);
CREATE TABLE skill_states (
 digest text PRIMARY KEY REFERENCES skill_versions(digest),
 status text NOT NULL CHECK(status IN ('candidate','tested','active','obsolete','revoked'))
);
CREATE TABLE skill_catalog (
 skill_id text PRIMARY KEY, owner_id text NOT NULL, active_digest text REFERENCES skill_versions(digest),
 generation bigint NOT NULL CHECK(generation>0)
);
CREATE TABLE skill_validations (
 validation_id bigserial PRIMARY KEY, digest text NOT NULL REFERENCES skill_versions(digest),
 report_json jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE skill_events (
 event_id bigserial PRIMARY KEY, skill_id text NOT NULL REFERENCES skill_catalog(skill_id),
 digest text NOT NULL REFERENCES skill_versions(digest), generation bigint NOT NULL,
 kind text NOT NULL, source_json jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE skill_usages (
 run_id text NOT NULL REFERENCES runs(run_id), digest text NOT NULL REFERENCES skill_versions(digest),
 task_id text NOT NULL REFERENCES tasks(task_id), task_revision bigint NOT NULL, constraints_revision bigint NOT NULL,
 loaded_at timestamptz NOT NULL DEFAULT clock_timestamp(), PRIMARY KEY(run_id,digest)
);
CREATE TABLE skill_correction_grants (
 grant_id text PRIMARY KEY, skill_id text NOT NULL REFERENCES skill_catalog(skill_id),
 parent_digest text NOT NULL REFERENCES skill_versions(digest), owner_id text NOT NULL,
 class text NOT NULL CHECK(class='descriptive_correction_v1'), mandate_id text NOT NULL,
 mandate_revision bigint NOT NULL, purpose text NOT NULL, expires_at timestamptz NOT NULL,
 revoked boolean NOT NULL DEFAULT false, source_json jsonb NOT NULL
);
CREATE FUNCTION reject_skill_history_update() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'skill versions, validation, usage and provenance are immutable'; END $$;
CREATE TRIGGER skill_version_immutable BEFORE UPDATE OR DELETE ON skill_versions FOR EACH ROW EXECUTE FUNCTION reject_skill_history_update();
CREATE TRIGGER skill_validation_immutable BEFORE UPDATE OR DELETE ON skill_validations FOR EACH ROW EXECUTE FUNCTION reject_skill_history_update();
CREATE TRIGGER skill_event_immutable BEFORE UPDATE OR DELETE ON skill_events FOR EACH ROW EXECUTE FUNCTION reject_skill_history_update();
CREATE TRIGGER skill_usage_immutable BEFORE UPDATE OR DELETE ON skill_usages FOR EACH ROW EXECUTE FUNCTION reject_skill_history_update();
