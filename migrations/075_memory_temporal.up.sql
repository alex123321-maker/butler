-- Temporal interpretation belongs to existing claims; owner heads remain the
-- only current acceptance authority. No validity timestamp is inferred.
CREATE TABLE memory_claim_temporal_heads (
 claim_id TEXT PRIMARY KEY REFERENCES memory_claims(claim_id),
 revision BIGINT NOT NULL DEFAULT 0 CHECK(revision>=0)
);
CREATE TABLE memory_claim_temporal_revisions (
 claim_id TEXT NOT NULL REFERENCES memory_claims(claim_id),
 revision BIGINT NOT NULL CHECK(revision>0),
 validity TEXT NOT NULL CHECK(validity IN ('unknown','interval')),
 valid_from TIMESTAMPTZ, valid_to TIMESTAMPTZ,
 freshness_class TEXT NOT NULL CHECK(freshness_class IN ('unknown','stable_preference','work_status','technical_fact','completed_episode')),
 verified_at TIMESTAMPTZ, verification_basis TEXT NOT NULL CHECK(verification_basis IN ('unknown','owner_statement','broker_read')),
 verification_action_id TEXT NOT NULL DEFAULT '',
 recorded_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(), recorded_by TEXT NOT NULL,
 PRIMARY KEY(claim_id,revision),
 CHECK(validity='interval' OR valid_from IS NULL AND valid_to IS NULL),
 CHECK(valid_from IS NULL OR valid_to IS NULL OR valid_from<valid_to),
 CHECK((verified_at IS NULL)=(verification_basis='unknown')),
 CHECK(verification_basis<>'broker_read' OR verification_action_id<>'')
);
CREATE INDEX memory_claim_temporal_known ON memory_claim_temporal_revisions(claim_id,recorded_at,revision);

-- Decision history is an audit of existing head CAS, not a second acceptance
-- store. Supersession chains recover prior decision order without trusting upload
-- order or arbitrarily sorting equal recording timestamps.
CREATE TABLE memory_fact_decision_history (
 owner_id TEXT NOT NULL, scope_type TEXT NOT NULL, scope_id TEXT NOT NULL, fact_key TEXT NOT NULL,
 head_revision BIGINT NOT NULL CHECK(head_revision>0), claim_id TEXT NOT NULL REFERENCES memory_claims(claim_id),
 recorded_at TIMESTAMPTZ NOT NULL,
 PRIMARY KEY(owner_id,scope_type,scope_id,fact_key,head_revision)
);
WITH RECURSIVE chain AS (
 SELECT h.owner_id,h.scope_type,h.scope_id,h.fact_key,h.revision AS head_revision,c.claim_id,c.supersedes_id,c.accepted_at
 FROM memory_fact_heads h JOIN memory_claims c ON c.claim_id=h.active_claim_id WHERE h.revision>0
 UNION ALL
 SELECT p.owner_id,p.scope_type,p.scope_id,p.fact_key,p.head_revision-1,c.claim_id,c.supersedes_id,c.accepted_at
 FROM chain p JOIN memory_claims c ON c.claim_id=p.supersedes_id WHERE p.head_revision>1
)
INSERT INTO memory_fact_decision_history
SELECT owner_id,scope_type,scope_id,fact_key,head_revision,claim_id,accepted_at FROM chain WHERE accepted_at IS NOT NULL;

CREATE FUNCTION butler_record_memory_fact_decision() RETURNS TRIGGER LANGUAGE plpgsql AS $$
DECLARE accepted TIMESTAMPTZ;
BEGIN
 IF NEW.active_claim_id='' OR NEW.active_claim_id=OLD.active_claim_id THEN RETURN NEW; END IF;
 SELECT accepted_at INTO accepted FROM memory_claims WHERE claim_id=NEW.active_claim_id AND owner_id=NEW.owner_id AND status='confirmed';
 IF accepted IS NULL OR NEW.revision<>OLD.revision+1 THEN RAISE EXCEPTION 'invalid memory owner head decision'; END IF;
 INSERT INTO memory_fact_decision_history VALUES(NEW.owner_id,NEW.scope_type,NEW.scope_id,NEW.fact_key,NEW.revision,NEW.active_claim_id,accepted);
 RETURN NEW;
END $$;
CREATE TRIGGER memory_fact_decision_record AFTER UPDATE ON memory_fact_heads FOR EACH ROW EXECUTE FUNCTION butler_record_memory_fact_decision();

CREATE FUNCTION butler_memory_temporal_metadata_hash(cid TEXT, known_at TIMESTAMPTZ) RETURNS TEXT STABLE LANGUAGE SQL SET TimeZone='UTC' AS $$
 SELECT encode(sha256(convert_to(jsonb_build_array(to_jsonb(t),c.accepted_at,c.created_at,v.extracted_at)::text,'UTF8')),'hex')
 FROM memory_claims c JOIN memory_source_revisions v ON v.source_id=c.source_id AND v.revision=c.source_revision
 LEFT JOIN LATERAL(SELECT * FROM memory_claim_temporal_revisions x WHERE x.claim_id=c.claim_id AND (known_at IS NULL OR x.recorded_at<=known_at) ORDER BY x.revision DESC LIMIT 1)t ON TRUE
 WHERE c.claim_id=cid
$$;

-- The finite freshness policy is code-owned and versioned with this migration.
-- Unknown verification is not refreshed by ingestion. Validity and freshness
-- expiry never alter accepted status, erase history or create a privacy tombstone.
CREATE FUNCTION butler_memory_temporal_freshness(cid TEXT, known_at TIMESTAMPTZ) RETURNS JSONB STABLE LANGUAGE SQL SET TimeZone='UTC' AS $$
 WITH meta AS(SELECT * FROM memory_claim_temporal_revisions WHERE claim_id=cid AND (known_at IS NULL OR recorded_at<=known_at) ORDER BY revision DESC LIMIT 1),
 state AS(SELECT
 CASE
 WHEN validity='interval' AND valid_from>statement_timestamp() THEN 'not_yet_valid'
 WHEN validity='interval' AND valid_to<=statement_timestamp() THEN 'expired'
 WHEN freshness_class='stable_preference' THEN 'stable'
 WHEN freshness_class='completed_episode' THEN 'historical'
 WHEN freshness_class='unknown' THEN 'unknown'
 WHEN verified_at IS NULL THEN 'unverified'
 WHEN verified_at+CASE freshness_class WHEN 'work_status' THEN interval '15 minutes' ELSE interval '24 hours' END<=statement_timestamp() THEN 'needs_revalidation'
 ELSE 'fresh' END AS status,
 CASE freshness_class WHEN 'work_status' THEN verified_at+interval '15 minutes' WHEN 'technical_fact' THEN verified_at+interval '24 hours' END AS revalidate_after
 FROM meta),fallback AS(SELECT 'unknown'::text AS status,NULL::timestamptz AS revalidate_after WHERE NOT EXISTS(SELECT 1 FROM meta))
 SELECT jsonb_build_object('status',status,'revalidate_after',revalidate_after,'requires_revalidation',status IN ('unknown','unverified','needs_revalidation'),'current_usable',status IN ('stable','historical','fresh')) FROM(SELECT * FROM state UNION ALL SELECT * FROM fallback)s
$$;

CREATE TABLE memory_temporal_source_pins (
 derived_source_id TEXT NOT NULL REFERENCES memory_sources(source_id),
 claim_id TEXT NOT NULL REFERENCES memory_claims(claim_id),
 head_revision BIGINT NOT NULL CHECK(head_revision>0), active_id TEXT NOT NULL,
 temporal_revision BIGINT NOT NULL CHECK(temporal_revision>=0),
 original_source_id TEXT NOT NULL REFERENCES memory_sources(source_id), source_revision BIGINT NOT NULL,
 access_revision BIGINT NOT NULL, content_hash TEXT NOT NULL CHECK(length(content_hash)=64),
 metadata_hash TEXT NOT NULL CHECK(length(metadata_hash)=64), freshness_status TEXT NOT NULL, current_usable BOOLEAN NOT NULL,
 valid_at TIMESTAMPTZ, known_at TIMESTAMPTZ,
 PRIMARY KEY(derived_source_id,claim_id)
);
CREATE INDEX memory_temporal_source_original ON memory_temporal_source_pins(original_source_id);

CREATE FUNCTION butler_memory_temporal_sources_current(ids TEXT[]) RETURNS BOOLEAN STABLE LANGUAGE SQL AS $$
WITH RECURSIVE roots AS(SELECT unnest(ids) AS id UNION SELECT d.parent_source_id FROM memory_source_dependencies d JOIN roots r ON r.id=d.child_source_id)
SELECT NOT EXISTS(
 SELECT 1 FROM memory_temporal_source_pins p JOIN roots root ON root.id=p.derived_source_id
 LEFT JOIN memory_claims c ON c.claim_id=p.claim_id
 LEFT JOIN memory_fact_heads h ON h.owner_id=c.owner_id AND h.scope_type=c.scope_type AND h.scope_id=c.scope_id AND h.fact_key=c.fact_key
 LEFT JOIN memory_claim_temporal_heads t ON t.claim_id=c.claim_id
 LEFT JOIN memory_sources s ON s.source_id=p.original_source_id
 LEFT JOIN memory_source_revisions v ON v.source_id=s.source_id AND v.revision=s.head_revision
 WHERE c.status NOT IN ('confirmed','superseded') OR c.status IS NULL OR c.accepted_at IS NULL
 OR h.revision IS DISTINCT FROM p.head_revision OR h.active_claim_id IS DISTINCT FROM p.active_id
 OR COALESCE(t.revision,0)<>p.temporal_revision
 OR c.source_id IS DISTINCT FROM p.original_source_id OR c.source_revision IS DISTINCT FROM p.source_revision
 OR s.status IS DISTINCT FROM 'active' OR s.head_revision IS DISTINCT FROM p.source_revision OR s.access_revision IS DISTINCT FROM p.access_revision OR v.content_hash IS DISTINCT FROM p.content_hash OR v.origin->>'sensitivity'='secret'
 OR p.metadata_hash IS DISTINCT FROM butler_memory_temporal_metadata_hash(p.claim_id,p.known_at)
 OR p.freshness_status IS DISTINCT FROM butler_memory_temporal_freshness(p.claim_id,p.known_at)->>'status'
 OR p.current_usable IS DISTINCT FROM (butler_memory_temporal_freshness(p.claim_id,p.known_at)->>'current_usable')::boolean
 OR (p.valid_at IS NULL AND p.known_at IS NULL AND (c.claim_id<>h.active_claim_id OR c.status<>'confirmed'))
 OR (p.known_at IS NOT NULL AND (c.created_at>p.known_at OR c.accepted_at>p.known_at OR v.extracted_at>p.known_at))
 OR (p.valid_at IS NOT NULL AND NOT EXISTS(SELECT 1 FROM memory_claim_temporal_revisions x WHERE x.claim_id=p.claim_id AND x.validity='interval' AND (p.known_at IS NULL OR x.recorded_at<=p.known_at) AND x.revision=(SELECT MAX(y.revision) FROM memory_claim_temporal_revisions y WHERE y.claim_id=x.claim_id AND (p.known_at IS NULL OR y.recorded_at<=p.known_at)) AND (x.valid_from IS NULL OR x.valid_from<=p.valid_at) AND (x.valid_to IS NULL OR p.valid_at<x.valid_to)))
 OR NOT EXISTS(SELECT 1 FROM memory_source_dependencies d WHERE d.child_source_id=p.derived_source_id AND d.parent_source_id=p.original_source_id AND d.parent_revision=p.source_revision)
)
$$;

CREATE FUNCTION butler_memory_temporal_immutable() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' AND COALESCE(current_setting('butler.memory_redaction',TRUE),'')<>'' THEN RETURN OLD; END IF;
 RAISE EXCEPTION 'memory temporal history is immutable';
END $$;
CREATE TRIGGER memory_temporal_revision_immutable BEFORE UPDATE OR DELETE ON memory_claim_temporal_revisions FOR EACH ROW EXECUTE FUNCTION butler_memory_temporal_immutable();
CREATE TRIGGER memory_temporal_pin_immutable BEFORE UPDATE OR DELETE ON memory_temporal_source_pins FOR EACH ROW EXECUTE FUNCTION butler_memory_temporal_immutable();
CREATE TRIGGER memory_temporal_decision_immutable BEFORE UPDATE OR DELETE ON memory_fact_decision_history FOR EACH ROW EXECUTE FUNCTION butler_memory_temporal_immutable();

CREATE FUNCTION butler_redact_memory_temporal() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.status='deleted' AND COALESCE(current_setting('butler.memory_redaction',TRUE),'')<>'' THEN
  DELETE FROM memory_temporal_source_pins WHERE claim_id=NEW.claim_id;
  DELETE FROM memory_claim_temporal_revisions WHERE claim_id=NEW.claim_id;
  DELETE FROM memory_claim_temporal_heads WHERE claim_id=NEW.claim_id;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER memory_temporal_redaction AFTER UPDATE OF status ON memory_claims FOR EACH ROW EXECUTE FUNCTION butler_redact_memory_temporal();
