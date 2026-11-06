package formatter_test

import (
	"strings"
	"testing"

	"github.com/heptau/pg_procrustes/config"
	"github.com/heptau/pg_procrustes/formatter"
)

// Found by corpus_check on the PostgreSQL regression tests.

// operator_spacing: compact must not glue two operators into a different one.
func TestCompactOperatorsKeepOperatorsApart(t *testing.T) {
	cfg, err := loadStyled("")
	if err != nil {
		t.Fatal(err)
	}
	cfg.OperatorSpacing = config.OperatorSpacingCompact
	src := "create operator === (leftarg = int4, rightarg = int4, commutator = ===);\nselect a = ~b, a = -1, x - -1, y = +2 from t;\n"
	got, err := formatter.Format(src, cfg)
	if err != nil {
		t.Fatalf("output does not parse: %v\n%s", err, got)
	}
	for _, want := range []string{"commutator= ===", "a= ~b", "a=-1", "x - -1", "y=+2"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	again, err := formatter.Format(got, cfg)
	if err != nil || again != got {
		t.Errorf("not idempotent: %v\n%s\n---\n%s", err, got, again)
	}
}

// A comment line before the leading comma of a VALUES row must not make the comma appear twice.
func TestLeadingCommaValuesWithCommentLine(t *testing.T) {
	cfg, err := loadStyled("")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Layout.Clauses.Break = config.BreakAlways
	cfg.Layout.Content.Break = config.BreakAlways
	cfg.Layout.Content.CommaPosition = config.CommaStart
	src := "insert into t values\n  (1) -- x\n  -- note\n  , (2)\n  , (3);\n"
	got, err := formatter.Format(src, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, ", ,") {
		t.Errorf("doubled comma:\n%s", got)
	}
	if !strings.Contains(got, "-- note") {
		t.Errorf("comment lost:\n%s", got)
	}
	again, err := formatter.Format(got, cfg)
	if err != nil || again != got {
		t.Errorf("not idempotent: %v\n%s\n---\n%s", err, got, again)
	}
}

// JOIN … USING (i) AS x names the join; aliases.as: remove must keep its AS even when
// another statement of the script has a table function aliased x.
func TestAliasAsRemoveKeepsJoinUsingAlias(t *testing.T) {
	cfg, err := loadStyled("")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Aliases.As = config.AliasAsRemove
	src := "select * from j1 join j2 using (i) as x where x.i = 1;\nselect * from unnest(array[1]) as x(a);\n"
	got, err := formatter.Format(src, cfg)
	if err != nil {
		t.Fatalf("output does not parse: %v\n%s", err, got)
	}
	if !strings.Contains(got, "USING (i) AS x") || strings.Contains(got, "unnest(ARRAY[1]) AS x") {
		t.Errorf("unexpected AS handling:\n%s", got)
	}
}

// With leading commas and aligned columns a comment line between two columns stayed on its
// own line on the first run and was pulled up behind the previous column on the second.
func TestCreateTableCommentLineBetweenColumnsIsStable(t *testing.T) {
	cfg, err := loadStyled("")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Layout.DDL.CreateTable = config.CreateTableCfg{Break: config.BreakAlways, Align: config.ColumnAlignNone}
	cfg.Layout.Content.CommaPosition = config.CommaStart
	src := "create table node (\n  nid serial,\n  created int not null default '0',\n  -- snip\n  other int\n);\n"
	got, err := formatter.Format(src, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "'0' -- snip") || !strings.Contains(got, "\n   -- snip\n") {
		t.Errorf("comment moved:\n%s", got)
	}
	again, err := formatter.Format(got, cfg)
	if err != nil || again != got {
		t.Errorf("not idempotent: %v\n%s\n---\n%s", err, got, again)
	}
}

// A commented-out last row (`--, (5, 6)` after the final row) is not an item: it must not get
// a separating comma, which would leave `,` right before the `;`.
func TestCommentedOutLastRowDoesNotLeaveDanglingComma(t *testing.T) {
	for _, pos := range []config.CommaPosition{config.CommaStart, config.CommaEnd} {
		cfg, err := loadStyled("")
		if err != nil {
			t.Fatal(err)
		}
		cfg.Layout.Clauses.Break = config.BreakAlways
		cfg.Layout.Content.Break = config.BreakAlways
		cfg.Layout.Content.CommaPosition = pos
		src := "insert into t (a, b)\nvalues (1, 2)\n     , (3, 4)\n     -- wrong entry\n     --, (5, 6)\n;\n"
		got, err := formatter.Format(src, cfg)
		if err != nil {
			t.Fatalf("%s: %v", pos, err)
		}
		if _, err := formatter.Format(got, cfg); err != nil {
			t.Errorf("%s: output does not parse: %v\n%s", pos, err, got)
		}
		if !strings.Contains(got, "-- wrong entry") || !strings.Contains(got, "--, (5, 6)") {
			t.Errorf("%s: comment lost:\n%s", pos, got)
		}
	}
}

// What is inside a multi-line string constant is data: no layout or whitespace rule may
// re-indent it (it got an extra indent under `modern`) or squeeze its blank lines
// (`blank_lines: max_1`). AST fingerprints ignore constant values, so this is checked by text.
func TestMultilineStringConstantsAreNeverChanged(t *testing.T) {
	src := "select 'a\nb  c' as x, 2 from t where y = 'p\n\n\n\n  q' and z in ('m\n    n', 'o');\n\n\n\n\nselect e'l1\n\tl2  \n' as w;\n"
	styles := map[string]func(c *config.Config){
		"layout always + leading commas": func(c *config.Config) {
			c.Layout.Clauses.Break = config.BreakAlways
			c.Layout.Content.Break = config.BreakAlways
			c.Layout.Content.CommaPosition = config.CommaStart
		},
		"tabs and indent normalization": func(c *config.Config) {
			c.Layout.Clauses.Break = config.BreakAlways
			c.Layout.Content.Break = config.BreakAlways
			c.Layout.Indent.Type = config.IndentTypeTab
			c.Layout.Indent.Normalize = config.IndentNormalizeChange
		},
		"blank lines max_1": func(c *config.Config) { c.BlankLines = config.BlankLinesMax1 },
	}
	for name, apply := range styles {
		cfg, err := loadStyled("")
		if err != nil {
			t.Fatal(err)
		}
		apply(cfg)
		got, err := formatter.Format(src, cfg)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for _, lit := range []string{"'a\nb  c'", "'p\n\n\n\n  q'", "'m\n    n'", "e'l1\n\tl2  \n'"} {
			if !strings.Contains(got, lit) {
				t.Errorf("%s: constant %q changed:\n%s", name, lit, got)
			}
		}
	}
}

// The parser reports a SIMILAR TO as a call of pg_catalog.similar_to_escape located at the
// keyword; "location + len(schema) + 1" then hit the pattern and its capitals were lowered.
func TestSimilarToPatternKeepsItsCase(t *testing.T) {
	cfg, err := loadStyled("")
	if err != nil {
		t.Fatal(err)
	}
	src := "select 1 where a similar to 'AB%' and b similar to '%A.*' escape 'X';\n"
	got, err := formatter.Format(src, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "'AB%'") || !strings.Contains(got, "'%A.*'") || !strings.Contains(got, "'X'") {
		t.Errorf("pattern changed:\n%s", got)
	}
}

// The body of a function in another language is code of that language: PL/Python text came
// out as SQL (`plan` became PLAN, `["int4"]` lost its quotes, a `;` was added).
func TestBodiesOfOtherLanguagesAreLeftAlone(t *testing.T) {
	cfg, err := loadStyled("")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Semicolons = config.SemicolonAdd
	py := "plan = plpy.prepare(\"select $1\", [\"int4\"])\nplpy.execute(plan, C())\n"
	pl := "my $x = shift;\nreturn $x;\n"
	src := "create function f() returns int as $$\n" + py + "$$ language plpythonu;\n" +
		"create function g() returns int language plperl as $$\n" + pl + "$$;\n" +
		"do language plpython3u $$\n" + py + "$$;\n" +
		"create function h() returns int as $$ select 1 $$ language sql;\n"
	got, err := formatter.Format(src, cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{py, pl} {
		if !strings.Contains(got, body) {
			t.Errorf("body changed: %q\n%s", body, got)
		}
	}
	if !strings.Contains(got, "SELECT 1") {
		t.Errorf("the SQL function body was not formatted:\n%s", got)
	}
}
