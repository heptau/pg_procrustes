DO LANGUAGE plpgsql
$$
DECLARE
    _id_user CONSTANT integer = 222222;
BEGIN
    PERFORM 1;
END
$$;
DO LANGUAGE plpython3u $$ x  =  1 $$;
