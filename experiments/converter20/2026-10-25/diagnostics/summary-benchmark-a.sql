\set ON_ERROR_STOP on
CREATE SCHEMA converter20_summary_benchmark_a;
SET search_path=converter20_summary_benchmark_a,public;
CREATE TABLE memory_summary_sources(source_id text PRIMARY KEY,session_key text,source_revision bigint,through_sequence bigint);
CREATE TABLE memory_source_dependencies(child_source_id text,parent_source_id text,parent_revision bigint);
CREATE TABLE messages(message_id text PRIMARY KEY,session_key text,message_sequence bigint);
CREATE TABLE transcript_heads(session_key text PRIMARY KEY,source_revision bigint);
CREATE TABLE memory_summary_dependencies(snapshot_source_id text,original_source_id text,source_revision bigint,access_revision bigint,content_hash text);
CREATE TABLE memory_sources(source_id text PRIMARY KEY,status text,head_revision bigint,access_revision bigint);
CREATE TABLE memory_source_revisions(source_id text,revision bigint,content_hash text,origin jsonb,PRIMARY KEY(source_id,revision));
INSERT INTO memory_summary_sources SELECT source_id,session_key,source_revision,through_sequence FROM memory_6a899ddb967e9aec1393609c48d0071c.memory_summary_sources;
INSERT INTO memory_source_dependencies SELECT child_source_id,parent_source_id,parent_revision FROM memory_6a899ddb967e9aec1393609c48d0071c.memory_source_dependencies;
INSERT INTO messages SELECT message_id,session_key,message_sequence FROM memory_6a899ddb967e9aec1393609c48d0071c.messages;
INSERT INTO transcript_heads SELECT session_key,source_revision FROM memory_6a899ddb967e9aec1393609c48d0071c.transcript_heads;
INSERT INTO memory_summary_dependencies SELECT snapshot_source_id,original_source_id,source_revision,access_revision,content_hash FROM memory_6a899ddb967e9aec1393609c48d0071c.memory_summary_dependencies;
INSERT INTO memory_sources SELECT source_id,status,head_revision,access_revision FROM memory_6a899ddb967e9aec1393609c48d0071c.memory_sources;
INSERT INTO memory_source_revisions SELECT source_id,revision,content_hash,origin FROM memory_6a899ddb967e9aec1393609c48d0071c.memory_source_revisions;
CREATE FUNCTION original_current(ids text[]) RETURNS boolean STABLE LANGUAGE SQL AS $original$
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
$original$;
CREATE FUNCTION candidate_current(ids text[]) RETURNS boolean STABLE LANGUAGE plpgsql AS $candidate$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM memory_summary_sources) THEN RETURN TRUE; END IF;
 RETURN (
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
 );
END
$candidate$;
SELECT count(*) AS summary_rows FROM memory_summary_sources;
EXPLAIN (ANALYZE,TIMING OFF) SELECT count(*) FILTER(WHERE original_current(ARRAY(SELECT source_id FROM memory_sources)||ARRAY['probe-'||i])) FROM generate_series(1,100) i;
EXPLAIN (ANALYZE,TIMING OFF) SELECT count(*) FILTER(WHERE candidate_current(ARRAY(SELECT source_id FROM memory_sources)||ARRAY['probe-'||i])) FROM generate_series(1,100) i;
TRUNCATE memory_summary_sources,memory_source_dependencies,messages,transcript_heads,memory_summary_dependencies,memory_sources,memory_source_revisions;
INSERT INTO memory_summary_sources VALUES('sum','session',1,1);
INSERT INTO messages VALUES('m','session',1);
INSERT INTO transcript_heads VALUES('session',1);
INSERT INTO memory_sources VALUES('message:m','active',1,1);
INSERT INTO memory_source_revisions VALUES('message:m',1,'hash','{"sensitivity":"public"}');
INSERT INTO memory_summary_dependencies VALUES('sum','message:m',1,1,'hash');
INSERT INTO memory_source_dependencies VALUES('derived','sum',1);
CREATE TEMP TABLE results(label text,original boolean,candidate boolean,expected boolean);
INSERT INTO results SELECT 'current',original_current(ARRAY['sum']),candidate_current(ARRAY['sum']),true;
INSERT INTO results SELECT 'derived',original_current(ARRAY['derived']),candidate_current(ARRAY['derived']),true;
INSERT INTO results SELECT 'unrelated',original_current(ARRAY['elsewhere']),candidate_current(ARRAY['elsewhere']),true;
INSERT INTO results SELECT 'empty',original_current(ARRAY[]::text[]),candidate_current(ARRAY[]::text[]),true;
INSERT INTO results SELECT 'null',original_current(NULL),candidate_current(NULL),true;
UPDATE transcript_heads SET source_revision=2;
INSERT INTO results SELECT 'changed transcript',original_current(ARRAY['sum']),candidate_current(ARRAY['sum']),false;
UPDATE transcript_heads SET source_revision=1;
UPDATE memory_source_revisions SET content_hash='changed';
INSERT INTO results SELECT 'changed hash',original_current(ARRAY['sum']),candidate_current(ARRAY['sum']),false;
UPDATE memory_source_revisions SET content_hash='hash',origin='{"sensitivity":"secret"}';
INSERT INTO results SELECT 'secret',original_current(ARRAY['sum']),candidate_current(ARRAY['sum']),false;
UPDATE memory_source_revisions SET origin='{"sensitivity":"public"}';
UPDATE memory_sources SET access_revision=2;
INSERT INTO results SELECT 'changed access',original_current(ARRAY['sum']),candidate_current(ARRAY['sum']),false;
UPDATE memory_sources SET access_revision=1,status='deleted';
INSERT INTO results SELECT 'deleted source',original_current(ARRAY['sum']),candidate_current(ARRAY['sum']),false;
UPDATE memory_sources SET status='active';
DELETE FROM memory_summary_dependencies;
INSERT INTO results SELECT 'missing dependency',original_current(ARRAY['sum']),candidate_current(ARRAY['sum']),false;
INSERT INTO memory_summary_dependencies VALUES('sum','message:m',1,1,'hash'),('sum','extra',1,1,'hash');
INSERT INTO results SELECT 'extra dependency',original_current(ARRAY['sum']),candidate_current(ARRAY['sum']),false;
TABLE results;
DO $check$ BEGIN IF EXISTS(SELECT 1 FROM results WHERE original IS DISTINCT FROM expected OR candidate IS DISTINCT FROM expected) THEN RAISE EXCEPTION 'summary predicate mismatch'; END IF; END $check$;