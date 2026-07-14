-- insert_columns: first_inline — first column on same line as (, rest indented

-- 1. Multi-column INSERT with VALUES
INSERT INTO users (id,
   name,
   email,
   role,
   created_at
)
VALUES
   (1, 'Alice', 'alice@example.com', 'admin', now());

-- 2. Multi-row INSERT
INSERT INTO orders (user_id,
   product_id,
   quantity,
   price,
   created_at
)
VALUES
   (1, 10, 2, 29.99, now()),
   (2, 11, 1, 49.99, now()),
   (3, 12, 3, 9.99, now());

-- 3. Single-column INSERT (no break for column list)
INSERT INTO log (message)
VALUES
   ('started');

-- 4. INSERT with schema-qualified table
INSERT INTO public.audit_log (entity,
   action,
   performed_by,
   performed_at
)
VALUES
   ('user', 'create', 42, now());

-- 5. INSERT ... SELECT (no column list paren to split)
INSERT INTO archived_orders
SELECT id, user_id, total
FROM orders
WHERE created_at < '2023-01-01';

-- 6. INSERT with ON CONFLICT
INSERT INTO products (id,
   name,
   price,
   stock
)
VALUES
   (1, 'Widget', 9.99, 100)
ON CONFLICT (id) DO
UPDATE
SET price = excluded.price, stock = excluded.stock;
