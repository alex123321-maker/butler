CREATE TABLE initiative_sources (
 source_id TEXT PRIMARY KEY, owner_id TEXT NOT NULL, goal_id TEXT NOT NULL,
 mandate_id TEXT NOT NULL, session_key TEXT NOT NULL REFERENCES sessions(session_key),
 source_json JSONB NOT NULL, enabled BOOLEAN NOT NULL, revision BIGINT NOT NULL DEFAULT 1,
 last_promoted_at TIMESTAMPTZ,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(), updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE initiative_schedules (
 schedule_id TEXT PRIMARY KEY, source_id TEXT NOT NULL REFERENCES initiative_sources(source_id),
 schedule_json JSONB NOT NULL, next_at TIMESTAMPTZ NOT NULL, enabled BOOLEAN NOT NULL,
 revision BIGINT NOT NULL DEFAULT 1, updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX initiative_schedules_ready ON initiative_schedules(next_at) WHERE enabled;
CREATE TABLE initiative_ticks (
 schedule_id TEXT NOT NULL REFERENCES initiative_schedules(schedule_id), schedule_revision BIGINT NOT NULL,
 slot_key TEXT NOT NULL, scheduled_at TIMESTAMPTZ NOT NULL, observed_at TIMESTAMPTZ NOT NULL,
 missed_count BIGINT NOT NULL, status TEXT NOT NULL CHECK(status IN ('pending','checked','skipped','failed')),
 source_revision BIGINT NOT NULL, claim_id TEXT NOT NULL DEFAULT '', claim_expires_at TIMESTAMPTZ,
 reason TEXT NOT NULL, PRIMARY KEY(schedule_id,schedule_revision,slot_key)
);
CREATE TABLE initiatives (
 initiative_id TEXT PRIMARY KEY, owner_id TEXT NOT NULL, goal_id TEXT NOT NULL, mandate_id TEXT NOT NULL,
 source_id TEXT NOT NULL REFERENCES initiative_sources(source_id), source_revision BIGINT NOT NULL,
 fingerprint TEXT NOT NULL, observation_ref TEXT NOT NULL,
 status TEXT NOT NULL CHECK(status IN ('observing','ready','proposed','deferred','promoted','completed','rejected','suppressed')),
 reason TEXT NOT NULL, occurrences INT NOT NULL, confidence INT NOT NULL, importance INT NOT NULL,
 expected_model_calls INT NOT NULL DEFAULT 0, attention_cost INT NOT NULL DEFAULT 0,
 not_before TIMESTAMPTZ NOT NULL, last_observed_at TIMESTAMPTZ NOT NULL,
 task_id TEXT UNIQUE REFERENCES tasks(task_id), input_id TEXT UNIQUE REFERENCES accepted_inputs(input_id),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(), updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(owner_id,fingerprint)
);
CREATE TABLE initiative_events (
 source_id TEXT NOT NULL REFERENCES initiative_sources(source_id), event_id TEXT NOT NULL,
 event_json JSONB NOT NULL, initiative_id TEXT REFERENCES initiatives(initiative_id), verdict TEXT NOT NULL,
 observed_at TIMESTAMPTZ NOT NULL, recorded_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(source_id,event_id)
);
CREATE INDEX initiatives_ready ON initiatives(not_before) WHERE status IN ('ready','deferred');
CREATE TABLE initiative_config_events (
 owner_id TEXT NOT NULL, channel TEXT NOT NULL, delivery_id TEXT NOT NULL,
 request_hash TEXT NOT NULL, subject_id TEXT NOT NULL, subject_kind TEXT NOT NULL,
 revision BIGINT NOT NULL, recorded_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(owner_id,channel,delivery_id)
);
CREATE TABLE initiative_attention_days (
 owner_id TEXT NOT NULL, day DATE NOT NULL, admitted_tasks INT NOT NULL DEFAULT 0 CHECK(admitted_tasks BETWEEN 0 AND 20),
 PRIMARY KEY(owner_id,day)
);
