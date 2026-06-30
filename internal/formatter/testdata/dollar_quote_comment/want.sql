-- Dollar-quoted strings in COMMENT ON must not have their content modified.
-- Any delimiter ($$ $_$ $fn$ $jen_test$) must work the same way.

-- 1. COMMENT with $$ — keywords in text must not be uppercased
COMMENT ON FUNCTION foo() IS $$This is a description with AND OR SELECT keywords.$$;

-- 2. COMMENT with custom delimiter $_$
COMMENT ON TABLE users IS $_$This table stores user AND session data.$_$;

-- 3. COMMENT with $fn$
COMMENT ON COLUMN t.col IS $fn$Column for storing data, NOT NULL by default.$fn$;

-- 4. COMMENT with long delimiter $jen_test$
COMMENT ON SCHEMA public IS $jen_test$Contains all public objects. SELECT * FROM anywhere.$jen_test$;

-- 5. Dollar-quote as string value in expression — must not be modified
SELECT $$Some text with AND OR SELECT keywords.$$;

-- 6. Dollar-quote in INSERT value — must not be modified
INSERT INTO t(description) VALUES ($$Some text WITH keywords like SELECT and FROM.$$);

-- 7. DO block — must be formatted (case applied to PL/pgSQL keywords)
DO $$
BEGIN
  raise notice 'test';
END;
$$;

-- 8. Function with AS $$ — body must be formatted
CREATE OR REPLACE FUNCTION bar() RETURNS void AS $$
BEGIN
  raise notice 'bar';
END;
$$ LANGUAGE plpgsql;

-- 9. Function with custom delimiter AS $fn$
CREATE OR REPLACE FUNCTION baz() RETURNS void AS $fn$
BEGIN
  raise notice 'baz';
END;
$fn$ LANGUAGE plpgsql;
