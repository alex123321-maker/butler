DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM execution_resource_leases) OR EXISTS(SELECT 1 FROM execution_resources) THEN
  RAISE EXCEPTION 'resource lease history must be preserved before downgrade';
 END IF;
END $$;
DROP TABLE execution_resource_leases;
DROP TABLE execution_resources;
