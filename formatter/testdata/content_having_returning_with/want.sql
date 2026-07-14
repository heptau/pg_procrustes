-- having_conds: first_inline, returning_list: first_inline, with_list: always

-- 1. HAVING with multiple AND conditions
SELECT department_id, count(*) AS cnt, avg(salary) AS avg_sal
FROM employees
GROUP BY department_id
HAVING count(*) > 5
   AND avg(salary) > 50000
   AND max(salary) < 200000;

-- 2. HAVING with OR
SELECT category, sum(amount)
FROM orders
GROUP BY category
HAVING sum(amount) > 10000
   OR count(*) > 100;

-- 3. RETURNING with multiple columns (INSERT)
INSERT INTO users(name, email, role)
VALUES ('Alice', 'alice@example.com', 'admin')
RETURNING id,
   name,
   created_at,
   updated_at;

-- 4. RETURNING with multiple columns (UPDATE)
UPDATE products
SET price = price * 1.1, updated_at = now()
WHERE category = 'electronics'
RETURNING id,
   name,
   price,
   updated_at;

-- 5. RETURNING single column (no break needed)
DELETE FROM sessions
WHERE expires_at < now()
RETURNING id;

-- 6. WITH multiple CTEs
WITH
   active_users AS (SELECT id, name FROM users WHERE active = TRUE),
   recent_orders AS (SELECT user_id, count(*) AS cnt FROM orders WHERE created_at > now() - interval '30 days' GROUP BY user_id),
   top_buyers AS (SELECT u.name, ro.cnt FROM active_users AS u JOIN recent_orders AS ro ON ro.user_id = u.id WHERE ro.cnt > 5)
SELECT *
FROM top_buyers
ORDER BY cnt DESC;

-- 7. WITH single CTE (no extra break)
WITH
   summary AS (SELECT department_id, avg(salary) AS avg_sal FROM employees GROUP BY department_id)
SELECT *
FROM summary
WHERE avg_sal > 60000;
