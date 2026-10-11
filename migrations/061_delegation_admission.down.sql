DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM delegation_admission_snapshots) THEN
  RAISE EXCEPTION 'cannot remove nonempty accepted delegation authority history';
 END IF;
END $$;
DROP TABLE delegation_admission_snapshots;
DROP FUNCTION reject_delegation_admission_mutation();
