-- union_blank_line: both — blank lines before and after set operators

-- 1. UNION ALL (no existing blank lines)
SELECT id, name FROM active_users
UNION ALL
SELECT id, name FROM archived_users;

-- 2. INTERSECT
SELECT id FROM premium_users
INTERSECT
SELECT id FROM verified_users;

-- 3. EXCEPT
SELECT id FROM all_users
EXCEPT
SELECT id FROM banned_users;

-- 4. Chained UNION ALL
SELECT id FROM a
UNION ALL
SELECT id FROM b
UNION ALL
SELECT id FROM c;
