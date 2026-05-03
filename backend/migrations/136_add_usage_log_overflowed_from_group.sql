-- Migration: Record quota_share overflow source group on usage logs
-- group_id remains the actual billed/selected group, while this column stores
-- the original quota_share group when overflow routing was used.

ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS overflowed_from_group_id BIGINT NULL;

COMMENT ON COLUMN usage_logs.overflowed_from_group_id IS
    'Original quota_share group id when this request overflowed to another group';
