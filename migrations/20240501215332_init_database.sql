-- +goose Up
-- +goose StatementBegin
create table if not exists users (
                                     id integer primary key autoincrement,
                                     user_id varchar(255) not null unique,
                                     username varchar(255) not null unique,
                                     created_at datetime not null default current_timestamp
);

create table if not exists activity (
                                        id integer primary key autoincrement,
                                        user_id varchar(255) not null references users(user_id) on delete cascade,
                                        activity_type integer not null,
                                        created_at datetime not null default current_timestamp
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
drop table if exists activity;
drop table if exists users;
-- +goose StatementEnd
