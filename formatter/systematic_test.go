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

// Found by running the sweep over a large real project (every pair of option values over 330 000 statements).

func TestSQLBodyIndentNoneWithMixedTabsAndBlanks(t *testing.T) {
	c := wsCfg(t, func(c *config.Config) { c.Layout.DollarQuote.SQL.BodyIndent = config.BodyIndentNone })
	formatTwice(t, c, "CREATE FUNCTION f() RETURNS int LANGUAGE sql\n    AS $_$\n        SELECT 1\n\t, 2\n$_$;\n")
}

func TestBeginKeepsItsIndentWhenDeclareIsRemoved(t *testing.T) {
	c := wsCfg(t, func(c *config.Config) { c.Layout.DollarQuote.PLpgSQL.DeclareEmpty = config.WhenEmptyRemove })
	got := formatTwice(t, c, "CREATE FUNCTION f() RETURNS trigger LANGUAGE plpgsql\n AS $function$\n DECLARE\n BEGIN\n   RETURN NEW;\n END;\n $function$;\n")
	if !strings.Contains(got, "\n BEGIN\n") {
		t.Errorf("BEGIN lost its indent:\n%s", got)
	}
}

func TestInequalityAndCompactOperatorSpacingBeforeMinus(t *testing.T) {
	c := wsCfg(t, func(c *config.Config) {
		c.InequalityOp = config.InequalityANSI
		c.OperatorSpacing = config.OperatorSpacingCompact
	})
	formatTwice(t, c, "select a != -99, b <> -1 from t;\n")
}

func TestTableAfterQuotedSchemaIsLocated(t *testing.T) {
	c := wsCfg(t, func(c *config.Config) {
		c.QuotedIdents = config.QuotedIdentRemoveSafe
		c.Tables.Case = config.CaseCapitalize
	})
	got := formatTwice(t, c, "select 1 from \"live_opportunities\".lst_oppty_types as o;\n")
	if !strings.Contains(strings.ToLower(got), "live_opportunities.lst_oppty_types") || strings.Contains(got, "\"") {
		t.Errorf("unexpected: %s", got)
	}
}

func TestBodyStartingWithCommentIsStillABlock(t *testing.T) {
	c := wsCfg(t, func(c *config.Config) { c.Layout.DollarQuote.SQL.BodyIndent = config.BodyIndentIndent })
	formatTwice(t, c, "CREATE FUNCTION f() RETURNS int LANGUAGE plpgsql\n    AS $$  /*DOCU\nx\nDOCU*/\nBEGIN\n  RETURN 1;\nEND;\n$$;\n")
}

func TestCaseBranchBodyThatMovesIsLaidOutAsLineStart(t *testing.T) {
	c := wsCfg(t, func(c *config.Config) {
		c.Layout.Clauses.Break = config.BreakAlways
		c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Simple.BodyBreak = config.BreakAlways
	})
	formatTwice(t, c, "CREATE FUNCTION f(a int) RETURNS void LANGUAGE plpgsql AS $$\nBEGIN\n    CASE a\n        WHEN 1\n            THEN INSERT INTO t(x)\n                 VALUES (1);\n\n        WHEN 2\n            THEN UPDATE t\n                    SET x = 2\n                  WHERE y = 1;\n    END CASE;\nEND;\n$$;\n")
}

func TestFlattenedIfConditionLineAfterCommentIsIndented(t *testing.T) {
	c := wsCfg(t, func(c *config.Config) {
		c.Layout.Subquery.Break = config.BreakNever
		c.Layout.DollarQuote.PLpgSQL.BeginBody.Indent = config.BodyIndentIndent
	})
	formatTwice(t, c, "do $$\nbegin\n\n    IF (\n            SELECT a = 1 -- retail\n            FROM t\n        )\n    THEN\n        RETURN NEW;\n    END IF;\n\n    x := 1;\nend $$;\n")
}

// A subquery that ends in a -- comment is not joined to one line: its ")" would have to stay on a
// line of its own, and at the margin it set the indent the whole body was moved by.
func TestJoinedSubqueryClosingParenAfterCommentStaysUnderItsOpener(t *testing.T) {
	c := plainCfg(t, func(c *config.Config) {
		c.Layout.Subquery.Break = config.BreakNever
		c.Layout.DollarQuote.PLpgSQL.BeginBody.Indent = config.BodyIndentIndent
	})
	formatTwice(t, c, "CREATE FUNCTION f() RETURNS int LANGUAGE plpgsql AS $$\nBEGIN\n\n    IF a THEN -- x\n\n        UPDATE t\n        SET a = 1;\n\n        IF NOT EXISTS(\n                        SELECT FROM t\n                        WHERE b = 1 -- c\n                     )\n        THEN\n            RETURN 1;\n        END IF;\n\n    END IF;\nEND\n$$;\n")
}

// A CASE branch whose body moves to a line of its own (always, or auto with several statements) is
// laid out as the line-start statement it becomes — river alignment included.
func TestCaseBranchMovingBodyKeepsRiverAlignment(t *testing.T) {
	c := wsCfg(t, func(c *config.Config) {
		c.Layout.Clauses.Break = config.BreakAlways
		c.Layout.Clauses.Align = config.AlignRight
		c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Simple.BodyBreak = config.BreakAlways
	})
	formatTwice(t, c, "DO $$\nBEGIN\n    CASE x\n        WHEN 'U'\n            THEN UPDATE t\n                    SET v = 1\n                  WHERE a = 2;\n\n                 INSERT INTO t(a)\n                 VALUES (1);\n    END CASE;\nEND;\n$$;\n")
	c = wsCfg(t, func(c *config.Config) {
		c.Layout.Clauses.Break = config.BreakAlways
		c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Simple.BodyBreak = config.BreakAuto
	})
	formatTwice(t, c, "DO $$\nBEGIN\n    CASE x\n        WHEN 'U'\n            THEN UPDATE t\n                    SET v = 1\n                  WHERE a = 2;\n\n                 INSERT INTO t(a)\n                 VALUES (1);\n    END CASE;\nEND;\n$$;\n")
}

func TestOwnLineParenOfStatementAfterThenKeepsTabs(t *testing.T) {
	c := wsCfg(t, func(c *config.Config) {
		c.KeywordParenSpacing = config.ParenSpacingRemove
		c.Layout.Indent.Type = config.IndentTypeTab
		c.Layout.ParenIndent.Open = config.ParenOpenOwnLine
		c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Simple.BodyBreak = config.BreakAuto
	})
	formatTwice(t, c, "DO $$\nBEGIN\n    CASE x\n        WHEN 'I' THEN\n\n            INSERT INTO t\n            (\n                a,\n                b\n            )\n            VALUES\n            (\n                1,\n                2\n            );\n    END CASE;\nEND;\n$$;\n")
}

// A subquery measured for subquery.break: auto counts blank runs as one (a CASE inside is reformatted
// afterwards, which takes them off).
func TestSubqueryAutoMeasuresWithoutBlankRuns(t *testing.T) {
	c := wsCfg(t, func(c *config.Config) {
		c.Layout.Subquery.Break = config.BreakAuto
		c.Layout.Case.Break = config.BreakAuto
		c.Layout.LineLength = 200
	})
	formatTwice(t, c, "DO $$\nBEGIN\n    RETURN QUERY\n        WITH\n            s AS(\n                SELECT 1 FROM t\n            )\n        ,   test AS(\n                SELECT\n                    CASE\n                        WHEN( 1 <(SELECT count(1) FROM s)\n                            )\n                        THEN    public.raise_exception('Function registration_api.get_data_for_reset_password(text) ERROR: found more than 1 row.')\n                        ELSE    1\n                    END AS cnt\n            )\n        SELECT * FROM s\n        WHERE 0 <= (SELECT * FROM test)\n    ;\nEND\n$$;\n")
}

func TestParenIndentIndentDoesNotShiftItemsEveryRun(t *testing.T) {
	c := wsCfg(t, func(c *config.Config) {
		c.Layout.Clauses.Break = config.BreakAuto
		c.Layout.Clauses.Align = config.AlignIndent
		c.Layout.Content.Break = config.BreakAlways
		c.Layout.ParenIndent.Mode = config.ParenIndentIndent
		c.Layout.ParenIndent.CloseFirst = config.ParenCloseAfter
	})
	formatTwice(t, c, "SELECT * FROM crosstab(\n'select 1\n  from t',\n'select sport_name from opportunities.lst_sports where id_sport in (184,185,200,201,186,183,224,188,187,217,238,222,229,223,198,202,195,226,209)'\n)\n\nAS ct(a text,\n   b text)\nORDER BY 1;\n")
	c = wsCfg(t, func(c *config.Config) {
		c.Layout.Clauses.Break = config.BreakAuto
		c.Layout.Clauses.Align = config.AlignRight
		c.Layout.Content.Break = config.BreakAuto
		c.Layout.ParenIndent.Mode = config.ParenIndentIndent
	})
	formatTwice(t, c, "CREATE TEMPORARY TABLE items AS\nSELECT ti.id_ticket, o.*\nFROM tickets AS tti\n    JOIN tickets AS ti USING(id_ticket)\n    JOIN LATERAL\n    (\n        SELECT id_odds, MAX(odds_history_time) AS odds_change_time\n        FROM h AS toh\n        WHERE id_odds = tti.id_odds\n        GROUP BY id_odds\n    ) AS o ON TRUE\nWHERE x IS NULL;\n")
	c = wsCfg(t, func(c *config.Config) {
		c.Layout.Clauses.Break = config.BreakAuto
		c.Layout.Clauses.Align = config.AlignIndent
		c.Layout.Content.Break = config.BreakAlways
		c.Layout.ParenIndent.Mode = config.ParenIndentIndent
	})
	formatTwice(t, c, "DO $$\nBEGIN\n    LOOP\n\n        INSERT INTO terminals.lst_terminals AS lt\n        (\n            id_terminal,\n            terminal_supports_cash\n        )\n        VALUES\n        (\n            _rcd.id_terminal,\n            _rcd.terminal_supports_cash\n        )\n        ON CONFLICT (id_terminal)\n        DO UPDATE SET\n            id_branch                = EXCLUDED.id_branch,\n            terminal_supports_cash   = EXCLUDED.terminal_supports_cash;\n\n        COMMIT;\n    END LOOP;\nEND\n$$;\n")
}

func TestDollarOpenWithTrailingBlankAndNewlineAfterOpenRemove(t *testing.T) {
	c := plainCfg(t, func(c *config.Config) { c.Layout.DollarQuote.NewlineAfterOpen = config.BlankLineRemove })
	got := formatTwice(t, c, "CREATE FUNCTION f() RETURNS trigger\n    LANGUAGE plpgsql\n    AS $$ \nBEGIN\n  RETURN NEW;\nEND;\n$$;\n")
	if strings.Contains(got, "$$\n\nBEGIN") || strings.Contains(got, "$$\n \nBEGIN") {
		t.Errorf("a blank line is left after the opening delimiter:\n%s", got)
	}
}

func TestIfBlankLineAfterRemoveTakesLinesOfBlanks(t *testing.T) {
	c := plainCfg(t, func(c *config.Config) {
		c.Layout.DollarQuote.PLpgSQL.ControlFlow.If.BlankLineAfter = config.BlankLineRemove
		c.Layout.Indent.Type = config.IndentTypeTab
		c.Layout.Indent.Normalize = config.IndentNormalizeChange
		c.Layout.Indent.Remainder = config.IndentRemainderRemove
		c.Layout.Indent.Size = 3
	})
	got := formatTwice(t, c, "CREATE FUNCTION f(a int) RETURNS int LANGUAGE plpgsql AS $$\nBEGIN\n  IF a IS NULL THEN\n  \n    RETURN NULL;\n  \n  END IF;\n  RETURN 1;\nEND;\n$$;\n")
	if strings.Contains(got, "THEN\n\n") || strings.Contains(got, "THEN\n \n") || strings.Contains(got, "THEN\n\t\n") {
		t.Errorf("a line of blanks was left after THEN:\n%s", got)
	}
}

// A statement whose own `;` stands in a comment runs on into the END of its block: it is no statement
// to lay out (end_semicolon: add makes it look like one on the next run).
func TestStatementWithSemicolonInCommentIsNotLaidOutWithTheEnd(t *testing.T) {
	c := wsCfg(t, func(c *config.Config) {
		c.Layout.DollarQuote.PLpgSQL.EndSemicolon = config.EndSemicolonAdd
		c.Layout.Clauses.Break = config.BreakAlways
	})
	formatTwice(t, c, "DO $$\nBEGIN\nUPDATE  t\n    SET a = 1\nWHERE   id = 820\n  AND   x = 174   -- EMAIL (kontakt);\nEND\n$$;\n")
}

func TestJoinedSubqueryKeepsParenSpacingAdd(t *testing.T) {
	c := wsCfg(t, func(c *config.Config) {
		c.ParenSpacing = config.ParenSpacingAdd
		c.Layout.Subquery.Break = config.BreakNever
		c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Simple.BodyBreak = config.BreakNever
	})
	formatTwice(t, c, "DO $$\nBEGIN\n    CASE tg_op\n        WHEN 'DELETE' THEN\n            PERFORM f(old.id, FALSE)\n               FROM m AS map\n              WHERE map.s = OLD.s\n                AND EXISTS(SELECT FROM u WHERE id = OLD.id)\n                AND EXISTS(SELECT FROM v WHERE id = OLD.id);\n    END CASE;\nEND\n$$;\n")
}

func TestReturnsTableBreakDoesNotDependOnTheBlankBeforeItsParen(t *testing.T) {
	c := plainCfg(t, func(c *config.Config) {
		c.Layout.DDL.CreateFunction.Params = config.BreakAuto
		c.Layout.DDL.CreateFunction.Options = config.BreakNever
		c.Layout.LineLength = 80
	})
	formatTwice(t, c, "CREATE OR REPLACE FUNCTION terminals_api.get_terminal_branch_daily_report_data_card_payments(_id_terminal INTEGER, _id_branch INTEGER, _date DATE)\n RETURNS TABLE\n (\n    report_type VARCHAR,\n    report_amount NUMERIC,\n    report_count BIGINT\n )\n LANGUAGE plpgsql\nAS $function$\nBEGIN\n  RETURN;\nEND;\n$function$;\n")
}

func TestForInUpdateReturningIsACursorQuery(t *testing.T) {
	c := plainCfg(t, func(c *config.Config) {
		c.Layout.Clauses.Align = config.AlignRight
		c.Layout.ParenIndent.Mode = config.ParenIndentNone
		c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Simple.BlankLineAfter = config.BlankLineRemove
	})
	formatTwice(t, c, "CREATE FUNCTION f() RETURNS void LANGUAGE plpgsql AS $$\nBEGIN\n\n    FOR r IN\n        UPDATE t\n        SET is_active = FALSE\n        WHERE expired_at < NOW()\n          AND is_active\n        RETURNING *\n    LOOP\n\n        INSERT INTO u(a) VALUES (r.a);\n\n    END LOOP;\nEND;\n$$;\n")
}

func TestItemAfterCommentLineMovesInWithItsFirstLine(t *testing.T) {
	c := wsCfg(t, func(c *config.Config) {
		c.Layout.Clauses.Break = config.BreakAuto
		c.Layout.Content.Break = config.BreakAlways
		c.Layout.Case.Break = config.BreakAlways
	})
	formatTwice(t, c, "DO $$\nBEGIN\n    SELECT\n        t.ticket_time_bet,\n        t.ticket_win,\n        --t.id_ticket_state\n        CASE WHEN t.id_ticket_state = 4 THEN 1 ELSE 0 END as id_ticket_state\n    FROM tick t\n    LEFT JOIN\n    (\n    SELECT dc.client_email, dc.id_client\n        FROM dblink('conn', _str_client, TRUE) AS dc(client_email VARCHAR, id_client INTEGER)\n    ) ci ON ci.id_client = t.id_client\n ORDER BY  ticket_time_bet ;\nEND\n$$;\n")
}

func TestIfConditionLineLeftOfTheKeywordMovesWithIt(t *testing.T) {
	c := wsCfg(t, func(c *config.Config) {
		c.KeywordParenSpacing = config.ParenSpacingRemove
		c.Layout.Subquery.Break = config.BreakAlways
		c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Simple.BlankLineAfter = config.BlankLineAdd
	})
	formatTwice(t, c, "DO $$\nBEGIN\n        IF a THEN\n\n            IF NEW.id_oppty_state = 9 -- State 9 allowed\n           AND NOT EXISTS\n                    (\n                        SELECT\n                        FROM opportunities.tab_odds\n                        WHERE id_opportunity = NEW.id_opportunity\n                          AND odds_is_winning IS NOT NULL\n                    )\n            THEN\n                _ok = FALSE;\n            END IF;\n        END IF;\nEND\n$$;\n")
}

// plainCfg is the default configuration (every layout option preserve) with the options of one test:
// what the sweeps over the option space ran.
func plainCfg(t *testing.T, mutate func(*config.Config)) *config.Config {
	t.Helper()
	c, err := config.Load("/nonexistent/.pg_procrustes.yaml")
	if err != nil {
		t.Fatal(err)
	}
	mutate(c)
	if err := config.Validate(c); err != nil {
		t.Fatal(err)
	}
	return c
}

// The body of a CASE branch is the text up to the next control-flow event: a branch that holds a
// nested IF after its first statement is not one of several statements for body_break: auto.
func TestCaseBranchAutoWithNestedIfIsOneStatementBody(t *testing.T) {
	c := plainCfg(t, func(c *config.Config) {
		c.Layout.Clauses.Break = config.BreakAlways
		c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Simple.BodyBreak = config.BreakAuto
	})
	formatTwice(t, c, "DO $$\nBEGIN\n    CASE x\n        WHEN 1 THEN\n            SELECT lower(v)\n              INTO prev\n              FROM t\n             WHERE a = 1\n            FOR UPDATE;\n\n            IF prev = 2 THEN\n                y := 1;\n            END IF;\n    END CASE;\nEND\n$$;\n")
}

func TestParenIndentIndentLeavesWhatFollowsACallAlone(t *testing.T) {
	c := plainCfg(t, func(c *config.Config) {
		c.Layout.Clauses.Break = config.BreakAuto
		c.Layout.Clauses.Align = config.AlignIndent
		c.Layout.Content.Align = config.AlignIndent
		c.Layout.Content.Break = config.BreakAlways
		c.Layout.ParenIndent.Mode = config.ParenIndentIndent
		c.Layout.ParenIndent.CloseFirst = config.ParenCloseAfter
	})
	formatTwice(t, c, "SELECT x.*\nFROM dblink('conn_clients_and_terminals',\n            'SELECT * FROM terminals.lst_branches AS lb\n            WHERE lb.id_branch <= ' || _process, TRUE)\nAS x\n(\n    id_branch INTEGER,\n    branch_name CHARACTER VARYING,\n    branch_authorization_limit_coef NUMERIC\n)\nORDER BY id_branch DESC;\n")
}

func TestIfConditionWithoutSubqueryKeepsItsContinuationLines(t *testing.T) {
	c := plainCfg(t, func(c *config.Config) {
		c.Layout.DollarQuote.PLpgSQL.BeginBody.Indent = config.BodyIndentIndent
		c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Simple.BlankLineAfter = config.BlankLineRemove
	})
	formatTwice(t, c, "DO $$\nBEGIN\n\n    IF( a > 0\n  AND b = 1\n  )THEN\n\n IF c = 1 THEN -- x\n    _str = 1;\n END IF;\n\n    END IF;\nEND\n$$;\n")
}

func TestSubqueryAutoMeasuresInnerSubqueryWithoutItsBlanks(t *testing.T) {
	c := plainCfg(t, func(c *config.Config) {
		c.Layout.Subquery.Break = config.BreakAuto
		c.Layout.LineLength = 200
	})
	formatTwice(t, c, "WITH filtered AS(SELECT 1 AS id_client)\n,   to_set AS(\n        SELECT  min(id_client_action_history)\n        FROM    clients.tab_client_action_history\n        WHERE   id_client IN( SELECT id_client FROM filtered )\n          AND   id_client_action_type = 1\n        GROUP BY id_client\n    )\nSELECT * FROM to_set;\n")
}

func TestItemAfterCommentAndBlankLineIsNotShiftedEveryRun(t *testing.T) {
	c := plainCfg(t, func(c *config.Config) {
		c.Layout.Clauses.Break = config.BreakAlways
		c.Layout.Content.Break = config.BreakAuto
		c.Layout.Content.Align = config.AlignIndent
		c.Layout.Content.CommaPosition = config.CommaEnd
		c.Layout.ParenIndent.Open = config.ParenOpenOwnLine
	})
	formatTwice(t, c, "INSERT -- VALIDATE\n    INTO validator.tab_scenario_queries\n        ( id_scenario_query\n        )\n    VALUES\n        (   1,   1,  1,  1,  1.1    , 'test - success' )\n    --  sts_pl_online\n\n        -- client_reg_step_1:\n    ,   (1201, 300, 11,  101, 1 , 'verify_token (have__valid_token)');\n")
}

func TestInsertColumnListInlineFirstItemGetsParenSpacingAdd(t *testing.T) {
	c := plainCfg(t, func(c *config.Config) {
		c.ParenSpacing = config.ParenSpacingAdd
		c.Layout.Clauses.Break = config.BreakAuto
		c.Layout.Content.Break = config.BreakAuto
		c.Layout.Content.FirstItem = config.FirstItemInline
	})
	formatTwice(t, c, "INSERT INTO dwh_kafka.lst_oppty_regions_conf_prematch\n(\n    oppty_region_is_bonus,\n    oppty_region_betradarunity_id,\n    oppty_region_betradarunity_name\n)\nVALUES (1, 2, 3);\n")
}

// An item that follows a comment line is moved to the item's indent as a whole before the generic
// indentation looks at it: a trailing -- comment inside it made that indent only the line after the comment.
func TestCteBehindCommentWithTrailingCommentAndTabs(t *testing.T) {
	c := plainCfg(t, func(c *config.Config) {
		c.Layout.Clauses.Break = config.BreakAlways
		c.Layout.Content.Align = config.AlignIndent
		c.Layout.Content.Break = config.BreakAuto
		c.Layout.Indent.Type = config.IndentTypeTab
		c.Layout.Subquery.Break = config.BreakAuto
	})
	formatTwice(t, c, "CREATE FUNCTION f() RETURNS int LANGUAGE sql AS $$\n    WITH\n    -- select related offer entries\n    oppty AS\n    (\n        SELECT o.id_opportunity, o.oppty_end_date + o.oppty_end_time AS match_timestamp, l.id_league, l.league_name\n        FROM live_opportunities.tab_opportunities AS o\n        JOIN live_opportunities.tab_odds            AS od   USING(id_opportunity)\n          AND oppty_end_date     <= _date_to\n          AND o.id_data_provider =  _provider -- 8=RTS\n    ),\n    b AS (SELECT 1)\n    SELECT * FROM oppty;\n$$;\n")
}

func TestRiverInsertColumnListClosesWithParenSpacingAdd(t *testing.T) {
	c := plainCfg(t, func(c *config.Config) {
		c.ParenSpacing = config.ParenSpacingAdd
		c.Layout.Clauses.Break = config.BreakAuto
		c.Layout.Clauses.Align = config.AlignRight
		c.Layout.Content.Break = config.BreakAuto
		c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Simple.BodyBreak = config.BreakAuto
	})
	formatTwice(t, c, "DO $$\nBEGIN\n    CASE TG_OP\n        WHEN 'INSERT' THEN\n            INSERT INTO dwh_kafka.lst_oppty_regions_conf_prematch\n            (\n                oppty_region_is_bonus,\n                oppty_region_betradarunity_id,\n                oppty_region_betradarunity_name\n            )\n            VALUES (1, 2, 3);\n    END CASE;\nEND;\n$$;\n")
}
