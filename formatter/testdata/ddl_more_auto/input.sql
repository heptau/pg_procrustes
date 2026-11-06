create or replace view v (a, b) with (security_barrier) as select a, b from t where x = 1 with local check option;
create trigger trg before insert or update on public.t for each row when (old.* is distinct from new.*) execute function f(1, 2);
create event trigger et on ddl_command_start when tag in ('create table', 'drop table') execute function f();
create constraint trigger ct after insert on t from r deferrable initially deferred for each row execute procedure f();
create policy p on t as permissive for select to r1, r2 using (a = 1) with check (b = 2);
grant select, insert, update on table a, b to r1, r2 with grant option;
grant r1 to r2;
revoke grant option for select on t from r1 cascade;
create type mood as enum ('sad', 'ok', 'happy');
create type pt as (x int, y int);
create sequence if not exists s as bigint increment by 2 minvalue 1 no maxvalue start with 5 cache 10 no cycle owned by t.id;
alter sequence s restart with 1 cycle;
create policy q on t -- the comment keeps this statement as written
  using (true);
