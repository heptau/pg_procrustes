-- operator_spacing: compact — no spaces around symbolic operators

-- 1. Basic comparisons with extra spaces
SELECT id FROM users WHERE salary > 50000 AND age >= 18 AND name != 'bot';

-- 2. Arithmetic and concatenation
SELECT price * 1.1, first_name || ' ' || last_name AS full_name FROM employees;

-- 3. Assignment in UPDATE
UPDATE products SET price = price * 2 WHERE id = 42;

-- 4. Complex expression with mixed operators
SELECT a + b * c - d / e FROM t WHERE x <= y AND z <> 0;

-- 5. Spaces inside parens should not be affected
SELECT count(*) FROM t WHERE (a = b) OR (c > d);
