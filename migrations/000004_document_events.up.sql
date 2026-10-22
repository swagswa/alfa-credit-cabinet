-- Статус документа меняют api (загрузка, подтверждение) и docs-worker (распознавание)
-- разными запросами. Уведомление в триггере, чтобы ни один из них его не забыл.
-- Канал и формат те же, что у заявок, слушатель SSE один.
CREATE FUNCTION notify_document() RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'UPDATE' AND OLD.status = NEW.status THEN
        RETURN NULL;
    END IF;
    PERFORM pg_notify('applications', json_build_object(
        'application_id', a.id,
        'company_id', a.company_id,
        'status', a.status,
        'version', a.version,
        'document_id', NEW.id,
        'document_status', NEW.status)::text)
    FROM applications a
    WHERE a.id = NEW.application_id;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER documents_notify
AFTER INSERT OR UPDATE OF status ON documents
FOR EACH ROW
EXECUTE FUNCTION notify_document();
