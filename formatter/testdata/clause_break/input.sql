-- Layout: clauses.break=always — every SQL clause on its own line.
-- Tests FROM, JOIN, WHERE, GROUP BY, HAVING, ORDER BY, LIMIT, OFFSET.

SELECT u.id, u.name, u.email, COUNT(o.id) AS order_count, SUM(o.total) AS revenue FROM users u INNER JOIN orders o ON o.user_id = u.id LEFT JOIN addresses a ON a.user_id = u.id AND a.primary = TRUE WHERE u.active = TRUE AND u.created_at >= '2023-01-01' AND o.status IN ('paid', 'shipped') GROUP BY u.id, u.name, u.email HAVING COUNT(o.id) > 0 ORDER BY revenue DESC, u.name ASC LIMIT 50 OFFSET 100
