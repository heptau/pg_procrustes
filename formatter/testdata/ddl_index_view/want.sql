CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS idx_users_email
   ON public.users
   USING btree (lower(email), created_at DESC)
   INCLUDE (name)
   WHERE deleted_at IS NULL AND active;
CREATE OR REPLACE VIEW v_active AS
SELECT
   id,
   name
FROM
   users
WHERE
   active;
CREATE MATERIALIZED VIEW mv AS
SELECT
   a,
   b
FROM
   t
WHERE
   x > 1
WITH NO DATA;
CREATE INDEX ON t (a);
CREATE INDEX CONCURRENTLY idx2
   ON ONLY t
   USING gin (doc jsonb_path_ops)
   WITH (fastupdate = OFF)
   TABLESPACE fast
   WHERE a > 1;
CREATE MATERIALIZED VIEW mv AS
SELECT
   a,
   b
FROM
   t
WHERE
   x > 1
WITH NO DATA;
CREATE VIEW v AS
SELECT
   a
FROM
   t
WHERE
   x > 1
WITH LOCAL CHECK OPTION;
WITH
   data AS (SELECT 1)
SELECT
   *
FROM
   data;
CREATE TABLE x AS
SELECT
   1
WITH DATA;
