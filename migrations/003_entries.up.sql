CREATE TABLE IF NOT EXISTS entries (
    id              BIGSERIAL PRIMARY KEY,
    content_type_id BIGINT NOT NULL REFERENCES content_types (id) ON DELETE CASCADE,
    data            JSONB NOT NULL DEFAULT '{}',
    published_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS entries_content_type_idx ON entries (content_type_id);
CREATE INDEX IF NOT EXISTS entries_published_idx   ON entries (published_at) WHERE published_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS entries_data_gin_idx    ON entries USING gin (data);
