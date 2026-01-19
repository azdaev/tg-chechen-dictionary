-- +goose Up
-- +goose StatementBegin
alter table users add column is_blocked integer not null default 0;
alter table users add column blocked_at datetime;
alter table users add column blocked_reason text;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- sqlite does not support drop column; no-op
-- +goose StatementEnd
