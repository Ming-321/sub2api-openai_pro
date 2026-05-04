-- Migration: Add quota_share fields to groups and api_keys tables
-- This migration supports the quota_share subscription type for fair
-- distribution of a single upstream subscription across multiple API keys.

-- Group: estimated limits and calibration state for quota_share
ALTER TABLE groups ADD COLUMN IF NOT EXISTS estimated_5h_limit_usd decimal(20,8) NOT NULL DEFAULT 0;
ALTER TABLE groups ADD COLUMN IF NOT EXISTS estimated_7d_limit_usd decimal(20,8) NOT NULL DEFAULT 0;
ALTER TABLE groups ADD COLUMN IF NOT EXISTS calibration_state jsonb;

-- API Key: weight for quota distribution in quota_share groups
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS quota_weight integer NOT NULL DEFAULT 1;
