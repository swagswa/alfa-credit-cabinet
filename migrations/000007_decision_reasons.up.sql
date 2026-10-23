-- тексты объяснений собираются по шаблонам с числами заявки, из одних кодов их потом
-- не восстановить, поэтому храним как показали: [{code, text}]
ALTER TABLE decisions ADD COLUMN reasons jsonb NOT NULL DEFAULT '[]';
