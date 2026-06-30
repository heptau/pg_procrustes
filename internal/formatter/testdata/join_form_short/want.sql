-- join_form: short — remove INNER / OUTER qualifiers

-- 1. INNER JOIN → JOIN
SELECT u.id, o.total FROM users AS u JOIN orders AS o ON o.user_id = u.id;

-- 2. LEFT OUTER JOIN → LEFT JOIN
SELECT u.id, o.total FROM users AS u LEFT JOIN orders AS o ON o.user_id = u.id;

-- 3. RIGHT OUTER JOIN → RIGHT JOIN
SELECT u.id, o.total FROM users AS u RIGHT JOIN orders AS o ON o.user_id = u.id;

-- 4. FULL OUTER JOIN → FULL JOIN
SELECT u.id, o.total FROM users AS u FULL JOIN orders AS o ON o.user_id = u.id;

-- 5. Already short form (should not change)
SELECT u.id, o.total FROM users AS u LEFT JOIN orders AS o ON o.user_id = u.id;
