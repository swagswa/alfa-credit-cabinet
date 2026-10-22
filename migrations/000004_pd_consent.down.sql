DROP VIEW application_cards;

CREATE VIEW application_cards AS
SELECT a.id, a.number, a.company_id, a.status, a.version,
       a.product, a.purpose, a.amount_kop, a.term_months, a.monthly_revenue_kop, a.equipment_cost_kop,
       (a.bki_consent_at IS NOT NULL)::boolean AS bki_consent, a.comment,
       a.score, a.grade, a.due_at, a.submitted_at, a.created_at, a.updated_at,
       c.inn, c.short_name, c.kind, c.simulated, m.representative_role
FROM applications a
JOIN companies c ON c.id = a.company_id
JOIN company_members m ON m.company_id = a.company_id AND m.user_id = a.created_by;

ALTER TABLE applications DROP COLUMN pd_consent_at;
