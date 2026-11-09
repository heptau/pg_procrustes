SELECT * FROM jsonb_to_recordset(a) AS x(database text, class text, object text, name text);
SELECT * FROM unnest(a) AS u(database, class, object);
WITH c(database, class) AS (SELECT 1, 2) SELECT * FROM c;
CREATE VIEW v (database, class) AS SELECT 1, 2;
CREATE TABLE t2 (database, class) AS SELECT 1, 2;
