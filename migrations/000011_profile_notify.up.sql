-- контакты такие, как их знает банк; у сотрудников пусто
ALTER TABLE users
    ADD COLUMN phone text NOT NULL DEFAULT '' CHECK (phone = '' OR phone ~ '^\+7\d{10}$'),
    ADD COLUMN email text NOT NULL DEFAULT '' CHECK (email = '' OR email ~ '^[^@\s]+@[^@\s]+\.[^@\s]+$');

-- расшифровка ОКВЭД из выписки, код без неё клиенту ничего не говорит
ALTER TABLE companies ADD COLUMN okved_name text NOT NULL DEFAULT '';

-- лента уведомлений в кабинете: пишется в одной транзакции со сменой статуса
CREATE TABLE notifications (
    id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id        uuid NOT NULL REFERENCES users (id),
    application_id uuid REFERENCES applications (id),
    kind           text NOT NULL,
    title          text NOT NULL,
    body           text NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),
    read_at        timestamptz
);

CREATE INDEX notifications_user_idx ON notifications (user_id, created_at DESC);
