ALTER TABLE mobile_devices ALTER COLUMN retention_days SET DEFAULT 360;
ALTER TABLE memory_deletions DROP CONSTRAINT memory_deletions_action_check;
ALTER TABLE memory_deletions ADD CONSTRAINT memory_deletions_action_check CHECK(action IN ('delete','revoke','archive_expire'));

-- An exact bounded quotation is durable memory, separate from the raw archive.
-- This typed privacy edge is traversed by explicit forgetting, not archive expiry.
CREATE TABLE memory_retained_mobile_evidence (
 source_id TEXT PRIMARY KEY REFERENCES memory_sources(source_id),
 original_source_id TEXT NOT NULL REFERENCES memory_sources(source_id),
 original_revision BIGINT NOT NULL CHECK(original_revision>0),
 original_access_revision BIGINT NOT NULL CHECK(original_access_revision>0),
 original_hash TEXT NOT NULL CHECK(length(original_hash)=64),
 message_id TEXT NOT NULL REFERENCES mobile_messages(message_id),
 evidence JSONB NOT NULL CHECK(jsonb_typeof(evidence)='object' AND octet_length(evidence::text)<=16384),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX memory_retained_mobile_privacy_parent ON memory_retained_mobile_evidence(original_source_id);
CREATE FUNCTION butler_retained_evidence_immutable() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' AND EXISTS(SELECT 1 FROM memory_deletions d JOIN memory_sources s ON s.owner_id=d.owner_id WHERE s.source_id=OLD.source_id AND d.deletion_id=current_setting('butler.memory_redaction',TRUE)) THEN RETURN OLD; END IF;
 RAISE EXCEPTION 'retained evidence is immutable except explicit privacy erasure';
END $$;
CREATE TRIGGER retained_evidence_immutable BEFORE UPDATE OR DELETE ON memory_retained_mobile_evidence FOR EACH ROW EXECUTE FUNCTION butler_retained_evidence_immutable();

CREATE FUNCTION butler_memory_retained_sources_current(ids TEXT[]) RETURNS BOOLEAN STABLE LANGUAGE SQL AS $$
 WITH RECURSIVE roots AS(SELECT unnest(ids) AS id UNION SELECT d.parent_source_id FROM memory_source_dependencies d JOIN roots r ON r.id=d.child_source_id)
 SELECT NOT EXISTS(SELECT 1 FROM roots r JOIN memory_retained_mobile_evidence e ON e.source_id=r.id
 LEFT JOIN memory_sources original ON original.source_id=e.original_source_id
 LEFT JOIN memory_source_revisions v ON v.source_id=original.source_id AND v.revision=original.head_revision
 JOIN memory_sources child ON child.source_id=e.source_id
 WHERE original.owner_id IS DISTINCT FROM child.owner_id OR original.head_revision IS DISTINCT FROM e.original_revision OR v.content_hash IS DISTINCT FROM e.original_hash
 OR NOT COALESCE((original.status='active' AND original.access_revision=e.original_access_revision AND v.content_hash=e.original_hash AND v.origin->>'sensitivity'<>'secret')
 OR (original.status='deleted' AND original.access_revision=e.original_access_revision+1
 AND EXISTS(SELECT 1 FROM memory_deletions d WHERE d.source_id=original.source_id AND d.action='archive_expire')
 AND NOT EXISTS(SELECT 1 FROM memory_deletions d WHERE d.source_id=original.source_id AND d.action IN ('delete','revoke'))),FALSE))
$$;

-- Age/count housekeeping cannot discard the searchable form of retained memory.
CREATE FUNCTION butler_memory_object_retained(typ TEXT,oid BIGINT) RETURNS BOOLEAN STABLE LANGUAGE SQL AS $$
 WITH RECURSIVE roots AS(SELECT source_id AS id FROM memory_object_sources WHERE memory_type=typ AND object_id=oid
 UNION SELECT d.parent_source_id FROM memory_source_dependencies d JOIN roots r ON r.id=d.child_source_id)
 SELECT EXISTS(SELECT 1 FROM roots r JOIN memory_retained_mobile_evidence e ON e.source_id=r.id)
$$;
