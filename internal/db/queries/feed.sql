-- компания для ленты, только если пользователь её представляет; без company_id первая по имени, как в /me.
-- bank это последний ответ данных банка по заявкам компании: '{}' если компания не наш клиент, NULL если не спрашивали
-- name: FeedCompany :one
SELECT c.id, c.inn, c.okved,
       coalesce((SELECT a.status FROM applications a WHERE a.company_id = c.id
                 ORDER BY a.created_at DESC LIMIT 1), 'none')::text AS last_status,
       (SELECT coalesce(r.payload, '{}')::jsonb FROM check_results r
        JOIN applications a ON a.id = r.application_id
        WHERE a.company_id = c.id AND r.source = 'internal' AND r.status <> 'unavailable'
        ORDER BY r.id DESC LIMIT 1) AS bank
FROM company_members m
JOIN companies c ON c.id = m.company_id
WHERE m.user_id = @user_id AND (sqlc.narg(company_id)::uuid IS NULL OR c.id = sqlc.narg(company_id))
ORDER BY c.short_name
LIMIT 1;

-- эскалации без статуса клиенту не показываем; членство проверяем и здесь, ленту не отдать мимо него
-- name: FeedEvents :many
SELECT e.id, e.application_id, a.number, e.to_status::text AS status, e.created_at
FROM application_events e
JOIN applications a ON a.id = e.application_id
JOIN company_members m ON m.company_id = a.company_id AND m.user_id = @user_id
WHERE a.company_id = @company_id AND e.to_status IS NOT NULL
ORDER BY e.id DESC
LIMIT @lim;
