-- CTEs (WITH), window functions, subqueries, UNION, EXCEPT.
-- Tests all SELECT sub-features in one query set.

WITH RECURSIVE org_tree AS (
    SELECT id, parent_id, name, 1 AS depth
    FROM public.departments
    WHERE parent_id IS NULL

    UNION ALL

    SELECT d.id, d.parent_id, d.name, ot.depth + 1
    FROM public.departments AS d
    INNER JOIN org_tree AS ot ON ot.id = d.parent_id
    WHERE ot.depth < 10
),
monthly_revenue AS (
    SELECT
        date_trunc('month', o.created_at)            AS month,
        d.id                                          AS dept_id,
        d.name                                        AS dept_name,
        sum(o.total)                                  AS revenue,
        count(DISTINCT o.user_id)                     AS customers
    FROM public.orders AS o
    INNER JOIN public.users AS u   ON u.id = o.user_id
    INNER JOIN public.departments AS d ON d.id = u.dept_id
    WHERE o.status = 'paid'
      AND o.created_at >= date_trunc('year', CURRENT_DATE)
    GROUP BY 1, 2, 3
),
ranked AS (
    SELECT
        month,
        dept_id,
        dept_name,
        revenue,
        customers,
        lag(revenue)  OVER (PARTITION BY dept_id ORDER BY MONTH)       AS prev_revenue,
        rank()        OVER (PARTITION BY MONTH   ORDER BY revenue DESC) AS revenue_rank,
        sum(revenue)  OVER (PARTITION BY MONTH)                         AS month_total,
        avg(revenue)  OVER (PARTITION BY dept_id ORDER BY MONTH ROWS BETWEEN 2 PRECEDING AND CURRENT ROW) AS moving_avg
    FROM monthly_revenue
)
SELECT
    month,
    dept_id,
    dept_name,
    revenue,
    customers,
    COALESCE(revenue - prev_revenue, 0)          AS delta,
    round(revenue / NULLIF(month_total, 0) * 100, 2) AS pct_of_month,
    round(moving_avg, 2)                          AS moving_avg_3m,
    revenue_rank
FROM ranked
WHERE revenue_rank <= 5

UNION ALL

SELECT
    NULL::timestamp AS month,
    NULL::integer   AS dept_id,
    'TOTAL'         AS dept_name,
    sum(revenue)    AS revenue,
    sum(customers)  AS customers,
    NULL            AS delta,
    100.00          AS pct_of_month,
    NULL            AS moving_avg_3m,
    NULL            AS revenue_rank
FROM ranked

ORDER BY month NULLS LAST, revenue_rank;


-- Correlated subquery and EXISTS
SELECT p.id, p.name, p.price
FROM public.products AS p
WHERE EXISTS (
    SELECT 1
    FROM public.order_items AS oi
    WHERE oi.product_id = p.id
      AND oi.created_at >= CURRENT_DATE - interval '7 days'
)
AND p.price > (
    SELECT avg(price) FROM public.products WHERE category_id = p.category_id
)
ORDER BY p.price DESC;
