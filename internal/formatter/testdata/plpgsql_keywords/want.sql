-- plpgsql_keywords.case: upper
-- Covers: RAISE, PERFORM, ELSIF, ELSEIF, FOREACH, REVERSE, SLICE,
--         EXIT, LOOP, WHILE, OPEN, ASSERT, DEBUG, INFO, NOTICE, WARNING, EXCEPTION.

-- 1. RAISE severity levels
CREATE OR replace function test_raise() returns void AS $$
begin
  RAISE DEBUG 'debug message';
  RAISE INFO 'info message';
  RAISE NOTICE 'notice message';
  RAISE WARNING 'warning message';
  RAISE EXCEPTION 'error: %', 'something went wrong';
END;
$$ language plpgsql;

-- 2. PERFORM, ELSIF, LOOP, EXIT
CREATE OR replace function test_control(n integer) returns void AS $$
declare
  i integer := 0;
begin
  PERFORM pg_sleep(0);
  LOOP
    EXIT WHEN i >= n;
    if i = 0 THEN
      RAISE NOTICE 'first iteration';
    ELSIF i = 1 THEN
      RAISE NOTICE 'second iteration';
    ELSE
      RAISE NOTICE 'iteration %', i;
    END if;
    i := i + 1;
  END LOOP;
END;
$$ language plpgsql;

-- 3. WHILE loop
CREATE OR replace function test_while(n integer) returns integer AS $$
declare
  total integer := 0;
  i integer := 1;
begin
  WHILE i <= n LOOP
    total := total + i;
    i := i + 1;
  END LOOP;
  return total;
END;
$$ language plpgsql;

-- 4. FOR LOOP with REVERSE and FOREACH with SLICE
CREATE OR replace function test_loops() returns void AS $$
declare
  arr integer[] := ARRAY[1, 2, 3, 4, 5];
  sub integer[];
begin
  FOR i IN REVERSE 10..1 LOOP
    RAISE NOTICE 'i=%', i;
  END LOOP;
  FOREACH sub SLICE 1 IN ARRAY arr LOOP
    RAISE NOTICE 'sub=%', sub;
  END LOOP;
END;
$$ language plpgsql;

-- 5. OPEN cursor, ASSERT
CREATE OR replace function test_cursor() returns void AS $$
declare
  cur refcursor;
  rec record;
begin
  OPEN cur FOR SELECT id FROM users ORDER by id;
  FETCH cur INTO rec;
  ASSERT rec.id IS NOT NULL, 'expected a row';
  close cur;
END;
$$ language plpgsql;

-- 6. EXCEPTION handler with RAISE EXCEPTION
CREATE OR replace function test_exception() returns void AS $$
begin
  insert INTO t(v) values ('x');
EXCEPTION
  WHEN unique_violation THEN
    RAISE EXCEPTION 'duplicate value';
  WHEN others THEN
    RAISE WARNING 'unexpected error';
END;
$$ language plpgsql;
