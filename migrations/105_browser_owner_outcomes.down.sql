DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM browser_owner_outcome_reviews) THEN RAISE EXCEPTION 'retained owner outcomes prevent downgrade'; END IF;
END $$;
DROP TRIGGER retain_browser_outcome ON browser_owner_outcome_reviews;
DROP FUNCTION retain_browser_owner_outcome();
DROP TABLE browser_owner_outcome_reviews;
