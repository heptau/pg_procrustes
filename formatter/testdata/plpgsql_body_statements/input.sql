DO $$
BEGIN
  CREATE TABLE t (data text, name text, label text, oid oid, path ltree);
  CREATE TABLE p PARTITION OF v FOR VALUES IN ('a');
  INSERT INTO caster (char) VALUES ('f'::text);
  CREATE VIEW v WITH (security_barrier) AS SELECT * FROM customer WHERE name = current_user;
  EXPLAIN (COSTS OFF) CREATE TABLE s.t (a) AS SELECT generate_series(1, 3) WITH NO DATA;
  ALTER DEFAULT PRIVILEGES FOR ROLE sazky IN SCHEMA m GRANT SELECT ON TABLES TO r;
  FOR r IN SELECT data, name FROM t LOOP
    PERFORM 1;
  END LOOP;
END
$$;
