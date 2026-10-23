-- override андеррайтера пишется в журнал тем же запросом
-- name: AddDecision :exec
WITH d AS (
    INSERT INTO decisions (application_id, stage, outcome, reason_codes, reasons, score, grade, score_breakdown,
                           terms, recommendation, scoring_version, policy_version, decided_by, is_override, comment)
    VALUES (@application_id, @stage, @outcome, coalesce(@reason_codes::text[], '{}'), @reasons,
            sqlc.narg(score), sqlc.narg(grade), sqlc.narg(score_breakdown), sqlc.narg(terms),
            sqlc.narg(recommendation), sqlc.narg(scoring_version), sqlc.narg(policy_version),
            sqlc.narg(decided_by), @is_override, sqlc.narg(comment))
)
INSERT INTO application_events (application_id, kind, actor_id, actor_role, reason_codes, comment)
SELECT @application_id, 'override', sqlc.narg(decided_by), 'underwriter', coalesce(@reason_codes::text[], '{}'),
       sqlc.narg(comment)
WHERE @is_override::boolean;

-- условия последнего решения становятся текущими, балл остаётся от скоринга
-- name: SetOffer :exec
UPDATE applications
SET offer_product     = sqlc.narg(product),
    offer_amount_kop  = sqlc.narg(amount_kop),
    offer_term_months = sqlc.narg(term_months),
    offer_rate_bp     = sqlc.narg(rate_bp),
    offer_payment_kop = sqlc.narg(payment_kop),
    offer_guarantor   = @guarantor,
    offer_pledge      = @pledge,
    score             = coalesce(sqlc.narg(score), score),
    grade             = coalesce(sqlc.narg(grade), grade),
    scoring_version   = coalesce(sqlc.narg(scoring_version), scoring_version),
    policy_version    = coalesce(sqlc.narg(policy_version), policy_version)
WHERE id = @id;

-- name: LatestDecision :one
SELECT stage, outcome, reasons, score_breakdown, terms, recommendation, scoring_version, policy_version, is_override
FROM decisions
WHERE application_id = @application_id
ORDER BY id DESC
LIMIT 1;
