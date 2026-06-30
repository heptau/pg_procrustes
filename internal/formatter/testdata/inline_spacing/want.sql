-- Inline spacing: collapse runs of 2+ spaces between tokens to a single space.
-- Typical use case: hand-aligned column lists and CREATE TABLE definitions.

-- Column-aligned SELECT.
SELECT u.id AS id,
       u.first_name AS first_name,
       u.last_name AS last_name,
       u.email AS email,
       u.created_at AS created_at
FROM public.users AS u
WHERE u.is_active = TRUE
  AND u.role = 'admin';

-- Column-aligned UPDATE SET.
UPDATE public.accounts
   SET balance = balance - 100,
       updated_at = now(),
       status = 'debited'
 WHERE id = 42;

-- Column-aligned CREATE TABLE.
CREATE TABLE public.products (
    id bigint NOT NULL,
    sku character varying(64) NOT NULL,
    name text NOT NULL,
    unit_price numeric(12, 2) NOT NULL DEFAULT 0,
    stock_qty integer NOT NULL DEFAULT 0,
    is_available boolean NOT NULL DEFAULT TRUE,
    created_at timestamp with time zone NOT NULL DEFAULT NOW(),
    CONSTRAINT products_pkey PRIMARY KEY (id),
    CONSTRAINT products_sku_key UNIQUE (sku)
);
