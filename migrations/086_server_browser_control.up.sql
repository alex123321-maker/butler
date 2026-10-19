CREATE TABLE server_browser_sessions (
 browser_session_id TEXT PRIMARY KEY CHECK(browser_session_id ~ '^[0-9a-f]{32}$'),
 run_id TEXT NOT NULL REFERENCES runs(run_id), owner_id TEXT NOT NULL,
 task_id TEXT NOT NULL DEFAULT '', expires_at TIMESTAMPTZ NOT NULL,
 mode TEXT NOT NULL DEFAULT 'agent' CHECK(mode IN ('agent','owner','closed')),
 epoch BIGINT NOT NULL DEFAULT 0 CHECK(epoch>=0), controller_hash TEXT NOT NULL DEFAULT '',
	 fresh_needed BOOLEAN NOT NULL DEFAULT false,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX server_browser_sessions_owner ON server_browser_sessions(owner_id,expires_at);
CREATE TABLE server_browser_control_events (
 event_id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 browser_session_id TEXT NOT NULL REFERENCES server_browser_sessions(browser_session_id),
 owner_id TEXT NOT NULL, epoch BIGINT NOT NULL, operation TEXT NOT NULL,
 status TEXT NOT NULL CHECK(status IN ('started','acknowledged','unknown')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(), finished_at TIMESTAMPTZ
);
