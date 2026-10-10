ALTER TABLE messages ADD COLUMN message_sequence BIGINT;
WITH ranked AS (
    SELECT message_id, ROW_NUMBER() OVER (PARTITION BY session_key ORDER BY created_at, message_id) AS position
    FROM messages
)
UPDATE messages m SET message_sequence = ranked.position FROM ranked WHERE m.message_id = ranked.message_id;
ALTER TABLE messages ALTER COLUMN message_sequence SET NOT NULL;
CREATE UNIQUE INDEX idx_messages_session_sequence ON messages (session_key, message_sequence);

CREATE TABLE transcript_heads (
    session_key TEXT PRIMARY KEY REFERENCES sessions (session_key) ON DELETE CASCADE,
    last_sequence BIGINT NOT NULL DEFAULT 0,
    source_revision BIGINT NOT NULL DEFAULT 1
);
INSERT INTO transcript_heads (session_key, last_sequence)
SELECT session_key, MAX(message_sequence) FROM messages GROUP BY session_key;

CREATE TABLE session_summaries (
    session_key TEXT PRIMARY KEY REFERENCES sessions (session_key) ON DELETE CASCADE,
    summary TEXT NOT NULL,
    through_message_id TEXT NOT NULL REFERENCES messages (message_id) ON DELETE CASCADE,
    through_sequence BIGINT NOT NULL CHECK (through_sequence > 0),
    source_revision BIGINT NOT NULL CHECK (source_revision > 0),
    format_version INTEGER NOT NULL DEFAULT 1 CHECK (format_version = 1),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE FUNCTION butler_assign_message_sequence() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    -- Session -> boundary/message -> head is the common deletion/publication
    -- lock order, preventing a cascade from racing a source append.
    PERFORM 1 FROM sessions WHERE session_key = NEW.session_key FOR KEY SHARE;
    INSERT INTO transcript_heads (session_key, last_sequence)
    VALUES (NEW.session_key, 1)
    ON CONFLICT (session_key) DO UPDATE SET last_sequence = transcript_heads.last_sequence + 1
    RETURNING last_sequence INTO NEW.message_sequence;
    RETURN NEW;
END;
$$;
CREATE TRIGGER assign_message_sequence BEFORE INSERT ON messages
FOR EACH ROW EXECUTE FUNCTION butler_assign_message_sequence();

CREATE FUNCTION butler_preserve_message_identity() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.session_key IS DISTINCT FROM OLD.session_key OR NEW.message_id IS DISTINCT FROM OLD.message_id
       OR NEW.message_sequence IS DISTINCT FROM OLD.message_sequence THEN
        RAISE EXCEPTION 'transcript message identity and sequence are immutable';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER preserve_message_identity BEFORE UPDATE ON messages
FOR EACH ROW EXECUTE FUNCTION butler_preserve_message_identity();

CREATE FUNCTION butler_invalidate_summary_source() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    -- Lock the same head used by append and summary publication. A summarizer
    -- reading an older source revision cannot publish deleted/edited content.
    UPDATE transcript_heads SET source_revision = source_revision + 1 WHERE session_key = OLD.session_key;
    DELETE FROM session_summaries WHERE session_key = OLD.session_key;
    RETURN NULL;
END;
$$;
CREATE TRIGGER invalidate_summary_source AFTER DELETE OR UPDATE ON messages
FOR EACH ROW EXECUTE FUNCTION butler_invalidate_summary_source();
