-- plpgsql_variables.case: upper
-- Covers: NEW, OLD, EXCLUDED, FOUND, TG_* trigger variables,
--         SQLSTATE, SQLERRM, ROW_COUNT, GET DIAGNOSTICS items.

-- 1. Row-level trigger: NEW, OLD, TG_OP, TG_TABLE_NAME
CREATE OR REPLACE FUNCTION trg_audit() RETURNS trigger AS $$
BEGIN
  IF tg_op = 'INSERT' THEN
    INSERT INTO audit(tbl, op, new_id) VALUES (tg_table_name, tg_op, new.id);
  ELSIF tg_op = 'UPDATE' THEN
    INSERT INTO audit(tbl, op, old_id, new_id) VALUES (tg_table_name, tg_op, old.id, new.id);
  ELSIF tg_op = 'DELETE' THEN
    INSERT INTO audit(tbl, op, old_id) VALUES (tg_table_name, tg_op, old.id);
  END IF;
  RETURN new;
END;
$$ LANGUAGE plpgsql;

-- 2. FOUND and ROW_COUNT via GET DIAGNOSTICS
CREATE OR REPLACE FUNCTION check_found(p_id integer) RETURNS boolean AS $$
DECLARE
  n integer;
BEGIN
  SELECT 1 INTO n FROM users WHERE id = p_id;
  IF NOT found THEN
    RETURN false;
  END IF;
  GET DIAGNOSTICS n = row_count;
  RETURN n > 0;
END;
$$ LANGUAGE plpgsql;

-- 3. Exception handling: SQLSTATE, SQLERRM
CREATE OR REPLACE FUNCTION safe_insert(p_val text) RETURNS void AS $$
BEGIN
  INSERT INTO t(v) VALUES (p_val);
EXCEPTION
  WHEN others THEN
    RAISE WARNING 'insert failed: % (state=%)', sqlerrm, sqlstate;
END;
$$ LANGUAGE plpgsql;

-- 4. ON CONFLICT EXCLUDED (regular SQL, not PL/pgSQL)
INSERT INTO products(id, price)
VALUES (1, 99.9)
ON CONFLICT (id) DO UPDATE
  SET price = excluded.price
  WHERE excluded.price < products.price;

-- 5. More TG_* variables: TG_NAME, TG_WHEN, TG_LEVEL, TG_NARGS, TG_ARGV, TG_SCHEMA
CREATE OR REPLACE FUNCTION trg_info() RETURNS trigger AS $$
BEGIN
  RAISE NOTICE 'trigger=% when=% level=% nargs=% schema=%',
    tg_name, tg_when, tg_level, tg_nargs, tg_table_schema;
  RETURN new;
END;
$$ LANGUAGE plpgsql;
