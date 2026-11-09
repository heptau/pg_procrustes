package layout_test

import (
	"strings"
	"testing"

	"github.com/heptau/pg_procrustes/config"
	"github.com/heptau/pg_procrustes/internal/layout"
)

func applyIdem(t *testing.T, sql string, cfg *config.LayoutConfig) string {
	t.Helper()
	got, err := layout.Apply(sql, cfg)
	if err != nil {
		t.Fatal(err)
	}
	again, err := layout.Apply(got, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if again != got {
		t.Errorf("not idempotent:\nfirst:  %q\nsecond: %q", got, again)
	}
	return got
}

func TestLockingClauseOnItsOwnLine(t *testing.T) {
	cfg := parenItemCfg()
	got := applyIdem(t, "SELECT a FROM t WHERE id = 1 AND x <> TRUE FOR UPDATE;", cfg)
	if !strings.HasSuffix(got, "\nFOR UPDATE;") {
		t.Errorf("FOR UPDATE not on its own line: %q", got)
	}
	// FOR UPDATE of CREATE POLICY and FOR EACH ROW are not locking clauses.
	for _, sql := range []string{
		"CREATE POLICY p ON t FOR UPDATE USING (id IN (SELECT 1));",
		"CREATE TRIGGER x BEFORE UPDATE ON t FOR EACH ROW EXECUTE FUNCTION f();",
	} {
		if got := applyIdem(t, sql, cfg); got != sql {
			t.Errorf("changed %q to %q", sql, got)
		}
	}
}

func TestCreateFunctionLayoutIgnoresLeadingComment(t *testing.T) {
	cfg := caseCfg(config.BreakPreserve, config.BodyIndentPreserve, 128)
	cfg.DDL.CreateFunction.Params = config.BreakAuto
	cfg.DDL.CreateFunction.Options = config.BreakAlways
	got := applyIdem(t, "-- note\nCREATE FUNCTION f(_a INTEGER) RETURNS INTEGER LANGUAGE sql AS $$ SELECT 1 $$;", cfg)
	want := "-- note\nCREATE FUNCTION f(_a INTEGER)\nRETURNS INTEGER\nLANGUAGE sql\nAS $$ SELECT 1 $$;"
	if got != want {
		t.Errorf("\ngot:  %q\nwant: %q", got, want)
	}
}

func TestReturnsTableBreaksWhenTooLong(t *testing.T) {
	cfg := caseCfg(config.BreakPreserve, config.BodyIndentPreserve, 60)
	cfg.DDL.CreateFunction.Params = config.BreakAuto
	cfg.DDL.CreateFunction.Options = config.BreakAlways
	got := applyIdem(t, "CREATE FUNCTION f() RETURNS TABLE(id_client INTEGER, user_login TEXT, client_comment_text TEXT) LANGUAGE sql AS $$ SELECT 1 $$;", cfg)
	want := "CREATE FUNCTION f()\nRETURNS TABLE(\n  id_client INTEGER,\n  user_login TEXT,\n  client_comment_text TEXT\n)\nLANGUAGE sql\nAS $$ SELECT 1 $$;"
	if got != want {
		t.Errorf("\ngot:  %q\nwant: %q", got, want)
	}
	short := applyIdem(t, "CREATE FUNCTION f() RETURNS TABLE(a INTEGER) LANGUAGE sql AS $$ SELECT 1 $$;", cfg)
	if short != "CREATE FUNCTION f()\nRETURNS TABLE(a INTEGER)\nLANGUAGE sql\nAS $$ SELECT 1 $$;" {
		t.Errorf("short RETURNS TABLE changed: %q", short)
	}
}

func TestCallArgumentsStayBelowTheCall(t *testing.T) {
	cfg := parenItemCfg()
	in := "PERFORM\n    f(\n    _a => 1,\n    _b => 2);"
	got := applyIdem(t, "PERFORM f(\n    _a => 1,\n    _b => 2);", cfg)
	want := "PERFORM\n    f(\n        _a => 1,\n        _b => 2);"
	if got != want {
		t.Errorf("\nin:   %q\ngot:  %q\nwant: %q", in, got, want)
	}
}

func TestForOrderByIsNotAClause(t *testing.T) {
	cfg := parenItemCfg()
	sql := "CREATE OPERATOR CLASS c FOR TYPE g USING gist AS\n    OPERATOR 13 <-> FOR ORDER BY pg_catalog.float_ops,\n    OPERATOR 14 <#> FOR ORDER BY pg_catalog.float_ops;"
	if got := applyIdem(t, sql, cfg); strings.Contains(got, "\nORDER BY") {
		t.Errorf("FOR ORDER BY treated as a clause: %q", got)
	}
}

func TestCommentAfterKeywordHangsContent(t *testing.T) {
	cfg := parenItemCfg()
	cfg.Clauses.Break = config.BreakAlways
	for _, sql := range []string{
		"SELECT a FROM t\nWHERE --c\n  a IS NOT NULL\nORDER BY a;",
		"SELECT a FROM t WHERE --c\na IS NOT NULL;",
	} {
		got := applyIdem(t, sql, cfg)
		if strings.Contains(got, "\na IS NOT NULL") {
			t.Errorf("content after the comment stands at the margin: %q", got)
		}
	}
}

func TestSetOperationAfterParenthesisStartsNoBlankLines(t *testing.T) {
	cfg := parenItemCfg()
	cfg.Clauses.Break = config.BreakAlways
	cfg.Subquery.Break = config.BreakAlways
	for _, bl := range []config.UnionBlankLine{config.UnionBlankLineNone, config.UnionBlankLineBefore, config.UnionBlankLineAfter, config.UnionBlankLineBoth} {
		cfg.Union.BlankLine = bl
		got := applyIdem(t, "(SELECT 1 UNION SELECT 4) INTERSECT SELECT 4;", cfg)
		if strings.HasPrefix(got, "\n") {
			t.Errorf("union.blank_line %s: output starts with a line break: %q", bl, got)
		}
	}
}

func TestWrapDecidesOnTheUnpaddedLength(t *testing.T) {
	cfg := parenItemCfg()
	cfg.LineLength = 60
	cfg.Content.WrapExpressions = config.WrapWrap
	cfg.Clauses.Break = config.BreakAlways
	cfg.Content.Break = config.BreakAlways
	sql := "SELECT a FROM (VALUES (1, 'bbbbbbbbbbbbbbbbbbb'      , 'cccccccccccccccccc'      , 'dd')) v;"
	applyIdem(t, sql, cfg)
}

func TestSubqueryAutoBreakIsStable(t *testing.T) {
	cfg := parenItemCfg()
	cfg.LineLength = 60
	cfg.Subquery.Break = config.BreakAuto
	cfg.ParenIndent.Open = config.ParenOpenOwnLine
	for _, sql := range []string{
		"SELECT * FROM (SELECT * FROM scan_table s WHERE a = 1 OFFSET 0);",
		"select x from (values (row(1, 2)), (row(1, 3))) _(x) union select x from (values (row(1, 2)), (row(1, 4))) _(x);",
		"SELECT t1.a, t2.b FROM (SELECT * FROM prt1 WHERE a < 450) t1 LEFT JOIN (SELECT * FROM prt2 WHERE b > 250) t2 ON t1.a = t2.b WHERE t1.b = 0;",
		"SELECT v AS value FROM   (VALUES (row('10'::varbit, 'aaa')::hash_test_t2)) x(v);",
	} {
		applyIdem(t, sql, cfg)
	}
	cfg.ParenIndent.Open = config.ParenOpenPreserve
	cfg.LineLength = 128
	// tabs are counted as the blanks indent.normalize turns them into
	cfg.Indent.Normalize = config.IndentNormalizeChange
	applyIdem(t, "SELECT t1.a FROM prt1 t1 LEFT JOIN LATERAL\n\t\t\t  (SELECT t2.a AS t2a, t3.a AS t3a, t2.b t2b, t2.c t2c, least(t1.a,t2.a,t3.a) FROM prt1 t2 JOIN prt2 t3 ON (t2.a = t3.b)) ss\n\t\t\t  ON t1.c = ss.t2c;", cfg)
}

func TestLineAfterCommentInFlattenedItemIsIndented(t *testing.T) {
	cfg := parenItemCfg()
	cfg.LineLength = 80
	cfg.Clauses.Break = config.BreakAlways
	cfg.Content.Break = config.BreakAlways
	cfg.ParenIndent.Open = config.ParenOpenOwnLine
	cfg.Subquery.Break = config.BreakNever
	cfg.Indent.Normalize = config.IndentNormalizeChange
	sql := "SELECT\n\tloader_macro_replace(\n\t\treplace(\n\t\t\tf(a\n\t\t\t\t, ARRAY['x'],\n\t\t\t\tARRAY[v]\n\t\t\t), '/', p) || '\n'  ||\n\tarray_to_string( ARRAY(SELECT lmr('cd ' || replace(v,'/', p) || '\n' || p.wget || '/'\n-- hardcoding zcta5 path since doesn't follow convention\n|| upper(t) || '/tl_'\n|| upper(t) || 'z'\n\t\t|| c(E'\\n' || lu.post , '') , ARRAY['loader'], ARRAY[p]\n\t\t\t)\n\t\t\t\tORDER BY a, b), E'\\n') ::text\n\t, ARRAY['psql'])\nFROM t;"
	applyIdem(t, sql, cfg)
}

// A DDL statement joined onto one line by `break: never` takes paren_spacing: remove
// into account, so no space is left inside the parentheses it had on separate lines.
func TestDDLFlattenHonoursParenSpacingRemove(t *testing.T) {
	cfg := &config.LayoutConfig{ParenSpacing: config.ParenSpacingRemove}
	cfg.DDL.AlterTable.Break = config.BreakNever
	got := applyIdem(t, "ALTER TABLE t ALTER COLUMN c ADD GENERATED ALWAYS AS IDENTITY (\n  START WITH 1\n  INCREMENT BY 1\n);\n", cfg)
	want := "ALTER TABLE t ALTER COLUMN c ADD GENERATED ALWAYS AS IDENTITY (START WITH 1 INCREMENT BY 1);\n"
	if got != want && got != strings.TrimSuffix(want, "\n") {
		t.Errorf("got %q want %q", got, want)
	}
}

// Call arguments with tabs for the indent unit and the keyword-aligned (river) margin of
// `clauses.align: right`: the second run finds the first one's indent where it is.
func TestCallArgumentsWithTabsAndRiverAlignAreStable(t *testing.T) {
	cfg := caseCfg(config.BreakPreserve, config.BodyIndentPreserve, 80)
	cfg.Indent.Type = config.IndentTypeTab
	cfg.Clauses.Break = config.BreakAuto
	cfg.Clauses.Align = config.AlignRight
	cfg.Content.Break = config.BreakAlways
	got := applyIdem(t, "SELECT hmac(\n'" + strings.Repeat("a", 90) + "'::bytea,\n'md5');\n", cfg)
	if !strings.Contains(got, "\n       \t'aaaa") {
		t.Errorf("unexpected indent of the arguments: %q", got)
	}
}

// A comment line above a leading comma belongs to the item after it, so the columns of a
// comma_position: start table stay aligned on the second run.
func TestCommentAboveLeadingCommaBelongsToNextItem(t *testing.T) {
	cfg := caseCfg(config.BreakPreserve, config.BodyIndentPreserve, 128)
	cfg.Content.CommaPosition = config.CommaStart
	cfg.DDL.CreateTable.Break = config.BreakAlways
	cfg.DDL.CreateTable.Align = config.ColumnAlignAll
	got := applyIdem(t, "CREATE TABLE node (\n    nid serial,\n    created integer NOT NULL,\n    -- snip\n    PRIMARY KEY (nid)\n);\n", cfg)
	if !strings.Contains(got, "nid     serial") {
		t.Errorf("columns not aligned: %q", got)
	}
}

// A statement standing at an indent of blanks, with tabs for the indent unit: the body of a
// parenthesis it opens moves behind the blanks of the statement ("<blanks><tab>"), which the
// second run finds in place.
func TestIndentedStatementWithTabsKeepsItsBlanksFirst(t *testing.T) {
	cfg := caseCfg(config.BreakPreserve, config.BodyIndentPreserve, 128)
	cfg.Indent.Type = config.IndentTypeTab
	cfg.Clauses.Break = config.BreakAuto
	cfg.Content.Break = config.BreakAlways
	cfg.Content.FirstItem = config.FirstItemInline
	name := "f.g_" + strings.Repeat("a", 130)
	got := applyIdem(t, "-- c\n    SELECT "+name+"\n    (\n        _a => 1\n    );\n", cfg)
	if !strings.Contains(got, "\n    \t(\n    \t    _a => 1\n    \t);") {
		t.Errorf("unexpected indent: %q", got)
	}
}

// With tabs for the indent unit and the river margin of `clauses.align: right` (blanks), the body of
// a subquery in a condition stands at "<river blanks><tab>", the order the second run finds again.
func TestSubqueryBodyInRiverWithTabsIsStable(t *testing.T) {
	cfg := caseCfg(config.BreakPreserve, config.BodyIndentPreserve, 30)
	cfg.Indent.Type = config.IndentTypeTab
	cfg.Clauses.Break = config.BreakAuto
	cfg.Clauses.Align = config.AlignRight
	cfg.Content.Break = config.BreakAlways
	got := applyIdem(t, "UPDATE t\nSET a = 4\nWHERE b = 3\n    AND id IN (\n        SELECT id\n        FROM e\n        )\nRETURNING a;\n", cfg)
	if !strings.Contains(got, "\n          \tSELECT id\n") {
		t.Errorf("unexpected indent: %q", got)
	}
}

// The operand of a simple CASE that is a multi-line parenthesised CASE is joined onto one line,
// so the lines of it do not keep an indent the rest of the expression has left.
func TestSimpleCaseOperandIsJoinedOntoOneLine(t *testing.T) {
	cfg := caseCfg(config.BreakAlways, config.BodyIndentPreserve, 128)
	cfg.Clauses.Break = config.BreakAlways
	cfg.Clauses.Align = config.AlignRight
	cfg.Content.Break = config.BreakAlways
	cfg.ParenIndent.Open = config.ParenOpenOwnLine
	got := applyIdem(t, "SELECT CASE\n  (CASE v('b')\n    WHEN 'f' THEN 'x' END\n  )\n  WHEN 'x' THEN 'y'\n  ELSE 'z' END;\n", cfg)
	if !strings.Contains(got, "CASE (CASE v('b') WHEN 'f' THEN 'x' END )") {
		t.Errorf("operand not on one line: %q", got)
	}
}
