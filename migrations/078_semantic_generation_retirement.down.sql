DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM semantic_index_generations WHERE retired_at IS NOT NULL) THEN
  RAISE EXCEPTION 'cannot discard semantic retirement history';
 END IF;
END $$;
DROP TRIGGER semantic_retirement_immutable ON semantic_index_generations;
DROP FUNCTION butler_semantic_retirement_immutable();
ALTER TABLE semantic_index_generations DROP CONSTRAINT semantic_generation_retirement_identity,
 DROP COLUMN retired_by,DROP COLUMN retired_at;
