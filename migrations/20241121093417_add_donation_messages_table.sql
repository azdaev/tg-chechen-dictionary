-- +goose Up
-- +goose StatementBegin
create table if not exists donation_messages (
    id serial primary key,
    user_id varchar(255) not null references users(user_id) on delete cascade,
    sent_at timestamptz not null default now()
);

create index donation_messages_user_id_sent_at_idx on donation_messages(user_id, sent_at);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
drop table if exists donation_messages;
-- +goose StatementEnd
