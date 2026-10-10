CREATE TABLE task_delegations (
 task_id TEXT PRIMARY KEY REFERENCES tasks(task_id) ON DELETE CASCADE,
 root_task_id TEXT NOT NULL REFERENCES tasks(task_id),
 parent_task_id TEXT NOT NULL REFERENCES tasks(task_id),
 parent_revision BIGINT NOT NULL CHECK(parent_revision>0),
 parent_constraints_revision BIGINT NOT NULL CHECK(parent_constraints_revision>0),
 contract_revision BIGINT NOT NULL DEFAULT 1 CHECK(contract_revision=1),
 contract_source_id TEXT NOT NULL REFERENCES memory_sources(source_id),
 contract_json JSONB NOT NULL,
 budget_json JSONB NOT NULL,
 deadline TIMESTAMPTZ NOT NULL,
 depth INT NOT NULL CHECK(depth BETWEEN 1 AND 3),
 state TEXT NOT NULL CHECK(state IN ('waiting','queued','executing','blocked','result_ready','accepted','rejected','cancelled')),
 input_id TEXT NOT NULL DEFAULT '',
 result_id TEXT NOT NULL DEFAULT '',
 decision_evidence_json JSONB NOT NULL DEFAULT '[]',
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 CHECK(task_id<>parent_task_id)
);
CREATE INDEX task_delegations_root ON task_delegations(root_task_id,created_at);
CREATE TABLE task_delegation_dependencies (
 task_id TEXT NOT NULL REFERENCES task_delegations(task_id) ON DELETE CASCADE,
 depends_on_task_id TEXT NOT NULL REFERENCES task_delegations(task_id),
 PRIMARY KEY(task_id,depends_on_task_id), CHECK(task_id<>depends_on_task_id)
);

-- Every bounded branch remains subordinate to the current parent revisions.
-- This is read by ordinary model admission, dispatch and final-result writers.
CREATE FUNCTION butler_task_check_delegation(tid TEXT) RETURNS VOID LANGUAGE plpgsql AS $$
DECLARE d RECORD;
BEGIN
 FOR d IN WITH RECURSIVE chain AS (
   SELECT task_id,parent_task_id FROM tasks WHERE task_id=tid
   UNION SELECT t.task_id,t.parent_task_id FROM tasks t JOIN chain c ON t.task_id=c.parent_task_id
 ) SELECT x.task_id,x.parent_revision,x.parent_constraints_revision,x.deadline,x.state,
          p.revision,p.constraints_revision,p.status,p.owner_id,c.owner_id AS child_owner
 FROM chain q JOIN task_delegations x ON x.task_id=q.task_id
 JOIN tasks p ON p.task_id=x.parent_task_id JOIN tasks c ON c.task_id=x.task_id
 ORDER BY x.task_id FOR SHARE OF x,p LOOP
  IF d.status<>'active' OR d.owner_id<>d.child_owner OR d.revision<>d.parent_revision
     OR d.constraints_revision<>d.parent_constraints_revision OR d.deadline<=clock_timestamp()
     OR d.state NOT IN ('queued','executing','result_ready') THEN
    RAISE EXCEPTION 'delegated task has stale parent authority' USING ERRCODE='40001';
  END IF;
 END LOOP;
END $$;
ALTER TABLE execution_budget_scopes DROP CONSTRAINT execution_budget_scopes_kind_check;
ALTER TABLE execution_budget_scopes ADD CONSTRAINT execution_budget_scopes_kind_check
 CHECK(kind IN ('task','mandate','owner_day','conversation','auxiliary','delegation'));
