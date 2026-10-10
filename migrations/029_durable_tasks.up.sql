-- Tasks own durable work; a run is one attempt and may have no task.
CREATE TABLE tasks (
    task_id TEXT PRIMARY KEY,
    session_key TEXT NOT NULL REFERENCES sessions(session_key) ON DELETE CASCADE,
    owner_id TEXT NOT NULL,
    parent_task_id TEXT REFERENCES tasks(task_id),
    goal TEXT NOT NULL CHECK (length(btrim(goal)) > 0),
    source_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','waiting_for_user','blocked','paused','cancelled','completed')),
    revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
    constraints_revision BIGINT NOT NULL DEFAULT 1 CHECK (constraints_revision > 0),
    constraints_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    completion_criteria_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    working_json JSONB NOT NULL DEFAULT '{"plan":[],"pending_actions":[],"scratch":{}}'::jsonb,
    working_revision BIGINT NOT NULL DEFAULT 1 CHECK (working_revision > 0),
    last_run_id TEXT REFERENCES runs(run_id) ON DELETE SET NULL,
    completion_result_id TEXT,
    legacy BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ
);
CREATE INDEX tasks_session_updated_idx ON tasks(session_key, updated_at DESC);
CREATE INDEX tasks_parent_idx ON tasks(parent_task_id) WHERE parent_task_id IS NOT NULL;
ALTER TABLE runs ADD COLUMN task_id TEXT REFERENCES tasks(task_id);
ALTER TABLE runs ADD COLUMN task_revision BIGINT;
ALTER TABLE runs ADD CONSTRAINT runs_task_revision_check CHECK ((task_id IS NULL AND task_revision IS NULL) OR (task_id IS NOT NULL AND task_revision > 0));
CREATE INDEX runs_task_started_idx ON runs(task_id, started_at) WHERE task_id IS NOT NULL;
CREATE TABLE task_results (
    result_id TEXT PRIMARY KEY,
    task_id TEXT NOT NULL REFERENCES tasks(task_id) ON DELETE CASCADE,
    task_revision BIGINT NOT NULL CHECK (task_revision > 0),
    run_id TEXT REFERENCES runs(run_id) ON DELETE SET NULL,
    summary TEXT NOT NULL,
    evidence_refs_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    verified_criteria_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    verdict TEXT NOT NULL CHECK (verdict IN ('unverified','verified')),
    verifier TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(task_id, result_id)
);
ALTER TABLE tasks ADD CONSTRAINT tasks_completion_result_fk FOREIGN KEY(task_id, completion_result_id) REFERENCES task_results(task_id, result_id);
-- Preserve every historical run URL and transcript. Legacy final answers are
-- unverified results, never proof of completed user work.
INSERT INTO tasks (task_id, session_key, owner_id, goal, source_json, status, last_run_id, legacy, created_at, updated_at)
SELECT r.run_id, r.session_key, s.user_id,
       COALESCE(NULLIF(btrim(first_user.content), ''), 'Legacy run ' || r.run_id),
       jsonb_build_object('type','legacy_run','run_id',r.run_id,'channel',s.channel,'completion_verified',false),
       CASE WHEN r.current_state = 'cancelled' THEN 'cancelled'
            WHEN r.current_state IN ('completed','failed','timed_out') THEN 'blocked'
            WHEN r.current_state = 'awaiting_approval' THEN 'waiting_for_user'
            ELSE 'paused' END,
       r.run_id, true, r.started_at, r.updated_at
FROM runs r JOIN sessions s ON s.session_key = r.session_key
LEFT JOIN LATERAL (SELECT content FROM messages WHERE run_id = r.run_id AND role = 'user' ORDER BY created_at LIMIT 1) first_user ON true;
UPDATE runs SET task_id = run_id, task_revision = 1;
UPDATE tasks t SET goal = COALESCE(NULLIF(btrim(w.goal), ''), t.goal),
    working_json = jsonb_build_object('plan', '[]'::jsonb, 'pending_actions', '[]'::jsonb,
        'scratch', w.scratch || jsonb_build_object('legacy_pending_steps',w.pending_steps,'legacy_entities',w.entities))
FROM memory_working w WHERE w.run_id = t.task_id;
INSERT INTO task_results (result_id, task_id, task_revision, run_id, summary, verdict, verifier, created_at)
SELECT 'legacy:' || r.run_id, r.run_id, 1, r.run_id, m.content, 'unverified', 'legacy_migration', r.updated_at
FROM runs r JOIN LATERAL (SELECT content FROM messages WHERE run_id = r.run_id AND role = 'assistant' ORDER BY created_at DESC LIMIT 1) m ON true;
