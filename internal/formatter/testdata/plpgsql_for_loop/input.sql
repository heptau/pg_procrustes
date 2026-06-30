-- FOR r IN SELECT...LOOP cursor query: clauses should be broken/indented,
-- LOOP should remain on its own line, and the loop body should be untouched.
CREATE OR REPLACE FUNCTION for_loop_test()
RETURNS void
LANGUAGE plpgsql
AS $$
BEGIN
    FOR r IN
        SELECT c.table_schema::text, c.table_name::text
        FROM information_schema.columns c
        WHERE
        c.column_name = 'target_col'
        AND c.table_schema != ALL (ARRAY['pg_catalog', 'information_schema'])
        AND (p_schema IS NULL OR c.table_schema = p_schema)
        ORDER BY
        c.table_schema,
        c.table_name
    LOOP
        v_sql := FORMAT('ALTER TABLE %I.%I RENAME COLUMN %I TO %I', r.table_schema, r.table_name, 'old', 'new');
        IF NOT p_dry_run THEN
            EXECUTE v_sql;
        END IF;
        RETURN NEXT;
    END LOOP;

    -- Regular SELECT statement (should also get clause-broken).
    SELECT id, name
    FROM users
    WHERE active = TRUE AND role = 'admin' AND created_at > '2020-01-01' AND updated_at IS NOT NULL;
END
$$;
