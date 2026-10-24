-- Уведомления SSE собираются в одном месте: поля заявки читаются из строки, а не
-- передаются каждым запросом. submitted нужен банку: заявку, которую ни разу не
-- подавали, он не видит, даже когда она уже отменена.
CREATE FUNCTION notify_application(app_id uuid, extra jsonb DEFAULT '{}') RETURNS void AS $$
    SELECT pg_notify('applications', (jsonb_build_object(
        'application_id', a.id,
        'company_id', a.company_id,
        'status', a.status,
        'version', a.version,
        'submitted', a.submitted_at IS NOT NULL) || extra)::text)
    FROM applications a
    WHERE a.id = app_id;
$$ LANGUAGE sql;

CREATE OR REPLACE FUNCTION notify_document() RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'UPDATE' AND OLD.status = NEW.status THEN
        RETURN NULL;
    END IF;
    PERFORM notify_application(NEW.application_id,
        jsonb_build_object('document_id', NEW.id, 'document_status', NEW.status));
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
