-- Согласие на запрос в БКИ отдельной записью: бюро нужен его id и дата получения.
-- Ставится при подаче, дата из анкеты (bki_consent_at). Срок, полгода, считает
-- domain.BKIConsent, в базе его не дублируем.
CREATE TABLE bki_consents (
    id             uuid PRIMARY KEY DEFAULT uuidv7(),
    application_id uuid NOT NULL UNIQUE REFERENCES applications (id),
    given_at       timestamptz NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now()
);

INSERT INTO bki_consents (application_id, given_at)
SELECT id, bki_consent_at
FROM applications
WHERE submitted_at IS NOT NULL;
