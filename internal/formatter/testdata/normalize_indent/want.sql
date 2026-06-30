-- normalize_indent: convert 3-space indentation to tabs at all levels.
-- Level 1 = 3 spaces → 1 tab, level 2 = 6 spaces → 2 tabs, etc.

-- Plain SQL with subqueries (multiple levels).
SELECT *
FROM (
	SELECT id, name
	FROM (
		SELECT uid AS id, full_name AS name
		FROM users
		WHERE active = TRUE
	) inner_q
	WHERE name LIKE 'A%'
) outer_q;

-- PL/pgSQL function body with multi-line SQL statements inside.
CREATE OR REPLACE FUNCTION normalize_test()
RETURNS void
LANGUAGE plpgsql
AS $$
DECLARE
	v integer;
	w text;
BEGIN
	SELECT id,
		name
	FROM users
	WHERE active = TRUE
		AND role = 'admin';

	UPDATE accounts
	SET balance = balance - 100,
		updated_at = now()
	WHERE id = 42;

	IF v > 0 THEN
		RAISE NOTICE 'positive';
	END IF;
END
$$;
