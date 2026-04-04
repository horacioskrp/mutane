-- 008_system_ct.up.sql
-- Adds: is_system flag + endpoint_config JSONB on content_types
--       data JSONB column on users (custom fields for the User CT)
--       Seeds the User system content type

-- ── content_types: new columns ───────────────────────────────────────────────
ALTER TABLE content_types
  ADD COLUMN IF NOT EXISTS is_system       BOOLEAN NOT NULL DEFAULT FALSE,
  ADD COLUMN IF NOT EXISTS endpoint_config JSONB   NOT NULL DEFAULT '{}';

-- ── users: custom-field data bag ─────────────────────────────────────────────
ALTER TABLE users
  ADD COLUMN IF NOT EXISTS data JSONB NOT NULL DEFAULT '{}';

-- ── Seed the User system content type ────────────────────────────────────────
-- Runs safely on repeated migrations: updates is_system + keeps existing config
INSERT INTO content_types (name, slug, description, is_system, endpoint_config)
VALUES (
  'User',
  'users',
  'Type système — utilisateurs de la plateforme',
  TRUE,
  '{"public":false,"methods":{"find":true,"findOne":true,"create":false,"update":false,"delete":false},"features":{"find":{"pagination":true,"filters":false,"sort":false,"search":false,"field_selection":false},"findOne":{"field_selection":false},"create":{"sanitize":true},"update":{"sanitize":true,"partial":true},"delete":{}}}'
)
ON CONFLICT (slug) DO UPDATE
  SET is_system       = TRUE,
      endpoint_config = CASE
        WHEN content_types.endpoint_config = '{}'::jsonb
        THEN EXCLUDED.endpoint_config
        ELSE content_types.endpoint_config
      END;
