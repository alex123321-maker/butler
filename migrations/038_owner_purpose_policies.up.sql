-- Separate additive step keeps an already tested foundation migration immutable.
CREATE TABLE owner_purpose_policies (
 owner_id TEXT NOT NULL,
 purpose TEXT NOT NULL,
 revision BIGINT NOT NULL DEFAULT 0 CHECK(revision>=0),
 PRIMARY KEY(owner_id,purpose)
);
INSERT INTO owner_purpose_policies(owner_id,purpose,revision)
 SELECT owner_id,goal_data->>'purpose',CASE WHEN bool_or(kind IN ('preference','constraint')) THEN 1 ELSE 0 END
 FROM owner_goals GROUP BY owner_id,goal_data->>'purpose';
ALTER TABLE task_authority_bindings ADD COLUMN policy_revision BIGINT NOT NULL DEFAULT 0 CHECK(policy_revision>=0);
-- Existing tasks are deliberately stale when confirmed purpose policy exists;
-- migration never invents proof that an old task reviewed those constraints.
