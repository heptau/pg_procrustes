EXPLAIN (VERBOSE, COSTS OFF) SELECT * FROM (SELECT 1 AS type, c1 FROM ft1) AS a ORDER BY type, c1;
MERGE INTO t USING (SELECT i AS id, i AS data FROM g) AS source ON t.id = source.id
WHEN MATCHED AND source.data > 1 THEN UPDATE SET type = source.data
WHEN NOT MATCHED THEN INSERT (id, type) VALUES (source.id, source.data);
PREPARE p AS SELECT 1 AS type FROM t AS x;
COPY (SELECT 1 AS type FROM t AS x) TO STDOUT;
SELECT persons.exit, p.loop FROM persons AS p;
CREATE FUNCTION f() RETURNS void LANGUAGE plpgsql AS $$
BEGIN
  PERFORM g((SELECT row_to_json(t) FROM (SELECT NEW.a AS type, NEW.b AS data) t));
END $$;
