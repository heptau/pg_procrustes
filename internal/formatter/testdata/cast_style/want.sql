-- cast_style: operator — CAST(x AS t) converted to x::t

-- 1. Simple type cast
SELECT price::numeric FROM products;

-- 2. Nested cast
SELECT raw::text::integer FROM t;

-- 3. Cast in WHERE
SELECT id FROM orders WHERE total::bigint > 1000;

-- 4. Cast in function argument
SELECT round(amount::numeric, 2) FROM t;

-- 5. Multiple casts in one expression
SELECT a::text || ' ' || b::text FROM t;

-- 6. Already using :: (should be left alone)
SELECT price::numeric FROM products;
