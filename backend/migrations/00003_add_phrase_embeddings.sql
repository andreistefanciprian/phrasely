-- +goose Up
-- +goose StatementBegin

CREATE EXTENSION IF NOT EXISTS vector;

-- Nullable: existing phrases start with NULL and are backfilled asynchronously.
-- 1536 dimensions matches text-embedding-3-small.
ALTER TABLE phrases ADD COLUMN embedding vector(1536);

-- HNSW index for fast approximate cosine-similarity search.
-- Partial index (WHERE embedding IS NOT NULL) avoids indexing unembedded rows.
CREATE INDEX idx_phrases_embedding ON phrases
    USING hnsw (embedding vector_cosine_ops)
    WHERE embedding IS NOT NULL;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS idx_phrases_embedding;
ALTER TABLE phrases DROP COLUMN IF EXISTS embedding;
DROP EXTENSION IF EXISTS vector;

-- +goose StatementEnd
