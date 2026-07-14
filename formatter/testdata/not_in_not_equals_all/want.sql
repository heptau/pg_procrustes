-- not_in: not_equals_all — convert NOT IN to <> ALL

-- 1. Simple NOT IN
SELECT id FROM users WHERE status <> ALL ('banned', 'deleted');

-- 2. NOT IN with subquery
SELECT id FROM orders WHERE user_id <> ALL (SELECT id FROM banned_users);

-- 3. NOT IN in JOIN condition
SELECT u.id FROM users AS u WHERE u.role <> ALL ('admin', 'moderator');

-- 4. Already <> ALL (should be left alone)
SELECT id FROM t WHERE status <> ALL (ARRAY['banned', 'deleted']);
