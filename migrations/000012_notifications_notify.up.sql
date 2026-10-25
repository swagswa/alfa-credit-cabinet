-- новое уведомление уходит в тот же канал SSE, адресат в notification_user:
-- поток отдаёт такое событие только ему
CREATE FUNCTION notify_notification() RETURNS trigger AS $$
BEGIN
    PERFORM notify_application(NEW.application_id, jsonb_build_object('notification_user', NEW.user_id));
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER notifications_notify AFTER INSERT ON notifications
    FOR EACH ROW EXECUTE FUNCTION notify_notification();
