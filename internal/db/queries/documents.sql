-- name: DocumentForRecognition :one
SELECT d.kind, d.status, d.content_type, d.content, c.inn
FROM documents d
JOIN applications a ON a.id = d.application_id
JOIN companies c ON c.id = a.company_id
WHERE d.id = $1;

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
