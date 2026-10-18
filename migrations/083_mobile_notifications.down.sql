DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM mobile_devices) OR EXISTS(SELECT 1 FROM mobile_events) THEN
  RAISE EXCEPTION 'mobile history exists; preserve data and roll forward';
 END IF;
END $$;
DROP TABLE mobile_request_limits,mobile_batches,mobile_events,mobile_pairings,mobile_devices;
