package formatter_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	pg_query "github.com/pganalyze/pg_query_go/v6"

	"github.com/heptau/pg_procrustes/config"
	"github.com/heptau/pg_procrustes/formatter"
)

// matrixVariant mutates a default config. All variants must keep the SQL
// semantically identical (same AST fingerprint), parseable and idempotent.
type matrixVariant struct {
	name  string
	apply func(c *config.Config)
}

var matrixVariants = []matrixVariant{
	{"call_paren_add_builtin_upper", func(c *config.Config) {
		c.CallParenSpacing = config.ParenSpacingAdd
		c.BuiltinFunctions.Case = config.CaseUpper
	}},
	{"call_paren_remove_builtin_capitalize", func(c *config.Config) {
		c.CallParenSpacing = config.ParenSpacingRemove
		c.BuiltinFunctions.Case = config.CaseCapitalize
	}},
	{"clauses_always", func(c *config.Config) { c.Layout.Clauses.Break = config.BreakAlways }},
	{"content_always", func(c *config.Config) { c.Layout.Content.Break = config.BreakAlways }},
	{"clauses_content_always", func(c *config.Config) {
		c.Layout.Clauses.Break = config.BreakAlways
		c.Layout.Content.Break = config.BreakAlways
	}},
	{"always_first_inline", func(c *config.Config) {
		c.Layout.Clauses.Break = config.BreakAlways
		c.Layout.Content.Break = config.BreakAlways
		c.Layout.Content.FirstItem = config.FirstItemInline
	}},
	{"always_align_indent", func(c *config.Config) {
		c.Layout.Clauses.Break = config.BreakAlways
		c.Layout.Clauses.Align = config.AlignIndent
		c.Layout.Content.Break = config.BreakAlways
	}},
	{"always_tabs", func(c *config.Config) {
		c.Layout.Clauses.Break = config.BreakAlways
		c.Layout.Content.Break = config.BreakAlways
		c.Layout.Indent.Type = config.IndentTypeTab
	}},
	{"auto_short_lines", func(c *config.Config) {
		c.Layout.LineLength = 30
		c.Layout.Clauses.Break = config.BreakAuto
		c.Layout.Content.Break = config.BreakAuto
	}},
	{"never", func(c *config.Config) {
		c.Layout.Clauses.Break = config.BreakNever
		c.Layout.Content.Break = config.BreakNever
	}},
	{"union_both", func(c *config.Config) {
		c.Layout.Clauses.Break = config.BreakAlways
		c.Layout.Union.BlankLine = config.UnionBlankLineBoth
	}},
	{"case_always", func(c *config.Config) { c.Layout.Case.Break = config.BreakAlways }},
	{"case_never", func(c *config.Config) { c.Layout.Case.Break = config.BreakNever }},
	{"all_layout", func(c *config.Config) {
		c.Layout.Clauses.Break = config.BreakAlways
		c.Layout.Content.Break = config.BreakAlways
		c.Layout.Case.Break = config.BreakAlways
		c.Layout.Union.BlankLine = config.UnionBlankLineBoth
	}},
	{"operators_compact", func(c *config.Config) { c.OperatorSpacing = config.OperatorSpacingCompact }},
	{"arith_normalize", func(c *config.Config) { c.ArithmeticSpacing = config.ArithmeticSpacingNormalize }},
	{"arith_compact", func(c *config.Config) { c.ArithmeticSpacing = config.ArithmeticSpacingCompact }},
	{"comma_compact", func(c *config.Config) { c.CommaSpacing = config.CommaSpacingCompact }},
	{"paren_add", func(c *config.Config) { c.ParenSpacing = config.ParenSpacingAdd }},
	{"inline_normalize", func(c *config.Config) { c.InlineSpacing = config.InlineSpacingNormalize }},
	{"aliases_no_as", func(c *config.Config) { c.Aliases.As = config.AliasAsRemove }},
	{"join_long", func(c *config.Config) { c.JoinForm = config.JoinFormLong }},
	{"cast_operator", func(c *config.Config) { c.CastStyle = config.CastStyleOperator }},
	{"align_aliases_set", func(c *config.Config) {
		c.Layout.Clauses.Break = config.BreakAlways
		c.Layout.Content.Break = config.BreakAlways
		c.Layout.Content.AlignAliases = config.AlignItemsAlign
		c.Layout.Content.AlignSet = config.AlignItemsAlign
	}},
	{"align_aliases_set_comma_start", func(c *config.Config) {
		c.Layout.Clauses.Break = config.BreakAlways
		c.Layout.Content.Break = config.BreakAlways
		c.Layout.Content.CommaPosition = config.CommaStart
		c.Layout.Content.AlignAliases = config.AlignItemsAlign
		c.Layout.Content.AlignSet = config.AlignItemsAlign
	}},
	{"comma_start", func(c *config.Config) {
		c.Layout.Clauses.Break = config.BreakAlways
		c.Layout.Content.Break = config.BreakAlways
		c.Layout.Content.CommaPosition = config.CommaStart
	}},
	{"comma_start_first_inline", func(c *config.Config) {
		c.Layout.Clauses.Break = config.BreakAlways
		c.Layout.Content.Break = config.BreakAlways
		c.Layout.Content.FirstItem = config.FirstItemInline
		c.Layout.Content.CommaPosition = config.CommaStart
	}},
	{"operator_end", func(c *config.Config) {
		c.Layout.Clauses.Break = config.BreakAlways
		c.Layout.Content.Break = config.BreakAlways
		c.Layout.Content.LogicalOperatorPosition = config.LogicalOperatorEnd
	}},
	{"join_on_newline", func(c *config.Config) {
		c.Layout.Clauses.Break = config.BreakAlways
		c.Layout.Clauses.Join.Align = config.AlignIndent
		c.Layout.Content.Break = config.BreakAlways
		c.Layout.Content.JoinOnPosition = config.JoinOnNewline
	}},
	{"positions_combined_tabs", func(c *config.Config) {
		c.Layout.Clauses.Break = config.BreakAlways
		c.Layout.Content.Break = config.BreakAlways
		c.Layout.Content.CommaPosition = config.CommaStart
		c.Layout.Content.LogicalOperatorPosition = config.LogicalOperatorEnd
		c.Layout.Content.JoinOnPosition = config.JoinOnNewline
		c.Layout.Indent.Type = config.IndentTypeTab
	}},
	{"ddl_create_always_align_all", func(c *config.Config) {
		c.Layout.DDL.CreateTable = config.CreateTableCfg{Break: config.BreakAlways, Align: config.ColumnAlignAll}
		c.Layout.DDL.AlterTable.Break = config.BreakAlways
	}},
	{"ddl_create_always_align_types", func(c *config.Config) {
		c.Layout.DDL.CreateTable = config.CreateTableCfg{Break: config.BreakAlways, Align: config.ColumnAlignTypes}
		c.Layout.DDL.AlterTable.Break = config.BreakAuto
	}},
	{"ddl_create_always_align_none", func(c *config.Config) {
		c.Layout.DDL.CreateTable = config.CreateTableCfg{Break: config.BreakAlways, Align: config.ColumnAlignNone}
	}},
	{"ddl_index_always", func(c *config.Config) {
		c.Layout.DDL.CreateIndex.Break = config.BreakAlways
		c.Layout.Clauses.Break = config.BreakAlways
		c.Layout.Content.Break = config.BreakAlways
	}},
	{"ddl_index_never", func(c *config.Config) { c.Layout.DDL.CreateIndex.Break = config.BreakNever }},
	{"ddl_index_auto_narrow", func(c *config.Config) {
		c.Layout.DDL.CreateIndex.Break = config.BreakAuto
		c.Layout.LineLength = 40
	}},
	{"ddl_never", func(c *config.Config) {
		c.Layout.DDL.CreateTable.Break = config.BreakNever
		c.Layout.DDL.AlterTable.Break = config.BreakNever
	}},
	{"ddl_auto_short", func(c *config.Config) {
		c.Layout.LineLength = 30
		c.Layout.DDL.CreateTable = config.CreateTableCfg{Break: config.BreakAuto, Align: config.ColumnAlignTypes}
		c.Layout.DDL.AlterTable.Break = config.BreakAuto
	}},
	{"ddl_comma_start_tabs", func(c *config.Config) {
		c.Layout.DDL.CreateTable = config.CreateTableCfg{Break: config.BreakAlways, Align: config.ColumnAlignAll}
		c.Layout.DDL.AlterTable.Break = config.BreakAlways
		c.Layout.Content.CommaPosition = config.CommaStart
		c.Layout.Indent.Type = config.IndentTypeTab
	}},
	{"ddl_with_everything", func(c *config.Config) {
		c.Layout.Clauses.Break = config.BreakAlways
		c.Layout.Content.Break = config.BreakAlways
		c.Layout.DDL.CreateTable = config.CreateTableCfg{Break: config.BreakAlways, Align: config.ColumnAlignAll}
		c.Layout.DDL.AlterTable.Break = config.BreakAlways
		c.Layout.ParenIndent.Mode = config.ParenIndentIndent
	}},
	{"comments_both", func(c *config.Config) {
		c.Comments = config.CommentsCfg{SpaceAfterMarker: config.CommentSpaceAdd, AlignTrailing: config.CommentAlignAlign}
	}},
	{"comments_with_layout", func(c *config.Config) {
		c.Comments = config.CommentsCfg{SpaceAfterMarker: config.CommentSpaceAdd, AlignTrailing: config.CommentAlignAlign}
		c.Layout.Clauses.Break = config.BreakAlways
		c.Layout.Content.Break = config.BreakAlways
		c.Layout.DDL.CreateTable = config.CreateTableCfg{Break: config.BreakAlways, Align: config.ColumnAlignAll}
	}},
	{"separation_0", func(c *config.Config) { c.BlankLinesBetweenStatements = "0" }},
	{"separation_2_trailing_newline", func(c *config.Config) {
		c.BlankLinesBetweenStatements = "2"
		c.TrailingNewline = config.TrailingNewlineAdd
	}},
	{"line_endings_crlf", func(c *config.Config) { c.LineEndings = config.LineEndingCRLF }},
	{"line_endings_lf_with_layout", func(c *config.Config) {
		c.LineEndings = config.LineEndingLF
		c.Layout.Clauses.Break = config.BreakAlways
	}},
	{"capitalize_all", func(c *config.Config) {
		for _, sec := range []*config.Section{
			&c.ReservedKeywords, &c.Keywords, &c.Literals, &c.Operators, &c.Schemas, &c.Tables,
			&c.Functions, &c.ConditionalFunctions, &c.SystemFunctions, &c.Columns,
		} {
			sec.Case = config.CaseCapitalize
		}
		c.DataTypes.Case = config.CaseCapitalize
		c.Aliases.Case = config.CaseCapitalize
	}},
	{"plpgsql_indent_all", func(c *config.Config) {
		pl := &c.Layout.DollarQuote.PLpgSQL
		pl.KeywordIndent = config.BodyIndentIndent
		pl.Declare.Indent = config.BodyIndentIndent
		pl.BeginBody.Indent = config.BodyIndentIndent
		pl.ControlFlow.If.BodyIndent = config.BodyIndentIndent
		pl.ControlFlow.Loop.BodyIndent = config.BodyIndentIndent
	}},
	{"plpgsql_blank_lines", func(c *config.Config) {
		pl := &c.Layout.DollarQuote.PLpgSQL
		pl.ControlFlow.If.BlankLineBefore = config.BlankLineAdd
		pl.ControlFlow.If.BlankLineAfter = config.BlankLineAdd
		pl.ControlFlow.Loop.BlankLineBefore = config.BlankLineAdd
		pl.ControlFlow.Loop.BlankLineAfter = config.BlankLineAdd
		pl.Declare.BlankLineBefore = config.BlankLineAdd
		pl.Declare.BlankLineAfter = config.BlankLineAdd
		pl.BeginBody.BlankLineBefore = config.BlankLineAdd
		pl.BeginBody.BlankLineAfter = config.BlankLineAdd
	}},
	{"plpgsql_case_expanded", func(c *config.Config) {
		pl := &c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case
		for _, b := range []*config.CaseBranchCfg{&pl.Simple, &pl.Searched} {
			b.WhenIndent = config.BodyIndentIndent
			b.ThenBreak = config.BreakAlways
			b.ThenIndent = config.BodyIndentIndent
			b.BodyBreak = config.BreakAlways
			b.BodyIndent = config.BodyIndentIndent
			b.BlankLineBefore = config.BlankLineAdd
			b.BlankLineAfter = config.BlankLineAdd
		}
	}},
	{"plpgsql_end_semicolon_declare", func(c *config.Config) {
		pl := &c.Layout.DollarQuote.PLpgSQL
		pl.EndSemicolon = config.EndSemicolonAdd
		pl.DeclareEmpty = config.WhenEmptyAdd
	}},
	{"dollar_boundary", func(c *config.Config) {
		dq := &c.Layout.DollarQuote
		dq.NewlineAfterOpen = config.BlankLineAdd
		dq.NewlineBeforeClose = config.BlankLineAdd
		dq.SQL.BodyIndent = config.BodyIndentIndent
		dq.SQL.BlankLineBefore = config.BlankLineAdd
		dq.SQL.BlankLineAfter = config.BlankLineAdd
	}},
	{"dollar_plus_clauses_always", func(c *config.Config) {
		c.Layout.Clauses.Break = config.BreakAlways
		c.Layout.Content.Break = config.BreakAlways
		c.Layout.Case.Break = config.BreakAlways
		c.Layout.DollarQuote.NewlineAfterOpen = config.BlankLineAdd
		c.Layout.DollarQuote.NewlineBeforeClose = config.BlankLineAdd
		c.Layout.DollarQuote.PLpgSQL.KeywordIndent = config.BodyIndentIndent
		c.Layout.DollarQuote.PLpgSQL.BeginBody.Indent = config.BodyIndentIndent
		c.Layout.DollarQuote.PLpgSQL.ControlFlow.If.BodyIndent = config.BodyIndentIndent
	}},
	{"paren_indent", func(c *config.Config) {
		c.Layout.ParenIndent.Mode = config.ParenIndentIndent
		c.Layout.Clauses.Break = config.BreakAlways
		c.Layout.Content.Break = config.BreakAlways
	}},
	{"paren_indent_close_after", func(c *config.Config) {
		c.Layout.ParenIndent.Mode = config.ParenIndentIndent
		c.Layout.ParenIndent.CloseFirst = config.ParenCloseAfter
		c.Layout.Clauses.Break = config.BreakAlways
	}},
	{"indent_normalize_tabs", func(c *config.Config) {
		c.Layout.Indent.Type = config.IndentTypeTab
		c.Layout.Indent.Normalize = config.IndentNormalizeChange
		c.Layout.Clauses.Break = config.BreakAlways
	}},
	{"whitespace_and_blank_lines", func(c *config.Config) {
		c.TrailingWhitespace = config.TrailingWSPreserve
		c.BlankLines = config.BlankLinesMax1
	}},
	{"semicolons_add", func(c *config.Config) { c.Semicolons = config.SemicolonAdd }},
	{"quoted_preserve", func(c *config.Config) { c.QuotedIdents = config.QuotedIdentPreserve }},
	{"capitalize_with_layout", func(c *config.Config) {
		c.ReservedKeywords.Case = config.CaseCapitalize
		c.Keywords.Case = config.CaseCapitalize
		c.Tables.Case = config.CaseCapitalize
		c.Layout.Clauses.Break = config.BreakAlways
	}},
	{"keywords_lower", func(c *config.Config) {
		c.ReservedKeywords.Case = config.CaseLower
		c.Keywords.Case = config.CaseLower
	}},
}

// matrixInline are extra statements exercising constructs the golden inputs miss
// (comments, set operations, CASE, quoted identifiers, arithmetic).
var matrixInline = []string{
	"create unique index concurrently if not exists idx on public.users using btree (lower(email), created_at desc) include (name) where deleted_at is null and active;",
	"create index on t (a); create index i2 on only t using gin (doc jsonb_path_ops) with (fastupdate = off) tablespace fast where a > 1;",
	"create materialized view mv as select a, b from t where x > 1 with no data; create view v as select a from t with local check option;",
	"with data as (select 1) select * from data;",
	"select a, -- first\n b -- second\n from t where x = 1 -- tail\n and y = 2;",
	"select a from u union select b from v union all select 1;",
	"select a from u where x = 1 union all select 1 order by 1;",
	`select "Id", "My Col" as "X" from "User" u where u."Id" = 1;`,
	"select case when a > 1 then 'x' when a < 0 then 'y' else 'z' end, case b when 1 then 2 end from t;",
	"select a+1, b - 2, c*d, -1, count(*), t.*, a - -1, (a+b)*(c-d) from t where n*2 > 1;",
	"insert into t (a, -- x\n b) values (1, 2) on conflict (a) do update set b = excluded.b returning *;",
	"update t set a = 1, b = b + 1 from u where t.id = u.id and t.x is not null returning t.id;",
	"with recursive r(n) as (select 1 union all select n+1 from r where n < 5) select * from r;",
	"select * from t, lateral (select 1) x join (select 2) y on true left join (select 3) as z on true;",
	"select sum(x) over (partition by a order by b rows between unbounded preceding and current row) as s from t window w as (order by c);",
	"create table if not exists public.t (id serial primary key, name varchar(20) not null default 'x', constraint fk foreign key (id) references o(id) on delete cascade);",
	"create trigger tr before insert or update on t for each row execute function f(); drop trigger tr on t;",
	"select date_trunc('month', o.created_at) as month, sum(x) over (order by y) as over from t o;",
	"select a from t where a in (select b from u) and exists (select 1 from v where v.x = t.a);",
	"create view v as select id, name from t where a = 1; create table x as select a from t; explain select a from t where b = 1;",
	"alter table t alter column a set not null, alter column b set default 1; alter table t set schema s; alter role bob set search_path = x; set search_path = public;",
	"create table x (a int references o(id) on delete cascade on update cascade) with (fillfactor = 70); alter table t alter column a type int using a::int;",
	"select * from t where a = 1 for update; create trigger tr before insert or update on t for each row execute function f();",
	"insert into t values (1) on conflict (id) do update set a = 2; delete from t using u where t.id = u.id;",
	"merge into public.Accounts as tgt using (select id, amount from staging where ok) as src on tgt.id = src.id and tgt.active when matched and src.amount > 0 then update set balance = tgt.balance + src.amount, updated = now() when matched then delete when not matched then insert (id, balance) values (src.id, case when src.amount > 0 then src.amount else 0 end) when not matched by source then do nothing;",
	"merge into t using u on t.id = u.id when matched then update set a = case when u.x then 1 else 2 end -- tail\n when not matched then insert values (u.id, 1);",
	`create function f(p int) returns int language plpgsql as $$
declare
v int := 1;
begin
if p > 1 then
v := v + 1;
elsif p = 0 then
raise notice 'x %', p;
else
perform now();
end if;
return v;
exception when others then
return 0;
end $$;`,
	`create function g(n int) returns setof int language plpgsql as $$
begin
for i in 1..n loop
return next i;
end loop;
while n > 0 loop
n := n - 1;
exit when n = 2;
end loop;
return;
end $$;`,
	`create function h(k text) returns text language plpgsql as $$
declare
r text;
begin
case k
when 'a' then
r := 'x';
when 'b' then
r := 'y';
else
r := 'z';
end case;
case
when k = 'q' then
r := 'w';
else
null;
end case;
return r;
end $$;`,
	`create function n() returns void language plpgsql as $$
begin
begin
insert into t values (1);
exception when unique_violation then
update t set a = 2 where id = 1;
end;
if exists (select 1 from t where a = 1) then
delete from t where a = 1;
end if;
end $$;`,
	`create function q() returns table (id int, v text) language plpgsql as $$
begin
return query select a.id, a.v from t a where a.id > 1 order by 1;
end $$;`,
	`create function c() returns int language plpgsql as $$
begin -- leading comment
if true then -- inside
return 1; /* block */
end if;
return 0;
end $$;`,
	`do $$
begin
perform 1;
if found then
raise exception 'boom';
end if;
end $$;`,
	"create function s() returns int language sql as $$ select a, b from t where x = 1 order by a $$;",
	"select a -- first col\n, b /* second */ , c\nfrom t -- the table\nwhere x = 1 -- cond one\n and y = 2 /* cond two */\n and z = 3 -- cond three\norder by a -- sort\n, b;",
	"select /* lead */ a, b from t /* before where */ where /* inside */ x = 1;",
	"select a,\n -- comment between items\n b,\n /* block between */\n c from t;",
	"select case -- c1\n when a > 1 then 'x' -- c2\n when a < 0 then 'y' /* c3 */\n else 'z' -- c4\n end as k from t;",
	"select a from t -- c1\nunion -- c2\nselect b from u /* c3 */;",
	"select a from t join u -- join comment\n on t.id = u.id -- on comment\n and t.x = u.x -- more\n where t.k = 1;",
	"insert into t (a, b) -- cols\nvalues (1, 2), -- row one\n (3, 4) -- row two\n;",
	"update t set a = 1, -- first\n b = 2 -- second\nwhere id = 3 -- key\n;",
	"with x as ( -- cte comment\n select 1 as a /* inline */ ) -- after cte\nselect * from x;",
	"-- leading comment\nselect 1; /* between statements */ select 2; -- trailing",
	"select a, /* c */ b from t where x in (1, -- one\n 2, /* two */ 3);",
	"select a from t where -- why\n x = 1 and -- more why\n y = 2;",
	"select a --\n, b from t;",
	"insert into t values (1, 2) -- one\n, (3, 4) -- two\n, (5, 6);",
	"select a from t where x = 1 -- one\n or y = 2 -- two\n or (z = 3 -- three\n and w = 4);",
	"select a, b from t -- tail one\n-- tail two\n;",
	"select coalesce(a, -- fallback\n b) as c, f(1, /* two */ 2) from t group by a -- g\n, b having count(*) > 1 -- h\n;",
	"select a from t order by a desc -- d\n limit 5 -- l\n offset 1 -- o\n;",
	"select a from t left join u on t.id = u.id -- lj\n left join v on v.id = u.id -- lj2\n;",
	"select (select max(x) -- inner\n from u) as m from t;",
	"select a from (select 1 as a -- inner\n) s;",
	"select a from t where x between 1 and 2 and y = 3 or z not between 4 and 5;",
	"select 1 from a join b on a.id = b.id and a.x between 1 and 2 join (select 1 as id from c join d on c.id = d.id) e on e.id = a.id and e.id > 0 left join f using (id) cross join g;",
	"select a, b, c from t where a = 1 and (b = 2 or c = 3) and d in (select 1 from u where u.x = 1 and u.y = 2) order by a, b;",
	"select a from t join u on t.id = u.id -- on1\n and t.x = u.x -- on2\n or t.y = u.y -- on3\n where a = 1 -- w1\n and b = 2 -- w2\n;",
	"select a -- c1\n, b -- c2\n, c from t group by a -- g1\n, b -- g2\n, c;",
	"insert into t (a, b, c) values (1, 2, 3), (4, 5, 6) returning a, b;",
	"update t set a = 1, b = 2 where c = 3 and d = 4 returning a, b;",
	"with x as (select 1 as a), y as (select 2 as b) select * from x, y;",
	"create table t (id int primary key, name varchar(20) not null default 'x', note text);",
	"create table if not exists public.accounts (id bigint not null generated always as identity, code character varying(64) not null default 'x', balance numeric(18, 4) not null default 0, created_at timestamp with time zone not null default now(), constraint pk primary key (id), constraint fk foreign key (code) references public.codes (code) on delete cascade) with (fillfactor = 70);",
	"create temp table if not exists tmp (a int, -- first\n b text /* inline */, c int[] default '{}', \"Mixed Case\" boolean not null, check (a > 0 and\n b <> ''));",
	"create unlogged table u (like t including all, extra int);\ncreate table p (id int, d date) partition by range (d);\ncreate table p1 partition of p for values from ('2020-01-01') to ('2021-01-01');",
	"create table x (a int, b int)",
	"alter table public.accounts add column notes text, drop column old_col, alter column code set not null, add constraint c check (a > 0 and b < 5);",
	"alter table only t alter column a type numeric(10, 2) using a::numeric, alter column b set default 1; alter table if exists t rename column a to b; alter table t rename to u; alter table all in tablespace x set tablespace y;",
	"alter table t add column a int, -- one\n add column b int /* two */",
	"-- lead\ncreate table c1 (a int); /* between */ create table c2 (b int, c int);",
	"select a, --first\n  bb, -- second\n  ccc --third\nfrom t; -- tail\n-- standalone\n--- banner ---\nselect 1;   -- one\nselect 2;     -- two\nselect 3;",
	"select 1;\n\n\n\nselect 2; -- c\n-- lead\nselect 3;select 4;\n\n-- end\n",
	"select a,\r\n  b -- c\r\nfrom t\r\nwhere x = 1;\r\n\r\nselect 2;\r\n",
	"create function f() returns int language plpgsql as $$\nbegin\n  return 1; --one\n  -- two\nend $$;\nselect 1;",
	"select f(a => 1, b := 2), x::int, cast(y as text), 'a' || 'b' from t where a != b and c <> d;",
}

func matrixInputs(t *testing.T) map[string]string {
	t.Helper()
	in := map[string]string{}
	files, _ := filepath.Glob("testdata/*/input.sql")
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		in[filepath.Base(filepath.Dir(f))] = string(b)
	}
	for i, s := range matrixInline {
		in[fmt.Sprintf("inline_%02d", i)] = s
	}
	return in
}

// TestConfigMatrix formats every input under every variant and checks that the
// result (1) parses when the input parses, (2) has the same AST fingerprint as
// the input formatted with the default (layout-preserving) config, and (3) is
// idempotent. It catches glued tokens, swallowed commas and similar breakage.
func TestConfigMatrix(t *testing.T) {
	inputs := matrixInputs(t)
	for name, src := range inputs {
		if _, err := pg_query.Parse(src); err != nil {
			continue // PL/pgSQL-only or partial snippets: semantic checks need a parseable input
		}
		base, err := config.Load("/nonexistent/.pg_procrustes.yaml")
		if err != nil {
			t.Fatal(err)
		}
		baseOut, err := formatter.Format(src, base)
		if err != nil {
			t.Fatalf("%s baseline: %v", name, err)
		}
		baseFP, err := pg_query.Fingerprint(baseOut)
		if err != nil {
			t.Fatalf("%s baseline output does not parse: %v\n%s", name, err, baseOut)
		}
		baseShape := plShape(baseOut)
		for _, v := range matrixVariants {
			t.Run(name+"/"+v.name, func(t *testing.T) {
				cfg, _ := config.Load("/nonexistent/.pg_procrustes.yaml")
				v.apply(cfg)
				out, err := formatter.Format(src, cfg)
				if err != nil {
					t.Fatalf("Format: %v", err)
				}
				fp, err := pg_query.Fingerprint(out)
				if err != nil {
					t.Fatalf("output does not parse: %v\n--- output ---\n%s", err, out)
				}
				if fp != baseFP {
					t.Errorf("AST fingerprint changed\n--- input ---\n%s\n--- output ---\n%s", src, out)
				}
				if shape := plShape(out); baseShape != "" && shape != baseShape { // an unparseable baseline (e.g. `;;` repaired by layout) has nothing to compare
					t.Errorf("PL/pgSQL structure changed\n--- input ---\n%s\n--- output ---\n%s", src, out)
				}
				out2, err := formatter.Format(out, cfg)
				if err != nil {
					t.Fatalf("second pass: %v\n--- first output ---\n%s", err, out)
				}
				if out2 != out {
					t.Errorf("not idempotent\n--- first ---\n%s\n--- second ---\n%s", out, out2)
				}
			})
		}
	}
}

var wsRE = regexp.MustCompile(`\s+`)

// plShape returns a normalised structural description of the PL/pgSQL functions in
// sql: statement structure and expressions with line numbers dropped and embedded SQL
// replaced by its AST fingerprint, so casing/spacing/layout differences vanish but any
// change in control flow or statements shows up. Empty when sql has no PL/pgSQL function.
func plShape(sql string) string {
	raw, err := pg_query.ParsePlPgSqlToJSON(sql)
	if err != nil || strings.TrimSpace(raw) == "" {
		return ""
	}
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return ""
	}
	var norm func(any) any
	norm = func(x any) any {
		switch t := x.(type) {
		case map[string]any:
			out := map[string]any{}
			for k, val := range t {
				switch k {
				case "lineno", "stmtid", "location":
					continue
				case "query":
					if q, ok := val.(string); ok {
						out[k] = normQuery(q)
						continue
					}
				}
				out[k] = norm(val)
			}
			return out
		case string:
			// identifiers and type names are case-insensitive and carry stray padding
			return strings.ToLower(strings.TrimSpace(t))
		case []any:
			out := make([]any, len(t))
			for i, e := range t {
				out[i] = norm(e)
			}
			return out
		}
		return x
	}
	b, _ := json.Marshal(norm(v))
	return string(b)
}

// normQuery fingerprints an embedded query/expression/assignment, falling back to
// lower-cased whitespace-free text when it cannot be parsed standalone.
func normQuery(q string) string {
	cands := []string{q, "SELECT " + q}
	if i := strings.Index(q, ":="); i >= 0 {
		cands = append(cands, "SELECT "+q[i+2:])
	}
	for _, c := range cands {
		if fp, err := pg_query.Fingerprint(c); err == nil {
			return "fp:" + fp
		}
	}
	return "txt:" + strings.ToLower(wsRE.ReplaceAllString(q, ""))
}
