-- One update stream belongs to a concrete bot and independently versioned
-- owner binding. This stores a transport cursor, never orchestration state.
CREATE TABLE communication_telegram_cursors (
 bot_id BIGINT PRIMARY KEY CHECK(bot_id>0),
 binding_id TEXT NOT NULL UNIQUE REFERENCES communication_bindings(binding_id) ON DELETE CASCADE,
 next_update_id BIGINT NOT NULL DEFAULT 0 CHECK(next_update_id>=0),
 last_outcome TEXT NOT NULL DEFAULT '' CHECK(last_outcome IN ('','accepted','ignored','rejected')),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
