-- casing_exceptions — pinned identifiers override section case

-- reserved_keywords: upper with exceptions [select, from]
-- Pinned keywords stay as-is; others are uppercased.
select id, name from users where active = TRUE;

-- columns: lower with exceptions [ID, CreatedAt]
-- Pinned columns keep their exact case; others are lowercased.
SELECT Id, Name, createdat FROM users;

-- functions: lower with exceptions [MyFunc]
SELECT myfunc(id), count(*) FROM t GROUP BY myfunc(id);
