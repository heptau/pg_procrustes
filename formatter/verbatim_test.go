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

// A line that looks like a directive but is a line of a string constant (or of a /* … */
// comment) is data: it starts no region and the text around it is formatted.
func TestDirectiveLineInsideStringOrCommentIsData(t *testing.T) {
	c := verbatimCfg(t)
	for _, in := range []string{
		"SELECT '\n-- pg_procrustes: off\n' FROM t;\nSELECT a FROM t;\n",
		"/* note\n-- pg_procrustes: off\n*/\nSELECT a FROM t;\n",
		"SELECT \"x\n-- pg_procrustes: off\n\" FROM t;\nSELECT a FROM t;\n",
	} {
		got, err := formatter.Format(in, c)
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		if !strings.Contains(got, "\n-- pg_procrustes: off\n") {
			t.Errorf("%q: the line was changed: %q", in, got)
		}
		if !strings.HasSuffix(got, "select a\nfrom t;\n") {
			t.Errorf("%q: the statement after the string/comment was not formatted: %q", in, got)
		}
	}
	// Inside a function body the directive still works, also after a string in the body.
	in := "DO $$\nBEGIN\n  x := 'a';\n  -- pg_procrustes: off\n  y   :=   2;\n  -- pg_procrustes: on\nEND $$;\n"
	got, err := formatter.Format(in, c)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "  y   :=   2;\n") {
		t.Errorf("region in a body was not kept: %q", got)
	}
}

// The positions of errors after off…on regions are those of the original text: also for an
// error that makes the whole text unreadable (unterminated string), and with several regions
// of which only the first lies before the error.
func TestOffOnErrorPositionsAfterRegions(t *testing.T) {
	c := verbatimCfg(t)
	in := "-- pg_procrustes: off\nfoo;\nbar;\n-- pg_procrustes: on\nselect 'abc;\n"
	_, err := formatter.Format(in, c)
	var se formatter.SyntaxErrors
	if !errors.As(err, &se) || len(se) != 1 {
		t.Fatalf("expected one syntax error, got %v", err)
	}
	if se[0].Line != 5 || se[0].Offset != strings.Index(in, "'abc") {
		t.Errorf("unterminated string reported at line %d offset %d, want line 5 offset %d", se[0].Line, se[0].Offset, strings.Index(in, "'abc"))
	}
	if _, _, err = formatter.FormatRange(in, c, 5, 5); !errors.As(err, &se) || len(se) != 1 || se[0].Line != 5 {
		t.Errorf("FormatRange: want the error on line 5, got %v", err)
	}

	region := "-- pg_procrustes: off\n" + strings.Repeat("x;\n", 10) + "-- pg_procrustes: on\n"
	in = region + "select 1;\nselect from from;\nselect 2;\nselect 3;\n" + region
	_, err = formatter.Format(in, c)
	if !errors.As(err, &se) || len(se) != 1 {
		t.Fatalf("expected one syntax error, got %v", err)
	}
	if se[0].Line != 14 {
		t.Errorf("error reported on line %d, want 14 (the region after it must not shift it)", se[0].Line)
	}
}

// A UTF-8 byte order mark at the start of a file stays there and the SQL after it is formatted.
func TestByteOrderMarkIsKept(t *testing.T) {
	c := verbatimCfg(t)
	const bom = "\uFEFF"
	got, err := formatter.Format(bom+"SELECT a FROM t;\n", c)
	if err != nil {
		t.Fatal(err)
	}
	if want := bom + "select a\nfrom t;\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if again, _ := formatter.Format(got, c); again != got {
		t.Errorf("not idempotent: %q", again)
	}
	got, _, err = formatter.FormatRange(bom+"SELECT a FROM t;\nSELECT b FROM t;\n", c, 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if want := bom + "SELECT a FROM t;\nselect b\nfrom t;\n"; got != want {
		t.Errorf("FormatRange: got %q, want %q", got, want)
	}
	in := bom + "select from from;\n"
	_, err = formatter.Format(in, c)
	var se formatter.SyntaxErrors
	if !errors.As(err, &se) || len(se) != 1 {
		t.Fatalf("expected one syntax error, got %v", err)
	}
	if se[0].Line != 1 || se[0].Col != 13 || se[0].Offset != strings.LastIndex(in, "from") {
		t.Errorf("error at line %d col %d offset %d, want 1, 13, %d", se[0].Line, se[0].Col, se[0].Offset, strings.LastIndex(in, "from"))
	}
	if got, err := formatter.Format(bom, c); err != nil || got != bom {
		t.Errorf("a file that is only a byte order mark: %q, %v", got, err)
	}
}
