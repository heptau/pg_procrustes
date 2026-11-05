package formatter_test

import (
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
