DROP INDEX IF EXISTS idx_approvals_approval_type_requested_at;

ALTER TABLE approvals
    DROP CONSTRAINT IF EXISTS approvals_approval_type_check;

ALTER TABLE approvals
    DROP COLUMN IF EXISTS payload_json,
    DROP COLUMN IF EXISTS approval_type;
