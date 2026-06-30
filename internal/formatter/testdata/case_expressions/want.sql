-- SQL-level CASE expressions: simple and searched, in SELECT, WHERE, ORDER BY.
-- Tests case.break=always and case.indent=indent.

SELECT o.id,
    o.status,
    CASE o.status
   WHEN 'pending' THEN 0
   WHEN 'paid' THEN 1
   WHEN 'shipped' THEN 2
   WHEN 'delivered' THEN 3
   WHEN 'cancelled' THEN -1
   ELSE -99
END                                              AS status_code,
    CASE
   WHEN o.total > 1000 THEN 'high'
   WHEN o.total > 100 THEN 'medium'
   ELSE 'low'
END                                              AS value_tier,
    CASE
   WHEN o.shipped_at IS NOT NULL AND o.delivered_at IS NOT NULL THEN age(o.delivered_at, o.shipped_at)
   WHEN o.shipped_at IS NOT NULL THEN age(now(), o.shipped_at)
   ELSE NULL
END                                              AS transit_time,
    COALESCE(
        CASE WHEN o.discount > 0 THEN o.total * (1 - o.discount / 100.0) END,
        o.total
    )                                                AS net_total
FROM public.orders AS o
WHERE CASE
   WHEN o.status = 'cancelled' THEN o.cancelled_at > now() - interval '30 days'
   ELSE TRUE
END
ORDER BY CASE o.status
   WHEN 'pending' THEN 1
   WHEN 'paid' THEN 2
   WHEN 'shipped' THEN 3
   ELSE 99
END,
    o.created_at DESC;
