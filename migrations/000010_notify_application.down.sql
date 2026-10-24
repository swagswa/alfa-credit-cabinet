CREATE OR REPLACE FUNCTION notify_document() RETURNS trigger AS $$
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

DROP FUNCTION notify_application(uuid, jsonb);
