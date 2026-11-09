CREATE FUNCTION pg_temp.assert(a integer) RETURNS void LANGUAGE plpgsql AS $$
DECLARE
    info record;
BEGIN
    ASSERT a = 1;
    FOR info IN SELECT * FROM t LOOP
        RAISE INFO 'x %', info.a;
        RAISE NOTICE 'y';
    END LOOP;
END;
$$;
DROP FUNCTION IF EXISTS pg_temp.assert(integer);
SELECT a
	FROM t
	INNER JOIN u AS y ON y.id = t.id;
