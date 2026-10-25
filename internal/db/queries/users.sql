-- name: UserByLogin :one
SELECT id, login, password_hash, full_name, role
FROM users
WHERE login = $1;

-- name: UserByID :one
SELECT id, login, full_name, role, phone, email
FROM users
WHERE id = $1;

-- name: SetUserContacts :exec
UPDATE users SET phone = $2, email = $3 WHERE id = $1;

-- name: MembershipsByUser :many
SELECT c.id, c.inn, c.short_name, c.kind, c.simulated, m.representative_role
FROM company_members m
JOIN companies c ON c.id = m.company_id
WHERE m.user_id = $1
ORDER BY c.short_name;

-- name: ProfileCompanies :many
SELECT c.id, c.inn, c.kind, c.ogrn, c.kpp, c.short_name, c.full_name, c.okved, c.okved_name, c.address,
       c.registered_at, c.simulated, m.representative_role
FROM company_members m
JOIN companies c ON c.id = m.company_id
WHERE m.user_id = $1
ORDER BY c.short_name;

-- кредит это заявка с выдачей; условия из последнего решения, по которому выдали
-- name: LoansByUser :many
SELECT a.id, a.company_id, a.number, a.status, p.created_at AS issued_at, d.terms,
       doc.id AS contract_id
FROM applications a
JOIN company_members m ON m.company_id = a.company_id AND m.user_id = @user_id
JOIN payouts p ON p.application_id = a.id
JOIN LATERAL (
    SELECT terms FROM decisions WHERE application_id = a.id AND terms IS NOT NULL ORDER BY id DESC LIMIT 1
) d ON true
LEFT JOIN documents doc ON doc.application_id = a.id AND doc.kind = 'loan_agreement'
ORDER BY p.created_at DESC;
