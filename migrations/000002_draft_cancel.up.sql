-- черновик можно отменить, не подав и не дав согласия на бки
ALTER TABLE applications
    DROP CONSTRAINT applications_check,
    DROP CONSTRAINT applications_check1,
    ADD CONSTRAINT applications_submitted_check
        CHECK (status IN ('draft', 'cancelled_by_client') OR submitted_at IS NOT NULL),
    ADD CONSTRAINT applications_bki_consent_check
        CHECK (status IN ('draft', 'cancelled_by_client') OR bki_consent_at IS NOT NULL);
