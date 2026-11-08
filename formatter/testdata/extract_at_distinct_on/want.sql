SELECT EXTRACT(MONTH FROM _at AT TIME ZONE 'Europe/Warsaw')::INTEGER % 2, now() AT TIME ZONE 'UTC';

SELECT DISTINCT ON (u.id) u.id, u.d FROM unnest(a, b) AS u(id, d);

CREATE FUNCTION g() RETURNS INTEGER LANGUAGE plpgsql AS $$
BEGIN
    RETURN QUERY
    SELECT x.a FROM (SELECT DISTINCT ON (u.id) u.id, u.d FROM unnest(a, b) AS u(id, d)) AS x;
END;
$$;
