-- Restore the original temporal predicates without changing any history.
CREATE OR REPLACE FUNCTION butler_memory_temporal_pins_current(pins JSONB) RETURNS BOOLEAN STABLE LANGUAGE SQL AS $$
SELECT NOT EXISTS(
 SELECT 1 FROM jsonb_to_recordset(pins) AS p(derived_source_id TEXT,run_id TEXT,claim_id TEXT,head_revision BIGINT,active_id TEXT,temporal_revision BIGINT,original_source_id TEXT,source_revision BIGINT,access_revision BIGINT,content_hash TEXT,metadata_hash TEXT,freshness_status TEXT,current_usable BOOLEAN,valid_at TIMESTAMPTZ,known_at TIMESTAMPTZ)
 LEFT JOIN memory_claims c ON c.claim_id=p.claim_id
 LEFT JOIN memory_fact_heads h ON h.owner_id=c.owner_id AND h.scope_type=c.scope_type AND h.scope_id=c.scope_id AND h.fact_key=c.fact_key
 LEFT JOIN memory_claim_temporal_heads t ON t.claim_id=c.claim_id
 LEFT JOIN memory_sources s ON s.source_id=p.original_source_id
 LEFT JOIN memory_source_revisions v ON v.source_id=s.source_id AND v.revision=s.head_revision
 WHERE c.status NOT IN ('confirmed','superseded') OR c.status IS NULL OR c.accepted_at IS NULL
 OR h.revision IS DISTINCT FROM p.head_revision OR h.active_claim_id IS DISTINCT FROM p.active_id
 OR COALESCE(t.revision,0)<>p.temporal_revision
 OR COALESCE(t.revision,0)<>(SELECT COALESCE(MAX(revision),0) FROM memory_claim_temporal_revisions WHERE claim_id=c.claim_id)
 OR c.source_id IS DISTINCT FROM p.original_source_id OR c.source_revision IS DISTINCT FROM p.source_revision
 OR s.status IS DISTINCT FROM 'active' OR s.head_revision IS DISTINCT FROM p.source_revision OR s.access_revision IS DISTINCT FROM p.access_revision OR v.content_hash IS DISTINCT FROM p.content_hash OR v.origin->>'sensitivity'='secret'
 OR p.metadata_hash IS DISTINCT FROM butler_memory_temporal_metadata_hash(p.claim_id,p.known_at)
 OR p.freshness_status IS DISTINCT FROM butler_memory_temporal_freshness(p.claim_id,p.known_at)->>'status'
 OR p.current_usable IS DISTINCT FROM (butler_memory_temporal_freshness(p.claim_id,p.known_at)->>'current_usable')::boolean
 OR (p.valid_at IS NULL AND p.known_at IS NULL AND (c.claim_id<>h.active_claim_id OR c.status<>'confirmed'))
 OR (p.known_at IS NOT NULL AND (c.created_at>p.known_at OR c.accepted_at>p.known_at OR v.extracted_at>p.known_at))
 OR (p.valid_at IS NULL AND p.known_at IS NOT NULL AND c.claim_id IS DISTINCT FROM (SELECT claim_id FROM memory_fact_decision_history d WHERE d.owner_id=c.owner_id AND d.scope_type=c.scope_type AND d.scope_id=c.scope_id AND d.fact_key=c.fact_key AND recorded_at<=p.known_at ORDER BY head_revision DESC LIMIT 1))
 OR (p.valid_at IS NOT NULL AND NOT EXISTS(SELECT 1 FROM memory_claim_temporal_revisions x WHERE x.claim_id=p.claim_id AND x.validity='interval' AND (p.known_at IS NULL OR x.recorded_at<=p.known_at) AND x.revision=(SELECT MAX(y.revision) FROM memory_claim_temporal_revisions y WHERE y.claim_id=x.claim_id AND (p.known_at IS NULL OR y.recorded_at<=p.known_at)) AND (x.valid_from IS NULL OR x.valid_from<=p.valid_at) AND (x.valid_to IS NULL OR p.valid_at<x.valid_to)))
 OR (p.derived_source_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM memory_source_dependencies d WHERE d.child_source_id=p.derived_source_id AND d.parent_source_id=p.original_source_id AND d.parent_revision=p.source_revision))
 OR (p.run_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM memory_run_sources r WHERE r.run_id=p.run_id AND r.source_id=p.original_source_id AND r.source_revision=p.source_revision AND r.access_revision=p.access_revision AND r.content_hash=p.content_hash))
)
$$;
CREATE OR REPLACE FUNCTION butler_memory_temporal_sources_current(ids TEXT[]) RETURNS BOOLEAN STABLE LANGUAGE SQL AS $$
WITH RECURSIVE roots AS(SELECT unnest(ids) AS id UNION SELECT d.parent_source_id FROM memory_source_dependencies d JOIN roots r ON r.id=d.child_source_id)
SELECT butler_memory_temporal_pins_current(COALESCE((SELECT jsonb_agg(to_jsonb(p)) FROM memory_temporal_source_pins p JOIN roots r ON r.id=p.derived_source_id),'[]'::jsonb))
$$;
CREATE OR REPLACE FUNCTION butler_memory_temporal_run_current(rid TEXT) RETURNS BOOLEAN STABLE LANGUAGE SQL AS $$
SELECT butler_memory_temporal_pins_current(COALESCE((SELECT jsonb_agg(to_jsonb(p)) FROM memory_temporal_run_pins p WHERE p.run_id=rid),'[]'::jsonb))
 AND butler_memory_temporal_sources_current(ARRAY(SELECT source_id FROM memory_run_sources WHERE run_id=rid))
$$;
