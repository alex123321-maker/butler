DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM communication_telegram_notices) THEN RAISE EXCEPTION 'refusing to discard Telegram notice outcome history'; END IF;
END $$;
DROP TABLE communication_telegram_notices;
