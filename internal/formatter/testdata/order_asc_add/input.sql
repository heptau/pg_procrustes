-- order_asc: add — make ASC explicit in ORDER BY

-- 1. Simple ORDER BY without direction
SELECT id, name FROM users ORDER BY name;

-- 2. Mixed explicit and implicit
SELECT id, name, created_at FROM users ORDER BY name, created_at DESC;

-- 3. Multiple columns, none explicit
SELECT id, score, rank FROM results ORDER BY score, rank, id;

-- 4. Already explicit ASC (should not duplicate)
SELECT id FROM t ORDER BY id ASC;

-- 5. ORDER BY with expression
SELECT id FROM t ORDER BY length(name), id;
