-- DDL layout: CREATE TABLE column list and ALTER TABLE actions, one per line, aligned.
create table if not exists public.Accounts (id bigint not null generated always as identity, tenant_id integer not null, code varchar(64) not null default 'x', balance numeric(18,4) not null default 0, -- money
 created_at timestamptz not null default now(), constraint accounts_pkey primary key (id), constraint accounts_tenant_fk foreign key (tenant_id) references public.tenants (id) on delete cascade) with (fillfactor = 70);

alter table public.accounts add column notes text, drop column old_col, alter column code set not null;

alter table public.accounts rename to ledger;

create table tiny (a int);
