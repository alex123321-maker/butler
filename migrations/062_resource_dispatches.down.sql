DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM execution_resource_dispatches) OR EXISTS(SELECT 1 FROM execution_browser_resources) THEN
  RAISE EXCEPTION 'refusing to discard durable resource dispatch history';
 END IF;
END $$;
DROP TABLE execution_resource_dispatches;
DROP TABLE execution_browser_resources;
