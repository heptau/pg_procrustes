CREATE OR REPLACE FUNCTION g(IN sql text, INOUT name text DEFAULT 'a')
RETURNS text AS $BODY$
DECLARE
  type text := 'x';
  key integer;
  data record;
BEGIN
  FOR data IN SELECT * FROM t WHERE t.type = type LOOP
    key = key + 1;
  END LOOP;
  FOREACH key IN ARRAY ARRAY[1, 2] LOOP
    name = name || key::text;
  END LOOP;
  SELECT XMLELEMENT(NAME foo) INTO type;
  RETURN type || sql || name;
END
$BODY$ LANGUAGE plpgsql;
CREATE FUNCTION h(sql text) RETURNS text LANGUAGE plpgsql AS $$
BEGIN
	RETURN LEFT(sql, 1) || FORMAT('%s', sql) || REPLACE(sql, 'a', 'b');
END
$$;
