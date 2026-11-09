CREATE FUNCTION g() RETURNS integer LANGUAGE plpgsql AS $$
BEGIN
    IF a THEN RETURN 1;
    ELSE RETURN 2;
    END IF;
    x := 1;
    y := 2;
    PERFORM f(1);
    PERFORM f(2);
    SELECT a INTO x FROM t;
    SELECT b INTO y FROM t;
END;
$$;
