-- Owner-directed derivative cleanup; original sources and all job/spend history
-- survive. The existing building/ready state is retained for audit.
ALTER TABLE semantic_index_generations
 ADD COLUMN retired_at TIMESTAMPTZ,
 ADD COLUMN retired_by TEXT NOT NULL DEFAULT '',
 ADD CONSTRAINT semantic_generation_retirement_identity
 CHECK ((retired_at IS NULL AND retired_by='') OR (retired_at IS NOT NULL AND retired_by<>''));

CREATE FUNCTION butler_semantic_retirement_immutable() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.retired_at IS NOT NULL AND
    (NEW.retired_at,NEW.retired_by) IS DISTINCT FROM (OLD.retired_at,OLD.retired_by) THEN
  RAISE EXCEPTION 'semantic generation retirement is irreversible';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER semantic_retirement_immutable BEFORE UPDATE ON semantic_index_generations
 FOR EACH ROW EXECUTE FUNCTION butler_semantic_retirement_immutable();
