alter table t add constraint c foreign key (a, b) references u (a, b) on delete cascade on update set null deferrable initially deferred;
alter table t add primary key (id), add column x int, add constraint u1 check (x > 0) not valid;
alter table t add column y int;
create table child partition of parent for values in (1, 2, 3);
create table c2 partition of p for values from (1) to (10) partition by range (x) tablespace ts;
create table c3 partition of p for values with (modulus 4, remainder 0);
create table c4 partition of p default;
drop table if exists a, b, c cascade;
drop table a;
drop index concurrently if exists i1, i2;
create domain d as int default 1 not null constraint pos check (value > 0) check (value < 100);
create extension if not exists pgcrypto with schema public version '1.3' cascade;
create extension hstore;
insert into t (a, b) values (1, 2) on conflict (a) do update set b = excluded.b where t.b < 3 returning a;
insert into t (a) values (1) on conflict do nothing;
alter table t add constraint k1 foreign key (a) references u (a) -- a comment keeps this as written
  on delete cascade;
