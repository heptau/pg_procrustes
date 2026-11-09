SELECT XMLFOREST(a AS costs, b AS number, c AS "date", d::text AS type) FROM t;
SELECT XMLELEMENT(NAME x, XMLATTRIBUTES(a AS name)) FROM t;
CREATE FUNCTION f() RETURNS text LANGUAGE plpgsql AS $$
BEGIN
	RETURN XMLFOREST(_r.costs AS costs, COALESCE(_r.n, 0) AS number)::text;
END
$$;
