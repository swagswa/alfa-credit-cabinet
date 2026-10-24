-- Решение это первый переход в approved, counter_offer или rejected: до него считаем
-- время от подачи, по автору видно, решила система или андеррайтер. Решение после
-- повтора проверок не считаем, иначе одна заявка дала бы два времени.
-- name: StatsSummary :one
WITH apps AS (
    SELECT id, submitted_at
    FROM applications
    WHERE submitted_at >= @since::timestamptz
), decided AS (
    SELECT DISTINCT ON (e.application_id)
           e.actor_role, e.to_status, extract(epoch FROM e.created_at - a.submitted_at)::float8 AS sec
    FROM application_events e
    JOIN apps a ON a.id = e.application_id
    WHERE e.kind = 'transition' AND e.to_status IN ('approved', 'counter_offer', 'rejected')
    ORDER BY e.application_id, e.id
)
SELECT (SELECT count(*) FROM apps)::integer AS submitted,
       count(*)::integer AS decided,
       count(*) FILTER (WHERE actor_role = 'system')::integer AS auto_decided,
       count(*) FILTER (WHERE to_status <> 'rejected')::integer AS approved,
       coalesce(percentile_cont(0.5) WITHIN GROUP (ORDER BY sec), 0)::integer AS median_sec,
       coalesce(percentile_cont(0.9) WITHIN GROUP (ORDER BY sec), 0)::integer AS p90_sec,
       (SELECT coalesce(sum(p.amount_kop), 0) FROM payouts p JOIN apps a ON a.id = p.application_id)::bigint
           AS issued_kop
FROM decided;

-- name: StatsByStatus :many
SELECT status, count(*)::integer AS count, count(*) FILTER (WHERE due_at < now())::integer AS overdue
FROM applications
WHERE submitted_at >= @since::timestamptz
GROUP BY status
ORDER BY status;
