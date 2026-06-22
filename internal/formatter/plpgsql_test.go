package formatter_test

import (
	"testing"

	"github.com/heptau/pg_procrustes/internal/config"
	"github.com/heptau/pg_procrustes/internal/formatter"
)

// plCfg returns a base config for PL/pgSQL formatting tests using all-preserve defaults.
func plCfg() *config.Config {
	return &config.Config{
		ReservedKeywords:     config.Section{Case: config.CasePreserve},
		Keywords:             config.Section{Case: config.CasePreserve},
		DataTypes:            config.DataTypesSection{Case: config.CasePreserve, Form: config.TypeFormPreserve},
		Literals:             config.Section{Case: config.CasePreserve},
		Operators:            config.Section{Case: config.CasePreserve},
		Schemas:              config.Section{Case: config.CasePreserve},
		Tables:               config.Section{Case: config.CasePreserve},
		Functions:            config.Section{Case: config.CasePreserve},
		ConditionalFunctions: config.Section{Case: config.CasePreserve},
		SystemFunctions:      config.Section{Case: config.CasePreserve},
		Aliases:              config.AliasSection{Case: config.CasePreserve, As: config.AliasAsPreserve},
		Columns:              config.Section{Case: config.CasePreserve},
		TrailingWhitespace:   config.TrailingWSPreserve,
		Semicolons:           config.SemicolonPreserve,
		InequalityOp:         config.InequalityPreserve,
		JoinForm:             config.JoinFormPreserve,
		OperatorSpacing:      config.OperatorSpacingPreserve,
		BlankLines:           config.BlankLinesPreserve,
		ParenSpacing:         config.ParenSpacingPreserve,
		QuotedIdents:         config.QuotedIdentPreserve,
		TrailingNewline:      config.TrailingNewlinePreserve,
		CommaSpacing:         config.CommaSpacingPreserve,
		OrderAsc:             config.OrderAscPreserve,
		CastStyle:            config.CastStylePreserve,
		NotIn:                config.NotInPreserve,
		SchemaQual:           config.SchemaQualPreserve,
		Layout:               config.DefaultLayout(),
	}
}

func wrap(body string) string {
	return "CREATE FUNCTION f() RETURNS void AS $$\n" + body + "\n$$ LANGUAGE plpgsql"
}

// ── IF formatting ─────────────────────────────────────────────────────────────

// TestPLpgSQLIfPreserve verifies that with all-preserve config, IF blocks are untouched.
func TestPLpgSQLIfPreserve(t *testing.T) {
	c := plCfg()
	src := wrap("BEGIN\n   IF x THEN\n      y := 1;\n   END IF;\nEND")
	got, err := formatter.Format(src, c)
	assertFormat(t, got, src, err)
}

// TestPLpgSQLIfBodyIndentIndent normalizes body to exactly one additional indent level.
func TestPLpgSQLIfBodyIndentIndent(t *testing.T) {
	c := plCfg()
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.If.BodyIndent = config.BodyIndentIndent
	c.Layout.Indent.Size = 3
	// body has no indent → should gain one indent level (3 spaces, matching IF depth+1 = depth 2)
	src := wrap("BEGIN\n   IF x THEN\ny := 1;\n   END IF;\nEND")
	got, err := formatter.Format(src, c)
	want := wrap("BEGIN\n   IF x THEN\n      y := 1;\n   END IF;\nEND")
	assertFormat(t, got, want, err)
}

// TestPLpgSQLIfBodyIndentNone is a no-op in the control-flow normalizer (only BodyIndentIndent normalizes).
func TestPLpgSQLIfBodyIndentNone(t *testing.T) {
	c := plCfg()
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.If.BodyIndent = config.BodyIndentNone
	src := wrap("BEGIN\n   IF x THEN\n      y := 1;\n   END IF;\nEND")
	got, err := formatter.Format(src, c)
	// BodyIndentNone is a no-op in normalizeBodyLines — body preserved as-is.
	assertFormat(t, got, src, err)
}

// TestPLpgSQLIfElsif verifies ELSIF/ELSE branches get the same body indent applied.
func TestPLpgSQLIfElsif(t *testing.T) {
	c := plCfg()
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.If.BodyIndent = config.BodyIndentIndent
	c.Layout.Indent.Size = 3
	src := wrap("BEGIN\n   IF x = 1 THEN\nstmt1;\n   ELSIF x = 2 THEN\nstmt2;\n   ELSE\nstmt3;\n   END IF;\nEND")
	got, err := formatter.Format(src, c)
	want := wrap("BEGIN\n   IF x = 1 THEN\n      stmt1;\n   ELSIF x = 2 THEN\n      stmt2;\n   ELSE\n      stmt3;\n   END IF;\nEND")
	assertFormat(t, got, want, err)
}

// TestPLpgSQLIfBlankLineBefore adds a blank line before IF and before END IF keywords.
func TestPLpgSQLIfBlankLineBefore(t *testing.T) {
	c := plCfg()
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.If.BlankLineBefore = config.BlankLineAdd
	src := wrap("BEGIN\n   IF x THEN\n      y := 1;\n   END IF;\nEND")
	got, err := formatter.Format(src, c)
	// BlankLineBefore applies to each keyword in the block: IF and END IF.
	want := wrap("BEGIN\n\n   IF x THEN\n      y := 1;\n\n   END IF;\nEND")
	assertFormat(t, got, want, err)
}

// TestPLpgSQLIfBlankLineAfter adds a blank line directly after the THEN keyword (before body).
func TestPLpgSQLIfBlankLineAfter(t *testing.T) {
	c := plCfg()
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.If.BlankLineAfter = config.BlankLineAdd
	src := wrap("BEGIN\n   IF x THEN\n      y := 1;\n   END IF;\nEND")
	got, err := formatter.Format(src, c)
	// BlankLineAfter applies after the THEN/ELSE opener keyword.
	want := wrap("BEGIN\n   IF x THEN\n\n      y := 1;\n   END IF;\nEND")
	assertFormat(t, got, want, err)
}

// TestPLpgSQLIfIdempotent verifies two consecutive passes produce identical output.
func TestPLpgSQLIfIdempotent(t *testing.T) {
	c := plCfg()
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.If.BodyIndent = config.BodyIndentIndent
	c.Layout.Indent.Size = 3
	src := wrap("BEGIN\n   IF x THEN\n      y := 1;\n   END IF;\nEND")
	pass1, err := formatter.Format(src, c)
	if err != nil {
		t.Fatalf("pass1 error: %v", err)
	}
	pass2, err := formatter.Format(pass1, c)
	if err != nil {
		t.Fatalf("pass2 error: %v", err)
	}
	if pass1 != pass2 {
		t.Errorf("not idempotent:\npass1: %q\npass2: %q", pass1, pass2)
	}
}

// ── LOOP formatting ───────────────────────────────────────────────────────────

// TestPLpgSQLLoopPreserve verifies LOOP body is untouched with preserve config.
func TestPLpgSQLLoopPreserve(t *testing.T) {
	c := plCfg()
	src := wrap("BEGIN\n   LOOP\n      x := x + 1;\n   END LOOP;\nEND")
	got, err := formatter.Format(src, c)
	assertFormat(t, got, src, err)
}

// TestPLpgSQLLoopBodyIndentIndent normalizes loop body to its target depth.
func TestPLpgSQLLoopBodyIndentIndent(t *testing.T) {
	c := plCfg()
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Loop.BodyIndent = config.BodyIndentIndent
	c.Layout.Indent.Size = 3
	src := wrap("BEGIN\n   LOOP\nx := x + 1;\n   END LOOP;\nEND")
	got, err := formatter.Format(src, c)
	want := wrap("BEGIN\n   LOOP\n      x := x + 1;\n   END LOOP;\nEND")
	assertFormat(t, got, want, err)
}

// TestPLpgSQLLoopBodyIndentNone is a no-op in the control-flow normalizer.
func TestPLpgSQLLoopBodyIndentNone(t *testing.T) {
	c := plCfg()
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Loop.BodyIndent = config.BodyIndentNone
	src := wrap("BEGIN\n   LOOP\n      x := x + 1;\n   END LOOP;\nEND")
	got, err := formatter.Format(src, c)
	assertFormat(t, got, src, err)
}

// TestPLpgSQLLoopBlankLines adds blank lines after LOOP keyword and before END LOOP.
func TestPLpgSQLLoopBlankLines(t *testing.T) {
	c := plCfg()
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Loop.BlankLineBefore = config.BlankLineAdd
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Loop.BlankLineAfter = config.BlankLineAdd
	src := wrap("BEGIN\n   LOOP\n      x := x + 1;\n   END LOOP;\nEND")
	got, err := formatter.Format(src, c)
	// BlankLineAfter → blank after LOOP keyword; BlankLineBefore → blank before END LOOP.
	want := wrap("BEGIN\n   LOOP\n\n      x := x + 1;\n\n   END LOOP;\nEND")
	assertFormat(t, got, want, err)
}

// TestPLpgSQLLoopIdempotent verifies loop formatting is idempotent.
func TestPLpgSQLLoopIdempotent(t *testing.T) {
	c := plCfg()
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Loop.BodyIndent = config.BodyIndentIndent
	c.Layout.Indent.Size = 3
	src := wrap("BEGIN\n   LOOP\n      x := x + 1;\n   END LOOP;\nEND")
	pass1, err := formatter.Format(src, c)
	if err != nil {
		t.Fatalf("pass1 error: %v", err)
	}
	pass2, err := formatter.Format(pass1, c)
	if err != nil {
		t.Fatalf("pass2 error: %v", err)
	}
	if pass1 != pass2 {
		t.Errorf("not idempotent:\npass1: %q\npass2: %q", pass1, pass2)
	}
}

// ── CASE simple formatting ────────────────────────────────────────────────────

// TestPLpgSQLCaseSimplePreserve verifies that simple CASE is untouched with preserve config.
func TestPLpgSQLCaseSimplePreserve(t *testing.T) {
	c := plCfg()
	src := wrap("BEGIN\n   CASE x\n      WHEN 1 THEN y := 'a';\n      WHEN 2 THEN y := 'b';\n      ELSE y := 'c';\n   END CASE;\nEND")
	got, err := formatter.Format(src, c)
	assertFormat(t, got, src, err)
}

// TestPLpgSQLCaseSimpleThenBreakNever collapses WHEN/THEN to a single line.
// WHEN is normalized to depth 1 (same level as CASE) when WhenIndent=preserve.
func TestPLpgSQLCaseSimpleThenBreakNever(t *testing.T) {
	c := plCfg()
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Simple.ThenBreak = config.BreakNever
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Simple.BodyBreak = config.BreakNever
	// Input: THEN on its own line.
	src := wrap("BEGIN\n   CASE x\n      WHEN 1\n      THEN\n         y := 'a';\n   END CASE;\nEND")
	got, err := formatter.Format(src, c)
	// WHEN at depth 1 (preserve → extraIndent+1 = 1), THEN collapses to WHEN line with space.
	want := wrap("BEGIN\n   CASE x\n   WHEN 1 THEN y := 'a';\n   END CASE;\nEND")
	assertFormat(t, got, want, err)
}

// TestPLpgSQLCaseSimpleThenBreakAlways forces THEN to its own line.
// With ThenIndent=preserve (default), THEN is at the same depth as WHEN.
func TestPLpgSQLCaseSimpleThenBreakAlways(t *testing.T) {
	c := plCfg()
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Simple.ThenBreak = config.BreakAlways
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Simple.BodyBreak = config.BreakNever
	c.Layout.Indent.Size = 3
	src := wrap("BEGIN\n   CASE x\n      WHEN 1 THEN y := 'a';\n   END CASE;\nEND")
	got, err := formatter.Format(src, c)
	// WHEN at depth 1 (3 sp). ThenIndent=preserve → THEN at same depth 1 (3 sp).
	want := wrap("BEGIN\n   CASE x\n   WHEN 1\n   THEN y := 'a';\n   END CASE;\nEND")
	assertFormat(t, got, want, err)
}

// TestPLpgSQLCaseSimpleThenBreakAlwaysIndent forces THEN one level deeper than WHEN.
func TestPLpgSQLCaseSimpleThenBreakAlwaysIndent(t *testing.T) {
	c := plCfg()
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Simple.ThenBreak = config.BreakAlways
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Simple.ThenIndent = config.BodyIndentIndent
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Simple.BodyBreak = config.BreakNever
	c.Layout.Indent.Size = 3
	src := wrap("BEGIN\n   CASE x\n      WHEN 1 THEN y := 'a';\n   END CASE;\nEND")
	got, err := formatter.Format(src, c)
	// WHEN at depth 1 (3 sp), ThenIndent=indent → THEN at depth 2 (6 sp).
	want := wrap("BEGIN\n   CASE x\n   WHEN 1\n      THEN y := 'a';\n   END CASE;\nEND")
	assertFormat(t, got, want, err)
}

// TestPLpgSQLCaseSimpleBodyBreakAlways forces body onto its own line after THEN.
// With BodyIndent=preserve the body starts at column 0 after the forced break.
func TestPLpgSQLCaseSimpleBodyBreakAlways(t *testing.T) {
	c := plCfg()
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Simple.ThenBreak = config.BreakNever
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Simple.BodyBreak = config.BreakAlways
	src := wrap("BEGIN\n   CASE x\n      WHEN 1 THEN y := 'a';\n   END CASE;\nEND")
	got, err := formatter.Format(src, c)
	// Body stripped of leading whitespace and placed after \n; BodyIndent=preserve skips renormalization.
	want := wrap("BEGIN\n   CASE x\n   WHEN 1 THEN\ny := 'a';\n   END CASE;\nEND")
	assertFormat(t, got, want, err)
}

// TestPLpgSQLCaseSimpleBodyBreakAlwaysWithIndent combines BodyBreak=always + BodyIndent=indent
// so the body is placed on a new line AND normalised to the correct depth.
func TestPLpgSQLCaseSimpleBodyBreakAlwaysWithIndent(t *testing.T) {
	c := plCfg()
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Simple.ThenBreak = config.BreakNever
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Simple.BodyBreak = config.BreakAlways
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Simple.BodyIndent = config.BodyIndentIndent
	c.Layout.Indent.Size = 3
	src := wrap("BEGIN\n   CASE x\n      WHEN 1 THEN y := 'a';\n   END CASE;\nEND")
	got, err := formatter.Format(src, c)
	// WHEN at depth 1 (3 sp), body at depth 2 (6 sp) after normalisation.
	want := wrap("BEGIN\n   CASE x\n   WHEN 1 THEN\n      y := 'a';\n   END CASE;\nEND")
	assertFormat(t, got, want, err)
}

// TestPLpgSQLCaseSimpleWhenIndentIndent sets WHEN one level deeper than CASE.
func TestPLpgSQLCaseSimpleWhenIndentIndent(t *testing.T) {
	c := plCfg()
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Simple.WhenIndent = config.BodyIndentIndent
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Simple.ThenBreak = config.BreakNever
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Simple.BodyBreak = config.BreakNever
	c.Layout.Indent.Size = 3
	src := wrap("BEGIN\n   CASE x\n      WHEN 1 THEN y := 'a';\n   END CASE;\nEND")
	got, err := formatter.Format(src, c)
	// WhenIndent=indent → depth extraIndent+2 = 2 (6 sp), body depth 3 (9 sp).
	want := wrap("BEGIN\n   CASE x\n      WHEN 1 THEN y := 'a';\n   END CASE;\nEND")
	assertFormat(t, got, want, err)
}

// TestPLpgSQLCaseSimpleMultiStmtBody verifies multi-statement body after BodyBreak=always.
func TestPLpgSQLCaseSimpleMultiStmtBody(t *testing.T) {
	c := plCfg()
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Simple.ThenBreak = config.BreakNever
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Simple.BodyBreak = config.BreakAlways
	src := wrap("BEGIN\n   CASE x\n      WHEN 1 THEN y := 'a'; z := 'b';\n   END CASE;\nEND")
	got, err := formatter.Format(src, c)
	want := wrap("BEGIN\n   CASE x\n   WHEN 1 THEN\ny := 'a'; z := 'b';\n   END CASE;\nEND")
	assertFormat(t, got, want, err)
}

// TestPLpgSQLCaseSimpleElse verifies ELSE is treated like WHEN (same depth).
func TestPLpgSQLCaseSimpleElse(t *testing.T) {
	c := plCfg()
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Simple.ThenBreak = config.BreakNever
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Simple.BodyBreak = config.BreakNever
	src := wrap("BEGIN\n   CASE x\n      WHEN 1 THEN y := 'a';\n      ELSE y := 'default';\n   END CASE;\nEND")
	got, err := formatter.Format(src, c)
	// WHEN and ELSE normalised to depth 1; body stays on same line.
	want := wrap("BEGIN\n   CASE x\n   WHEN 1 THEN y := 'a';\n   ELSE y := 'default';\n   END CASE;\nEND")
	assertFormat(t, got, want, err)
}

// TestPLpgSQLCaseSimpleIdempotentThenBreakNever verifies idempotence for ThenBreak=never.
func TestPLpgSQLCaseSimpleIdempotentThenBreakNever(t *testing.T) {
	c := plCfg()
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Simple.ThenBreak = config.BreakNever
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Simple.BodyBreak = config.BreakNever
	src := wrap("BEGIN\n   CASE x\n      WHEN 1 THEN y := 'a';\n      WHEN 2 THEN y := 'b';\n   END CASE;\nEND")
	pass1, err := formatter.Format(src, c)
	if err != nil {
		t.Fatalf("pass1: %v", err)
	}
	pass2, err := formatter.Format(pass1, c)
	if err != nil {
		t.Fatalf("pass2: %v", err)
	}
	if pass1 != pass2 {
		t.Errorf("not idempotent:\npass1: %q\npass2: %q", pass1, pass2)
	}
}

// TestPLpgSQLCaseSimpleIdempotentThenBreakAlways verifies idempotence for ThenBreak=always.
func TestPLpgSQLCaseSimpleIdempotentThenBreakAlways(t *testing.T) {
	c := plCfg()
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Simple.ThenBreak = config.BreakAlways
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Simple.BodyBreak = config.BreakAlways
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Simple.BodyIndent = config.BodyIndentIndent
	c.Layout.Indent.Size = 3
	src := wrap("BEGIN\n   CASE x\n      WHEN 1 THEN y := 'a';\n   END CASE;\nEND")
	pass1, err := formatter.Format(src, c)
	if err != nil {
		t.Fatalf("pass1: %v", err)
	}
	pass2, err := formatter.Format(pass1, c)
	if err != nil {
		t.Fatalf("pass2: %v", err)
	}
	if pass1 != pass2 {
		t.Errorf("not idempotent:\npass1: %q\npass2: %q", pass1, pass2)
	}
}

// TestPLpgSQLCaseSimpleAutoShortFits verifies auto mode keeps short branches on one line.
func TestPLpgSQLCaseSimpleAutoShortFits(t *testing.T) {
	c := plCfg()
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Simple.ThenBreak = config.BreakAuto
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Simple.BodyBreak = config.BreakAuto
	c.Layout.LineLength = 120
	c.Layout.Indent.Size = 3
	src := wrap("BEGIN\n   CASE x\n      WHEN 1 THEN y := 'a';\n   END CASE;\nEND")
	got, err := formatter.Format(src, c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Short branch — auto keeps THEN and body on same line as WHEN.
	// WHEN normalised to depth 1 (preserve → extraIndent+1 = 1).
	want := wrap("BEGIN\n   CASE x\n   WHEN 1 THEN y := 'a';\n   END CASE;\nEND")
	assertFormat(t, got, want, err)
}

// ── CASE searched formatting ──────────────────────────────────────────────────

// TestPLpgSQLCaseSearchedPreserve verifies searched CASE is untouched with preserve config.
func TestPLpgSQLCaseSearchedPreserve(t *testing.T) {
	c := plCfg()
	src := wrap("BEGIN\n   CASE\n      WHEN x = 1 THEN y := 'a';\n      WHEN x = 2 THEN y := 'b';\n      ELSE y := 'c';\n   END CASE;\nEND")
	got, err := formatter.Format(src, c)
	assertFormat(t, got, src, err)
}

// TestPLpgSQLCaseSearchedThenBreakNever collapses WHEN/THEN to a single line.
func TestPLpgSQLCaseSearchedThenBreakNever(t *testing.T) {
	c := plCfg()
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Searched.ThenBreak = config.BreakNever
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Searched.BodyBreak = config.BreakNever
	src := wrap("BEGIN\n   CASE\n      WHEN x = 1\n      THEN\n         y := 'a';\n   END CASE;\nEND")
	got, err := formatter.Format(src, c)
	want := wrap("BEGIN\n   CASE\n   WHEN x = 1 THEN y := 'a';\n   END CASE;\nEND")
	assertFormat(t, got, want, err)
}

// TestPLpgSQLCaseSearchedThenBreakAlways forces THEN to its own line at same depth as WHEN.
func TestPLpgSQLCaseSearchedThenBreakAlways(t *testing.T) {
	c := plCfg()
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Searched.ThenBreak = config.BreakAlways
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Searched.BodyBreak = config.BreakNever
	c.Layout.Indent.Size = 3
	src := wrap("BEGIN\n   CASE\n      WHEN x = 1 THEN y := 'a';\n   END CASE;\nEND")
	got, err := formatter.Format(src, c)
	// ThenIndent=preserve (default) → THEN at same depth 1 as WHEN.
	want := wrap("BEGIN\n   CASE\n   WHEN x = 1\n   THEN y := 'a';\n   END CASE;\nEND")
	assertFormat(t, got, want, err)
}

// TestPLpgSQLCaseSearchedThenBreakAlwaysIndent forces THEN one level deeper than WHEN.
func TestPLpgSQLCaseSearchedThenBreakAlwaysIndent(t *testing.T) {
	c := plCfg()
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Searched.ThenBreak = config.BreakAlways
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Searched.ThenIndent = config.BodyIndentIndent
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Searched.BodyBreak = config.BreakNever
	c.Layout.Indent.Size = 3
	src := wrap("BEGIN\n   CASE\n      WHEN x = 1 THEN y := 'a';\n   END CASE;\nEND")
	got, err := formatter.Format(src, c)
	want := wrap("BEGIN\n   CASE\n   WHEN x = 1\n      THEN y := 'a';\n   END CASE;\nEND")
	assertFormat(t, got, want, err)
}

// TestPLpgSQLCaseSearchedBodyBreakAlways forces body onto its own line after THEN.
func TestPLpgSQLCaseSearchedBodyBreakAlways(t *testing.T) {
	c := plCfg()
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Searched.ThenBreak = config.BreakNever
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Searched.BodyBreak = config.BreakAlways
	src := wrap("BEGIN\n   CASE\n      WHEN x = 1 THEN y := 'a';\n   END CASE;\nEND")
	got, err := formatter.Format(src, c)
	want := wrap("BEGIN\n   CASE\n   WHEN x = 1 THEN\ny := 'a';\n   END CASE;\nEND")
	assertFormat(t, got, want, err)
}

// TestPLpgSQLCaseSearchedBodyBreakAlwaysWithIndent combines BodyBreak=always + BodyIndent=indent.
func TestPLpgSQLCaseSearchedBodyBreakAlwaysWithIndent(t *testing.T) {
	c := plCfg()
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Searched.ThenBreak = config.BreakNever
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Searched.BodyBreak = config.BreakAlways
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Searched.BodyIndent = config.BodyIndentIndent
	c.Layout.Indent.Size = 3
	src := wrap("BEGIN\n   CASE\n      WHEN x = 1 THEN y := 'a';\n   END CASE;\nEND")
	got, err := formatter.Format(src, c)
	want := wrap("BEGIN\n   CASE\n   WHEN x = 1 THEN\n      y := 'a';\n   END CASE;\nEND")
	assertFormat(t, got, want, err)
}

// TestPLpgSQLCaseSearchedIdempotentBreakAlways verifies idempotence for always-break modes.
func TestPLpgSQLCaseSearchedIdempotentBreakAlways(t *testing.T) {
	c := plCfg()
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Searched.ThenBreak = config.BreakAlways
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Searched.BodyBreak = config.BreakAlways
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Searched.BodyIndent = config.BodyIndentIndent
	c.Layout.Indent.Size = 3
	src := wrap("BEGIN\n   CASE\n      WHEN x = 1 THEN y := 'a';\n      WHEN x = 2 THEN y := 'b';\n   END CASE;\nEND")
	pass1, err := formatter.Format(src, c)
	if err != nil {
		t.Fatalf("pass1: %v", err)
	}
	pass2, err := formatter.Format(pass1, c)
	if err != nil {
		t.Fatalf("pass2: %v", err)
	}
	if pass1 != pass2 {
		t.Errorf("not idempotent:\npass1: %q\npass2: %q", pass1, pass2)
	}
}

// ── Nested control flow ───────────────────────────────────────────────────────

// TestPLpgSQLNestedCaseInsideIf verifies that a CASE inside an IF body formats correctly.
func TestPLpgSQLNestedCaseInsideIf(t *testing.T) {
	c := plCfg()
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.If.BodyIndent = config.BodyIndentIndent
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Searched.ThenBreak = config.BreakNever
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Searched.BodyBreak = config.BreakNever
	c.Layout.Indent.Size = 3
	src := wrap("BEGIN\n   IF cond THEN\n      CASE\n         WHEN x = 1 THEN y := 1;\n      END CASE;\n   END IF;\nEND")
	got, err := formatter.Format(src, c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pass2, err := formatter.Format(got, c)
	if err != nil {
		t.Fatalf("pass2 error: %v", err)
	}
	if got != pass2 {
		t.Errorf("nested not idempotent:\npass1: %q\npass2: %q", got, pass2)
	}
}

// ── EXCEPTION section ─────────────────────────────────────────────────────────

// TestPLpgSQLExceptionWithIf verifies that reformatControlFlow is applied to EXCEPTION body.
func TestPLpgSQLExceptionWithIf(t *testing.T) {
	c := plCfg()
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.If.BodyIndent = config.BodyIndentIndent
	c.Layout.Indent.Size = 3
	src := wrap("BEGIN\n   x := 1;\nEXCEPTION\n   WHEN OTHERS THEN\n      IF err THEN\nraise_me();\n      END IF;\nEND")
	got, err := formatter.Format(src, c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pass2, err := formatter.Format(got, c)
	if err != nil {
		t.Fatalf("pass2 error: %v", err)
	}
	if got != pass2 {
		t.Errorf("exception IF not idempotent:\npass1: %q\npass2: %q", got, pass2)
	}
}

// TestPLpgSQLExceptionWithCase verifies CASE formatting inside EXCEPTION section.
func TestPLpgSQLExceptionWithCase(t *testing.T) {
	c := plCfg()
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Searched.ThenBreak = config.BreakNever
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Searched.BodyBreak = config.BreakAlways
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Searched.BodyIndent = config.BodyIndentIndent
	c.Layout.Indent.Size = 3
	src := wrap("BEGIN\n   x := 1;\nEXCEPTION\n   WHEN OTHERS THEN\n      CASE\n         WHEN sqlstate = '40001' THEN y := 1;\n      END CASE;\nEND")
	got, err := formatter.Format(src, c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pass2, err := formatter.Format(got, c)
	if err != nil {
		t.Fatalf("pass2 error: %v", err)
	}
	if got != pass2 {
		t.Errorf("exception CASE not idempotent:\npass1: %q\npass2: %q", got, pass2)
	}
}

// ── FOR / WHILE loops ─────────────────────────────────────────────────────────

// TestPLpgSQLForLoopBodyIndentIndent verifies that FOR ... LOOP body is reformatted.
// The FOR header (before LOOP) is preserved verbatim; only the body is affected.
func TestPLpgSQLForLoopBodyIndentIndent(t *testing.T) {
	c := plCfg()
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Loop.BodyIndent = config.BodyIndentIndent
	c.Layout.Indent.Size = 3
	src := wrap("BEGIN\n   FOR i IN 1..10 LOOP\nx := x + i;\n   END LOOP;\nEND")
	got, err := formatter.Format(src, c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := wrap("BEGIN\n   FOR i IN 1..10 LOOP\n      x := x + i;\n   END LOOP;\nEND")
	assertFormat(t, got, want, err)
}

// TestPLpgSQLForLoopIdempotent verifies FOR loop formatting is idempotent.
func TestPLpgSQLForLoopIdempotent(t *testing.T) {
	c := plCfg()
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Loop.BodyIndent = config.BodyIndentIndent
	c.Layout.Indent.Size = 3
	src := wrap("BEGIN\n   FOR i IN 1..10 LOOP\n      x := x + i;\n   END LOOP;\nEND")
	pass1, err := formatter.Format(src, c)
	if err != nil {
		t.Fatalf("pass1: %v", err)
	}
	pass2, err := formatter.Format(pass1, c)
	if err != nil {
		t.Fatalf("pass2: %v", err)
	}
	if pass1 != pass2 {
		t.Errorf("not idempotent:\npass1: %q\npass2: %q", pass1, pass2)
	}
}

// TestPLpgSQLForQueryLoop verifies FOR ... IN SELECT ... LOOP works.
func TestPLpgSQLForQueryLoop(t *testing.T) {
	c := plCfg()
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Loop.BodyIndent = config.BodyIndentIndent
	c.Layout.Indent.Size = 3
	src := wrap("BEGIN\n   FOR rec IN SELECT id FROM t LOOP\nPERFORM process(rec.id);\n   END LOOP;\nEND")
	got, err := formatter.Format(src, c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := wrap("BEGIN\n   FOR rec IN SELECT id FROM t LOOP\n      PERFORM process(rec.id);\n   END LOOP;\nEND")
	assertFormat(t, got, want, err)
}

// TestPLpgSQLWhileLoop verifies WHILE ... LOOP body indentation.
func TestPLpgSQLWhileLoop(t *testing.T) {
	c := plCfg()
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Loop.BodyIndent = config.BodyIndentIndent
	c.Layout.Indent.Size = 3
	src := wrap("BEGIN\n   WHILE i < 10 LOOP\ni := i + 1;\n   END LOOP;\nEND")
	got, err := formatter.Format(src, c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := wrap("BEGIN\n   WHILE i < 10 LOOP\n      i := i + 1;\n   END LOOP;\nEND")
	assertFormat(t, got, want, err)
}

// TestPLpgSQLWhileLoopIdempotent verifies WHILE loop formatting is idempotent.
func TestPLpgSQLWhileLoopIdempotent(t *testing.T) {
	c := plCfg()
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Loop.BodyIndent = config.BodyIndentIndent
	c.Layout.Indent.Size = 3
	src := wrap("BEGIN\n   WHILE i < 10 LOOP\n      i := i + 1;\n   END LOOP;\nEND")
	pass1, err := formatter.Format(src, c)
	if err != nil {
		t.Fatalf("pass1: %v", err)
	}
	pass2, err := formatter.Format(pass1, c)
	if err != nil {
		t.Fatalf("pass2: %v", err)
	}
	if pass1 != pass2 {
		t.Errorf("not idempotent:\npass1: %q\npass2: %q", pass1, pass2)
	}
}

// TestPLpgSQLForLoopBlankLines verifies blank line config applies to FOR loops.
func TestPLpgSQLForLoopBlankLines(t *testing.T) {
	c := plCfg()
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Loop.BlankLineBefore = config.BlankLineAdd
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Loop.BlankLineAfter = config.BlankLineAdd
	src := wrap("BEGIN\n   FOR i IN 1..10 LOOP\n      x := i;\n   END LOOP;\nEND")
	got, err := formatter.Format(src, c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := wrap("BEGIN\n   FOR i IN 1..10 LOOP\n\n      x := i;\n\n   END LOOP;\nEND")
	assertFormat(t, got, want, err)
}

// ── DO blocks ─────────────────────────────────────────────────────────────────

// TestPLpgSQLDoBlockPreserve verifies a DO block with preserve config is untouched.
func TestPLpgSQLDoBlockPreserve(t *testing.T) {
	c := plCfg()
	src := "DO $$ BEGIN x := 1; END $$"
	got, err := formatter.Format(src, c)
	assertFormat(t, got, src, err)
}

// TestPLpgSQLDoBlockIfIndent verifies IF body indentation inside a DO block.
func TestPLpgSQLDoBlockIfIndent(t *testing.T) {
	c := plCfg()
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.If.BodyIndent = config.BodyIndentIndent
	c.Layout.Indent.Size = 3
	src := "DO $$\nBEGIN\n   IF x THEN\ny := 1;\n   END IF;\nEND\n$$"
	got, err := formatter.Format(src, c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pass2, err := formatter.Format(got, c)
	if err != nil {
		t.Fatalf("pass2: %v", err)
	}
	if got != pass2 {
		t.Errorf("DO block not idempotent:\npass1: %q\npass2: %q", got, pass2)
	}
}

// ── Config validation ─────────────────────────────────────────────────────────

// TestCaseBranchCfgValidationInvalidThenBreak verifies invalid then_break is rejected.
func TestCaseBranchCfgValidationInvalidThenBreak(t *testing.T) {
	c := plCfg()
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Simple.ThenBreak = "bogus"
	if err := config.Validate(c); err == nil {
		t.Fatal("expected validation error for invalid then_break")
	}
}

// TestCaseBranchCfgValidationInvalidBodyBreak verifies invalid body_break is rejected.
func TestCaseBranchCfgValidationInvalidBodyBreak(t *testing.T) {
	c := plCfg()
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Searched.BodyBreak = "bogus"
	if err := config.Validate(c); err == nil {
		t.Fatal("expected validation error for invalid body_break")
	}
}

// TestCaseBranchCfgValidationInvalidWhenIndent verifies invalid when_indent is rejected.
func TestCaseBranchCfgValidationInvalidWhenIndent(t *testing.T) {
	c := plCfg()
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Simple.WhenIndent = "bogus"
	if err := config.Validate(c); err == nil {
		t.Fatal("expected validation error for invalid when_indent")
	}
}

// TestCaseBranchCfgValidationValidValues verifies that all valid field values pass validation.
func TestCaseBranchCfgValidationValidValues(t *testing.T) {
	c := plCfg()
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Simple.ThenBreak = config.BreakAuto
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Simple.BodyBreak = config.BreakAlways
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Simple.WhenIndent = config.BodyIndentIndent
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Searched.ThenBreak = config.BreakNever
	c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Searched.BodyBreak = config.BreakAuto
	if err := config.Validate(c); err != nil {
		t.Fatalf("unexpected error for valid config: %v", err)
	}
}
