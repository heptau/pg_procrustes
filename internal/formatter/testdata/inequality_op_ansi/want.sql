-- inequality_op: ansi — convert != to <>

-- 1. Simple inequality
SELECT id FROM users WHERE status <> 'active';

-- 2. Multiple != in one query
SELECT id FROM t WHERE a <> 1 AND b <> 2;

-- 3. Already using <> (should be left alone)
SELECT id FROM t WHERE status <> 'active';
