-- DDL layout: CREATE TABLE column list and ALTER TABLE actions, one per line, aligned.
CREATE TABLE IF NOT EXISTS public.accounts (
   id         bigint                   NOT NULL GENERATED ALWAYS AS IDENTITY,
   tenant_id  integer                  NOT NULL,
   code       character varying(64)    NOT NULL DEFAULT 'x',
   balance    numeric(18, 4)           NOT NULL DEFAULT 0, -- money
   created_at timestamp with time zone NOT NULL DEFAULT now(),
   CONSTRAINT accounts_pkey PRIMARY KEY (id),
   CONSTRAINT accounts_tenant_fk FOREIGN KEY (tenant_id) REFERENCES public.tenants (id) ON DELETE CASCADE
) WITH (fillfactor = 70);

ALTER TABLE public.accounts
   ADD COLUMN notes text,
   DROP COLUMN old_col,
   ALTER COLUMN code SET NOT NULL;

ALTER TABLE public.accounts RENAME TO ledger;

CREATE TABLE tiny (a integer);
