-- +goose Up
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM phrases WHERE headwords IS NULL) THEN
        RAISE EXCEPTION 'cannot finalize structured headwords: phrases with NULL headwords remain; complete and review the conversion first';
    END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE phrases ALTER COLUMN headwords SET NOT NULL;

DROP INDEX idx_phrases_headwords;
DROP INDEX idx_phrases_headwords_trgm;
DROP FUNCTION headwords_text(TEXT[]);

ALTER TABLE phrases
    DROP COLUMN legacy_phrase,
    DROP COLUMN legacy_headwords,
    DROP COLUMN legacy_source_urls;

-- +goose Down
-- A reverse conversion would be lossy. Restore the retained pre-conversion
-- backup instead of rolling this migration down.
-- +goose StatementBegin
DO $$
BEGIN
    RAISE EXCEPTION 'migration 00009 cannot be rolled back safely; restore the retained pre-conversion backup';
END $$;
-- +goose StatementEnd
