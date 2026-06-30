-- DDL: CREATE TABLE with all column types, constraints, defaults.
-- Also CREATE INDEX, CREATE VIEW, ALTER TABLE.

CREATE TABLE public.accounts (
    id            bigint                   NOT NULL GENERATED ALWAYS AS IDENTITY,
    tenant_id     integer                  NOT NULL,
    owner_id      bigint,
    code          CHARACTER VARYING(64)    NOT NULL,
    label         TEXT,
    balance       NUMERIC(18,4)            NOT NULL DEFAULT 0,
    currency      CHAR(3)                  NOT NULL DEFAULT 'CZK',
    account_type  CHARACTER VARYING(32)    NOT NULL DEFAULT 'standard',
    flags         INTEGER                  NOT NULL DEFAULT 0,
    is_active     BOOLEAN                  NOT NULL DEFAULT TRUE,
    opened_on     DATE,
    closed_on     DATE,
    meta          JSONB                    NOT NULL DEFAULT '{}',
    created_at    TIMESTAMP WITHOUT TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMP WITHOUT TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT accounts_pkey PRIMARY KEY (id),
    CONSTRAINT accounts_code_tenant_key UNIQUE (code, tenant_id),
    CONSTRAINT accounts_tenant_fk FOREIGN KEY (tenant_id) REFERENCES public.tenants (id) ON DELETE CASCADE,
    CONSTRAINT accounts_balance_nn CHECK (balance >= 0),
    CONSTRAINT accounts_dates_ck CHECK (closed_on IS NULL OR closed_on >= opened_on)
);

CREATE INDEX accounts_tenant_idx ON public.accounts (tenant_id);
CREATE INDEX accounts_owner_idx  ON public.accounts (owner_id) WHERE owner_id IS NOT NULL;
CREATE UNIQUE INDEX accounts_active_code ON public.accounts (tenant_id, code) WHERE is_active = TRUE;

CREATE VIEW public.active_accounts AS
    SELECT id, tenant_id, code, label, balance, currency, account_type, created_at
    FROM public.accounts
    WHERE is_active = TRUE AND closed_on IS NULL;

ALTER TABLE public.accounts ADD COLUMN notes TEXT;
ALTER TABLE public.accounts DROP COLUMN notes;
ALTER TABLE public.accounts ALTER COLUMN label SET NOT NULL;
ALTER TABLE public.accounts RENAME COLUMN label TO display_name;
