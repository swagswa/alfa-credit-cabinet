-- name: UserByLogin :one
SELECT id, login, password_hash, full_name, role
FROM users
WHERE login = $1;

-- name: UserByID :one
SELECT id, login, full_name, role
FROM users
WHERE id = $1;

-- name: MembershipsByUser :many
SELECT c.id, c.inn, c.short_name, c.kind, c.simulated, m.representative_role
FROM company_members m
JOIN companies c ON c.id = m.company_id
WHERE m.user_id = $1
ORDER BY c.short_name;
