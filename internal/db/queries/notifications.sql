-- адресаты: клиенты компании заявки, сотрудникам банка не пишем
-- name: AddNotifications :exec
INSERT INTO notifications (user_id, application_id, kind, title, body)
SELECT m.user_id, a.id, @kind, @title, @body
FROM applications a
JOIN company_members m ON m.company_id = a.company_id
JOIN users u ON u.id = m.user_id AND u.role = 'client'
WHERE a.id = @application_id;

-- сумма у выданной заявки из выдачи, до выдачи из действующих условий
-- name: NoticeVars :one
SELECT a.number, coalesce(p.amount_kop, a.offer_amount_kop, 0)::bigint AS amount_kop,
       coalesce(p.account, '')::text AS account
FROM applications a
LEFT JOIN payouts p ON p.application_id = a.id
WHERE a.id = $1;

-- name: ListNotifications :many
SELECT id, application_id, kind, title, body, created_at, read_at
FROM notifications
WHERE user_id = @user_id AND (NOT @unread::boolean OR read_at IS NULL)
ORDER BY id DESC
LIMIT @lim;

-- name: ReadNotification :execrows
UPDATE notifications SET read_at = coalesce(read_at, now())
WHERE id = @id AND user_id = @user_id;

-- name: ReadAllNotifications :exec
UPDATE notifications SET read_at = now()
WHERE user_id = $1 AND read_at IS NULL;
