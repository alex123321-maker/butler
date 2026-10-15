ALTER TABLE session_summaries ADD COLUMN result_source_id TEXT REFERENCES memory_sources(source_id);

-- Explicit owner policy anchors one session snapshot. Source deletion expands
-- through these anchors before ordinary derivative traversal and erases all
-- copied revisions, including a job's input that has not yet published.
CREATE TABLE memory_summary_sources (
 source_id TEXT PRIMARY KEY REFERENCES memory_sources(source_id),
 session_key TEXT NOT NULL REFERENCES sessions(session_key) ON DELETE CASCADE
);
CREATE INDEX memory_summary_sources_session ON memory_summary_sources(session_key);

CREATE FUNCTION butler_invalidate_tool_summary() RETURNS TRIGGER LANGUAGE plpgsql AS $$
DECLARE sid TEXT;
BEGIN
 SELECT session_key INTO sid FROM runs WHERE run_id=OLD.run_id;
 UPDATE transcript_heads SET source_revision=source_revision+1 WHERE session_key=sid;
 DELETE FROM session_summaries WHERE session_key=sid;
 RETURN NULL;
END;
$$;
CREATE TRIGGER invalidate_tool_summary AFTER UPDATE OR DELETE ON tool_calls
FOR EACH ROW EXECUTE FUNCTION butler_invalidate_tool_summary();
