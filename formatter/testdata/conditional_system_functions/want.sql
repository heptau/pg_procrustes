-- conditional_functions and system_functions: lower case

-- 1. Conditional functions
SELECT coalesce(name, 'unknown') FROM users;
SELECT nullif(score, 0) FROM results;
SELECT greatest(a, b, c) FROM t;
SELECT least(x, y) FROM t;

-- 2. System functions / date-time constants
SELECT now();
SELECT current_date;
SELECT current_timestamp;
SELECT current_user;
SELECT session_user;

-- 3. Mixed in one query
SELECT coalesce(name, current_user) AS display_name, now() AS ts FROM users;
