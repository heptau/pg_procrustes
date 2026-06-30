-- join_form: long — add INNER / OUTER qualifiers

-- 1. JOIN → INNER JOIN
SELECT u.id, o.total FROM users AS u INNER JOIN orders AS o ON o.user_id = u.id;

-- 2. LEFT JOIN → LEFT OUTER JOIN
SELECT u.id, o.total FROM users AS u LEFT OUTER JOIN orders AS o ON o.user_id = u.id;

-- 3. RIGHT JOIN → RIGHT OUTER JOIN
SELECT u.id, o.total FROM users AS u RIGHT OUTER JOIN orders AS o ON o.user_id = u.id;

-- 4. FULL JOIN → FULL OUTER JOIN
SELECT u.id, o.total FROM users AS u FULL OUTER JOIN orders AS o ON o.user_id = u.id;

-- 5. CROSS JOIN (no qualifier added)
SELECT u.id, o.total FROM users AS u CROSS JOIN orders AS o;

-- 6. Already long form (should not change)
SELECT u.id, o.total FROM users AS u INNER JOIN orders AS o ON o.user_id = u.id;
