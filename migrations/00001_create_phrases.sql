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

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index on keyword so filtering by word is fast even with thousands of phrases
CREATE INDEX idx_phrases_keyword ON phrases (keyword);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Rolls back the migration: drops the table and its index (index is dropped automatically)
DROP TABLE IF EXISTS phrases;

-- +goose StatementEnd
