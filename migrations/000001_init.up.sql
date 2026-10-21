-- Значения CHECK совпадают с enum в api/openapi.yaml и docs/state-diagram.md.

CREATE TABLE users (
    id            uuid PRIMARY KEY DEFAULT uuidv7(),
    login         text NOT NULL UNIQUE,
    password_hash text NOT NULL,
    full_name     text NOT NULL,
    role          text NOT NULL CHECK (role IN ('client', 'manager', 'underwriter', 'admin')),
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE companies (
    id            uuid PRIMARY KEY DEFAULT uuidv7(),
    inn           text NOT NULL UNIQUE CHECK (inn ~ '^(\d{10}|\d{12})$'),
    kind          text NOT NULL CHECK (kind IN ('UL', 'IP')),
    ogrn          text NOT NULL CHECK (ogrn ~ '^(\d{13}|\d{15})$'),
    kpp           text CHECK (kpp ~ '^\d{9}$'),
    short_name    text NOT NULL,
    full_name     text NOT NULL,
    okved         text NOT NULL,
    address       text NOT NULL,
    registered_at date NOT NULL,
    -- компания из генератора симулятора, а не из фикстур
    simulated     boolean NOT NULL DEFAULT false,
    created_at    timestamptz NOT NULL DEFAULT now(),
    CHECK ((kind = 'UL') = (length(inn) = 10)),
    CHECK (kind = 'UL' OR kpp IS NULL)
);

CREATE TABLE company_members (
    user_id             uuid NOT NULL REFERENCES users (id),
    company_id          uuid NOT NULL REFERENCES companies (id),
    representative_role text NOT NULL CHECK (representative_role IN ('director', 'owner_ip', 'proxy')),
    created_at          timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, company_id)
);

CREATE INDEX company_members_company_idx ON company_members (company_id);

CREATE TABLE applications (
    id                  uuid PRIMARY KEY DEFAULT uuidv7(),
    number              bigint NOT NULL GENERATED ALWAYS AS IDENTITY UNIQUE,
    company_id          uuid NOT NULL REFERENCES companies (id),
    created_by          uuid NOT NULL REFERENCES users (id),
    idempotency_key     text NOT NULL,
    status              text NOT NULL DEFAULT 'draft' CHECK (status IN (
                            'draft', 'submitted', 'checking', 'need_info', 'scoring',
                            'approved', 'counter_offer', 'manual_review', 'rejected',
                            'accepted', 'signing', 'issued', 'closed',
                            'cancelled_by_client', 'expired')),
    version             integer NOT NULL DEFAULT 1,

    product             text NOT NULL CHECK (product IN ('working_capital', 'overdraft', 'equipment')),
    purpose             text NOT NULL CHECK (purpose IN ('cash_gap', 'purchase', 'renovation', 'equipment', 'other')),
    amount_kop          bigint NOT NULL CHECK (amount_kop > 0 AND amount_kop % 1000000 = 0),
    term_months         integer NOT NULL CHECK (term_months BETWEEN 3 AND 60),
    monthly_revenue_kop bigint CHECK (monthly_revenue_kop > 0),
    equipment_cost_kop  bigint CHECK (equipment_cost_kop > 0),
    bki_consent_at      timestamptz,
    comment             text CHECK (length(comment) <= 1000),

    -- текущие условия: последнее решение политики или андеррайтера, история в decisions
    offer_product       text CHECK (offer_product IN ('working_capital', 'overdraft', 'equipment')),
    offer_amount_kop    bigint CHECK (offer_amount_kop > 0),
    offer_term_months   integer CHECK (offer_term_months BETWEEN 3 AND 60),
    offer_rate_bp       integer CHECK (offer_rate_bp > 0),
    offer_payment_kop   bigint CHECK (offer_payment_kop > 0),
    offer_guarantor     boolean NOT NULL DEFAULT false,
    offer_pledge        boolean NOT NULL DEFAULT false,
    offer_msp_guarantee boolean NOT NULL DEFAULT false,

    score               smallint CHECK (score BETWEEN 0 AND 100),
    grade               text CHECK (grade IN ('A', 'B', 'C', 'D', 'E')),
    scoring_version     text,
    policy_version      text,

    -- срок текущего шага: ответ клиента, SLA андеррайтера; ставит Transition
    due_at              timestamptz,
    submitted_at        timestamptz,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),

    UNIQUE (created_by, idempotency_key),
    CHECK (status = 'draft' OR submitted_at IS NOT NULL),
    CHECK (status = 'draft' OR bki_consent_at IS NOT NULL),
    CHECK (status NOT IN ('approved', 'counter_offer', 'accepted', 'signing', 'issued', 'closed')
           OR (offer_product IS NOT NULL AND offer_amount_kop IS NOT NULL
               AND offer_term_months IS NOT NULL AND offer_rate_bp IS NOT NULL))
);

-- очередь банка: фильтр по статусу, сортировка по времени
CREATE INDEX applications_queue_idx ON applications (status, created_at DESC);
CREATE INDEX applications_company_idx ON applications (company_id, created_at DESC);

CREATE TABLE application_events (
    id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    application_id uuid NOT NULL REFERENCES applications (id),
    kind           text NOT NULL DEFAULT 'transition' CHECK (kind IN ('transition', 'escalation', 'override')),
    from_status    text,
    to_status      text,
    actor_id       uuid REFERENCES users (id),
    actor_role     text NOT NULL CHECK (actor_role IN ('client', 'manager', 'underwriter', 'admin', 'system')),
    reason_codes   text[] NOT NULL DEFAULT '{}',
    comment        text,
    payload        jsonb NOT NULL DEFAULT '{}',
    created_at     timestamptz NOT NULL DEFAULT now(),
    CHECK (kind <> 'transition' OR to_status IS NOT NULL),
    CHECK ((actor_role = 'system') = (actor_id IS NULL))
);

CREATE INDEX application_events_app_idx ON application_events (application_id, id);

-- журнал только дописывается, правка задним числом должна падать
CREATE FUNCTION forbid_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION '% is append-only', TG_TABLE_NAME;
END
$$;

CREATE TRIGGER application_events_append_only
    BEFORE UPDATE OR DELETE ON application_events
    FOR EACH ROW EXECUTE FUNCTION forbid_change();

CREATE TABLE documents (
    id                uuid PRIMARY KEY DEFAULT uuidv7(),
    application_id    uuid NOT NULL REFERENCES applications (id),
    kind              text NOT NULL CHECK (kind IN (
                          'passport', 'power_of_attorney', 'statement',
                          'financials', 'supply_contract', 'tax_receipt')),
    status            text NOT NULL DEFAULT 'uploaded' CHECK (status IN (
                          'uploaded', 'recognizing', 'recognized', 'failed', 'skipped', 'confirmed')),
    file_name         text NOT NULL,
    content_type      text NOT NULL CHECK (content_type IN ('application/pdf', 'image/png', 'image/jpeg')),
    size_bytes        integer NOT NULL CHECK (size_bytes BETWEEN 1 AND 10485760),
    sha256            bytea NOT NULL CHECK (length(sha256) = 32),
    -- файлы до 10 МБ, отдельное хранилище ради них не нужно
    content           bytea NOT NULL,
    uploaded_by       uuid NOT NULL REFERENCES users (id),

    text_source       text CHECK (text_source IN ('pdf_text', 'ocr')),
    extracted_raw     jsonb,
    extracted         jsonb,
    validation_errors jsonb,
    error             text,
    model             text,
    model_digest      text,
    extractor_version text,
    duration_ms       integer,

    confirmed         jsonb,
    edited_fields     text[] NOT NULL DEFAULT '{}',
    confirmed_by      uuid REFERENCES users (id),
    confirmed_at      timestamptz,
    created_at        timestamptz NOT NULL DEFAULT now(),

    -- распознаём только выписку и отчётность, остальное просто файл
    CHECK (status = 'uploaded' OR kind IN ('statement', 'financials')),
    CHECK ((status = 'confirmed') = (confirmed IS NOT NULL AND confirmed_at IS NOT NULL))
);

CREATE INDEX documents_app_idx ON documents (application_id);

CREATE TABLE check_results (
    id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    application_id uuid NOT NULL REFERENCES applications (id),
    source         text NOT NULL CHECK (source IN ('egrul', 'fns', 'bki', 'internal')),
    status         text NOT NULL CHECK (status IN ('ok', 'not_found', 'unavailable')),
    payload        jsonb,
    error          text,
    created_at     timestamptz NOT NULL DEFAULT now(),
    CHECK (status <> 'ok' OR payload IS NOT NULL)
);

-- после need_info источники запрашиваются заново, берём последний по каждому
CREATE INDEX check_results_app_idx ON check_results (application_id, source, id DESC);

CREATE TABLE decisions (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    application_id  uuid NOT NULL REFERENCES applications (id),
    -- checks: стоп-факторы, policy: скоринг и политика, underwriter: ручное решение
    stage           text NOT NULL CHECK (stage IN ('checks', 'policy', 'underwriter')),
    outcome         text NOT NULL CHECK (outcome IN ('approved', 'counter_offer', 'manual_review', 'rejected', 'need_info')),
    reason_codes    text[] NOT NULL DEFAULT '{}',
    score           smallint CHECK (score BETWEEN 0 AND 100),
    grade           text CHECK (grade IN ('A', 'B', 'C', 'D', 'E')),
    score_breakdown jsonb,
    terms           jsonb,
    recommendation  jsonb,
    scoring_version text,
    policy_version  text,
    decided_by      uuid REFERENCES users (id),
    is_override     boolean NOT NULL DEFAULT false,
    comment         text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    CHECK ((stage = 'underwriter') = (decided_by IS NOT NULL)),
    CHECK (stage <> 'underwriter' OR (comment IS NOT NULL AND outcome IN ('approved', 'counter_offer', 'rejected'))),
    CHECK (stage <> 'policy' OR (score IS NOT NULL AND grade IS NOT NULL AND policy_version IS NOT NULL))
);

CREATE INDEX decisions_app_idx ON decisions (application_id, id DESC);
