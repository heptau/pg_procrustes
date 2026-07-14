-- FOR r IN SELECT...LOOP: double semicolons before LOOP, implicit table aliases,
-- multi-line SELECT list with CASE expression (case.break=preserve), WHERE indentation.
CREATE OR REPLACE FUNCTION complex_for_loop_test(p_schema text DEFAULT NULL)
RETURNS void
LANGUAGE plpgsql
AS $$
DECLARE
	r record;
	c_system_schemas CONSTANT text[] := ARRAY['pg_catalog', 'information_schema'];
BEGIN

	-- First loop: double semicolon before LOOP (bug in source), implicit alias.
	FOR r IN
		SELECT c.table_schema::text, c.table_name::text
		FROM information_schema.columns AS c
		WHERE
			c.column_name = 'target_col'
			AND c.table_schema != ALL (c_system_schemas)
			AND (p_schema IS NULL OR c.table_schema = p_schema)
		ORDER BY c.table_schema, c.table_name
	LOOP
		RAISE NOTICE '%', r.table_name;
	END LOOP;

	-- Second loop: CASE expression in SELECT list, multiple implicit aliases.
	FOR r IN
		SELECT
			n.nspname::text AS func_schema,
			p.proname::text AS func_name,
			p.oid,
			CASE p.prokind
			WHEN 'f' THEN 'FUNCTION'
			WHEN 'p' THEN 'PROCEDURE'
			END AS kind
		FROM pg_proc AS p
		JOIN pg_namespace AS n ON n.oid = p.pronamespace
		WHERE
			n.nspname != ALL (c_system_schemas)
			AND p.prokind IN ('f', 'p')
			AND (p_schema IS NULL OR n.nspname = p_schema)
		ORDER BY n.nspname, p.proname
	LOOP
		RAISE NOTICE '%', r.func_name;
	END LOOP;

END;
$$;
