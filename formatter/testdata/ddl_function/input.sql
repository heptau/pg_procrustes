create or replace function public.add_user(p_name text, p_age int default 18, variadic tags text[] default '{}') returns table(id bigint, name text) language plpgsql security definer set search_path = public, pg_temp stable parallel safe cost 100 as $$
begin
  return query select 1, 'a';
end;
$$;
create function f(name text, status int) returns int language sql immutable as 'select 1';
create function g() returns trigger as $$ begin return new; end $$ language plpgsql;
create procedure p(inout name text, in event int) language sql as $$ select 1 $$;
