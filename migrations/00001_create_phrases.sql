-- +goose Up
-- +goose StatementBegin

-- pgcrypto gives us gen_random_uuid() to generate UUIDs inside Postgres.
-- Without this extension we'd have to generate UUIDs in application code instead.
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- pg_trgm breaks text into overlapping 3-character chunks so ILIKE '%term%'
-- can use an index instead of scanning every row.
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- users are identified by email; no password ever stored.
CREATE TABLE users (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    email      TEXT        UNIQUE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE phrases (
    -- UUID primary key: globally unique, safe to expose in URLs, no sequential ID guessing
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),

    -- The full example sentence with the definition embedded inline,
    -- matching the "phrase" field in phrases.json
    phrase     TEXT        NOT NULL,

    -- One or more headwords illustrated by the phrase.
    -- Stored as an array so compound entries ("unfettered vs inalienable") need no string parsing.
    -- source_urls aligns by index: headwords[0] → source_urls[0].
    headwords  TEXT[]      NOT NULL,

    -- Usage guidance: when and how to use the word
    note       TEXT        NOT NULL DEFAULT '',

    -- One URL per headword; compound headwords (e.g. "word1 vs word2") get one URL per word.
    -- Empty array for phrases without external references.
    source_urls TEXT[]      NOT NULL DEFAULT '{}',

    -- Nullable for now; required once JWT middleware is wired.
    user_id    UUID        REFERENCES users(id) ON DELETE CASCADE,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- GIN index on headwords array enables fast ANY() lookups across the array elements
CREATE INDEX idx_phrases_headwords ON phrases USING GIN (headwords);

-- Postgres requires index expression functions to be IMMUTABLE.
-- array_to_string is only STABLE, so we wrap it in an IMMUTABLE function.
CREATE OR REPLACE FUNCTION headwords_text(TEXT[])
RETURNS TEXT LANGUAGE SQL IMMUTABLE AS $$
    SELECT array_to_string($1, ' ')
$$;

-- Trigram GIN index enables fast ILIKE search across all headwords.
CREATE INDEX idx_phrases_headwords_trgm ON phrases
    USING GIN (headwords_text(headwords) gin_trgm_ops);

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
DROP TABLE IF EXISTS users;
DROP FUNCTION IF EXISTS headwords_text;

-- +goose StatementEnd
