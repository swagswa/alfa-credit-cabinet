-- ответ источника сразу уходит в SSE: на живой карте видно, кто уже ответил
-- name: AddCheckResult :exec
WITH r AS (
    INSERT INTO check_results (application_id, source, status, payload, error)
    VALUES (@application_id, @source, @status, sqlc.narg(payload), sqlc.narg(error))
)
SELECT notify_application(@application_id, jsonb_build_object(
    'source', @source::text, 'check_status', @status::text,
    'attempt', @attempt::integer, 'attempts', @attempts::integer));

-- name: NotifyCheckAttempt :exec
SELECT notify_application(@application_id, jsonb_build_object(
    'source', @source::text, 'attempt', @attempt::integer, 'attempts', @attempts::integer,
    'error', @error::text));
