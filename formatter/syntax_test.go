package formatter_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/heptau/pg_procrustes/config"
	"github.com/heptau/pg_procrustes/formatter"
)

func syntaxCfg(t *testing.T, mode config.SyntaxErrorMode) *config.Config {
	t.Helper()
	c, err := loadStyled("")
	if err != nil {
		t.Fatal(err)
	}
	c.SyntaxErrors = mode
	return c
}

const brokenScript = "select 1 from a;\nselect from from;\nselect 2 from b;\nselect 'ž', from c;\n"

func TestSyntaxErrorsDefaultIsFailAndListsEveryStatement(t *testing.T) {
	c, err := loadStyled("")
	if err != nil {
		t.Fatal(err)
	}
	if c.SyntaxErrors != config.SyntaxErrorFail {
		t.Fatalf("default must be fail, got %q", c.SyntaxErrors)
	}
	out, err := formatter.Format(brokenScript, c)
	if out != "" {
		t.Fatalf("fail mode must not produce output, got %q", out)
	}
	var se formatter.SyntaxErrors
	if !errors.As(err, &se) || len(se) != 2 {
		t.Fatalf("want 2 syntax errors, got %v", err)
	}
	if se[0].Line != 2 || se[0].Col != 13 || !strings.Contains(se[0].Message, `"from"`) {
		t.Errorf("first error position/message wrong: %+v", se[0])
	}
	// The second error is on line 4; the multi-byte 'ž' must count as one column.
	if se[1].Line != 4 || se[1].Col != 13 {
		t.Errorf("second error position wrong: %+v", se[1])
	}
}

func TestSyntaxErrorsSkipFormatsValidAndKeepsInvalidVerbatim(t *testing.T) {
	c := syntaxCfg(t, config.SyntaxErrorSkip)
	out, diags, err := formatter.FormatWithDiagnostics(brokenScript, c)
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 2 {
		t.Fatalf("want 2 diagnostics, got %v", diags)
	}
	want := "SELECT 1 FROM a;\nselect from from;\nSELECT 2 FROM b;\nselect 'ž', from c;\n"
	if out != want {
		t.Fatalf("got:\n%q\nwant:\n%q", out, want)
	}
	// Format (without diagnostics) also succeeds in skip mode.
	if out2, err := formatter.Format(brokenScript, c); err != nil || out2 != want {
		t.Fatalf("Format in skip mode: %q, %v", out2, err)
	}
	if again, _, err := formatter.FormatWithDiagnostics(out, c); err != nil || again != out {
		t.Fatalf("skip mode not idempotent (err=%v):\n%q", err, again)
	}
}

func TestSyntaxErrorsIgnoreDoesNotCheck(t *testing.T) {
	c := syntaxCfg(t, config.SyntaxErrorIgnore)
	out, diags, err := formatter.FormatWithDiagnostics("selec a frm;", c)
	if err != nil || len(diags) != 0 || out == "" {
		t.Fatalf("ignore mode must format without complaint: %q %v %v", out, diags, err)
	}
}

func TestSyntaxErrorsValidSQLIsUnaffected(t *testing.T) {
	src := "select a from t;\nselect b from u;\n"
	var outs []string
	for _, m := range []config.SyntaxErrorMode{config.SyntaxErrorFail, config.SyntaxErrorSkip, config.SyntaxErrorIgnore} {
		out, diags, err := formatter.FormatWithDiagnostics(src, syntaxCfg(t, m))
		if err != nil || len(diags) != 0 {
			t.Fatalf("%s: %v %v", m, diags, err)
		}
		outs = append(outs, out)
	}
	if outs[0] != outs[1] || outs[1] != outs[2] {
		t.Fatalf("modes disagree on valid SQL: %q", outs)
	}
}

func TestSyntaxErrorsPsqlMetaCommandsAreVerbatimNotErrors(t *testing.T) {
	src := "\\set ON_ERROR_STOP on\nselect 1;\n  \\connect other\nselect 2 from t;\n"
	for _, m := range []config.SyntaxErrorMode{config.SyntaxErrorFail, config.SyntaxErrorSkip} {
		out, diags, err := formatter.FormatWithDiagnostics(src, syntaxCfg(t, m))
		if err != nil || len(diags) != 0 {
			t.Fatalf("%s: meta-commands must not be errors: %v %v", m, diags, err)
		}
		want := "\\set ON_ERROR_STOP on\nSELECT 1;\n  \\connect other\nSELECT 2 FROM t;\n"
		if out != want {
			t.Fatalf("%s: got %q want %q", m, out, want)
		}
	}
}

func TestSyntaxErrorsUnterminatedStringIsFatalEvenInSkipMode(t *testing.T) {
	for _, m := range []config.SyntaxErrorMode{config.SyntaxErrorFail, config.SyntaxErrorSkip} {
		_, err := formatter.Format("select 1;\nselect 'oops;\nselect 3;", syntaxCfg(t, m))
		var se formatter.SyntaxErrors
		if !errors.As(err, &se) {
			t.Fatalf("%s: want SyntaxErrors, got %v", m, err)
		}
		if se[0].Line != 2 {
			t.Errorf("%s: error should point at line 2, got %+v", m, se[0])
		}
	}
}

func TestSyntaxErrorsBeginAtomicBodyIsOneStatement(t *testing.T) {
	src := "create function f() returns int language sql begin atomic select 1; select 2; end;\nselect 3;\n"
	out, diags, err := formatter.FormatWithDiagnostics(src, syntaxCfg(t, config.SyntaxErrorSkip))
	if err != nil || len(diags) != 0 {
		t.Fatalf("valid script reported problems: %v %v", diags, err)
	}
	if !strings.Contains(out, "SELECT 3;") {
		t.Fatalf("second statement not formatted:\n%s", out)
	}
}

func TestSyntaxErrorsSkipAppliesWholeTextSettingsOnce(t *testing.T) {
	c := syntaxCfg(t, config.SyntaxErrorSkip)
	c.TrailingNewline = config.TrailingNewlineAdd
	c.BlankLines = config.BlankLinesMax1
	out, _, err := formatter.FormatWithDiagnostics("select 1;\n\n\n\nselect from from;\n\n\n\nselect 2;", c)
	if err != nil {
		t.Fatal(err)
	}
	want := "SELECT 1;\n\nselect from from;\n\nSELECT 2;\n"
	if out != want {
		t.Fatalf("got %q want %q", out, want)
	}
}

func TestSyntaxErrorModeIsValidated(t *testing.T) {
	c := syntaxCfg(t, "sometimes")
	if err := config.Validate(c); err == nil {
		t.Fatal("expected validation error for syntax_errors")
	}
}
