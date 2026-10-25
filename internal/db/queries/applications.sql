-- name: LockApplication :one
SELECT id, company_id, status, version
FROM applications
WHERE id = $1
FOR UPDATE;

-- name: SetApplicationStatus :exec
UPDATE applications
SET status = @status::text,
    version = version + 1,
    due_at = sqlc.narg(due_at),
    submitted_at = CASE WHEN @status::text = 'submitted' THEN now() ELSE submitted_at END,
    updated_at = now()
WHERE id = @id;

-- name: AddTransitionEvent :exec
INSERT INTO application_events (application_id, from_status, to_status, actor_id, actor_role, reason_codes, comment)
VALUES (@application_id, @from_status, @to_status, sqlc.narg(actor_id), @actor_role,
        coalesce(@reason_codes::text[], '{}'), sqlc.narg(comment));

-- уведомление уходит слушателям после коммита, на нём живёт SSE
-- name: NotifyApplication :exec
SELECT notify_application(@id);

-- name: CompanyByINN :one
SELECT c.id, EXISTS (
    SELECT 1 FROM company_members m WHERE m.company_id = c.id AND m.user_id = @user_id
) AS member
FROM companies c
WHERE c.inn = @inn;

-- name: AddCompany :one
INSERT INTO companies (inn, kind, ogrn, kpp, short_name, full_name, okved, okved_name, address, registered_at, simulated)
VALUES (@inn, @kind, @ogrn, sqlc.narg(kpp), @short_name, @full_name, @okved, @okved_name, @address, @registered_at, @simulated)
RETURNING id;

-- name: SetMember :exec
INSERT INTO company_members (user_id, company_id, representative_role)
VALUES (@user_id, @company_id, @representative_role)
ON CONFLICT (user_id, company_id) DO UPDATE SET representative_role = excluded.representative_role;

-- Повтор с тем же ключом ничего не вставляет и не возвращает, тогда заявку
-- достаёт ApplicationByKey отдельным запросом: в снимке этого её может не быть.
-- name: CreateApplication :one
INSERT INTO applications (company_id, created_by, idempotency_key, request_hash, product, purpose,
                          amount_kop, term_months, monthly_revenue_kop, equipment_cost_kop, bki_consent_at,
                          pd_consent_at, comment)
VALUES (@company_id, @created_by, @idempotency_key, @request_hash, @product, @purpose,
        @amount_kop, @term_months, sqlc.narg(monthly_revenue_kop), sqlc.narg(equipment_cost_kop),
        CASE WHEN @bki_consent::boolean THEN now() END, CASE WHEN @pd_consent::boolean THEN now() END,
        sqlc.narg(comment))
ON CONFLICT (created_by, idempotency_key) DO NOTHING
RETURNING id;

-- name: ApplicationByKey :one
SELECT id, request_hash
FROM applications
WHERE created_by = @created_by AND idempotency_key = @idempotency_key;

-- owner задан у клиента: чужая заявка для него просто не находится
-- name: ApplicationCard :one
SELECT v.*
FROM application_cards v
WHERE v.id = @id
  AND (sqlc.narg(owner)::uuid IS NULL OR EXISTS (
      SELECT 1 FROM company_members o WHERE o.company_id = v.company_id AND o.user_id = sqlc.narg(owner)));

-- без owner это очередь банка: заявку, которую ни разу не подавали (черновик или отменённый
-- черновик), видит только клиент, согласий у банка на неё нет
-- name: ApplicationCards :many
SELECT sqlc.embed(v), count(*) OVER () AS total
FROM application_cards v
WHERE (sqlc.narg(owner)::uuid IS NULL AND v.submitted_at IS NOT NULL OR EXISTS (
          SELECT 1 FROM company_members o WHERE o.company_id = v.company_id AND o.user_id = sqlc.narg(owner)))
  AND (sqlc.narg(statuses)::text[] IS NULL OR v.status = ANY (sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(grades)::text[] IS NULL OR v.grade = ANY (sqlc.narg(grades)::text[]))
  AND (sqlc.narg(product)::text IS NULL OR v.product = sqlc.narg(product))
-- порядок из параметра sort, по одной строке на вариант: у столбцов разные типы,
-- один CASE на всех не собрать. Без срока и без балла всегда в конце
ORDER BY CASE WHEN @sort::text = 'created_at' THEN v.created_at END,
         CASE WHEN @sort::text = 'updated_at' THEN v.updated_at END,
         CASE WHEN @sort::text = '-updated_at' THEN v.updated_at END DESC,
         CASE WHEN @sort::text = 'amount_kop' THEN v.amount_kop END,
         CASE WHEN @sort::text = '-amount_kop' THEN v.amount_kop END DESC,
         CASE WHEN @sort::text = 'due_at' THEN v.due_at END,
         CASE WHEN @sort::text = 'score' THEN v.score END,
         CASE WHEN @sort::text = '-score' THEN v.score END DESC NULLS LAST,
         v.created_at DESC, v.id DESC
LIMIT @lim OFFSET @off;

-- после need_info источники опрашиваются заново, в карточку идёт последний ответ
-- name: LatestChecks :many
SELECT DISTINCT ON (source) source, status, payload, error, created_at
FROM check_results
WHERE application_id = @application_id
ORDER BY source, id DESC;

-- name: ApplicationEvents :many
SELECT e.id, e.kind, e.from_status, e.to_status, e.actor_role, u.full_name, e.reason_codes, e.comment, e.created_at
FROM application_events e
LEFT JOIN users u ON u.id = e.actor_id
WHERE e.application_id = @application_id
ORDER BY e.id;

-- версия и статус в WHERE: между чтением и записью заявку могли подать или поправить в другой вкладке
-- name: UpdateDraft :execrows
UPDATE applications
SET product             = @product,
    purpose             = @purpose,
    amount_kop          = @amount_kop,
    term_months         = @term_months,
    monthly_revenue_kop = sqlc.narg(monthly_revenue_kop),
    equipment_cost_kop  = sqlc.narg(equipment_cost_kop),
    bki_consent_at      = CASE WHEN @bki_consent::boolean THEN coalesce(bki_consent_at, now()) END,
    pd_consent_at       = CASE WHEN @pd_consent::boolean THEN coalesce(pd_consent_at, now()) END,
    comment             = sqlc.narg(comment),
    version             = version + 1,
    updated_at          = now()
WHERE id = @id AND version = @version AND status = 'draft';

-- выручку после подачи клиент уточняет только в ответ на запрос банка
-- name: SetRevenue :execrows
UPDATE applications
SET monthly_revenue_kop = @monthly_revenue_kop::bigint,
    version             = version + 1,
    updated_at          = now()
WHERE id = @id AND status = 'need_info';

-- эскалация статус не меняет, но карточку и очередь стоит обновить
-- name: AddEscalation :exec
WITH e AS (
    INSERT INTO application_events (application_id, kind, actor_role) VALUES (@id, 'escalation', 'system')
)
SELECT notify_application(@id);

-- name: AddNote :exec
WITH e AS (
    INSERT INTO application_events (application_id, kind, actor_id, actor_role, comment)
    VALUES (@id, 'signing', sqlc.narg(actor_id), @actor_role, @comment)
)
SELECT notify_application(@id);

-- согласие из анкеты становится записью при подаче, повтор ничего не меняет
-- name: AddBKIConsent :exec
INSERT INTO bki_consents (application_id, given_at)
SELECT a.id, a.bki_consent_at
FROM applications a
WHERE a.id = @application_id
ON CONFLICT (application_id) DO NOTHING;

-- name: BKIConsent :one
SELECT id, given_at
FROM bki_consents
WHERE application_id = @application_id;

-- name: AddInfoRequest :exec
INSERT INTO info_requests (application_id, documents, reasons, comment, requested_by)
VALUES (@application_id, coalesce(@documents::text[], '{}'), @reasons, sqlc.narg(comment), sqlc.narg(requested_by));

-- name: CloseInfoRequest :exec
UPDATE info_requests
SET closed_at = now()
WHERE application_id = @application_id AND closed_at IS NULL;

-- name: OpenInfoRequest :one
SELECT documents, reasons, comment
FROM info_requests
WHERE application_id = @application_id AND closed_at IS NULL;
