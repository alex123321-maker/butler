-- Authority remains in task/ownership/resource stores. These rows are immutable
-- namespace bindings and physical-file observations, not permission grants.
CREATE TABLE code_workspaces (
  workspace_id text PRIMARY KEY CHECK (workspace_id ~ '^[a-f0-9]{32}$'),
  owner_id text NOT NULL,
  session_key text NOT NULL,
  task_id text NOT NULL,
  task_revision bigint NOT NULL CHECK (task_revision > 0),
  constraints_revision bigint NOT NULL CHECK (constraints_revision > 0),
  state text NOT NULL DEFAULT 'active' CHECK (state IN ('active','blocked')),
  generation bigint NOT NULL DEFAULT 1 CHECK (generation > 0),
  blocked_reason text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX code_workspaces_task_idx ON code_workspaces(owner_id,task_id);
CREATE TABLE code_workspace_files (
  workspace_id text NOT NULL REFERENCES code_workspaces(workspace_id),
  path text NOT NULL,
  version bigint NOT NULL CHECK (version > 0),
  content_sha256 text NOT NULL CHECK (content_sha256 ~ '^[a-f0-9]{64}$'),
  byte_length bigint NOT NULL CHECK (byte_length >= 0),
  updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  PRIMARY KEY(workspace_id,path)
);
