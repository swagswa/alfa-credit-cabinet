-- код подписи один на заявку, новый перезаписывает старый. Сам код не храним
CREATE TABLE signing_codes (
    application_id uuid PRIMARY KEY REFERENCES applications (id),
    code_hash      bytea NOT NULL CHECK (length(code_hash) = 32),
    attempts_left  smallint NOT NULL CHECK (attempts_left BETWEEN 0 AND 3),
    expires_at     timestamptz NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now()
);

-- выдача смоделирована: строка вместо проводки, ключ по заявке не даёт выдать дважды
CREATE TABLE payouts (
    application_id uuid PRIMARY KEY REFERENCES applications (id),
    amount_kop     bigint NOT NULL CHECK (amount_kop > 0),
    account        text NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now()
);

-- договор формирует банк, у него нет автора среди пользователей
ALTER TABLE documents
    DROP CONSTRAINT documents_kind_check,
    ADD CONSTRAINT documents_kind_check CHECK (kind IN (
        'passport', 'power_of_attorney', 'statement',
        'financials', 'supply_contract', 'tax_receipt', 'loan_agreement')),
    ALTER COLUMN uploaded_by DROP NOT NULL,
    ADD CONSTRAINT documents_author_check CHECK ((uploaded_by IS NULL) = (kind = 'loan_agreement'));

CREATE UNIQUE INDEX documents_contract_idx ON documents (application_id) WHERE kind = 'loan_agreement';

ALTER TABLE application_events
    DROP CONSTRAINT application_events_kind_check,
    ADD CONSTRAINT application_events_kind_check
        CHECK (kind IN ('transition', 'escalation', 'override', 'signing'));
