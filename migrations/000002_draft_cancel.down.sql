ALTER TABLE applications
    DROP CONSTRAINT applications_submitted_check,
    DROP CONSTRAINT applications_bki_consent_check,
    ADD CONSTRAINT applications_check CHECK (status = 'draft' OR submitted_at IS NOT NULL),
    ADD CONSTRAINT applications_check1 CHECK (status = 'draft' OR bki_consent_at IS NOT NULL);
