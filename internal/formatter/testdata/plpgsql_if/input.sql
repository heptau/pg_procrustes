-- PL/pgSQL: IF/ELSIF/ELSE with body_indent=indent and blank lines around blocks.

CREATE OR REPLACE FUNCTION classify(val integer, label text DEFAULT 'x') RETURNS text
LANGUAGE plpgsql AS $$
DECLARE
  result text;
BEGIN
IF val > 100 THEN
result := 'large';
ELSIF val > 10 THEN
result := 'medium';
ELSIF val > 0 THEN
result := 'small';
ELSE
result := 'non-positive';
END IF;
RETURN result;
END
$$
