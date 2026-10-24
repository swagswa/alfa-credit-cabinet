-- что банк ждёт от клиента в need_info. Открытый запрос на заявку один,
-- ответ клиента или выход из need_info по сроку закрывает его
CREATE TABLE info_requests (
    id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    application_id uuid NOT NULL REFERENCES applications (id),
    documents      text[] NOT NULL DEFAULT '{}'
        CHECK (documents <@ '{passport,power_of_attorney,statement,financials,supply_contract,tax_receipt}'),
    -- как показали клиенту: [{code, text}], как в decisions
    reasons        jsonb NOT NULL DEFAULT '[]',
    comment        text,
    -- NULL, если запросила система по стоп-фактору
    requested_by   uuid REFERENCES users (id),
    created_at     timestamptz NOT NULL DEFAULT now(),
    closed_at      timestamptz
);

CREATE UNIQUE INDEX info_requests_open_idx ON info_requests (application_id) WHERE closed_at IS NULL;
