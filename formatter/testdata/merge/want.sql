-- MERGE: target, USING source, ON condition and WHEN actions each get their own line.
MERGE INTO public.accounts AS tgt
USING
   (SELECT id, amount FROM staging WHERE ok) AS src
ON
   tgt.id = src.id
   AND tgt.active
WHEN MATCHED AND src.amount > 0 THEN UPDATE SET balance = tgt.balance + src.amount, updated = now()
WHEN MATCHED THEN DELETE
WHEN NOT MATCHED THEN INSERT (id, balance) VALUES (src.id, CASE WHEN src.amount > 0 THEN src.amount ELSE 0 END)
WHEN NOT MATCHED BY SOURCE THEN DO NOTHING;

CREATE OR REPLACE FUNCTION sync() RETURNS void LANGUAGE plpgsql AS $$
BEGIN
MERGE INTO t
USING
   u
ON
   t.id = u.id
WHEN MATCHED THEN UPDATE SET a = u.a
WHEN NOT MATCHED THEN INSERT (id, a) VALUES (u.id, u.a);
END $$;
