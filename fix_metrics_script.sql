-- Скрипт для проверки и исправления проблем с метриками
-- Выполняется вручную для проверки данных

-- 1. Проверяем текущую схему
.schema users
.schema activity

-- 2. Проверяем текущие данные (если есть)
SELECT COUNT(*) as total_users FROM users;
SELECT COUNT(*) as total_activity FROM activity;

-- 3. Проверяем типы данных user_id
SELECT typeof(user_id) as user_id_type FROM users LIMIT 1;
SELECT typeof(user_id) as user_id_type FROM activity LIMIT 1;

-- 4. Проверяем проблемные месяцы (должны возвращать 0 для месяцев 1-9 до исправления)
SELECT 
    strftime('%m', datetime('now')) as current_month_str,
    CAST(strftime('%m', datetime('now')) AS INTEGER) as current_month_int;

-- 5. После применения миграции - тестовые данные для проверки
-- INSERT INTO users (user_id, username) VALUES (123456789, 'test_user');
-- INSERT INTO activity (user_id, activity_type) VALUES (123456789, 1);

-- 6. Тест исправленных запросов
-- SELECT COUNT(*) FROM users WHERE strftime('%m', created_at) = printf('%02d', CAST(strftime('%m', datetime('now')) AS INTEGER));
