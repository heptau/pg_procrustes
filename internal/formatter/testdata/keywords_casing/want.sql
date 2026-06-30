-- Keyword and identifier casing: reserved keywords, operators, literals, aliases, columns.
-- Expected output (default config): keywords UPPER, types lower, aliases lower, columns lower.

SELECT
    u.id          AS user_id,
    u.full_name   AS fullname,
    u.email,
    COALESCE(u.phone, 'n/a')   AS phone,
    NULLIF(u.score, 0)         AS score,
    CURRENT_DATE               AS today,
    SESSION_USER               AS actor,
    u.created_at::date         AS signup,
    u.active = TRUE            AS is_active,
    u.type IN ('admin', 'mod') AS is_staff
FROM public.users AS u
INNER JOIN public.roles AS r ON r.user_id = u.id
WHERE u.active = TRUE
  AND u.deleted_at IS NULL
  AND u.score >= 0
  AND u.email NOT LIKE '%@internal%'
  AND u.type != 'bot'
  AND u.id != 0
ORDER BY u.full_name ASC, u.created_at DESC
LIMIT 100 OFFSET 0
