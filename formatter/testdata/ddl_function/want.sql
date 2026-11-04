CREATE OR REPLACE FUNCTION public.add_user(
   p_name text,
   p_age integer DEFAULT 18,
   VARIADIC tags text[] DEFAULT '{}'
)
RETURNS TABLE(id bigint, name text)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public, pg_temp
STABLE
PARALLEL safe
COST 100
AS $$
BEGIN
  RETURN query SELECT 1, 'a';
END;
$$;
CREATE FUNCTION f(
   name text,
   status integer
)
RETURNS integer
LANGUAGE SQL
IMMUTABLE
AS 'select 1';
CREATE FUNCTION g()
RETURNS trigger
AS $$ BEGIN RETURN NEW; END $$
LANGUAGE plpgsql;
CREATE PROCEDURE p(
   INOUT name text,
   IN event integer
)
LANGUAGE SQL
AS $$ SELECT 1 $$;
