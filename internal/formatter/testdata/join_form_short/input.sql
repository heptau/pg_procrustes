-- join_form: short — remove INNER / OUTER qualifiers

-- 1. INNER JOIN → JOIN
SELECT u.id, o.total FROM users u INNER JOIN orders o ON o.user_id = u.id;

-- 2. LEFT OUTER JOIN → LEFT JOIN
SELECT u.id, o.total FROM users u LEFT OUTER JOIN orders o ON o.user_id = u.id;

-- 3. RIGHT OUTER JOIN → RIGHT JOIN
SELECT u.id, o.total FROM users u RIGHT OUTER JOIN orders o ON o.user_id = u.id;

-- 4. FULL OUTER JOIN → FULL JOIN
SELECT u.id, o.total FROM users u FULL OUTER JOIN orders o ON o.user_id = u.id;

-- 5. Already short form (should not change)
SELECT u.id, o.total FROM users u LEFT JOIN orders o ON o.user_id = u.id;
