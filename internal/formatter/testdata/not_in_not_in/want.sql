-- not_in: not_in — convert <> ALL / != ALL to NOT IN

-- 1. Using <> ALL
SELECT id FROM users WHERE status NOT IN (ARRAY['banned', 'deleted']);

-- 2. Using != ALL
SELECT id FROM orders WHERE state NOT IN (ARRAY['cancelled', 'refunded']);

-- 3. Already NOT IN (should be left alone)
SELECT id FROM t WHERE role NOT IN ('admin', 'guest');
