-- name: UserByLogin :one
SELECT id, login, password_hash, full_name, role
FROM users
WHERE login = $1;
