-- +goose Up
-- +goose StatementBegin

-- Per-user preferences for the "Phrase Digest" email.
-- frequency: daily | weekly | monthly | disabled (default disabled = opt-in)
CREATE TABLE digest_preferences (
    user_id      UUID        PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    frequency    TEXT        NOT NULL DEFAULT 'disabled' CHECK (frequency IN ('daily', 'weekly', 'monthly', 'disabled')),
    last_sent_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TRIGGER digest_preferences_set_updated_at
    BEFORE UPDATE ON digest_preferences
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TRIGGER IF EXISTS digest_preferences_set_updated_at ON digest_preferences;
DROP TABLE IF EXISTS digest_preferences;

-- +goose StatementEnd
