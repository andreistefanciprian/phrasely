-- +goose Up
-- +goose StatementBegin

-- RFC 8707 Resource Indicators bind every OAuth grant to the MCP server the
-- client intends to call. Existing short-lived codes and refresh tokens are
-- deliberately left with an empty resource so they cannot be upgraded into an
-- audience-bound access token; affected clients re-authorize once.
ALTER TABLE oauth_authorization_codes
    ADD COLUMN resource TEXT NOT NULL DEFAULT '';

ALTER TABLE oauth_refresh_tokens
    ADD COLUMN resource TEXT NOT NULL DEFAULT '';

-- The default only exists to migrate existing rows safely. New grants must
-- always provide an explicit resource through the application.
ALTER TABLE oauth_authorization_codes ALTER COLUMN resource DROP DEFAULT;
ALTER TABLE oauth_refresh_tokens ALTER COLUMN resource DROP DEFAULT;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE oauth_refresh_tokens DROP COLUMN resource;
ALTER TABLE oauth_authorization_codes DROP COLUMN resource;

-- +goose StatementEnd
