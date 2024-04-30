-- +goose Up
-- +goose StatementBegin
create table if not exists users (
    id serial primary key,
    user_id varchar(255) not null unique,
    username varchar(255) not null unique,
    created_at timestamptz not null default now()
);

create table if not exists activity (
    id serial primary key,
    user_id varchar(255) not null references users(user_id) on delete cascade,
    activity_type integer not null,
    created_at timestamptz not null default now()
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
drop table if exists users;
drop table if exists activity;
-- +goose StatementEnd
