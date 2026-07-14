-- Paren indentation: content inside multi-line paren blocks is indented
-- by the number of unclosed opening parentheses on the preceding line.

-- Subquery in FROM.
SELECT s.id, s.total
FROM (
SELECT id, SUM(unit_price * qty) AS total
FROM orders
WHERE status = 'paid'
GROUP BY id
) s
WHERE s.total > 100;

-- JOIN with subquery.
SELECT u.name, sub.order_count
FROM users u
JOIN (
SELECT user_id, COUNT(*) AS order_count
FROM orders
WHERE created_at > NOW() - INTERVAL '30 days'
GROUP BY user_id
HAVING COUNT(*) > 5
) sub ON sub.user_id = u.id;

-- IN with subquery.
DELETE FROM sessions
WHERE user_id IN (
SELECT id FROM users WHERE is_banned = TRUE
);

-- CREATE TABLE with column list.
CREATE TABLE account_balances (
id          bigint       NOT NULL,
account_id  bigint       NOT NULL,
currency    varchar(3)   NOT NULL,
amount      numeric      NOT NULL DEFAULT 0,
updated_at  timestamptz  NOT NULL DEFAULT NOW(),
CONSTRAINT pk_account_balances PRIMARY KEY (id),
CONSTRAINT fk_account FOREIGN KEY (account_id) REFERENCES accounts (id)
);

-- PL/pgSQL function: dollar-quoted body must not be re-indented.
CREATE OR REPLACE FUNCTION add_nums(a integer, b integer)
RETURNS integer
LANGUAGE plpgsql AS $$
BEGIN
RETURN a + b;
END;
$$;
