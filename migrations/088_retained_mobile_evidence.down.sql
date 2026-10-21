DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM memory_retained_mobile_evidence) OR EXISTS(SELECT 1 FROM memory_deletions WHERE action='archive_expire') THEN
 RAISE EXCEPTION 'cannot discard durable retained evidence or archive expiry tombstones'; END IF;
END $$;
DROP FUNCTION butler_memory_retained_sources_current(TEXT[]);
DROP FUNCTION butler_memory_object_retained(TEXT,BIGINT);
DROP TABLE memory_retained_mobile_evidence;
DROP FUNCTION butler_retained_evidence_immutable();
ALTER TABLE memory_deletions DROP CONSTRAINT memory_deletions_action_check;
ALTER TABLE memory_deletions ADD CONSTRAINT memory_deletions_action_check CHECK(action IN ('delete','revoke'));
ALTER TABLE mobile_devices ALTER COLUMN retention_days SET DEFAULT 30;
