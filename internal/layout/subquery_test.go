package layout_test

import (
	"testing"

	"github.com/heptau/pg_procrustes/config"
	"github.com/heptau/pg_procrustes/internal/layout"
)

func subqueryCfg(brk config.BreakMode, clauses config.BreakMode, lineLen int) *config.LayoutConfig {
	cfg := caseCfg(config.BreakPreserve, config.BodyIndentIndent, lineLen)
	cfg.Clauses.Break = clauses
	cfg.Subquery.Break = brk
	return cfg
}

func assertSubqueryIdempotent(t *testing.T, sql string, cfg *config.LayoutConfig) {
	t.Helper()
	once, err := layout.Apply(sql, cfg)
	if err != nil {
		t.Fatal(err)
	}
	twice, err := layout.Apply(once, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if once != twice {
		t.Errorf("not idempotent\nfirst:  %q\nsecond: %q", once, twice)
	}
}

func TestSubqueryPreserveLeavesTextAlone(t *testing.T) {
	sql := "select * from (select a\nfrom b) s"
	cfg := subqueryCfg(config.BreakPreserve, config.BreakPreserve, 80)
	assertLayoutFormat(t, sql, cfg, sql)
}

func TestSubqueryAlwaysOpensFromAndWhere(t *testing.T) {
	cfg := subqueryCfg(config.BreakAlways, config.BreakAlways, 80)
	sql := "select * from (select id from u where a) s where id in (select id from v)"
	want := "select *\n" +
		"from (\n" +
		"  select id\n" +
		"  from u\n" +
		"  where a\n" +
		") s\n" +
		"where id in (\n" +
		"  select id\n" +
		"  from v\n" +
		")"
	assertLayoutFormat(t, sql, cfg, want)
	assertSubqueryIdempotent(t, sql, cfg)
}

func TestSubqueryCTEs(t *testing.T) {
	cfg := subqueryCfg(config.BreakAlways, config.BreakAlways, 80)
	sql := "with a as (select 1), b as (select 2) select * from a"
	want := "with a as (\n" +
		"  select 1\n" +
		"), b as (\n" +
		"  select 2\n" +
		")\n" +
		"select *\n" +
		"from a"
	assertLayoutFormat(t, sql, cfg, want)
	assertSubqueryIdempotent(t, sql, cfg)
}

func TestSubqueryNested(t *testing.T) {
	cfg := subqueryCfg(config.BreakAlways, config.BreakAlways, 80)
	sql := "select * from (select * from (select 1) i) o"
	want := "select *\n" +
		"from (\n" +
		"  select *\n" +
		"  from (\n" +
		"    select 1\n" +
		"  ) i\n" +
		") o"
	assertLayoutFormat(t, sql, cfg, want)
	assertSubqueryIdempotent(t, sql, cfg)
}

func TestSubqueryNeverCollapses(t *testing.T) {
	cfg := subqueryCfg(config.BreakNever, config.BreakPreserve, 80)
	sql := "select * from (\n  select a\n  from b\n) s"
	assertLayoutFormat(t, sql, cfg, "select * from (select a from b) s")
	assertSubqueryIdempotent(t, sql, cfg)
}

func TestSubqueryAutoOnlyWhenLineTooLong(t *testing.T) {
	cfg := subqueryCfg(config.BreakAuto, config.BreakPreserve, 40)
	short := "select * from (select 1) s"
	assertLayoutFormat(t, short, cfg, short)
	long := "select * from (select aaaaaaaaaa, bbbbbbbbbb from cccccccccc) s"
	want := "select * from (\n  select aaaaaaaaaa, bbbbbbbbbb from cccccccccc\n) s"
	assertLayoutFormat(t, long, cfg, want)
	assertSubqueryIdempotent(t, long, cfg)
}

func TestSubqueryWithCommentIsLeftAlone(t *testing.T) {
	cfg := subqueryCfg(config.BreakAlways, config.BreakPreserve, 80)
	sql := "select * from (select 1 -- note\nfrom t) s"
	assertLayoutFormat(t, sql, cfg, sql)
}

func TestSubqueryListIsBrokenWhenItOpens(t *testing.T) {
	cfg := subqueryCfg(config.BreakAlways, config.BreakPreserve, 120)
	cfg.Content.Break = config.BreakAuto
	cfg.Clauses.Break = config.BreakAlways
	sql := "select * from t where a = 1 and b in (select 1)"
	want := "select *\n" +
		"from t\n" +
		"where\n" +
		"  a = 1\n" +
		"  and b in (\n" +
		"    select 1\n" +
		"  )"
	assertLayoutFormat(t, sql, cfg, want)
	assertSubqueryIdempotent(t, sql, cfg)
}

func TestSubqueryLeadingCommaIndent(t *testing.T) {
	cfg := subqueryCfg(config.BreakAlways, config.BreakAlways, 120)
	cfg.Content.Break = config.BreakAuto
	cfg.Content.SelectList = config.ContentRule{Break: config.BreakAlways}
	cfg.Content.CommaPosition = config.CommaStart
	sql := "select a, (select max(x) from t) as m from s"
	want := "select\n" +
		"  a\n" +
		", (\n" +
		"    select\n" +
		"      max(x)\n" +
		"    from t\n" +
		"  ) as m\n" +
		"from s"
	assertLayoutFormat(t, sql, cfg, want)
	assertSubqueryIdempotent(t, sql, cfg)
}

func TestParenOpenOwnLine(t *testing.T) {
	cfg := subqueryCfg(config.BreakAlways, config.BreakAlways, 80)
	cfg.ParenIndent.Open = config.ParenOpenOwnLine
	sql := "select * from (select a from b where x in (select 1)) s where id in (select 2)"
	want := "select *\n" +
		"from\n" +
		"(\n" +
		"  select a\n" +
		"  from b\n" +
		"  where x in\n" +
		"  (\n" +
		"    select 1\n" +
		"  )\n" +
		") s\n" +
		"where id in\n" +
		"(\n" +
		"  select 2\n" +
		")"
	assertLayoutFormat(t, sql, cfg, want)
	assertSubqueryIdempotent(t, sql, cfg)
}

func TestParenOpenPreserveLeavesParens(t *testing.T) {
	cfg := subqueryCfg(config.BreakAlways, config.BreakAlways, 80)
	sql := "select * from (select 1) s"
	assertLayoutFormat(t, sql, cfg, "select *\nfrom (\n  select 1\n) s")
}

func TestParenOpenOwnLineLeavesCommasAndOwnLineParens(t *testing.T) {
	cfg := subqueryCfg(config.BreakPreserve, config.BreakPreserve, 80)
	cfg.ParenIndent.Open = config.ParenOpenOwnLine
	sql := "select a,\n  (\n    select 1\n  ) as x,\n  f(\n    1\n  )\nfrom t"
	want := "select a,\n  (\n    select 1\n  ) as x,\n  f\n  (\n    1\n  )\nfrom t"
	assertLayoutFormat(t, sql, cfg, want)
}
