-- согласие на ПДн с сентября 2025 отдельное (152-ФЗ), момент храним, как у БКИ.
-- NOT VALID: старые поданные заявки на стендах без него, проверяем только новые записи
ALTER TABLE applications
    ADD COLUMN pd_consent_at timestamptz,
    ADD CONSTRAINT applications_pd_consent_check
        CHECK (status IN ('draft', 'cancelled_by_client') OR pd_consent_at IS NOT NULL) NOT VALID;

CREATE OR REPLACE VIEW application_cards AS
SELECT a.id, a.number, a.company_id, a.status, a.version,
       a.product, a.purpose, a.amount_kop, a.term_months, a.monthly_revenue_kop, a.equipment_cost_kop,
       (a.bki_consent_at IS NOT NULL)::boolean AS bki_consent, a.comment,
       a.score, a.grade, a.due_at, a.submitted_at, a.created_at, a.updated_at,
       c.inn, c.short_name, c.kind, c.simulated, m.representative_role,
       (a.pd_consent_at IS NOT NULL)::boolean AS pd_consent
FROM applications a
JOIN companies c ON c.id = a.company_id
JOIN company_members m ON m.company_id = a.company_id AND m.user_id = a.created_by;
