-- name: LockApplication :one
SELECT id, company_id, status, version
FROM applications
WHERE id = $1
FOR UPDATE;

-- name: SetApplicationStatus :one
UPDATE applications
SET status = @status::text,
    version = version + 1,
    due_at = sqlc.narg(due_at),
    submitted_at = CASE WHEN @status::text = 'submitted' THEN now() ELSE submitted_at END,
    updated_at = now()
WHERE id = @id
RETURNING version;

-- name: AddTransitionEvent :exec
INSERT INTO application_events (application_id, from_status, to_status, actor_id, actor_role, reason_codes, comment)
VALUES (@application_id, @from_status, @to_status, sqlc.narg(actor_id), @actor_role,
        coalesce(@reason_codes::text[], '{}'), sqlc.narg(comment));

-- уведомление уходит слушателям после коммита, на нём живёт SSE
-- name: NotifyApplication :exec
SELECT pg_notify('applications', json_build_object(
    'application_id', @id::uuid,
    'company_id', @company_id::uuid,
    'status', @status::text,
    'version', @version::integer)::text);
