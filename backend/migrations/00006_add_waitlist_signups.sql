-- +goose Up
-- +goose StatementBegin

-- Emails collected from the landing page's "join the ChatGPT list" capture.
-- No user_id: signups happen before the visitor has an account.
CREATE TABLE waitlist_signups (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    email      TEXT        NOT NULL UNIQUE,
    source     TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS waitlist_signups;

-- +goose StatementEnd
