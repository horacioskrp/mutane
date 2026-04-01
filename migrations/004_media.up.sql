CREATE TABLE IF NOT EXISTS media (
    id            BIGSERIAL PRIMARY KEY,
    filename      TEXT NOT NULL UNIQUE,
    original_name TEXT NOT NULL,
    mime_type     TEXT NOT NULL DEFAULT '',
    size          BIGINT NOT NULL DEFAULT 0,
    url           TEXT NOT NULL,
    uploaded_by   BIGINT REFERENCES users (id) ON DELETE SET NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS media_uploaded_by_idx ON media (uploaded_by);
CREATE INDEX IF NOT EXISTS media_created_at_idx  ON media (created_at DESC);
