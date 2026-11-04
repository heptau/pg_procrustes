create unique index concurrently if not exists idx_users_email on public.users using btree (lower(email), created_at desc) include (name) where deleted_at is null and active;
create or replace view v_active as select id, name from users where active;
create materialized view mv as select a, b from t where x > 1 with no data;
create index on t (a);
create index concurrently idx2 on only t using gin (doc jsonb_path_ops) with (fastupdate = off) tablespace fast where a > 1;
create materialized view mv as select a, b from t where x > 1 with no data;
create view v as select a from t where x > 1 with local check option;
with data as (select 1) select * from data;
create table x as select 1 with data;
