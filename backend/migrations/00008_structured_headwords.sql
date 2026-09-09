-- Structural only. Linguistic conversion is reviewed separately; see docs/structured-headwords.md.
-- +goose Up
ALTER TABLE phrases RENAME COLUMN headwords TO legacy_headwords;
ALTER TABLE phrases RENAME COLUMN source_urls TO legacy_source_urls;
ALTER TABLE phrases ALTER COLUMN legacy_headwords SET DEFAULT '{}';
ALTER TABLE phrases ADD COLUMN legacy_phrase TEXT;
ALTER TABLE phrases DISABLE TRIGGER phrases_set_updated_at;
UPDATE phrases SET legacy_phrase = phrase;
ALTER TABLE phrases ENABLE TRIGGER phrases_set_updated_at;
ALTER TABLE phrases ADD COLUMN headwords JSONB CHECK (headwords IS NULL OR (jsonb_typeof(headwords) = 'array' AND jsonb_array_length(headwords) > 0));

-- Embedding maintenance must not change the user's last-edit timestamp.
DROP TRIGGER phrases_set_updated_at ON phrases;
CREATE TRIGGER phrases_set_updated_at BEFORE UPDATE OF phrase, headwords, note ON phrases
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose Down
-- Deliberately refuse a lossy automatic rollback after conversion/new writes.
-- Restore the pre-migration backup instead.
SELECT 1 / 0;
