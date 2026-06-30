-- plpgsql_keywords.case: upper
-- Covers: RAISE, PERFORM, ELSIF, ELSEIF, FOREACH, REVERSE, SLICE,
--         EXIT, LOOP, WHILE, OPEN, ASSERT, DEBUG, INFO, NOTICE, WARNING, EXCEPTION.

-- 1. RAISE severity levels
CREATE OR REPLACE FUNCTION test_raise() RETURNS void AS $$
BEGIN
  raise debug 'debug message';
  raise info 'info message';
  raise notice 'notice message';
  raise warning 'warning message';
  raise exception 'error: %', 'something went wrong';
END;
$$ LANGUAGE plpgsql;

-- 2. PERFORM, ELSIF, LOOP, EXIT
CREATE OR REPLACE FUNCTION test_control(n integer) RETURNS void AS $$
DECLARE
  i integer := 0;
BEGIN
  perform pg_sleep(0);
  loop
    exit when i >= n;
    if i = 0 then
      raise notice 'first iteration';
    elsif i = 1 then
      raise notice 'second iteration';
    else
      raise notice 'iteration %', i;
    end if;
    i := i + 1;
  end loop;
END;
$$ LANGUAGE plpgsql;

-- 3. WHILE loop
CREATE OR REPLACE FUNCTION test_while(n integer) RETURNS integer AS $$
DECLARE
  total integer := 0;
  i integer := 1;
BEGIN
  while i <= n loop
    total := total + i;
    i := i + 1;
  end loop;
  RETURN total;
END;
$$ LANGUAGE plpgsql;

-- 4. FOR LOOP with REVERSE and FOREACH with SLICE
CREATE OR REPLACE FUNCTION test_loops() RETURNS void AS $$
DECLARE
  arr integer[] := ARRAY[1,2,3,4,5];
  sub integer[];
BEGIN
  for i in reverse 10..1 loop
    raise notice 'i=%', i;
  end loop;
  foreach sub slice 1 IN ARRAY arr loop
    raise notice 'sub=%', sub;
  end loop;
END;
$$ LANGUAGE plpgsql;

-- 5. OPEN cursor, ASSERT
CREATE OR REPLACE FUNCTION test_cursor() RETURNS void AS $$
DECLARE
  cur refcursor;
  rec record;
BEGIN
  open cur FOR SELECT id FROM users ORDER BY id;
  FETCH cur INTO rec;
  assert rec.id IS NOT NULL, 'expected a row';
  CLOSE cur;
END;
$$ LANGUAGE plpgsql;

-- 6. EXCEPTION handler with RAISE EXCEPTION
CREATE OR REPLACE FUNCTION test_exception() RETURNS void AS $$
BEGIN
  INSERT INTO t(v) VALUES ('x');
exception
  WHEN unique_violation THEN
    raise exception 'duplicate value';
  WHEN others THEN
    raise warning 'unexpected error';
END;
$$ LANGUAGE plpgsql;
