-- cast_style: operator — CAST(x AS t) converted to x::t

-- 1. Simple type cast
SELECT CAST(price AS numeric) FROM products;

-- 2. Nested cast
SELECT CAST(CAST(raw AS text) AS integer) FROM t;

-- 3. Cast in WHERE
SELECT id FROM orders WHERE CAST(total AS bigint) > 1000;

-- 4. Cast in function argument
SELECT round(CAST(amount AS numeric), 2) FROM t;

-- 5. Multiple casts in one expression
SELECT CAST(a AS text) || ' ' || CAST(b AS text) FROM t;

-- 6. Already using :: (should be left alone)
SELECT price::numeric FROM products;
