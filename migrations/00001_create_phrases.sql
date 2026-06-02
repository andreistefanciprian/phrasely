-- +goose Up
-- +goose StatementBegin

-- pgcrypto gives us gen_random_uuid() to generate UUIDs inside Postgres.
-- Without this extension we'd have to generate UUIDs in application code instead.
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE phrases (
    -- UUID primary key: globally unique, safe to expose in URLs, no sequential ID guessing
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),

    -- The full example sentence with the definition embedded inline,
    -- matching the "phrase" field in phrases.json
    phrase     TEXT        NOT NULL,

    -- The word or expression being illustrated (used for search/filtering)
    keyword    TEXT        NOT NULL,

    -- Usage guidance: when and how to use the word
    note       TEXT        NOT NULL DEFAULT '',

    -- One URL per keyword; compound keywords (e.g. "word1 vs word2") get one URL per word.
    -- Empty array for phrases without external references.
    source_urls TEXT[]      NOT NULL DEFAULT '{}',

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index on keyword so filtering by word is fast even with thousands of phrases
CREATE INDEX idx_phrases_keyword ON phrases (keyword);

-- Reusable function: sets updated_at to NOW() on any UPDATE.
-- Defined once here; other tables can reuse it by creating their own trigger pointing at it.
CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Fire set_updated_at() automatically before every UPDATE on phrases.
CREATE TRIGGER phrases_set_updated_at
    BEFORE UPDATE ON phrases
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TRIGGER IF EXISTS phrases_set_updated_at ON phrases;
DROP FUNCTION IF EXISTS set_updated_at;
DROP TABLE IF EXISTS phrases;

-- +goose StatementEnd
