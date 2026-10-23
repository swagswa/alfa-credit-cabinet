-- name: AddDocument :one
INSERT INTO documents (application_id, kind, status, file_name, content_type, size_bytes, sha256, content, uploaded_by)
VALUES (@application_id, @kind, @status, @file_name, @content_type, @size_bytes, @sha256, @content, @uploaded_by)
RETURNING id;

-- owner задан у клиента: чужой документ для него просто не находится, как и заявка
-- name: DocumentFile :one
SELECT d.kind, d.status, d.file_name, d.content_type, d.content, c.inn
FROM documents d
JOIN applications a ON a.id = d.application_id
JOIN companies c ON c.id = a.company_id
WHERE d.id = @id
  AND (sqlc.narg(owner)::uuid IS NULL OR EXISTS (
      SELECT 1 FROM company_members o WHERE o.company_id = a.company_id AND o.user_id = sqlc.narg(owner)));

-- name: DocumentCard :one
SELECT d.id, d.application_id, d.kind, d.status, d.file_name, d.content_type, d.size_bytes, d.sha256, c.inn,
       d.text_source, d.extracted, d.confirmed, d.edited_fields, d.validation_errors, d.error, d.model,
       d.created_at, d.confirmed_at
FROM documents d
JOIN applications a ON a.id = d.application_id
JOIN companies c ON c.id = a.company_id
WHERE d.id = @id
  AND (sqlc.narg(owner)::uuid IS NULL OR EXISTS (
      SELECT 1 FROM company_members o WHERE o.company_id = a.company_id AND o.user_id = sqlc.narg(owner)));

-- confirmed нужен решению: сверка с ФНС берёт только подтверждённые итоги
-- name: ApplicationDocuments :many
SELECT id, kind, status, file_name, confirmed
FROM documents
WHERE application_id = $1
ORDER BY created_at;

-- Подтверждают один раз: второе подтверждение из другой вкладки ничего не меняет
-- name: ConfirmDocument :execrows
UPDATE documents
SET status        = 'confirmed',
    confirmed     = @confirmed,
    edited_fields = coalesce(@edited_fields::text[], '{}'),
    confirmed_by  = @confirmed_by,
    confirmed_at  = now()
WHERE id = @id AND status <> 'confirmed';

-- Статус меняем, только пока распознавание не закончено: подтверждённый
-- клиентом документ повтор activity трогать не должен.
-- name: SetDocumentStatus :exec
UPDATE documents
SET status = @status, error = @error
WHERE id = @id AND status IN ('uploaded', 'recognizing');

-- name: SaveRecognition :exec
UPDATE documents
SET status            = @status,
    text_source       = @text_source,
    extracted_raw     = @extracted_raw,
    extracted         = @extracted,
    validation_errors = @validation_errors,
    error             = @error,
    model             = @model,
    model_digest      = @model_digest,
    extractor_version = @extractor_version,
    duration_ms       = @duration_ms
WHERE id = @id AND status = 'recognizing';
