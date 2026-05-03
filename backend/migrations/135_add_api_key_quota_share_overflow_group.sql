-- Migration: Add key-level quota_share overflow group
-- Allows selected downstream API keys to route to a personal OpenAI standard group
-- only after their primary quota_share allocation is exceeded.

ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS quota_share_overflow_group_id BIGINT NULL;

COMMENT ON COLUMN api_keys.quota_share_overflow_group_id IS
    'Key-level OpenAI standard group used after quota_share limits are exceeded';
