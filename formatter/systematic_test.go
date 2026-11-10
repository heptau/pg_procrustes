package formatter_test

import (
	"strings"
	"testing"

	"github.com/heptau/pg_procrustes/config"
	"github.com/heptau/pg_procrustes/formatter"
)

// Found by running every option value, and pairs of them, over real files
// (scripts/stmt_matrix.go with configs made from the reference config): each case is
// formatted twice and the second run must not change the first one's text.
func formatTwice(t *testing.T, c *config.Config, in string) string {
	t.Helper()
	got, err := formatter.Format(in, c)
	if err != nil {
		t.Fatal(err)
	}
	if again, _ := formatter.Format(got, c); again != got {
		t.Errorf("not idempotent:\n%s\n--\n%s", got, again)
	}
	return got
}

func TestIfConditionLinesMoveWithTheKeyword(t *testing.T) {
	c := wsCfg(t, func(c *config.Config) {
		c.Layout.DollarQuote.PLpgSQL.ControlFlow.If.BodyIndent = config.BodyIndentIndent
		c.Layout.Subquery.Break = config.BreakAlways
	})
	got := formatTwice(t, c, "create function f() returns int language plpgsql as $fn$\nbegin\n\tif exists (select 1 from t where a = b) then\n\t\treturn 1;\n\tend if;\nend;\n$fn$;\n")
	if !strings.Contains(got, "   IF EXISTS (\n      SELECT 1 FROM t WHERE a = b\n   ) THEN\n") {
		t.Errorf("condition lines did not follow the IF:\n%s", got)
	}
}

func TestQuotedNameAfterDotCaseAfterUnquoting(t *testing.T) {
	c := wsCfg(t, func(c *config.Config) {
		c.QuotedIdents = config.QuotedIdentRemoveSafe
		c.Columns.Case = config.CaseUpper
	})
	got := formatTwice(t, c, "comment on column cas.\"cas\" is 'x';\n")
	if !strings.Contains(got, "cas.CAS") {
		t.Errorf("unquoted name not cased like a column: %s", got)
	}
}

func TestTrailingCommentsOfLinesThatChangeTheirIndentType(t *testing.T) {
	in := "DO $$\nBEGIN\n-- Sensor table\nCREATE TABLE telemetry.sensor (\n\tchannel CHAR(1) PRIMARY KEY,\n\tlocation_type TEXT NOT NULL, -- 'start'\n\ttime_rounding_decimals SMALLINT NOT NULL DEFAULT 2, -- 1=tenths\n\tsuppress_interval_ms INTEGER NOT NULL DEFAULT 500, -- ignore\n\tis_active BOOLEAN DEFAULT TRUE\n);\nEND\n$$;\n"
	for _, typ := range []config.IndentTypeVal{config.IndentTypeTab, config.IndentTypeSpaces} {
		c := wsCfg(t, func(c *config.Config) {
			c.Comments.AlignTrailing = config.CommentAlignAlign
			c.Layout.Indent.Type = typ
			c.Layout.Indent.Normalize = config.IndentNormalizeChange
			c.Layout.DollarQuote.PLpgSQL.BeginBody.Indent = config.BodyIndentIndent
		})
		formatTwice(t, c, in)
	}
}

func TestStringConstantLinesAreDataInIndentedBodyStatements(t *testing.T) {
	c := wsCfg(t, func(c *config.Config) {
		c.Layout.Clauses.Break = config.BreakAlways
		c.Layout.Content.Break = config.BreakAlways
	})
	in := "create function f() returns void language plpgsql as $$\nbegin\n  insert into t(a)\n  values ('line1\nline2\n    line3');\nend\n$$;\n"
	got := formatTwice(t, c, in)
	if !strings.Contains(got, "('line1\nline2\n    line3')") {
		t.Errorf("lines of the string constant were indented:\n%s", got)
	}
}

func TestIndentNormalizeLeavesDollarQuotedTextAlone(t *testing.T) {
	c := wsCfg(t, func(c *config.Config) {
		c.Layout.Indent.Type = config.IndentTypeTab
		c.Layout.Indent.Normalize = config.IndentNormalizeChange
	})
	in := "do $$\nbegin\ncomment on table t is 'a\n   b\n      c';\nend\n$$;\ncomment on table t is $x$u\n   v\n$x$;\n"
	got := formatTwice(t, c, in)
	if !strings.Contains(got, "'a\n   b\n      c'") || !strings.Contains(got, "$x$u\n   v\n$x$") {
		t.Errorf("the blanks of a text were changed:\n%s", got)
	}
}

func TestCaseStatementCommentBeforeThenStaysOnItsLine(t *testing.T) {
	in := "create function f(int) returns int language plpgsql as $$\nbegin\n  case $1\n    when 1 -- why\n      then return 1;\n    else -- other\n      return 2;\n  end case;\nend;\n$$;\n"
	for _, tb := range []config.BreakMode{config.BreakNever, config.BreakAuto, config.BreakAlways} {
		c := wsCfg(t, func(c *config.Config) {
			c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Simple.ThenBreak = tb
			c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Simple.BodyBreak = tb
		})
		got := formatTwice(t, c, in)
		if !strings.Contains(got, "-- why\n") || !strings.Contains(got, "-- other\n") {
			t.Errorf("then_break %s: code joined onto a comment:\n%s", tb, got)
		}
	}
}

func TestDeclareAndBeginOnTheLineOfTheirKeyword(t *testing.T) {
	c := wsCfg(t, func(c *config.Config) {
		c.Layout.DollarQuote.PLpgSQL.Declare.Indent = config.BodyIndentNone
		c.Layout.DollarQuote.PLpgSQL.BeginBody.Indent = config.BodyIndentNone
	})
	got := formatTwice(t, c, "do $$ declare a int[];\nbegin a := array[1,2]; raise notice 'x'; end$$;\n")
	if strings.Contains(strings.ToLower(got), "declarea") || strings.Contains(strings.ToLower(got), "begina") {
		t.Errorf("keyword glued to its text:\n%s", got)
	}
}

func TestIfBlankLineBeforeLeavesOneLineIfAlone(t *testing.T) {
	for _, act := range []config.BlankLineAction{config.BlankLineRemove, config.BlankLineAdd} {
		c := wsCfg(t, func(c *config.Config) { c.Layout.DollarQuote.PLpgSQL.ControlFlow.If.BlankLineBefore = act })
		got := formatTwice(t, c, "do $$\nbegin\n  for i in 1 .. 3 loop\n    if i > 1 then x := 1; end if;\n    y := 2;\n  end loop;\nend $$;\n")
		if !strings.Contains(got, "IF i > 1 THEN x := 1; END IF;") {
			t.Errorf("%s: one-line IF broken up:\n%s", act, got)
		}
	}
}

func TestPlpgsqlBodyFlushLeftKeywordsAreFlush(t *testing.T) {
	c := wsCfg(t, func(c *config.Config) {
		c.Layout.DollarQuote.PLpgSQL.BeginBody.Indent = config.BodyIndentNone
		c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Simple.BlankLineAfter = config.BlankLineAdd
	})
	got := formatTwice(t, c, "create function f(a int) returns int language plpgsql as $fn$\nbegin\n  if a is null then\n    return 1;\n  end if;\n  return 2;\nend;\n$fn$;\n")
	if !strings.Contains(got, "\nIF a IS NULL THEN\n") || !strings.Contains(got, "\nEND IF;\n") {
		t.Errorf("IF not flush left:\n%s", got)
	}
}

func TestIfBodyIndentLeavesStringConstantLinesAlone(t *testing.T) {
	c := wsCfg(t, func(c *config.Config) {
		c.Layout.DollarQuote.PLpgSQL.ControlFlow.If.BodyIndent = config.BodyIndentIndent
	})
	in := "create function f(a int) returns text language plpgsql as $$\nbegin\n  if a > 1 then\n    return 'x\n      y\n  z';\n  end if;\n  return '';\nend;\n$$;\n"
	got := formatTwice(t, c, in)
	if !strings.Contains(got, "'x\n      y\n  z'") {
		t.Errorf("string constant changed:\n%s", got)
	}
}

func TestIfBlankLineAfterWithInlineStatementAndBlanks(t *testing.T) {
	for _, act := range []config.BlankLineAction{config.BlankLineRemove, config.BlankLineAdd} {
		c := wsCfg(t, func(c *config.Config) { c.Layout.DollarQuote.PLpgSQL.ControlFlow.If.BlankLineAfter = act })
		in := "do $$\nbegin\n  if a then x := 1;\n  end if;\n  if b then\n  y := 1;\n  else \n    y := 2;\n  end if;\nend $$;\n"
		got := formatTwice(t, c, in)
		if strings.Contains(got, "\n \n") || strings.Contains(got, "\n x := 1") {
			t.Errorf("%s: stray blanks:\n%q", act, got)
		}
	}
}

func TestSubqueryAutoInBodyDoesNotCountIndentSetLater(t *testing.T) {
	c := wsCfg(t, func(c *config.Config) {
		c.Layout.LineLength = 80
		c.Layout.Indent.Size = 4
		c.Layout.Subquery.Break = config.BreakAuto
		c.Layout.DollarQuote.PLpgSQL.ControlFlow.If.BodyIndent = config.BodyIndentIndent
	})
	in := "create function f() returns trigger language plpgsql as $$\nbegin\n  if TG_OP='INSERT' then\n    if new.p is not null and not exists (select true from accounting.id_p where cislo=new.cislo or cislo=old.cislo) then\n    end if;\n  end if;\nend;\n$$;\n"
	formatTwice(t, c, in)
}

func TestQuotedNameAfterAsInXmlFunctionKeepsItsQuotes(t *testing.T) {
	c := wsCfg(t, func(c *config.Config) {
		c.QuotedIdents = config.QuotedIdentRemoveSafe
		c.Columns.Case = config.CaseUpper
	})
	formatTwice(t, c, "create function f() returns xml language plpgsql as $$\nbegin\n  return xmlelement(name a, xmlattributes(1 as \"period\"));\nend;\n$$;\n")
}
