-- order_asc: remove — strip redundant ASC from ORDER BY

-- 1. Explicit ASC should be removed
SELECT id, name FROM users ORDER BY name;

-- 2. Mixed: ASC removed, DESC kept
SELECT id, name, created_at FROM users ORDER BY name, created_at DESC;

-- 3. Multiple columns all ASC
SELECT id, score, rank FROM results ORDER BY score, rank, id;

-- 4. No explicit direction (should not change)
SELECT id FROM t ORDER BY id;

-- 5. DESC only (should not change)
SELECT id FROM t ORDER BY id DESC;
