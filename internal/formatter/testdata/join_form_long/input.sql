-- join_form: long — add INNER / OUTER qualifiers

-- 1. JOIN → INNER JOIN
SELECT u.id, o.total FROM users u JOIN orders o ON o.user_id = u.id;

-- 2. LEFT JOIN → LEFT OUTER JOIN
SELECT u.id, o.total FROM users u LEFT JOIN orders o ON o.user_id = u.id;

-- 3. RIGHT JOIN → RIGHT OUTER JOIN
SELECT u.id, o.total FROM users u RIGHT JOIN orders o ON o.user_id = u.id;

-- 4. FULL JOIN → FULL OUTER JOIN
SELECT u.id, o.total FROM users u FULL JOIN orders o ON o.user_id = u.id;

-- 5. CROSS JOIN (no qualifier added)
SELECT u.id, o.total FROM users u CROSS JOIN orders o;

-- 6. Already long form (should not change)
SELECT u.id, o.total FROM users u INNER JOIN orders o ON o.user_id = u.id;
