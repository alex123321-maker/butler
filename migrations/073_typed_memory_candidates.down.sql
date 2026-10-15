DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM memory_claims WHERE memory_type='episodic' OR episode_id IS NOT NULL)
 OR EXISTS(SELECT 1 FROM memory_episodes WHERE embedding IS NULL) THEN
  RAISE EXCEPTION 'typed candidate history must be preserved; rollback denied';
 END IF;
END $$;
ALTER TABLE memory_episodes ALTER COLUMN embedding SET NOT NULL;
ALTER TABLE memory_claims DROP CONSTRAINT memory_claim_single_projection;
ALTER TABLE memory_claims DROP COLUMN episode_id;
ALTER TABLE memory_claims DROP COLUMN memory_type;
