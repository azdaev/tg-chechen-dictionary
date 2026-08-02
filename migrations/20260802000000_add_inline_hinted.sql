-- +goose Up
-- +goose StatementBegin
-- The inline-mode hint is a one-time lesson. Repeated under every long answer
-- it is just a paragraph the reader learns to skip, and it sits between them
-- and the translations they asked for.
ALTER TABLE users ADD COLUMN inline_hinted INTEGER NOT NULL DEFAULT 0;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE users DROP COLUMN inline_hinted;
-- +goose StatementEnd
