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
		// the line breaks inside a string constant are data: preserve leaves them as written
		{"preserve keeps LF in a string of a CRLF file", config.LineEndingPreserve, "select 'x\ny'\r\nfrom t;\r\n", "SELECT 'x\ny'\r\nFROM t;\r\n"},
		{"preserve keeps CRLF in a string of an LF file", config.LineEndingPreserve, "\nselect 'x\r\r\ny'\nfrom t;\n", "\nSELECT 'x\r\r\ny'\nFROM t;\n"},
		// a lone CR ends a line (and a -- comment) as well: CR-only files keep CR, strings keep theirs
		{"preserve keeps CR", config.LineEndingPreserve, "select a -- c\rfrom t\rwhere b = 'x\ny';\r", "SELECT a -- c\rFROM t\rWHERE b = 'x\ny';\r"},
		{"preserve: breaks in strings do not count", config.LineEndingPreserve, "select 1,\r'\n\n';\r", "SELECT 1,\r'\n\n';\r"},
		{"lf converts CR", config.LineEndingLF, "select a -- c\rfrom t;\r", "SELECT a -- c\nFROM t;\n"},
		{"lf converts a lone CR in a string too", config.LineEndingLF, "select 'x\r\r\ny';\n", "SELECT 'x\n\ny';\n"},
		{"crlf converts a lone CR in a string too", config.LineEndingCRLF, "select 'x\r\r\ny';\n", "SELECT 'x\r\n\r\ny';\r\n"},
		{"lf converts inside strings", config.LineEndingLF, "select 'x\r\ny'\r\nfrom t;\r\n", "SELECT 'x\ny'\nFROM t;\n"},
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

// PERFORM q is read as the query q: aliases in it get their AS, and RETURN QUERY follows
// plpgsql_keywords (unless QUERY is a variable that is returned).
func TestPerformAliasesAndReturnQuery(t *testing.T) {
	c := wsCfg(t, func(c *config.Config) {
		c.Aliases.As = config.AliasAsAdd
		c.PLpgSQLKeywords.Case = config.CaseUpper
	})
	in := "create function f() returns setof int language plpgsql as $$\nbegin\n  return query select 1;\n  perform (select max(x) m from t);\n  perform 1 from (select a b from t) s;\nend $$;\n" +
		"create function g() returns int language plpgsql as $$ declare query int; begin return query; end $$;\n"
	got, err := formatter.Format(in, c)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"RETURN QUERY SELECT 1", "(SELECT max(x) AS m FROM t)", "(SELECT a AS b FROM t) AS s", "RETURN query;"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if again, _ := formatter.Format(got, c); again != got {
		t.Errorf("not idempotent:\n%s\n--\n%s", got, again)
	}
}

// Calls in the expressions of a PL/pgSQL body that no statement tree covers (assignments,
// IF conditions, RETURN, statements with INTO) follow functions.case like those of SQL statements;
// a table name before "(" (INSERT INTO t (a)) is no call.
func TestFunctionCaseInBodyExpressions(t *testing.T) {
	c := wsCfg(t, func(c *config.Config) {
		c.Functions.Case = config.CaseUpper
	})
	in := "create function f(p int) returns int language plpgsql as $$\ndeclare v int;\nbegin\n  v := abs(p) + my_func(p);\n  v := v + -- a comment\n    trunc(p);\n  if lower(v::text) = 'x' then\n    insert into t (a) values (round(v));\n  end if;\n  return length(my_other(v));\nend $$;\n"
	got, err := formatter.Format(in, c)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"ABS(p) + MY_FUNC(p)", "LOWER(v::text)", "insert into t (a)", "ROUND(v)", "LENGTH(MY_OTHER(v))", "TRUNC(p)"} {
		if !strings.Contains(strings.ToLower(got), strings.ToLower(want)) || (want != "insert into t (a)" && !strings.Contains(got, want)) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "INSERT INTO T (") || strings.Contains(got, "T (a)") {
		t.Errorf("table name taken for a function:\n%s", got)
	}
	if again, _ := formatter.Format(got, c); again != got {
		t.Errorf("not idempotent:\n%s\n--\n%s", got, again)
	}
}

// DDL statements in a PL/pgSQL body (CREATE TABLE / INDEX, ALTER TABLE, DROP) are laid out like
// those at the top level: a table that does not fit on a line is broken up into one column per line.
func TestDDLInBodyIsLaidOut(t *testing.T) {
	c := wsCfg(t, func(c *config.Config) {
		c.Layout.LineLength = 60
		c.Layout.DDL.CreateTable.Break = config.BreakAuto
	})
	in := "do $$\nbegin\ncreate table t (colname text, typ text, op text[], value text[], matches int[]);\nend\n$$;\n"
	got, err := formatter.Format(in, c)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "\n   colname text,\n") {
		t.Errorf("table not broken up:\n%s", got)
	}
	if again, _ := formatter.Format(got, c); again != got {
		t.Errorf("not idempotent:\n%s\n--\n%s", got, again)
	}
}

// Function calls and columns are found wherever an expression stands, not only in the places the
// walker lists: row constructors, window frames and OVER clauses, CASE operands, IN lists, SQL
// function bodies, EXCLUDE, domain and trigger conditions, policies. VALUE of a domain and
// OLD / NEW of a trigger stay keywords.
func TestExpressionsEverywhereReadLikeQueries(t *testing.T) {
	c := wsCfg(t, func(c *config.Config) {
		c.Functions.Case = config.CaseUpper
		c.Keywords.Case = config.CaseUpper
		c.ReservedKeywords.Case = config.CaseUpper
	})
	in := "select (a, abs(x)) = any (select 1);\n" +
		"select row(abs(x), 1), array[abs(x)], case abs(x) when abs(1) then 1 end;\n" +
		"select sum(x) over (order by abs(x) rows between unbounded preceding and abs(3) following) from t;\n" +
		"select a from t where abs(y) not in (select abs(z) from u);\n" +
		"create function g() returns int return abs(1);\n" +
		"create table t2 (c int, exclude using gist (abs(c) with =));\n" +
		"create domain d int check (abs(value) > 0);\n" +
		"create trigger tr before update on t for each row when (abs(new.a) > 0) execute function f();\n" +
		"create policy p on t using (abs(a) > 0);\n" +
		"select xmlforest(a as costs);\n"
	got, err := formatter.Format(in, c)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "abs(") {
		t.Errorf("a call was left as written:\n%s", got)
	}
	for _, want := range []string{"ABS(VALUE)", "ABS(NEW.a)", "XMLFOREST(a AS costs)"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if again, _ := formatter.Format(got, c); again != got {
		t.Errorf("not idempotent:\n%s\n--\n%s", got, again)
	}
}

// CREATE RULE / TRIGGER / POLICY in a PL/pgSQL body are statements of their own: the UPDATE of
// `ON UPDATE TO t` or `BEFORE UPDATE ON t` does not start a query that is broken up and read
// without the rest of its statement.
func TestRuleAndTriggerInBodyAreNotBrokenUp(t *testing.T) {
	c := wsCfg(t, func(c *config.Config) {
		c.Layout.Clauses.Break = config.BreakAlways
		c.Layout.Content.Break = config.BreakAlways
		c.Keywords.Case = config.CaseUpper
		c.DataTypes.Case = config.CaseUpper
	})
	in := "do $$\nbegin\ncreate rule r as on update to w.s do delete from w.s where oid = 1;\ncreate trigger tr before update on t for each row execute function f();\nend\n$$;\n"
	got, err := formatter.Format(in, c)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"CREATE RULE r AS ON UPDATE TO w.s DO DELETE FROM w.s WHERE oid = 1;\n", "CREATE TRIGGER tr BEFORE UPDATE ON t FOR EACH ROW EXECUTE FUNCTION f();\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("statement broken up (missing %q) in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "OID") {
		t.Errorf("column oid written as a type:\n%s", got)
	}
	if again, _ := formatter.Format(got, c); again != got {
		t.Errorf("not idempotent:\n%s\n--\n%s", got, again)
	}
}

// blank_lines: max_1 collapses blank lines in code, but a dollar-quoted text that is no function
// or DO body (COMMENT ON … IS $$…$$) is data and keeps its lines as written.
func TestBlankLinesLeaveDollarQuotedTextAlone(t *testing.T) {
	c := wsCfg(t, func(c *config.Config) { c.BlankLines = config.BlankLinesMax1 })
	in := "comment on table t is $$one\n\n\n\ntwo$$;\n\n\n\nselect 1;\n"
	got, err := formatter.Format(in, c)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "$$one\n\n\n\ntwo$$;\n\nSELECT 1;") {
		t.Errorf("text changed or code not collapsed:\n%q", got)
	}
	if again, _ := formatter.Format(got, c); again != got {
		t.Errorf("not idempotent:\n%s\n--\n%s", got, again)
	}
}

// semicolons: remove takes off the ";" that ends the text; the ones between statements and in
// PL/pgSQL bodies are needed, the output must stay valid SQL and be stable.
func TestSemicolonsRemoveKeepsRequiredOnes(t *testing.T) {
	c := wsCfg(t, func(c *config.Config) { c.Semicolons = config.SemicolonRemove })
	in := "do $$\nbegin\nselect * from t;\nend\n$$;\nselect 1;\nselect 2;\n"
	got, err := formatter.Format(in, c)
	if err != nil {
		t.Fatal(err)
	}
	if want := "DO $$\nBEGIN\nSELECT * FROM t;\nEND\n$$;\nSELECT 1;\nSELECT 2\n"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
	if again, _ := formatter.Format(got, c); again != got {
		t.Errorf("not idempotent:\n%s\n--\n%s", got, again)
	}
}

// begin_body.indent moves the lines of the body as a whole, but not the lines that start inside a
// string constant or a nested dollar-quoted body (their blanks are data).
func TestBodyIndentLeavesNestedBodiesAlone(t *testing.T) {
	c := wsCfg(t, func(c *config.Config) {
		c.Layout.DollarQuote.PLpgSQL.BeginBody.Indent = config.BodyIndentIndent
	})
	in := "do $$\nbegin\ncreate function f() returns int language sql as $fn$\n\n   select 1;\n\n$fn$;\nend\n$$;\n"
	got, err := formatter.Format(in, c)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "$fn$\n\n   SELECT 1;\n\n$fn$;") && !strings.Contains(got, "$fn$\n\n   select 1;\n\n$fn$;") {
		t.Errorf("nested body changed:\n%s", got)
	}
	if again, _ := formatter.Format(got, c); again != got {
		t.Errorf("not idempotent:\n%s\n--\n%s", got, again)
	}
}

// A comment after ELSE / THEN stays on that line when if.blank_line_after is set.
func TestIfBlankLineAfterKeepsCommentOnKeywordLine(t *testing.T) {
	c := wsCfg(t, func(c *config.Config) {
		c.Layout.DollarQuote.PLpgSQL.ControlFlow.If.BlankLineAfter = config.BlankLineRemove
	})
	in := "do $$\nbegin\n  if a then\n    x := 1;\n  else -- other\n    x := 2;\n  end if;\nend\n$$;\n"
	got, err := formatter.Format(in, c)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "ELSE -- other\n") {
		t.Errorf("comment left its line:\n%s", got)
	}
	if again, _ := formatter.Format(got, c); again != got {
		t.Errorf("not idempotent:\n%s\n--\n%s", got, again)
	}
}

// A nested block comment after the ';' (PostgreSQL nests /* … */) ended at its first "*/" for the
// statement separation: the line break went into the middle of the comment and the rest of it
// became code (`select 1; /* x /* y */` / `z */`, which does not parse).
func TestNestedBlockCommentAfterSemicolonStaysWhole(t *testing.T) {
	for _, set := range []func(*config.Config){
		func(c *config.Config) { c.BlankLinesBetweenStatements = "1" },
		func(c *config.Config) { c.StatementBreak = config.StatementBreakAlways },
	} {
		c := wsCfg(t, set)
		for _, in := range []string{"select 1; /* x /* y */ z */\n", "select 1; /* x /* y */ z */ select 2;\n"} {
			got, err := formatter.Format(in, c)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(got, "/* x /* y */ z */") {
				t.Errorf("%q: comment cut: %q", in, got)
			}
			if _, err := pg_query.Parse(got); err != nil {
				t.Errorf("%q: output does not parse: %v\n%s", in, err, got)
			}
		}
	}
}
