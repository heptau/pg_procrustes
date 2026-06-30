-- PL/pgSQL: comprehensive single function covering DECLARE, IF/ELSIF/ELSE,
-- WHILE loop, FOR loop, CASE (simple + searched), EXCEPTION.

CREATE OR REPLACE FUNCTION ledger.post_transaction(
    p_account_id  bigint,
    p_amount      numeric,
    p_currency    character varying,
    p_type        character varying,
    p_ref         text DEFAULT NULL
) RETURNS bigint
LANGUAGE plpgsql AS $$
DECLARE
    v_balance    numeric;
    v_status     character varying;
    v_tx_id      bigint;
    v_rate       numeric;
    v_i          integer := 0;
BEGIN
    -- Validate inputs
    IF p_amount <= 0 THEN
RAISE EXCEPTION 'amount must be positive, got %', p_amount;
END IF;

    IF p_type NOT IN ('credit', 'debit', 'transfer') THEN
RAISE EXCEPTION 'unknown transaction type: %', p_type;
END IF;

    -- Fetch account with lock
    SELECT balance, status
      INTO v_balance, v_status
      FROM ledger.accounts
     WHERE id = p_account_id
       FOR UPDATE;

    IF NOT FOUND THEN
RAISE EXCEPTION 'account % not found', p_account_id;
END IF;

    IF v_status <> 'active' THEN
RAISE EXCEPTION 'account % is %, expected active', p_account_id, v_status;
END IF;

    -- Currency conversion
    IF p_currency <> 'CZK' THEN
SELECT rate INTO v_rate FROM ledger.fx_rates WHERE from_currency = p_currency AND to_currency = 'CZK' AND valid_on = CURRENT_DATE;
IF NOT FOUND THEN
RAISE EXCEPTION 'no FX rate for % on %', p_currency, CURRENT_DATE;
END IF;
p_amount := p_amount * v_rate;
END IF;

    -- Apply transaction using simple CASE
    CASE p_type
WHEN 'credit' THEN
v_balance := v_balance + p_amount;
WHEN 'debit' THEN
IF v_balance < p_amount THEN
RAISE EXCEPTION 'insufficient funds: balance=%, requested=%', v_balance, p_amount;
END IF;
v_balance := v_balance - p_amount;
WHEN 'transfer' THEN
v_balance := v_balance + p_amount;
END CASE;

    UPDATE ledger.accounts SET balance = v_balance, updated_at = NOW() WHERE id = p_account_id;

    -- Log with searched CASE for priority
    CASE
WHEN p_amount > 10000 THEN
RAISE NOTICE 'large transaction: % % on account %', p_amount, p_currency, p_account_id;
WHEN p_amount > 1000 THEN
RAISE NOTICE 'medium transaction: %', p_amount;
ELSE
NULL;
END CASE;

    INSERT INTO ledger.transactions (account_id, amount, currency, tx_type, ref, created_at)
    VALUES (p_account_id, p_amount, p_currency, p_type, p_ref, NOW())
    RETURNING id INTO v_tx_id;

    -- FOR loop: write audit entries for each related account
    FOR v_i IN 1..3 LOOP
INSERT INTO ledger.audit (tx_id, pass, recorded_at) VALUES (v_tx_id, v_i, NOW());
END LOOP;

    RETURN v_tx_id;

EXCEPTION
    WHEN foreign_key_violation THEN
RAISE EXCEPTION 'referential integrity failure for account %', p_account_id;
    WHEN numeric_value_out_of_range THEN
RAISE EXCEPTION 'amount % overflows account balance type', p_amount;
END;
$$;
