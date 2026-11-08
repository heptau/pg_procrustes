SELECT extract(month FROM _at at time zone 'Europe/Warsaw')::integer % 2, now() at time zone 'UTC';

SELECT DISTINCT ON (u.id) u.id, u.d FROM unnest(a, b) AS u(id, d);

CREATE FUNCTION g() RETURNS integer LANGUAGE plpgsql AS $$
BEGIN
    RETURN QUERY
    SELECT x.a FROM (SELECT DISTINCT ON (u.id) u.id, u.d FROM unnest(a, b) AS u(id, d)) AS x;
END;
$$;
