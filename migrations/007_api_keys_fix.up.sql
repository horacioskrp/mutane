-- Safety migration: ensures all v2 api_keys columns exist regardless of
-- whether 006 ran fully or partially. Uses IF NOT EXISTS on every statement
-- so re-running is always safe.

ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS type
    TEXT NOT NULL DEFAULT 'public' CHECK (type IN ('public', 'private'));

ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS description
    TEXT NOT NULL DEFAULT '';

ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS expires_at
    TIMESTAMPTZ;

ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS revoked_at
    TIMESTAMPTZ;

ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS rotated_from_id
    BIGINT REFERENCES api_keys (id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS api_keys_prefix_idx
    ON api_keys (prefix);

CREATE INDEX IF NOT EXISTS api_keys_revoked_idx
    ON api_keys (revoked_at) WHERE revoked_at IS NULL;

CREATE INDEX IF NOT EXISTS api_keys_expires_idx
    ON api_keys (expires_at) WHERE expires_at IS NOT NULL;
