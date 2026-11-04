package formatter_test

import (
	"os"
	"path/filepath"
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
	{"keywords_lower", func(c *config.Config) {
		c.ReservedKeywords.Case = config.CaseLower
		c.Keywords.Case = config.CaseLower
	}},
}

// matrixInline are extra statements exercising constructs the golden inputs miss
// (comments, set operations, CASE, quoted identifiers, arithmetic).
var matrixInline = []string{
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
		in["inline_"+string(rune('a'+i))] = s
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
