-- +goose Up
-- +goose StatementBegin
alter table users drop constraint users_username_key;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE users ADD CONSTRAINT users_username_key UNIQUE(username);
-- +goose StatementEnd
