DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM server_browser_sessions WHERE mode<>'closed' AND expires_at>clock_timestamp()) THEN
  RAISE EXCEPTION 'live server browser control sessions must be closed first';
 END IF;
END $$;
DROP TABLE server_browser_control_events;
DROP TABLE server_browser_sessions;
