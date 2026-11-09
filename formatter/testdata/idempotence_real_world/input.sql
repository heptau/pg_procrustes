SELECT hmac(
'Hi There',
'\x0b0b'::bytea,
'md5');
select a from t
left join
  (select join_key, bug_field from
    (select ss1.join_key, ss1.bug_field from
      (select f1 as join_key, 666 as bug_field from int4_tbl i1) ss1
    ) foo2
   left join
    (select unique2 as join_key from tenk1 i2) ss2
   using (join_key)
  ) foo3
using (join_key);
CREATE FUNCTION f() RETURNS void LANGUAGE plpgsql AS $$
BEGIN
	INSERT INTO t
	SELECT r, 0
	FROM (
		SELECT unnest(ARRAY[1, 3, 4, 13, 23, 31, 32, 39, 48, 58, 63, 67, 68, 69, 73, 80, 83, 87, 88, 89, 94, 105, 117, 121]) r
   ) AS t
	WHERE r NOT IN (SELECT f.row_no FROM x f);
END $$;
SELECT a,
       sum(b)      revenues,
       sum(c) costs
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
