-- MERGE: target, USING source, ON condition and WHEN actions each get their own line.
merge into public.Accounts as tgt using (select id, amount from staging where ok) as src on tgt.id = src.id and tgt.active
when matched and src.amount > 0 then update set balance = tgt.balance + src.amount, updated = now()
when matched then delete
when not matched then insert (id, balance) values (src.id, case when src.amount > 0 then src.amount else 0 end)
when not matched by source then do nothing;

create or replace function sync() returns void language plpgsql as $$
begin
merge into t using u on t.id = u.id when matched then update set a = u.a when not matched then insert (id, a) values (u.id, u.a);
end $$;
