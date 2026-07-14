-- content.first_inline: first item on keyword line, rest indented below.
-- select_list and where_conds use first_inline; other clauses use default.

-- 1. SELECT with multiple columns, WHERE with multiple conditions
SELECT u.id,
   u.name,
   u.email,
   u.created_at
FROM users AS u
WHERE u.active = TRUE
   AND u.account_type = 'premium'
   AND u.created_at >= '2024-01-01'
ORDER BY u.name;

-- 2. Single SELECT column and single WHERE condition (no break needed, but first_inline should still work)
SELECT id
FROM users
WHERE active = TRUE;

-- 3. Subquery in SELECT list
SELECT u.id,
   u.name,
   (SELECT count(*) FROM orders AS o WHERE o.user_id = u.id) AS order_count
FROM users AS u
WHERE u.active = TRUE
   AND u.role = 'admin';

-- 4. AND/OR mix in WHERE
SELECT id,
   name
FROM products
WHERE category = 'electronics'
   AND price < 1000
   OR category = 'books'
   AND price < 50;

-- 5. No WHERE clause — only SELECT list breaks
SELECT id,
   name,
   email,
   phone,
   address,
   city
FROM contacts;
