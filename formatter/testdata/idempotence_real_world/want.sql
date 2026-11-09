SELECT hmac('Hi There', '\x0b0b'::bytea, 'md5');
SELECT a
FROM t
LEFT JOIN (SELECT join_key, bug_field FROM (SELECT ss1.join_key, ss1.bug_field FROM (SELECT f1 AS join_key, 666 AS bug_field FROM int4_tbl AS i1) AS ss1) AS foo2 LEFT JOIN (SELECT unique2 AS join_key FROM tenk1 AS i2) AS ss2 USING (join_key)) AS foo3 USING (join_key);
CREATE FUNCTION f() RETURNS void LANGUAGE plpgsql AS $$
BEGIN
	INSERT INTO t
	SELECT r, 0
	FROM (SELECT unnest(ARRAY[1, 3, 4, 13, 23, 31, 32, 39, 48, 58, 63, 67, 68, 69, 73, 80, 83, 87, 88, 89, 94, 105, 117, 121]) AS r) AS t
	WHERE r NOT IN (SELECT f.row_no FROM x AS f);
END $$;
SELECT a, sum(b) AS revenues, sum(c) AS costs
FROM t;
CREATE FUNCTION g(_d date) RETURNS real LANGUAGE plpgsql AS $$
DECLARE _rate real;
BEGIN
  _rate := (SELECT rate FROM r WHERE "date" = _d AND "text" = 'x');
  RETURN _rate;
END;
$$;
CREATE OPERATOR CLASS c FOR TYPE g USING gist AS
    OPERATOR 13 <-> FOR ORDER BY pg_catalog.float_ops,
    OPERATOR 14 <#> FOR ORDER BY pg_catalog.float_ops;
