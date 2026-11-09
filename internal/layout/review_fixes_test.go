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
