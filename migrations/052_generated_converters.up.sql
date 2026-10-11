-- Executable/prose/document bytes remain deletion-linked Catalog DATA only.
CREATE TABLE converter_needs (
 owner_id TEXT NOT NULL,
 need_id TEXT NOT NULL,
 converter_id TEXT NOT NULL,
 need_hash TEXT NOT NULL,
 source_ref_json JSONB NOT NULL,
 task_id TEXT NOT NULL REFERENCES tasks(task_id),
 task_revision BIGINT NOT NULL CHECK(task_revision>0),
 constraints_revision BIGINT NOT NULL CHECK(constraints_revision>0),
 run_id TEXT NOT NULL REFERENCES runs(run_id),
 discovery_json JSONB NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 PRIMARY KEY(owner_id,need_id)
);
CREATE TABLE converter_catalog (
 owner_id TEXT NOT NULL,
 converter_id TEXT NOT NULL,
 generation BIGINT NOT NULL CHECK(generation>0),
 active_digest TEXT,
 updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 PRIMARY KEY(owner_id,converter_id)
);
CREATE TABLE converter_versions (
 owner_id TEXT NOT NULL,
 digest TEXT NOT NULL,
 converter_id TEXT NOT NULL,
 version TEXT NOT NULL,
 need_id TEXT NOT NULL,
 need_hash TEXT NOT NULL,
 source_ref_json JSONB NOT NULL,
 profile_pin TEXT NOT NULL,
 permission_shape_json JSONB NOT NULL,
 created_by_run TEXT NOT NULL REFERENCES runs(run_id),
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 PRIMARY KEY(owner_id,digest),
 UNIQUE(owner_id,converter_id,version),
 FOREIGN KEY(owner_id,converter_id) REFERENCES converter_catalog(owner_id,converter_id),
 FOREIGN KEY(owner_id,need_id) REFERENCES converter_needs(owner_id,need_id)
);
ALTER TABLE converter_catalog ADD FOREIGN KEY(owner_id,active_digest) REFERENCES converter_versions(owner_id,digest);
CREATE TABLE converter_states (
 owner_id TEXT NOT NULL,
 digest TEXT NOT NULL,
 status TEXT NOT NULL CHECK(status IN ('candidate','tested','active','obsolete','revoked')),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 PRIMARY KEY(owner_id,digest),
 FOREIGN KEY(owner_id,digest) REFERENCES converter_versions(owner_id,digest)
);
CREATE TABLE converter_validations (
 validation_id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 owner_id TEXT NOT NULL,
 digest TEXT NOT NULL,
 run_id TEXT NOT NULL REFERENCES runs(run_id),
 action_id TEXT NOT NULL REFERENCES action_effects(action_id),
 report_json JSONB NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 FOREIGN KEY(owner_id,digest) REFERENCES converter_versions(owner_id,digest),
 UNIQUE(owner_id,action_id)
);
CREATE TABLE converter_trials (
 owner_id TEXT NOT NULL,
 action_id TEXT NOT NULL REFERENCES action_effects(action_id),
 digest TEXT NOT NULL,
 run_id TEXT NOT NULL REFERENCES runs(run_id),
 task_id TEXT NOT NULL REFERENCES tasks(task_id),
 task_revision BIGINT NOT NULL CHECK(task_revision>0),
 input_ref_json JSONB NOT NULL,
 status TEXT NOT NULL CHECK(status IN ('running','passed','failed','unknown')),
 output_ref_json JSONB,
 output_hash TEXT NOT NULL DEFAULT '',
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 finished_at TIMESTAMPTZ,
 PRIMARY KEY(owner_id,action_id),
 FOREIGN KEY(owner_id,digest) REFERENCES converter_versions(owner_id,digest)
);
CREATE TABLE converter_usages (
 owner_id TEXT NOT NULL,
 action_id TEXT NOT NULL REFERENCES action_effects(action_id),
 digest TEXT NOT NULL,
 run_id TEXT NOT NULL REFERENCES runs(run_id),
 task_id TEXT NOT NULL REFERENCES tasks(task_id),
 task_revision BIGINT NOT NULL CHECK(task_revision>0),
 constraints_revision BIGINT NOT NULL CHECK(constraints_revision>0),
 generation BIGINT NOT NULL CHECK(generation>0),
 output_ref_json JSONB NOT NULL,
 output_hash TEXT NOT NULL,
 recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 PRIMARY KEY(owner_id,action_id),
 FOREIGN KEY(owner_id,digest) REFERENCES converter_versions(owner_id,digest)
);
CREATE TABLE converter_events (
 event_id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 owner_id TEXT NOT NULL,
 converter_id TEXT NOT NULL,
 digest TEXT,
 generation BIGINT NOT NULL CHECK(generation>0),
 kind TEXT NOT NULL,
 run_id TEXT,
 action_id TEXT,

 source_channel TEXT NOT NULL DEFAULT '',
 delivery_id TEXT NOT NULL DEFAULT '',
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 FOREIGN KEY(owner_id,converter_id) REFERENCES converter_catalog(owner_id,converter_id)
);
CREATE UNIQUE INDEX converter_owner_deliveries ON converter_events(owner_id,source_channel,delivery_id) WHERE delivery_id<>'';
CREATE FUNCTION converter_immutable_record() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'converter need/package/validation/use history is immutable'; END;
$$;
CREATE TRIGGER converter_need_immutable BEFORE UPDATE OR DELETE ON converter_needs FOR EACH ROW EXECUTE FUNCTION converter_immutable_record();
CREATE TRIGGER converter_version_immutable BEFORE UPDATE OR DELETE ON converter_versions FOR EACH ROW EXECUTE FUNCTION converter_immutable_record();
CREATE TRIGGER converter_validation_immutable BEFORE UPDATE OR DELETE ON converter_validations FOR EACH ROW EXECUTE FUNCTION converter_immutable_record();
CREATE TRIGGER converter_use_immutable BEFORE UPDATE OR DELETE ON converter_usages FOR EACH ROW EXECUTE FUNCTION converter_immutable_record();
CREATE TRIGGER converter_event_immutable BEFORE UPDATE OR DELETE ON converter_events FOR EACH ROW EXECUTE FUNCTION converter_immutable_record();
