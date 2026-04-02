-- Extend api_keys table with type, description, expiry, soft-revoke, and rotation tracking.
-- Also purges any legacy bcrypt-hashed keys (created before v2).
-- The new system uses SHA-256 for O(1) validation; legacy keys are incompatible.
DELETE FROM api_keys;

-- Key type: 'public' (browser-safe, read-only) or 'private' (server-to-server, full read)
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS type        TEXT        NOT NULL DEFAULT 'public'
    CHECK (type IN ('public', 'private'));

ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS description TEXT        NOT NULL DEFAULT '';

-- Optional hard expiry date. NULL = never expires.
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS expires_at  TIMESTAMPTZ;

-- Soft-revoke: set on manual revocation or after rotation. Replaced hard DELETE.
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS revoked_at  TIMESTAMPTZ;

-- If this key was created by rotating another key, store the source key id.
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS rotated_from_id BIGINT REFERENCES api_keys (id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS api_keys_prefix_idx    ON api_keys (prefix);
CREATE INDEX IF NOT EXISTS api_keys_revoked_idx   ON api_keys (revoked_at) WHERE revoked_at IS NULL;
CREATE INDEX IF NOT EXISTS api_keys_expires_idx   ON api_keys (expires_at) WHERE expires_at IS NOT NULL;
