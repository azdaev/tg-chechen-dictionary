-- +goose Up
-- +goose StatementBegin

-- Создаем новые таблицы с правильными типами данных
CREATE TABLE users_new (
    id integer primary key autoincrement,
    user_id integer not null unique,
    username varchar(255),
    created_at datetime not null default current_timestamp
);

CREATE TABLE activity_new (
    id integer primary key autoincrement,
    user_id integer not null references users_new(user_id) on delete cascade,
    activity_type integer not null,
    created_at datetime not null default current_timestamp
);

-- Копируем данные, конвертируя user_id из varchar в integer
INSERT INTO users_new (id, user_id, username, created_at)
SELECT id, CAST(user_id AS INTEGER), username, created_at FROM users;

INSERT INTO activity_new (id, user_id, activity_type, created_at)
SELECT id, CAST(user_id AS INTEGER), activity_type, created_at FROM activity;

-- Удаляем старые таблицы
DROP TABLE activity;
DROP TABLE users;

-- Переименовываем новые таблицы
ALTER TABLE users_new RENAME TO users;
ALTER TABLE activity_new RENAME TO activity;

-- Создаем индексы для производительности
CREATE INDEX idx_users_user_id ON users(user_id);
CREATE INDEX idx_activity_user_id ON activity(user_id);
CREATE INDEX idx_activity_created_at ON activity(created_at);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Создаем старые таблицы
CREATE TABLE users_old (
    id integer primary key autoincrement,
    user_id varchar(255) not null unique,
    username varchar(255) not null unique,
    created_at datetime not null default current_timestamp
);

CREATE TABLE activity_old (
    id integer primary key autoincrement,
    user_id varchar(255) not null references users_old(user_id) on delete cascade,
    activity_type integer not null,
    created_at datetime not null default current_timestamp
);

-- Копируем данные обратно, конвертируя user_id из integer в varchar
INSERT INTO users_old (id, user_id, username, created_at)
SELECT id, CAST(user_id AS TEXT), username, created_at FROM users;

INSERT INTO activity_old (id, user_id, activity_type, created_at)
SELECT id, CAST(user_id AS TEXT), activity_type, created_at FROM activity;

-- Удаляем новые таблицы
DROP TABLE activity;
DROP TABLE users;

-- Переименовываем старые таблицы
ALTER TABLE users_old RENAME TO users;
ALTER TABLE activity_old RENAME TO activity;

-- +goose StatementEnd
