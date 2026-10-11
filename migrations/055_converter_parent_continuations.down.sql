DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM converter_parent_continuations) THEN
  RAISE EXCEPTION 'cannot discard retained converter continuation history';
 END IF;
END $$;
DROP TABLE converter_parent_continuations;
DROP FUNCTION converter_continuation_immutable();
