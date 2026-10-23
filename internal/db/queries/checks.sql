-- name: AddCheckResult :exec
INSERT INTO check_results (application_id, source, status, payload, error)
VALUES (@application_id, @source, @status, sqlc.narg(payload), sqlc.narg(error));
