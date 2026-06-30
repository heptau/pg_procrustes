-- conditional_functions and system_functions: lower case

-- 1. Conditional functions
SELECT COALESCE(name, 'unknown') FROM users;
SELECT NULLIF(score, 0) FROM results;
SELECT GREATEST(a, b, c) FROM t;
SELECT LEAST(x, y) FROM t;

-- 2. System functions / date-time constants
SELECT NOW();
SELECT CURRENT_DATE;
SELECT CURRENT_TIMESTAMP;
SELECT CURRENT_USER;
SELECT SESSION_USER;

-- 3. Mixed in one query
SELECT COALESCE(name, CURRENT_USER) AS display_name, NOW() AS ts FROM users;
