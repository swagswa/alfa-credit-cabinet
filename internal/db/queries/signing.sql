-- name: SaveSigningCode :exec
INSERT INTO signing_codes (application_id, code_hash, attempts_left, expires_at)
VALUES (@application_id, @code_hash, @attempts_left, @expires_at)
ON CONFLICT (application_id) DO UPDATE
SET code_hash = excluded.code_hash, attempts_left = excluded.attempts_left,
    expires_at = excluded.expires_at, created_at = now();

-- name: LockSigningCode :one
SELECT code_hash, attempts_left, expires_at FROM signing_codes WHERE application_id = @application_id FOR UPDATE;

-- name: SetSigningAttempts :exec
UPDATE signing_codes SET attempts_left = @attempts_left WHERE application_id = @application_id;

-- name: DeleteSigningCode :exec
DELETE FROM signing_codes WHERE application_id = @application_id;

-- счёт из последнего ответа данных банка по заявке; не наш клиент, условный счёт.
-- Деньги всё равно никуда не идут
-- name: AddPayout :exec
INSERT INTO payouts (application_id, amount_kop, account)
VALUES (@application_id, @amount_kop, coalesce(
    (SELECT r.payload ->> 'account' FROM check_results r
     WHERE r.application_id = @application_id AND r.source = 'internal' AND r.status = 'ok'
     ORDER BY r.id DESC LIMIT 1),
    'расчётный счёт в банке'))
ON CONFLICT (application_id) DO NOTHING;
