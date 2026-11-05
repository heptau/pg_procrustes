package formatter_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/heptau/pg_procrustes/config"
	"github.com/heptau/pg_procrustes/formatter"
)

func verbatimCfg(t *testing.T) *config.Config {
	t.Helper()
	c, err := config.Load("/nonexistent/.pg_procrustes.yaml")
	if err != nil {
		t.Fatal(err)
	}
	c.ReservedKeywords.Case = config.CaseLower
	c.Layout.Clauses.Break = config.BreakAlways
	return c
}

func TestOffOnKeepsRegionVerbatim(t *testing.T) {
	c := verbatimCfg(t)
	in := "SELECT a FROM t;\n\n-- pg_procrustes: off\nINSERT  INTO t   VALUES\n  (1,   'a'),\n  (22,  'bb');\n-- pg_procrustes: on\n\nSELECT b FROM u;\n"
	want := "select a\nfrom t;\n\n-- pg_procrustes: off\nINSERT  INTO t   VALUES\n  (1,   'a'),\n  (22,  'bb');\n-- pg_procrustes: on\n\nselect b\nfrom u;\n"
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

func TestOffWithoutOnRunsToTheEnd(t *testing.T) {
	c := verbatimCfg(t)
	in := "SELECT a FROM t;\n/* pg_procrustes: off */\nSELECT   b  FROM   u;\nSELECT c FROM v;\n"
	want := "select a\nfrom t;\n/* pg_procrustes: off */\nSELECT   b  FROM   u;\nSELECT c FROM v;\n"
	got, err := formatter.Format(in, c)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestStrayOnIsJustAComment(t *testing.T) {
	c := verbatimCfg(t)
	got, err := formatter.Format("-- pg_procrustes: on\nSELECT a FROM t;\n", c)
	if err != nil {
		t.Fatal(err)
	}
	if got != "-- pg_procrustes: on\nselect a\nfrom t;\n" {
		t.Errorf("got %q", got)
	}
}

func TestOffOnInsideFunctionBody(t *testing.T) {
	c := verbatimCfg(t)
	c.PLpgSQLAssignment = config.AssignColonEquals
	in := "create function f() returns void as $$\nbegin\n  x = 1;\n  -- pg_procrustes: off\n  y   =   2;\n  -- pg_procrustes: on\n  z = 3;\nend\n$$ language plpgsql;\n"
	want := "create function f() returns void as $$\nbegin\n  x := 1;\n  -- pg_procrustes: off\n  y   =   2;\n  -- pg_procrustes: on\n  z := 3;\nend\n$$ language plpgsql;\n"
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

func TestOffOnKeepsCRLFAndLineNumbersOfErrors(t *testing.T) {
	c := verbatimCfg(t)
	in := "select 1;\n-- pg_procrustes: off\nfoo   bar;\nbaz;\n-- pg_procrustes: on\nselect from from;\n"
	_, err := formatter.Format(in, c)
	var se formatter.SyntaxErrors
	if !errors.As(err, &se) || len(se) != 1 {
		t.Fatalf("expected one syntax error, got %v", err)
	}
	if se[0].Line != 6 {
		t.Errorf("error reported on line %d, want 6 (the original line)", se[0].Line)
	}
	crlf := strings.ReplaceAll("SELECT a FROM t;\n-- pg_procrustes: off\nX   Y;\n-- pg_procrustes: on\n", "\n", "\r\n")
	c.LineEndings = config.LineEndingPreserve
	got, err := formatter.Format(crlf, c)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "X   Y;\r\n") {
		t.Errorf("verbatim region lost its CRLF: %q", got)
	}
}
