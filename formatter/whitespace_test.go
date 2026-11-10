package formatter_test

import (
	pg_query "github.com/pganalyze/pg_query_go/v6"

	"errors"
	"strings"
	"testing"

	"github.com/heptau/pg_procrustes/config"
	"github.com/heptau/pg_procrustes/formatter"
)

func wsCfg(t *testing.T, mutate func(*config.Config)) *config.Config {
	t.Helper()
	c, err := loadStyled("")
	if err != nil {
		t.Fatal(err)
	}
	mutate(c)
	if err := config.Validate(c); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestLineEndings(t *testing.T) {
	crlf := "select a,\r\n  b\r\nfrom t;\r\n"
	lf := "select a,\n  b\nfrom t;\n"
	for _, tc := range []struct {
		name string
		mode config.LineEndingMode
		in   string
		want string
	}{
		{"preserve keeps CRLF", config.LineEndingPreserve, crlf, "SELECT a,\r\n  b\r\nFROM t;\r\n"},
		{"preserve keeps LF", config.LineEndingPreserve, lf, "SELECT a,\n  b\nFROM t;\n"},
		{"lf converts CRLF", config.LineEndingLF, crlf, "SELECT a,\n  b\nFROM t;\n"},
		{"crlf converts LF", config.LineEndingCRLF, lf, "SELECT a,\r\n  b\r\nFROM t;\r\n"},
	} {
		c := wsCfg(t, func(c *config.Config) { c.LineEndings = tc.mode })
		got, err := formatter.Format(tc.in, c)
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Errorf("%s: got %q want %q", tc.name, got, tc.want)
		}
		if again, _ := formatter.Format(got, c); again != got {
			t.Errorf("%s: not idempotent: %q", tc.name, again)
		}
	}
	// Errors keep their line numbers when the file is CRLF.
	_, err := formatter.Format("select 1;\r\nselect from from;\r\n", wsCfg(t, func(*config.Config) {}))
	var se formatter.SyntaxErrors
	if !errors.As(err, &se) || se[0].Line != 2 || se[0].Col != 13 {
		t.Errorf("CRLF diagnostics wrong: %v", err)
	}
}

func TestCommentSpaceAndAlign(t *testing.T) {
	c := wsCfg(t, func(c *config.Config) {
		c.Comments = config.CommentsCfg{SpaceAfterMarker: config.CommentSpaceAdd, AlignTrailing: config.CommentAlignAlign}
	})
	in := "select a, --first\n  bb, -- second\n  ccc --third\nfrom t; -- tail\n--- banner ---\n--+ hint\nselect 1;   -- lone\n"
	want := "SELECT a, -- first\n  bb,     -- second\n  ccc     -- third\nFROM t;   -- tail\n--- banner ---\n--+ hint\nSELECT 1;   -- lone\n"
	got, err := formatter.Format(in, c)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	if again, _ := formatter.Format(got, c); again != got {
		t.Fatalf("not idempotent:\n%s", again)
	}
	// Defaults leave comments alone.
	d := wsCfg(t, func(*config.Config) {})
	if out, _ := formatter.Format("select 1; --note\n", d); !strings.Contains(out, "--note") {
		t.Fatalf("default must keep --note: %q", out)
	}
}

func TestBlankLinesBetweenStatements(t *testing.T) {
	in := "select 1;\n\n\n\nselect 2; -- c\n-- lead\nselect 3;select 4;\n\\set x 1\nselect 5;\n\n-- end\n"
	for n, want := range map[string]string{
		"0": "SELECT 1;\nSELECT 2; -- c\n-- lead\nSELECT 3;\nSELECT 4;\n\\set x 1\nSELECT 5;\n-- end\n",
		"1": "SELECT 1;\n\nSELECT 2; -- c\n\n-- lead\nSELECT 3;\n\nSELECT 4;\n\n\\set x 1\nSELECT 5;\n\n-- end\n",
	} {
		c := wsCfg(t, func(c *config.Config) { c.BlankLinesBetweenStatements = n })
		got, err := formatter.Format(in, c)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("n=%s:\ngot:\n%q\nwant:\n%q", n, got, want)
		}
		if again, _ := formatter.Format(got, c); again != got {
			t.Errorf("n=%s not idempotent: %q", n, again)
		}
	}
	// preserve (default) keeps whatever the source has.
	d := wsCfg(t, func(*config.Config) {})
	if got, _ := formatter.Format("select 1;\n\n\n\nselect 2;\n", d); got != "SELECT 1;\n\n\n\nSELECT 2;\n" {
		t.Fatalf("preserve changed the blank lines: %q", got)
	}
}

func TestWhitespaceOptionsValidated(t *testing.T) {
	for name, mutate := range map[string]func(*config.Config){
		"line_endings":       func(c *config.Config) { c.LineEndings = "mac" },
		"space_after_marker": func(c *config.Config) { c.Comments.SpaceAfterMarker = "remove" },
		"align_trailing":     func(c *config.Config) { c.Comments.AlignTrailing = "right" },
		"blank_lines_between_statements": func(c *config.Config) {
			c.BlankLinesBetweenStatements = "many"
		},
	} {
		c, _ := loadStyled("")
		mutate(c)
		if config.Validate(c) == nil {
			t.Errorf("%s: expected a validation error", name)
		}
	}
}

func TestCommentAlignNoneCollapsesGap(t *testing.T) {
	c, err := config.Load("/nonexistent/.pg_procrustes.yaml")
	if err != nil {
		t.Fatal(err)
	}
	c.Comments.AlignTrailing = config.CommentAlignNone
	in := "select a,      -- one\n  b          -- two\n-- own line\nfrom t;\n"
	want := "select a, -- one\n  b -- two\n-- own line\nfrom t;\n"
	got, err := formatter.Format(in, c)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
	again, _ := formatter.Format(got, c)
	if again != got {
		t.Errorf("not idempotent: %q", again)
	}
}

func TestStatementBreakAlways(t *testing.T) {
	c, err := config.Load("/nonexistent/.pg_procrustes.yaml")
	if err != nil {
		t.Fatal(err)
	}
	in := "select 1; select 2; -- note\nselect 3;\n\n\nselect 4; /* c */ select 5;   select 6\n"
	// preserve (default) leaves everything as written
	if got, err := formatter.Format(in, c); err != nil || got != in {
		t.Fatalf("preserve changed the input: %q (%v)", got, err)
	}
	c.StatementBreak = config.StatementBreakAlways
	want := "select 1;\nselect 2; -- note\nselect 3;\n\n\nselect 4; /* c */\nselect 5;\nselect 6\n"
	got, err := formatter.Format(in, c)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
	if again, _ := formatter.Format(got, c); again != got {
		t.Errorf("not idempotent: %q", again)
	}
}

func TestTrailingNewlineStripLeavesDollarBodyAlone(t *testing.T) {
	c, err := config.Load("/nonexistent/.pg_procrustes.yaml")
	if err != nil {
		t.Fatal(err)
	}
	c.TrailingNewline = config.TrailingNewlineStrip
	in := "create function g() returns int as $$\nbegin\n  return 1;\nend;\n$$ language plpgsql;\n"
	want := "create function g() returns int as $$\nbegin\n  return 1;\nend;\n$$ language plpgsql;"
	got, err := formatter.Format(in, c)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestCompactCommasSurviveLayoutRejoin(t *testing.T) {
	c, err := config.Load("/nonexistent/.pg_procrustes.yaml")
	if err != nil {
		t.Fatal(err)
	}
	c.CommaSpacing = config.CommaSpacingCompact
	c.Layout.Clauses.Break = config.BreakAlways
	for _, in := range []string{
		"select id,\n name from users;",
		"create function f() returns int as $$\nbegin\n  select id,\n    name from users;\n  return 1;\nend\n$$ language plpgsql;",
	} {
		got, err := formatter.Format(in, c)
		if err != nil {
			t.Fatal(err)
		}
		if again, _ := formatter.Format(got, c); again != got {
			t.Errorf("not idempotent:\n%s\n---\n%s", got, again)
		}
		if strings.Contains(got, ", ") {
			t.Errorf("comma gap left in %q", got)
		}
	}
}

// trailing_newline: add is a whole-file rule: it must not put a line break in front of the
// ';' of a statement inside a function body.
func TestTrailingNewlineAddNotAppliedToBodyStatements(t *testing.T) {
	c, err := config.Load("/nonexistent/.pg_procrustes.yaml")
	if err != nil {
		t.Fatal(err)
	}
	c.TrailingNewline = config.TrailingNewlineAdd
	c.Layout.Clauses.Break = config.BreakAlways
	in := "create function f() returns void as $$\nbegin\n  update t set a = 1 where id = 2;\nend\n$$ language plpgsql;\n"
	got, err := formatter.Format(in, c)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "\n;") {
		t.Errorf("statement ';' pushed onto its own line:\n%s", got)
	}
	if again, _ := formatter.Format(got, c); again != got {
		t.Errorf("not idempotent:\n%s\n---\n%s", got, again)
	}
}

// Subqueries inside a function body are opened too, and keep the body's indent.
func TestSubqueryAlwaysInsideFunctionBody(t *testing.T) {
	c, err := config.Load("/nonexistent/.pg_procrustes.yaml")
	if err != nil {
		t.Fatal(err)
	}
	c.Layout.Clauses.Break = config.BreakAlways
	c.Layout.Subquery.Break = config.BreakAlways
	in := "create function f() returns void as $$\nbegin\n  delete from w where k in (select k from z);\nend\n$$ language plpgsql;\n"
	got, err := formatter.Format(in, c)
	if err != nil {
		t.Fatal(err)
	}
	want := "create function f() returns void as $$\nbegin\n  delete from w\n  where k in (\n    select k\n    from z\n  );\nend\n$$ language plpgsql;\n"
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
	if again, _ := formatter.Format(got, c); again != got {
		t.Errorf("not idempotent:\n%s\n---\n%s", got, again)
	}
}

// A subquery in an IF / ELSIF / WHILE condition is opened like any other subquery.
func TestSubqueryAlwaysInPLpgSQLConditions(t *testing.T) {
	c, err := config.Load("/nonexistent/.pg_procrustes.yaml")
	if err != nil {
		t.Fatal(err)
	}
	c.Layout.Subquery.Break = config.BreakAlways
	c.Layout.Clauses.Break = config.BreakAlways
	in := "create function f() returns void as $$\nbegin\n  drop table if exists tmp;\n  if exists (select 1 from v) then\n    null;\n  elsif not exists (select 1 from w) then\n    null;\n  end if;\n  while exists (select 1 from q) loop\n    null;\n  end loop;\nend\n$$ language plpgsql;\n"
	want := "create function f() returns void as $$\nbegin\n  drop table if exists tmp;\n  if exists (\n    select 1\n    from v\n  ) then\n    null;\n  elsif not exists (\n    select 1\n    from w\n  ) then\n    null;\n  end if;\n  while exists (\n    select 1\n    from q\n  ) loop\n    null;\n  end loop;\nend\n$$ language plpgsql;\n"
	got, err := formatter.Format(in, c)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	if again, _ := formatter.Format(got, c); again != got {
		t.Errorf("not idempotent:\n%s\n---\n%s", got, again)
	}
}

func TestSubqueryPreserveLeavesPLpgSQLConditions(t *testing.T) {
	c, err := config.Load("/nonexistent/.pg_procrustes.yaml")
	if err != nil {
		t.Fatal(err)
	}
	c.Layout.Clauses.Break = config.BreakAlways
	in := "create function f() returns void as $$\nbegin\n  if exists (select 1 from v) then\n    null;\n  end if;\nend\n$$ language plpgsql;\n"
	got, err := formatter.Format(in, c)
	if err != nil {
		t.Fatal(err)
	}
	if got != in {
		t.Errorf("changed:\n%s", got)
	}
}

// A PL/pgSQL body written on one line stays on one line (nothing to lay out), and the
// blank line between statements is not added inside a body (it would also precede END).
func TestOneLinePLpgSQLBodyStaysCompact(t *testing.T) {
	c, err := config.Load("/nonexistent/.pg_procrustes.yaml")
	if err != nil {
		t.Fatal(err)
	}
	c.Layout.Clauses.Break = config.BreakAlways
	c.Layout.Content.Break = config.BreakAlways
	c.StatementBreak = config.StatementBreakAlways
	in := "do $$ begin perform 1; end $$;\n"
	got, err := formatter.Format(in, c)
	if err != nil {
		t.Fatal(err)
	}
	if got != in {
		t.Errorf("got %q", got)
	}
}

func TestNoBlankLineBeforeEndWithBlankLinesBetweenStatements(t *testing.T) {
	c, err := config.Load("/nonexistent/.pg_procrustes.yaml")
	if err != nil {
		t.Fatal(err)
	}
	c.BlankLinesBetweenStatements = "1"
	in := "create function f() returns void as $$\nbegin\n  x := 1;\n  y := 2;\nend\n$$ language plpgsql;\n"
	got, err := formatter.Format(in, c)
	if err != nil {
		t.Fatal(err)
	}
	if got != in {
		t.Errorf("body was changed:\n%s", got)
	}
}

// A line break between a function name and its "(" goes with call_paren_spacing: remove
// (layout used to join the lines with a space that the next run removed).
func TestCallParenRemoveAcrossLineBreak(t *testing.T) {
	c, err := config.Load("/nonexistent/.pg_procrustes.yaml")
	if err != nil {
		t.Fatal(err)
	}
	c.CallParenSpacing = config.ParenSpacingRemove
	c.Layout.Clauses.Break = config.BreakAlways
	got, err := formatter.Format("select coalesce\n(a, b)\nfrom t;", c)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "coalesce(a, b)") {
		t.Errorf("got %q", got)
	}
	if again, _ := formatter.Format(got, c); again != got {
		t.Errorf("not idempotent: %q vs %q", got, again)
	}
}

// operator_spacing: compact holds for operators whose operands layout joined onto one line.
func TestCompactOperatorsSurviveLayoutRejoin(t *testing.T) {
	c, err := config.Load("/nonexistent/.pg_procrustes.yaml")
	if err != nil {
		t.Fatal(err)
	}
	c.OperatorSpacing = config.OperatorSpacingCompact
	c.Layout.Clauses.Break = config.BreakAlways
	c.Layout.Content.Break = config.BreakAuto
	in := "update t\nset (a, b)\n=\n(new.a, new.b)\nwhere id = 1;"
	got, err := formatter.Format(in, c)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, " = ") {
		t.Errorf("operator padding left in %q", got)
	}
	if again, _ := formatter.Format(got, c); again != got {
		t.Errorf("not idempotent: %q vs %q", got, again)
	}
}

// Found by checking the SQL inside formatted function bodies of real files: each of these
// produced code that PostgreSQL rejects (or commented code out).
func TestInsertTargetAliasKeepsAs(t *testing.T) {
	c, err := config.Load("/nonexistent/.pg_procrustes.yaml")
	if err != nil {
		t.Fatal(err)
	}
	c.Aliases.As = config.AliasAsRemove
	got, err := formatter.Format("insert into s.t as x (a) values (1) on conflict (a) do update set a = excluded.a;\nupdate t as u set a = 1;\n", c)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "insert into s.t as x (a)") {
		t.Errorf("AS of the INSERT target was removed: %q", got)
	}
	if !strings.Contains(got, "update t u set") {
		t.Errorf("AS of the UPDATE alias should still go: %q", got)
	}
}

func TestSemicolonAddSkipsTrailingBlockComment(t *testing.T) {
	c, err := config.Load("/nonexistent/.pg_procrustes.yaml")
	if err != nil {
		t.Fatal(err)
	}
	c.Semicolons = config.SemicolonAdd
	in := "create function f() returns void language plpgsql as $$\nbegin\n  null;\nend\n/*DOCU\nx\nDOCU*/\n$$;\n"
	want := "create function f() returns void language plpgsql as $$\nbegin\n  null;\nend;\n/*DOCU\nx\nDOCU*/\n$$;\n"
	got, err := formatter.Format(in, c)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestLongTypeFormKeepsPrecisionOfTimestamptz(t *testing.T) {
	c, err := config.Load("/nonexistent/.pg_procrustes.yaml")
	if err != nil {
		t.Fatal(err)
	}
	c.DataTypes.Form = config.TypeFormLong
	got, err := formatter.Format("select now()::timestamptz(0), x::timetz(2), y::timestamptz, z::varchar(3) from t;\n", c)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"::timestamptz(0)", "::timetz(2)", "::timestamp with time zone,", "::character varying(3)"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
}

func TestCaseWithLineCommentIsNotRebuilt(t *testing.T) {
	c, err := config.Load("/nonexistent/.pg_procrustes.yaml")
	if err != nil {
		t.Fatal(err)
	}
	c.Layout.Clauses.Break = config.BreakAlways
	c.Layout.Case.Break = config.BreakAlways
	c.Layout.Subquery.Break = config.BreakAlways
	in := "with r as (\n  select case\n    when a not in ('A') -- keep this\n    then f(1)\n    else null\n  end\n  from t\n)\nselect * from r;\n"
	got, err := formatter.Format(in, c)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "-- keep this then") || strings.Contains(got, "-- keep this f(1)") {
		t.Errorf("code ended up inside the comment:\n%s", got)
	}
	if _, err := pg_query.Parse(got); err != nil {
		t.Errorf("output does not parse: %v\n%s", err, got)
	}
}

func TestTableFunctionAliasAsRemoved(t *testing.T) {
	c, err := config.Load("/nonexistent/.pg_procrustes.yaml")
	if err != nil {
		t.Fatal(err)
	}
	c.Aliases.As = config.AliasAsRemove
	c.Functions.Case = config.CaseLower
	in := "update t set x = a.v from l join UNNEST(_a, _b) as a(n, v) on l.n = a.n;\n"
	once, err := formatter.Format(in, c)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(once, "join unnest(_a, _b) a(n, v) on") {
		t.Errorf("got %q", once)
	}
	if again, _ := formatter.Format(once, c); again != once {
		t.Errorf("not idempotent: %q vs %q", once, again)
	}
}

// A quoted name spelled like a keyword that the parser cannot place (inside a PL/pgSQL
// expression) keeps its quotes: unquoted it was upper-cased as a keyword on the next run.
func TestQuotedKeywordNameInPLpgSQLExpressionIsStable(t *testing.T) {
	c, err := config.Load("../templates/postgresql.yaml")
	if err != nil {
		t.Fatal(err)
	}
	in := "create function f(_year text) returns text language plpgsql as $$\nbegin\n  _xml := common.to_text(xmlforest(nullif(_year, '') as \"year\"));\n  return _xml;\nend;\n$$;\n"
	once, err := formatter.Format(in, c)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(once, `AS "year"`) {
		t.Errorf("quotes were removed:\n%s", once)
	}
	if again, _ := formatter.Format(once, c); again != once {
		t.Errorf("not idempotent:\n%s\n---\n%s", once, again)
	}
}

// keyword_paren_spacing: the space between a keyword / operator word and its "(" (and between a
// ")" and the keyword after it) is set apart from function calls and names (call_paren_spacing).
func TestKeywordParenSpacing(t *testing.T) {
	const in = "SELECT count(*), coalesce(a, 0), x::varchar(10) FROM t WHERE(a = 1)AND(b IN(1, 2))OR EXISTS(SELECT 1);\n" +
		"INSERT INTO t(a, b) VALUES(1, 2) ON CONFLICT(a) DO NOTHING;\n" +
		"CREATE FUNCTION f(a int) RETURNS int LANGUAGE plpgsql AS $$\nBEGIN\n  IF(a = 1)THEN RETURN(a); END IF;\n  x = left(y, 1) || format('a') || json_object('a': 1);\nEND; $$;\n"
	cases := []struct {
		mode      config.ParenSpacingMode
		has, lack []string
	}{
		{config.ParenSpacingAdd,
			[]string{"count(*)", "coalesce(a, 0)", "varchar(10)", "WHERE (a = 1) AND (b IN (1, 2)) OR EXISTS (SELECT 1)",
				"INSERT INTO t(a, b) VALUES (1, 2)", "ON CONFLICT (a)", "IF (a = 1) THEN RETURN (a)", "left(y, 1)", "format('a')", "json_object('a': 1)"},
			[]string{"WHERE(", "IN(", "IF("}},
		{config.ParenSpacingRemove,
			[]string{"count(*)", "WHERE(a = 1)", "IN(1, 2)", "EXISTS(SELECT 1)", "VALUES(1, 2)", "ON CONFLICT(a)", "IF(a = 1)", "RETURN(a)"},
			[]string{"WHERE (", "IN (", "IF ("}},
		{config.ParenSpacingPreserve,
			[]string{"WHERE(a = 1)AND(b IN(1, 2))", "IF(a = 1)THEN"},
			nil},
	}
	for _, c := range cases {
		t.Run(string(c.mode), func(t *testing.T) {
			cfg, err := config.Load("/nonexistent/.pg_procrustes.yaml")
			if err != nil {
				t.Fatal(err)
			}
			cfg.CallParenSpacing = config.ParenSpacingRemove
			cfg.KeywordParenSpacing = c.mode
			got, err := formatter.Format(in, cfg)
			if err != nil {
				t.Fatal(err)
			}
			for _, s := range c.has {
				if !strings.Contains(got, s) {
					t.Errorf("missing %q in\n%s", s, got)
				}
			}
			for _, s := range c.lack {
				if strings.Contains(got, s) {
					t.Errorf("unexpected %q in\n%s", s, got)
				}
			}
			if again, _ := formatter.Format(got, cfg); again != got {
				t.Errorf("not idempotent:\n%s\n---\n%s", got, again)
			}
		})
	}
}

// Trailing comments in a DO body are aligned after the layout has moved the lines: comments of
// lines that stood next to each other and are separated now are no run, and the second run
// finds the text as the first left it.
func TestTrailingCommentsInBodyAlignedAfterLayout(t *testing.T) {
	c := wsCfg(t, func(c *config.Config) {
		c.Comments = config.CommentsCfg{SpaceAfterMarker: config.CommentSpaceAdd, AlignTrailing: config.CommentAlignAlign}
		c.Layout.Clauses.Break = config.BreakAlways
		c.Layout.Content.Break = config.BreakAlways
	})
	in := "DO $$\nBEGIN\nUPDATE t\nSET a = 4 --Stopped\nWHERE bb = 3 --Running\n    AND c = 1;\nEND\n$$;\n"
	got, err := formatter.Format(in, c)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "a = 4 -- Stopped") {
		t.Errorf("lone comment kept the padding of the lines it stood next to:\n%s", got)
	}
	if again, _ := formatter.Format(got, c); again != got {
		t.Errorf("not idempotent:\n%s\n--\n%s", got, again)
	}
}

// The expressions of CHECK / DEFAULT constraints and of CREATE INDEX … WHERE and of index
// expressions are read like any other: a column named like a keyword (value) keeps its case
// and functions follow functions.case.
func TestConstraintAndIndexExpressionsReadLikeQueries(t *testing.T) {
	c := wsCfg(t, func(c *config.Config) {
		c.Keywords.Case = config.CaseUpper
		c.ReservedKeywords.Case = config.CaseUpper
		c.Functions.Case = config.CaseUpper
	})
	in := "create table t (value int, x int default abs(1), check (value > 0));\ncreate index i on t (value, (lower(x::text))) where value > 0;\n"
	got, err := formatter.Format(in, c)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"CHECK (value > 0)", "DEFAULT ABS(1)", "(LOWER(x::text))", "WHERE value > 0"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if again, _ := formatter.Format(got, c); again != got {
		t.Errorf("not idempotent:\n%s\n--\n%s", got, again)
	}
}
