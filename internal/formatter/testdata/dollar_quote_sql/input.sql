-- dollar_quote sql section — SQL body inside $$ is formatted too

-- 1. PL/pgSQL function with SQL body
CREATE OR REPLACE FUNCTION get_active_users()
RETURNS TABLE(id integer, name text) AS $$
SELECT id, name FROM users WHERE active = TRUE ORDER BY name;
$$ LANGUAGE sql;

-- 2. Inline SQL body with multiple statements
CREATE OR REPLACE FUNCTION reset_counters() RETURNS void AS $$
UPDATE stats SET count = 0 WHERE updated_at < now() - interval '1 day'; DELETE FROM temp_logs WHERE created_at < now() - interval '7 days';
$$ LANGUAGE sql;
