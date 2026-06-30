-- comma_spacing: compact — no spaces around commas

-- 1. SELECT list
SELECT id,name,email,created_at FROM users;

-- 2. Function call arguments
SELECT COALESCE(name,'unknown','n/a') FROM t;

-- 3. INSERT values
INSERT INTO orders (user_id,product_id,quantity) VALUES (1,10,3);

-- 4. Multi-row INSERT
INSERT INTO t (a,b) VALUES (1,2),(3,4),(5,6);

-- 5. Spaces before comma (should also be removed)
SELECT a,b,c FROM t;
