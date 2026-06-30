-- DML: INSERT, UPDATE, DELETE, UPSERT (ON CONFLICT), RETURNING.
-- Tests operator spacing, paren spacing, semicolons, schema-qualified names.

INSERT INTO public.orders (user_id, product_id, qty, unit_price, status, notes)
VALUES
    (1, 10, 2, 49.99, 'pending', NULL),
    (1, 11, 1, 19.00, 'pending', 'gift wrap'),
    (2, 10, 5, 47.50, 'paid', 'bulk discount');

INSERT INTO public.order_log (order_id, event, occurred_at)
SELECT id, 'created', created_at FROM public.orders WHERE created_at >= CURRENT_DATE
ON CONFLICT (order_id, event) DO NOTHING;

INSERT INTO public.inventory (product_id, qty_reserved)
SELECT product_id, sum(qty) FROM public.orders WHERE status = 'pending' GROUP BY product_id
ON CONFLICT (product_id) DO UPDATE SET qty_reserved = excluded.qty_reserved, updated_at = now();

UPDATE public.orders
   SET status = 'shipped',
       shipped_at = now(),
       updated_at = now()
 WHERE status = 'paid'
   AND shipped_at IS NULL
   AND created_at < now() - interval '2 days'
RETURNING id, status, shipped_at;

UPDATE public.accounts a
   SET balance = a.balance - o.total,
       updated_at = now()
  FROM public.orders AS o
 WHERE o.user_id = a.owner_id
   AND o.status = 'paid'
   AND o.settled = FALSE;

DELETE FROM public.orders
 WHERE status = 'cancelled'
   AND created_at < now() - interval '90 days';

DELETE FROM public.sessions AS s
  USING public.users AS u
  WHERE s.user_id = u.id
    AND u.is_active = FALSE
RETURNING s.id;
