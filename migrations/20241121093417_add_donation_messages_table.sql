-- +goose Up
-- +goose StatementBegin
create table if not exists donation_messages (
    id integer primary key autoincrement,
    user_id varchar(255) not null references users(user_id) on delete cascade,
    sent_at datetime not null default current_timestamp
);

create index donation_messages_user_id_sent_at_idx on donation_messages(user_id, sent_at);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
drop table if exists donation_messages;
-- +goose StatementEnd
