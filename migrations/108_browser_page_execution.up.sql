ALTER TABLE server_browser_sessions ADD COLUMN page_mode BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE server_browser_sessions ADD COLUMN page_origin TEXT NOT NULL DEFAULT '';
ALTER TABLE server_browser_pages ADD COLUMN runtime_version BIGINT NOT NULL DEFAULT 0 CHECK(runtime_version>=0);
