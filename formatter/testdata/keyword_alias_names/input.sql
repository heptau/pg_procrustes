EXPLAIN (VERBOSE, COSTS OFF) SELECT * FROM (SELECT 1 AS type, c1 FROM ft1) a ORDER BY type, c1;
MERGE INTO t USING (SELECT i AS id, i AS data FROM g) AS source ON t.id = source.id
WHEN MATCHED AND source.data > 1 THEN UPDATE SET type = source.data
WHEN NOT MATCHED THEN INSERT (id, type) VALUES (source.id, source.data);
PREPARE p AS SELECT 1 AS type FROM t x;
COPY (SELECT 1 AS type FROM t x) TO stdout;
SELECT persons.exit, p.loop FROM persons p;
CREATE FUNCTION f() RETURNS void LANGUAGE plpgsql AS $$
BEGIN
  PERFORM g((SELECT row_to_json(t) FROM (SELECT new.a AS type, new.b AS data) t));
END $$;
