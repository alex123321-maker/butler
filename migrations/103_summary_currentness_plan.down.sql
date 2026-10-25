-- Restore the original predicate and keep every source/snapshot row.
CREATE OR REPLACE FUNCTION butler_memory_summary_sources_current(ids TEXT[]) RETURNS BOOLEAN STABLE LANGUAGE SQL AS $$
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
