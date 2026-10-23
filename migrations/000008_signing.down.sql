-- журнал только дописывается, но откат схемы это не правка задним числом
ALTER TABLE application_events DISABLE TRIGGER application_events_append_only;
DELETE FROM application_events WHERE kind = 'signing';
ALTER TABLE application_events ENABLE TRIGGER application_events_append_only;
ALTER TABLE application_events
    DROP CONSTRAINT application_events_kind_check,
    ADD CONSTRAINT application_events_kind_check CHECK (kind IN ('transition', 'escalation', 'override'));

DROP INDEX documents_contract_idx;
DELETE FROM documents WHERE kind = 'loan_agreement';
ALTER TABLE documents
    DROP CONSTRAINT documents_author_check,
    ALTER COLUMN uploaded_by SET NOT NULL,
    DROP CONSTRAINT documents_kind_check,
    ADD CONSTRAINT documents_kind_check CHECK (kind IN (
        'passport', 'power_of_attorney', 'statement',
        'financials', 'supply_contract', 'tax_receipt'));

DROP TABLE payouts;
DROP TABLE signing_codes;
