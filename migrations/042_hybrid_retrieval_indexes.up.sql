-- Derived lexical indexes only; current chunks and catalog authority stay truth.
CREATE INDEX memory_chunks_hybrid_text ON memory_chunks USING GIN (
    (to_tsvector('simple'::regconfig,coalesce(title,'')||' '||coalesce(content,'')||' '||coalesce(source_id,'')) ||
     to_tsvector('russian'::regconfig,coalesce(title,'')||' '||coalesce(content,'')||' '||coalesce(source_id,'')))
);
