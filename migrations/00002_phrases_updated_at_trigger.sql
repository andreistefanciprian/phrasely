-- +goose Up
-- +goose StatementBegin

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

-- +goose StatementEnd
