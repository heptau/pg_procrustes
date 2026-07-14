-- plpgsql_variables.case: upper
-- Covers: NEW, OLD, EXCLUDED, FOUND, TG_* trigger variables,
--         SQLSTATE, SQLERRM, ROW_COUNT, GET DIAGNOSTICS items.

-- 1. Row-level trigger: NEW, OLD, TG_OP, TG_TABLE_NAME
CREATE OR replace function trg_audit() returns trigger AS $$
begin
  if TG_OP = 'INSERT' THEN
    insert INTO audit(tbl, op, new_id) values (TG_TABLE_NAME, TG_OP, NEW.id);
  ELSIF TG_OP = 'UPDATE' THEN
    insert INTO audit(tbl, op, old_id, new_id) values (TG_TABLE_NAME, TG_OP, OLD.id, NEW.id);
  ELSIF TG_OP = 'DELETE' THEN
    insert INTO audit(tbl, op, old_id) values (TG_TABLE_NAME, TG_OP, OLD.id);
  END if;
  return NEW;
END;
$$ language plpgsql;

-- 2. FOUND and ROW_COUNT via GET DIAGNOSTICS
CREATE OR replace function check_found(p_id integer) returns boolean AS $$
declare
  n integer;
begin
  SELECT 1 INTO n FROM users WHERE id = p_id;
  if NOT FOUND THEN
    return FALSE;
  END if;
  GET DIAGNOSTICS n = ROW_COUNT;
  return n > 0;
END;
$$ language plpgsql;

-- 3. Exception handling: SQLSTATE, SQLERRM
CREATE OR replace function safe_insert(p_val text) returns void AS $$
begin
  insert INTO t(v) values (p_val);
EXCEPTION
  WHEN others THEN
    RAISE WARNING 'insert failed: % (state=%)', SQLERRM, SQLSTATE;
END;
$$ language plpgsql;

-- 4. ON CONFLICT EXCLUDED (regular SQL, not PL/pgSQL)
insert INTO products(id, price)
values (1, 99.9)
ON conflict (id) DO update
  set price = EXCLUDED.price
  WHERE EXCLUDED.price < products.price;

-- 5. More TG_* variables: TG_NAME, TG_WHEN, TG_LEVEL, TG_NARGS, TG_ARGV, TG_SCHEMA
CREATE OR replace function trg_info() returns trigger AS $$
begin
  RAISE NOTICE 'trigger=% when=% level=% nargs=% schema=%',
    TG_NAME, TG_WHEN, TG_LEVEL, TG_NARGS, TG_TABLE_SCHEMA;
  return NEW;
END;
$$ language plpgsql;
