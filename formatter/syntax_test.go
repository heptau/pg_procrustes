package formatter_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pg_query "github.com/pganalyze/pg_query_go/v6"

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

// In skip mode every statement is formatted on its own. A statement that follows another one
// on its line, or the trailing comment of the previous statement, starts with the blanks that
// separate the two; they are no indentation of the statement.
func TestSyntaxErrorsSkipStatementsStartingMidLine(t *testing.T) {
	files, _ := filepath.Glob("../templates/*.yaml")
	if len(files) == 0 {
		t.Fatal("no templates found in templates/")
	}
	const bad = "select 1 frm frm;\n"
	for _, src := range []string{
		"select * from parent; select * from child;\nupdate parent set val1 = 1;\n",
		"begin;\n  select * from parent; select * from child;\ncommit;\n",
		"update parent set val1 = 'b' where aid = 1; -- should fail\nselect * from parent where a = 1;\nselect 2;\n",
		"select a from t\n\t\twhere b = 1;\t\t-- fails\nselect * from parent where a = 1;\n",
		"select 1;\t/* commuted */\nselect * from lp where a is not null;\n",
	} {
		for _, f := range files {
			cfg, err := config.Load(f)
			if err != nil {
				t.Fatalf("%s: %v", f, err)
			}
			whole, err := formatter.Format(src, cfg)
			if err != nil {
				t.Fatalf("Format: %v", err)
			}
			cfg.SyntaxErrors = config.SyntaxErrorSkip
			out, diags, err := formatter.FormatWithDiagnostics(src+bad, cfg)
			if err != nil || len(diags) != 1 {
				t.Fatalf("skip: %v %v", err, diags)
			}
			i := strings.LastIndex(out, strings.TrimSpace(bad))
			if i < 0 {
				t.Fatalf("the bad statement is gone:\n%s", out)
			}
			norm := func(s string) string { return strings.TrimRight(s, "\n") }
			if norm(out[:i]) != norm(whole) {
				t.Errorf("%s: statements formatted one by one differ from the whole script\n--- input ---\n%s--- whole ---\n%s\n--- skip ---\n%s",
					filepath.Base(f), src, whole, out[:i])
			}
			again, _, err := formatter.FormatWithDiagnostics(out, cfg)
			if err != nil {
				t.Fatalf("second pass: %v", err)
			}
			if again != out {
				t.Errorf("%s: not idempotent\n--- first ---\n%s\n--- second ---\n%s", filepath.Base(f), out, again)
			}
		}
	}
}

// The blanks before the first token of a script are the indentation of its first line.
func TestInlineSpacingKeepsIndentOfFirstLine(t *testing.T) {
	empty := filepath.Join(t.TempDir(), "empty.yaml")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(empty)
	if err != nil {
		t.Fatal(err)
	}
	cfg.InlineSpacing = config.InlineSpacingNormalize
	const src = "    select *   from child;\n    select 2;\n"
	const want = "    select * from child;\n    select 2;\n"
	if got, err := formatter.Format(src, cfg); err != nil || got != want {
		t.Errorf("got:\n%q (%v)\nwant:\n%q", got, err, want)
	}
	// A dollar-quoted body is no script: the blanks after $$ are inline spacing.
	if got, err := formatter.Format("do $$   begin   perform 1;   end $$;\n", cfg); err != nil || got != "do $$ begin perform 1; end $$;\n" {
		t.Errorf("body: got %q (%v)", got, err)
	}
}

// A comment between an implicit alias and the next clause must not hide the alias: the search
// went on into the next SELECT and put the AS before the column `b` there (SELECT AS b).
func TestAliasFollowedByCommentIsFound(t *testing.T) {
	c := verbatimCfg(t)
	c.Aliases.As = config.AliasAsAdd
	for _, in := range []string{
		"select 0 b -- c\nunion select b, a;\n",
		"select 0 b /* c */ union select b, a;\n",
	} {
		got, err := formatter.Format(in, c)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(got, "0 as b") || strings.Contains(got, "select as") {
			t.Errorf("%q:\n%s", in, got)
		}
		if _, err := pg_query.Parse(got); err != nil {
			t.Errorf("%q: output does not parse: %v\n%s", in, err, got)
		}
	}
}

// A number that ends in a dot (0.) followed by a quoted alias: the inserted AS needs a blank
// before it (0.AS "0" is a lexical error).
func TestAliasAfterNumberEndingInDot(t *testing.T) {
	c := verbatimCfg(t)
	c.Aliases.As = config.AliasAsAdd
	got, err := formatter.Format("select 0.\"0\", 2. x from t;\n", c)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pg_query.Parse(got); err != nil {
		t.Errorf("output does not parse: %v\n%s", err, got)
	}
}

// The alias of a table name directly before a keyword that is no list separator (`UPDATE t b SET`):
// the search ran on and took a later `b` for the alias, so `aliases.as: add` wrote `SET x = AS b`.
func TestTableAliasIsTheTokenAfterTheName(t *testing.T) {
	c := verbatimCfg(t)
	c.Aliases.As = config.AliasAsAdd
	for in, want := range map[string]string{
		"update a b set x = b;\n":           "update a as b set x = b;\n",
		"update a ɴ set ɴ = ɴ;\n":           "update a as ɴ set ɴ = ɴ;\n",
		"update only s.a b set x = b;\n":    "update only s.a as b set x = b;\n",
		"select * from a /*x*/ b, c.d e;\n": "select * from a /*x*/ as b, c.d as e;\n",
	} {
		got, err := formatter.Format(in, c)
		if err != nil {
			t.Fatal(err)
		}
		if strings.ReplaceAll(got, "\n", " ") != strings.ReplaceAll(want, "\n", " ") {
			t.Errorf("%q:\n got %q\nwant %q", in, got, want)
		}
	}
}

// A vertical tab or form feed between an expression and its alias: the inserted AS kept it in front
// of itself, a later rule dropped it, and the AS was glued to the expression (`(a)AS b`).
func TestAliasAfterVerticalTabGetsABlank(t *testing.T) {
	c := verbatimCfg(t)
	c.Aliases.As = config.AliasAsAdd
	for _, in := range []string{"select (a)\vb;\n", "select (a)\fb;\n", "select (a)\tb;\n"} {
		got, err := formatter.Format(in, c)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(got, "(a) as b") {
			t.Errorf("%q: %q", in, got)
		}
	}
}

// cast_style: operator wrote CAST(a + 1 AS int) as a + 1::int, which casts only the 1, and glued a
// word that followed the ")" to the type (CAST(0 AS int)AND x → 0::intAND x). An argument that is
// no operand keeps the parentheses; the parse tree stays the same.
func TestCastStyleOperatorKeepsTheMeaning(t *testing.T) {
	c := verbatimCfg(t)
	c.CastStyle = config.CastStyleOperator
	for in, want := range map[string]string{
		"select cast(a + 1 as int);\n":                   "(a + 1)::int",
		"select cast(-1 as int);\n":                      "(-1)::int",
		"select cast(not a as int);\n":                   "(not a)::int",
		"select cast(a at time zone 'utc' as date);\n":   "(a at time zone 'utc')::date",
		"select cast(a in (select 1) as int);\n":         "(a in (select 1))::int",
		"select cast((a + 1) as int);\n":                 "(a + 1)::int",
		"select cast(f(a) as int), cast(a[1] as int);\n": "f(a)::int, a[1]::int",
		"select cast(0 as int)and true;\n":               "0::int and true",
		"select cast(exists(select 1) as int);\n":        "exists(select 1)::int",
	} {
		got, err := formatter.Format(in, c)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(got, want) {
			t.Errorf("%q: %q, want %q in it", in, got, want)
		}
		a, _ := pg_query.Fingerprint(in)
		b, err := pg_query.Fingerprint(got)
		if err != nil || a != b {
			t.Errorf("%q: parse tree changed (%v): %q", in, err, got)
		}
	}
}

// A table named as (s.as) is no AS keyword: aliases.as: remove dropped the table's name
// (merge into public.as a → merge into public.a), and add took it for the AS of the alias.
func TestTableNamedAsIsNoKeyword(t *testing.T) {
	c := verbatimCfg(t)
	c.Aliases.As = config.AliasAsRemove
	got, err := formatter.Format("select * from s.as x, t as y;\n", c)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "s.as x") || strings.Contains(got, "t as y") {
		t.Errorf("remove: %q", got)
	}
	c.Aliases.As = config.AliasAsAdd
	// (a column named as: the AS after it is the keyword, the alias follows it)
	got, err = formatter.Format("select t.as \"x\";\n", c)
	if err != nil {
		t.Fatal(err)
	}
	if again, _ := formatter.Format(got, c); again != got || !strings.Contains(got, "t.as as \"x\"") {
		t.Errorf("column named as: %q → %q", got, again)
	}
	got, err = formatter.Format("select * from s.as x, t y;\n", c)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "s.as as x") || !strings.Contains(got, "t as y") {
		t.Errorf("add: %q", got)
	}
}

// quoted_identifiers: remove_safe after a number that ends in a dot (select 0."a"): the name was
// glued to it (0.a, a lexical error).
func TestUnquotedAliasAfterNumberEndingInDot(t *testing.T) {
	c := verbatimCfg(t)
	c.QuotedIdents = config.QuotedIdentRemoveSafe
	got, err := formatter.Format("select 0.\"a0\";\n", c)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pg_query.Parse(got); err != nil {
		t.Errorf("output does not parse: %v\n%s", err, got)
	}
}

// A comment between AS and the alias: the comment was taken for the alias (the alias itself got no
// aliases.case, cased only by the next run), and aliases.as: add wrote a second AS after it.
func TestCommentBetweenAsAndAlias(t *testing.T) {
	c := verbatimCfg(t)
	c.Aliases.As = config.AliasAsAdd
	c.Aliases.Case = config.CaseLower
	got, err := formatter.Format("select 0 as --\nA, 1 as /* x */ B;\n", c)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(strings.ToLower(got), " as ") != 2 || !strings.Contains(got, "a,") || !strings.Contains(got, "*/ b") {
		t.Errorf("%q", got)
	}
	if again, _ := formatter.Format(got, c); again != got {
		t.Errorf("not idempotent: %q", again)
	}
}

// quoted_identifiers: remove_safe with a word right behind the closing quote ("a"é, a type name that
// starts with a letter beyond ASCII): the unquoted name was glued to it (a column named aé, no type).
func TestUnquotedNameBeforeAGluedWord(t *testing.T) {
	c, err := config.Load("../templates/ai.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, in := range []string{"create table t (\"a\"int);\n", "select 1 from t where \"x\"is null;\n", "create table a(\"a\"\u899f)", "create table a(\"a\"é)"} {
		got, err := formatter.Format(in, c)
		if err != nil {
			t.Fatal(err)
		}
		a, _ := pg_query.Fingerprint(in)
		if b, err := pg_query.Fingerprint(got); err != nil || a != b {
			t.Errorf("%q: %q (%v)", in, got, err)
		}
	}
}

// A keyword alias without AS is a bare label only where the parser does not read it as the next part
// of the expression: !0 at (AT TIME ZONE), count(*) over (a window). quoted_identifiers: remove_safe
// unquoted such aliases and aliases.as: remove dropped their AS, which left SQL that does not parse.
func TestKeywordAliasStaysParseable(t *testing.T) {
	c := verbatimCfg(t)
	c.QuotedIdents = config.QuotedIdentRemoveSafe
	c.Aliases.As = config.AliasAsRemove
	in := "select !0 \"at\", -1 as \"at\", count(*) as over, 2 as x from t;\n"
	got, err := formatter.Format(in, c)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := pg_query.Fingerprint(in)
	if b, err := pg_query.Fingerprint(got); err != nil || a != b {
		t.Errorf("%q (%v)", got, err)
	}
	if !strings.Contains(got, "2 x") {
		t.Errorf("AS of an ordinary alias not removed: %q", got)
	}
}
