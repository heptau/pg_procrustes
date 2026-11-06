package formatter_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/heptau/pg_procrustes/config"
	"github.com/heptau/pg_procrustes/formatter"
)

func rangeCfg(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := loadStyled("")
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestFormatRangeOnlyTouchedStatements(t *testing.T) {
	cfg := rangeCfg(t)
	src := "select   1;\nselect   2;\n\nselect\n  3\n  from t;\nselect   4;\n"
	cases := []struct {
		a, b int
		want string
	}{
		{2, 2, "select   1;\nSELECT   2;\n\nselect\n  3\n  from t;\nselect   4;\n"},
		{1, 1, "SELECT   1;\nselect   2;\n\nselect\n  3\n  from t;\nselect   4;\n"},
		// A statement only partly inside the range is formatted completely.
		{6, 6, "select   1;\nselect   2;\n\nSELECT\n  3\n  FROM t;\nselect   4;\n"},
		// A blank line between statements touches none.
		{3, 3, src},
		{1, 7, "SELECT   1;\nSELECT   2;\n\nSELECT\n  3\n  FROM t;\nSELECT   4;\n"},
		{99, 120, src},
	}
	for _, c := range cases {
		got, _, err := formatter.FormatRange(src, cfg, c.a, c.b)
		if err != nil {
			t.Fatalf("%d:%d: %v", c.a, c.b, err)
		}
		if got != c.want {
			t.Errorf("%d:%d\n got: %q\nwant: %q", c.a, c.b, got, c.want)
		}
		again, _, err := formatter.FormatRange(got, cfg, c.a, c.b)
		if err != nil || again != got {
			t.Errorf("%d:%d not idempotent: %q (%v)", c.a, c.b, again, err)
		}
	}
}

func TestFormatRangeEverythingEqualsFormat(t *testing.T) {
	cfg := rangeCfg(t)
	src := "select   a ,b from t;\n\nselect 1 from   x;\n"
	want, err := formatter.Format(src, cfg)
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := formatter.FormatRange(src, cfg, 1, 100)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestFormatRangeIgnoresErrorsOutside(t *testing.T) {
	cfg := rangeCfg(t)
	src := "select   1;\nselect from from;\nselect   3;\n"
	got, diags, err := formatter.FormatRange(src, cfg, 3, 3)
	if err != nil || len(diags) != 0 {
		t.Fatalf("err=%v diags=%v", err, diags)
	}
	if want := "select   1;\nselect from from;\nSELECT   3;\n"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
	// A selected statement that does not parse is reported (fail) ...
	out, diags, err := formatter.FormatRange(src, cfg, 2, 2)
	var se formatter.SyntaxErrors
	if err != nil && !errors.As(err, &se) {
		t.Fatal(err)
	}
	if len(diags) != 1 || diags[0].Line != 2 || out != "" {
		t.Errorf("want one error on line 2, got %v out=%q", diags, out)
	}
	// ... and copied as written with syntax_errors: skip.
	cfg.SyntaxErrors = config.SyntaxErrorSkip
	out, diags, _ = formatter.FormatRange(src, cfg, 1, 3)
	if want := "SELECT   1;\nselect from from;\nSELECT   3;\n"; out != want || len(diags) != 1 {
		t.Errorf("skip: got %q diags=%v", out, diags)
	}
}

func TestFormatRangeVerbatimRegionsAndCRLF(t *testing.T) {
	cfg := rangeCfg(t)
	src := "select   1;\r\n-- pg_procrustes: off\r\nselect    9   ;\r\nselect    8;\r\n-- pg_procrustes: on\r\nselect    3;\r\n"
	// Line numbers are those of the original file: line 6 is the last statement.
	got, _, err := formatter.FormatRange(src, cfg, 6, 6)
	if err != nil {
		t.Fatal(err)
	}
	want := "select   1;\r\n-- pg_procrustes: off\r\nselect    9   ;\r\nselect    8;\r\n-- pg_procrustes: on\r\nSELECT    3;\r\n"
	if got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
	got, _, _ = formatter.FormatRange(src, cfg, 1, 1)
	if !strings.HasPrefix(got, "SELECT   1;\r\n-- pg_procrustes: off\r\nselect    9   ;") || !strings.HasSuffix(got, "select    3;\r\n") {
		t.Errorf("got %q", got)
	}
}

func TestFormatRangeInvalid(t *testing.T) {
	if _, _, err := formatter.FormatRange("select 1;", rangeCfg(t), 3, 2); err == nil {
		t.Error("end before start should be an error")
	}
}
