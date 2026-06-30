-- paren_indent: close_first_on_line: after — closing ) at inner indent level

-- 1. Subquery in FROM
SELECT s.id, s.total
FROM (
SELECT id, SUM(unit_price * qty) AS total
FROM orders
WHERE status = 'paid'
GROUP BY id
) s
WHERE s.total > 100;

-- 2. IN with subquery
DELETE FROM sessions
WHERE user_id IN (
SELECT id FROM users WHERE is_banned = TRUE
);
