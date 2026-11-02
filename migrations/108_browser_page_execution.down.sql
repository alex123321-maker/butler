DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM server_browser_sessions WHERE page_mode)
 OR EXISTS(SELECT 1 FROM server_browser_pages WHERE runtime_version>0) THEN
  RAISE EXCEPTION 'retained page execution state prevents downgrade';
 END IF;
END $$;
ALTER TABLE server_browser_pages DROP COLUMN runtime_version;
ALTER TABLE server_browser_sessions DROP COLUMN page_origin;
ALTER TABLE server_browser_sessions DROP COLUMN page_mode;
