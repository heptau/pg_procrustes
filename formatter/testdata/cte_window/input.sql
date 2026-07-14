-- CTEs (WITH), window functions, subqueries, UNION, EXCEPT.
-- Tests all SELECT sub-features in one query set.

WITH RECURSIVE org_tree AS (
    SELECT id, parent_id, name, 1 AS depth
    FROM public.departments
    WHERE parent_id IS NULL

    UNION ALL

    SELECT d.id, d.parent_id, d.name, ot.depth + 1
    FROM public.departments d
    INNER JOIN org_tree ot ON ot.id = d.parent_id
    WHERE ot.depth < 10
),
monthly_revenue AS (
    SELECT
        DATE_TRUNC('month', o.created_at)            AS month,
        d.id                                          AS dept_id,
        d.name                                        AS dept_name,
        SUM(o.total)                                  AS revenue,
        COUNT(DISTINCT o.user_id)                     AS customers
    FROM public.orders o
    INNER JOIN public.users u   ON u.id = o.user_id
    INNER JOIN public.departments d ON d.id = u.dept_id
    WHERE o.status = 'paid'
      AND o.created_at >= DATE_TRUNC('year', CURRENT_DATE)
    GROUP BY 1, 2, 3
),
ranked AS (
    SELECT
        month,
        dept_id,
        dept_name,
        revenue,
        customers,
        LAG(revenue)  OVER (PARTITION BY dept_id ORDER BY month)       AS prev_revenue,
        RANK()        OVER (PARTITION BY month   ORDER BY revenue DESC) AS revenue_rank,
        SUM(revenue)  OVER (PARTITION BY month)                         AS month_total,
        AVG(revenue)  OVER (PARTITION BY dept_id ORDER BY month ROWS BETWEEN 2 PRECEDING AND CURRENT ROW) AS moving_avg
    FROM monthly_revenue
)
SELECT
    month,
    dept_id,
    dept_name,
    revenue,
    customers,
    COALESCE(revenue - prev_revenue, 0)          AS delta,
    ROUND(revenue / NULLIF(month_total, 0) * 100, 2) AS pct_of_month,
    ROUND(moving_avg, 2)                          AS moving_avg_3m,
    revenue_rank
FROM ranked
WHERE revenue_rank <= 5

UNION ALL

SELECT
    NULL::timestamp AS month,
    NULL::integer   AS dept_id,
    'TOTAL'         AS dept_name,
    SUM(revenue)    AS revenue,
    SUM(customers)  AS customers,
    NULL            AS delta,
    100.00          AS pct_of_month,
    NULL            AS moving_avg_3m,
    NULL            AS revenue_rank
FROM ranked

ORDER BY month NULLS LAST, revenue_rank;


-- Correlated subquery and EXISTS
SELECT p.id, p.name, p.price
FROM public.products p
WHERE EXISTS (
    SELECT 1
    FROM public.order_items oi
    WHERE oi.product_id = p.id
      AND oi.created_at >= CURRENT_DATE - INTERVAL '7 days'
)
AND p.price > (
    SELECT AVG(price) FROM public.products WHERE category_id = p.category_id
)
ORDER BY p.price DESC;
