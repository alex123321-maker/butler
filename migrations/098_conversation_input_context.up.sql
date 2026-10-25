-- Existing work retains its original session/day budget and replay policy.
ALTER TABLE accepted_inputs ADD COLUMN context_epoch smallint NOT NULL DEFAULT 0 CHECK (context_epoch IN (0,1));
ALTER TABLE accepted_inputs ALTER COLUMN context_epoch SET DEFAULT 1;
