DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM server_browser_pages) THEN
  RAISE EXCEPTION 'retained browser page bindings prevent downgrade';
 END IF;
END $$;
DROP TABLE server_browser_pages;
DROP FUNCTION retain_server_browser_page();
