SELECT xmlforest(a AS costs, b AS number, c AS "date", d::text AS type) FROM t;
SELECT xmlelement(name x, xmlattributes(a AS name)) FROM t;
CREATE FUNCTION f() RETURNS text LANGUAGE plpgsql AS $$
BEGIN
	RETURN xmlforest(_r.costs AS costs, coalesce(_r.n, 0) AS number)::text;
END
$$;
