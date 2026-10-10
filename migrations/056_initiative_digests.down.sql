DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM initiative_digest_outbox) OR EXISTS(SELECT 1 FROM initiative_digest_events) THEN
  RAISE EXCEPTION 'refuse rollback of retained initiative delivery history';
 END IF;
END $$;
DROP TABLE initiative_digest_events;
DROP TABLE initiative_digest_outbox;
DROP FUNCTION butler_immutable_initiative_digest();
