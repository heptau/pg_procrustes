CREATE FUNCTION pg_temp.assert(a int) RETURNS void LANGUAGE plpgsql AS $$
DECLARE
    info record;
BEGIN
    ASSERT a = 1;
    FOR info IN SELECT * FROM t LOOP
        RAISE info 'x %', info.a;
        RAISE notice 'y';
    END LOOP;
END;
$$;
DROP FUNCTION IF EXISTS pg_temp.assert(int);
SELECT a
	FROM	t
	INNER JOIN	u	AS y	ON y.id = t.id;
