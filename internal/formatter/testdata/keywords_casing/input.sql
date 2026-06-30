-- Keyword and identifier casing: reserved keywords, operators, literals, aliases, columns.
-- Expected output (default config): keywords UPPER, types lower, aliases lower, columns lower.

select
    u.ID          as user_id,
    u.FULL_NAME   AS fullname,
    u.EMAIL,
    coalesce(u.PHONE, 'n/a')   as phone,
    nullif(u.SCORE, 0)         as score,
    current_date               as today,
    session_user               as actor,
    u.CREATED_AT::date         as signup,
    u.ACTIVE = true            as is_active,
    u.TYPE in ('admin', 'mod') as is_staff
from PUBLIC.USERS u
inner join PUBLIC.ROLES r on r.USER_ID = u.ID
where u.ACTIVE = true
  and u.DELETED_AT is null
  and u.SCORE >= 0
  and u.EMAIL not like '%@internal%'
  and u.TYPE <> 'bot'
  and u.ID != 0
order by u.FULL_NAME asc, u.CREATED_AT desc
limit 100 offset 0
