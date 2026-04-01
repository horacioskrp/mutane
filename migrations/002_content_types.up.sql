CREATE TABLE IF NOT EXISTS content_types (
    id          BIGSERIAL PRIMARY KEY,
    name        TEXT NOT NULL,
    slug        TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS fields (
    id              BIGSERIAL PRIMARY KEY,
    content_type_id BIGINT NOT NULL REFERENCES content_types (id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    type            TEXT NOT NULL,
    required        BOOLEAN NOT NULL DEFAULT FALSE,
    "order"         INT NOT NULL DEFAULT 0,
    UNIQUE (content_type_id, name)
);

CREATE INDEX IF NOT EXISTS fields_content_type_idx ON fields (content_type_id);
