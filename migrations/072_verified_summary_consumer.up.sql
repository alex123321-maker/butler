ALTER TABLE session_summaries ADD COLUMN result_source_id TEXT REFERENCES memory_sources(source_id);

-- Explicit owner policy anchors one session snapshot. Source deletion expands
-- through these anchors before ordinary derivative traversal and erases all
-- copied revisions, including a job's input that has not yet published.
CREATE TABLE memory_summary_sources (
 source_id TEXT PRIMARY KEY REFERENCES memory_sources(source_id),
 session_key TEXT NOT NULL REFERENCES sessions(session_key) ON DELETE CASCADE,
 source_revision BIGINT NOT NULL CHECK(source_revision>0),
 through_sequence BIGINT NOT NULL CHECK(through_sequence>0)
);
CREATE INDEX memory_summary_sources_session ON memory_summary_sources(session_key);
CREATE TABLE memory_summary_dependencies (
 snapshot_source_id TEXT NOT NULL REFERENCES memory_summary_sources(source_id),
 original_source_id TEXT NOT NULL REFERENCES memory_sources(source_id),
 source_revision BIGINT NOT NULL,
 access_revision BIGINT NOT NULL,
 content_hash TEXT NOT NULL,
 PRIMARY KEY(snapshot_source_id,original_source_id)
);

CREATE FUNCTION butler_memory_summary_sources_current(ids TEXT[]) RETURNS BOOLEAN STABLE LANGUAGE SQL AS $$
WITH RECURSIVE roots AS(
 SELECT unnest(ids) AS id UNION SELECT d.parent_source_id FROM memory_source_dependencies d JOIN roots r ON r.id=d.child_source_id
),snapshots AS(SELECT s.* FROM memory_summary_sources s JOIN roots r ON r.id=s.source_id),
originals(snapshot_source_id,id) AS(
 SELECT s.source_id,'message:'||m.message_id FROM snapshots s JOIN messages m ON m.session_key=s.session_key AND m.message_sequence<=s.through_sequence
 UNION SELECT o.snapshot_source_id,d.parent_source_id FROM originals o JOIN memory_source_dependencies d ON d.child_source_id=o.id
)
SELECT NOT EXISTS(SELECT 1 FROM snapshots s LEFT JOIN transcript_heads h USING(session_key) WHERE h.source_revision IS DISTINCT FROM s.source_revision)
 AND NOT EXISTS(SELECT 1 FROM originals o WHERE NOT EXISTS(SELECT 1 FROM memory_summary_dependencies d WHERE d.snapshot_source_id=o.snapshot_source_id AND d.original_source_id=o.id))
 AND NOT EXISTS(SELECT 1 FROM memory_summary_dependencies d JOIN snapshots s ON s.source_id=d.snapshot_source_id WHERE NOT EXISTS(SELECT 1 FROM originals o WHERE o.snapshot_source_id=d.snapshot_source_id AND o.id=d.original_source_id))
 AND NOT EXISTS(SELECT 1 FROM snapshots s JOIN memory_summary_dependencies d ON d.snapshot_source_id=s.source_id
 LEFT JOIN memory_sources o ON o.source_id=d.original_source_id
 LEFT JOIN memory_source_revisions v ON v.source_id=o.source_id AND v.revision=o.head_revision
 WHERE o.status IS DISTINCT FROM 'active' OR o.head_revision IS DISTINCT FROM d.source_revision OR o.access_revision IS DISTINCT FROM d.access_revision OR v.content_hash IS DISTINCT FROM d.content_hash OR v.origin->>'sensitivity'='secret')
$$;

CREATE FUNCTION butler_memory_summary_publication_fence() RETURNS TRIGGER LANGUAGE plpgsql AS $$
DECLARE rid TEXT;
BEGIN
 IF TG_TABLE_NAME='messages' THEN IF NEW.role='user' THEN RETURN NEW; END IF; END IF;
 IF TG_TABLE_NAME='tasks' THEN rid:=NEW.last_run_id; ELSE rid:=NEW.run_id; END IF;
 IF NOT butler_memory_summary_sources_current(ARRAY(SELECT source_id FROM memory_run_sources WHERE run_id=rid)) THEN
  RAISE EXCEPTION 'run summary original sources changed' USING ERRCODE='40001';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER summary_final_publication_fence BEFORE INSERT ON run_final_results FOR EACH ROW EXECUTE FUNCTION butler_memory_summary_publication_fence();
CREATE TRIGGER summary_artifact_publication_fence BEFORE INSERT ON artifacts FOR EACH ROW EXECUTE FUNCTION butler_memory_summary_publication_fence();
CREATE TRIGGER summary_message_publication_fence BEFORE INSERT ON messages FOR EACH ROW EXECUTE FUNCTION butler_memory_summary_publication_fence();
CREATE TRIGGER summary_working_publication_fence BEFORE INSERT OR UPDATE ON memory_working FOR EACH ROW EXECUTE FUNCTION butler_memory_summary_publication_fence();
CREATE TRIGGER summary_task_working_publication_fence BEFORE UPDATE OF working_json ON tasks FOR EACH ROW EXECUTE FUNCTION butler_memory_summary_publication_fence();

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
