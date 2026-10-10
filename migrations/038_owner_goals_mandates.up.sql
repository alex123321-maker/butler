CREATE TABLE owner_goals (
 goal_id TEXT PRIMARY KEY,
 owner_id TEXT NOT NULL,
 kind TEXT NOT NULL CHECK(kind IN ('objective','preference','constraint','assumption')),
 status TEXT NOT NULL CHECK(status IN ('active','paused','revoked','proposed')),
 revision BIGINT NOT NULL CHECK(revision>0),
 expires_at TIMESTAMPTZ NOT NULL,
 goal_data JSONB NOT NULL,
 source_data JSONB NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 CHECK(kind<>'assumption' OR status='proposed')
);
CREATE INDEX owner_goals_owner ON owner_goals(owner_id,kind,status);
ALTER TABLE action_mandates ADD COLUMN scope_kind TEXT NOT NULL DEFAULT 'task' CHECK(scope_kind IN ('task','owner_goal'));
ALTER TABLE action_mandates ADD COLUMN goal_id TEXT REFERENCES owner_goals(goal_id);
ALTER TABLE action_mandates ADD COLUMN goal_revision BIGINT NOT NULL DEFAULT 0;
ALTER TABLE action_mandates ADD COLUMN status TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active','paused','revoked'));
CREATE INDEX action_mandates_goal ON action_mandates(goal_id,mandate_id);
CREATE TABLE task_authority_bindings (
 task_id TEXT PRIMARY KEY REFERENCES tasks(task_id),
 owner_id TEXT NOT NULL,
 goal_id TEXT NOT NULL REFERENCES owner_goals(goal_id),
 goal_revision BIGINT NOT NULL CHECK(goal_revision>0),
 mandate_id TEXT NOT NULL REFERENCES action_mandates(mandate_id),
 mandate_revision BIGINT NOT NULL CHECK(mandate_revision>0),
 purpose TEXT NOT NULL,
 initiative_reason TEXT NOT NULL,
 observation_ref TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX task_authority_mandate ON task_authority_bindings(mandate_id,mandate_revision);
CREATE INDEX task_authority_goal ON task_authority_bindings(goal_id,goal_revision);
CREATE TABLE owner_authority_events (
 event_id TEXT PRIMARY KEY,
 owner_id TEXT NOT NULL,
 source TEXT NOT NULL,
 delivery_id TEXT NOT NULL,
 subject_kind TEXT NOT NULL CHECK(subject_kind IN ('goal','mandate')),
 subject_id TEXT NOT NULL,
 revision BIGINT NOT NULL,
 request_hash TEXT NOT NULL,
 snapshot JSONB NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 UNIQUE(owner_id,source,delivery_id)
);
CREATE TABLE owner_authority_jobs (
 event_id TEXT PRIMARY KEY REFERENCES owner_authority_events(event_id),
 owner_id TEXT NOT NULL,
 subject_kind TEXT NOT NULL,
 subject_id TEXT NOT NULL,
 current_revision BIGINT NOT NULL,
 control_kind TEXT NOT NULL CHECK(control_kind IN ('pause','stop')),
 state TEXT NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','applied')),
 attempts INT NOT NULL DEFAULT 0,
 last_error TEXT NOT NULL DEFAULT '',
 updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
