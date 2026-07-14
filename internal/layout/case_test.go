package layout_test

import (
	"strings"
	"testing"

	"github.com/heptau/pg_procrustes/config"
	"github.com/heptau/pg_procrustes/internal/layout"
)

func caseCfg(brk config.BreakMode, indent config.BodyIndentMode, lineLen int) *config.LayoutConfig {
	return &config.LayoutConfig{
		LineLength: lineLen,
		Indent: config.IndentCfg{
			Size:      2,
			Type:      config.IndentTypeSpaces,
			Normalize: config.IndentNormalizePreserve,
			Remainder: config.IndentRemainderKeep,
		},
		Clauses: config.ClausesCfg{Break: config.BreakPreserve, Align: config.AlignSame},
		Union:   config.UnionCfg{BlankLine: config.UnionBlankLinePreserve},
		Content: config.ContentCfg{Break: config.BreakPreserve, Align: config.AlignIndent},
		Case:    config.SQLCaseCfg{Break: brk, Indent: indent},
	}
}

func assertLayoutFormat(t *testing.T, sql string, cfg *config.LayoutConfig, want string) {
	t.Helper()
	got, err := layout.Apply(sql, cfg)
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}
	if got != want {
		t.Errorf("\ngot:  %q\nwant: %q", got, want)
	}
}

// ── preserve (no-op) ──────────────────────────────────────────────────────────

func TestSQLCasePreserve(t *testing.T) {
	sql := "SELECT CASE WHEN x = 1 THEN 'a' ELSE 'b' END FROM t"
	cfg := caseCfg(config.BreakPreserve, config.BodyIndentIndent, 128)
	assertLayoutFormat(t, sql, cfg, sql)
}

// ── break: never (collapse) ───────────────────────────────────────────────────

func TestSQLCaseNeverCollapsesFlat(t *testing.T) {
	// Already flat — should stay flat.
	sql := "SELECT CASE WHEN x = 1 THEN 'a' ELSE 'b' END FROM t"
	cfg := caseCfg(config.BreakNever, config.BodyIndentIndent, 128)
	got, err := layout.Apply(sql, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "\n") {
		t.Errorf("break=never should not introduce newlines, got:\n%s", got)
	}
}

func TestSQLCaseNeverCollapsesMultiLine(t *testing.T) {
	// Multi-line input → collapse to single line.
	sql := "SELECT CASE\nWHEN x = 1 THEN 'a'\nELSE 'b'\nEND FROM t"
	cfg := caseCfg(config.BreakNever, config.BodyIndentIndent, 128)
	got, err := layout.Apply(sql, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "\n") {
		t.Errorf("break=never should remove newlines, got:\n%s", got)
	}
	if !strings.Contains(got, "WHEN x = 1 THEN") {
		t.Errorf("expected WHEN clause in output, got:\n%s", got)
	}
}

// ── break: always (expand) ────────────────────────────────────────────────────

func TestSQLCaseAlwaysExpandsSearched(t *testing.T) {
	sql := "SELECT CASE WHEN x = 1 THEN 'a' WHEN x = 2 THEN 'b' ELSE 'c' END FROM t"
	cfg := caseCfg(config.BreakAlways, config.BodyIndentIndent, 128)
	got, err := layout.Apply(sql, cfg)
	if err != nil {
		t.Fatal(err)
	}
	// Each WHEN and ELSE should be on its own line; END on its own line.
	// WHEN/ELSE are indented relative to CASE, so we just check the keyword appears.
	if !strings.Contains(got, "WHEN x = 1 THEN 'a'") {
		t.Errorf("expected first WHEN clause, got:\n%s", got)
	}
	if !strings.Contains(got, "WHEN x = 2 THEN 'b'") {
		t.Errorf("expected second WHEN clause, got:\n%s", got)
	}
	if !strings.Contains(got, "ELSE 'c'") {
		t.Errorf("expected ELSE clause, got:\n%s", got)
	}
	if strings.Count(got, "\n") < 3 {
		t.Errorf("expected multiple newlines for expanded CASE, got:\n%s", got)
	}
}

func TestSQLCaseAlwaysExpandsSimple(t *testing.T) {
	sql := "SELECT CASE status WHEN 'active' THEN 1 WHEN 'pending' THEN 0 ELSE -1 END FROM t"
	cfg := caseCfg(config.BreakAlways, config.BodyIndentIndent, 128)
	got, err := layout.Apply(sql, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "CASE status") {
		t.Errorf("expected 'CASE status' in output, got:\n%s", got)
	}
	if !strings.Contains(got, "WHEN 'active' THEN 1") {
		t.Errorf("expected WHEN clause, got:\n%s", got)
	}
	if strings.Count(got, "\n") < 2 {
		t.Errorf("expected expanded CASE with newlines, got:\n%s", got)
	}
}

func TestSQLCaseAlwaysExpandsNoElse(t *testing.T) {
	sql := "SELECT CASE WHEN x > 0 THEN 1 END FROM t"
	cfg := caseCfg(config.BreakAlways, config.BodyIndentIndent, 128)
	got, err := layout.Apply(sql, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "WHEN x > 0 THEN 1") {
		t.Errorf("expected WHEN clause, got:\n%s", got)
	}
	if strings.Count(got, "\n") < 2 {
		t.Errorf("expected expanded CASE with newlines, got:\n%s", got)
	}
}

func TestSQLCaseIndentNone(t *testing.T) {
	sql := "SELECT CASE WHEN x = 1 THEN 'a' ELSE 'b' END FROM t"
	cfg := caseCfg(config.BreakAlways, config.BodyIndentNone, 128)
	got, err := layout.Apply(sql, cfg)
	if err != nil {
		t.Fatal(err)
	}
	// With indent=none, WHEN/ELSE/END should be at same level as CASE (no extra indent).
	// They should still be on own lines.
	if !strings.Contains(got, "\nWHEN") {
		t.Errorf("expected WHEN on own line, got:\n%s", got)
	}
}

// ── break: auto ───────────────────────────────────────────────────────────────

func TestSQLCaseAutoShortStaysFlat(t *testing.T) {
	sql := "SELECT CASE WHEN x = 1 THEN 'a' ELSE 'b' END FROM t"
	cfg := caseCfg(config.BreakAuto, config.BodyIndentIndent, 200)
	got, err := layout.Apply(sql, cfg)
	if err != nil {
		t.Fatal(err)
	}
	// Short CASE — should stay on one line.
	if strings.Contains(got, "\nWHEN") {
		t.Errorf("short CASE with auto should stay flat, got:\n%s", got)
	}
}

func TestSQLCaseAutoLongExpands(t *testing.T) {
	sql := "SELECT CASE WHEN some_very_long_condition_field_name = 'some_value' THEN 'result_one' WHEN another_long_condition = 'other_value' THEN 'result_two' ELSE 'fallback_value' END FROM t"
	cfg := caseCfg(config.BreakAuto, config.BodyIndentIndent, 80)
	got, err := layout.Apply(sql, cfg)
	if err != nil {
		t.Fatal(err)
	}
	// Long CASE — should expand with newlines.
	if strings.Count(got, "\n") < 2 {
		t.Errorf("long CASE with auto should expand, got:\n%s", got)
	}
}

// ── idempotence ───────────────────────────────────────────────────────────────

func TestSQLCaseIdempotentAlways(t *testing.T) {
	sql := "SELECT CASE WHEN x = 1 THEN 'a' WHEN x = 2 THEN 'b' ELSE 'c' END FROM t"
	cfg := caseCfg(config.BreakAlways, config.BodyIndentIndent, 128)
	pass1, err := layout.Apply(sql, cfg)
	if err != nil {
		t.Fatalf("pass1: %v", err)
	}
	pass2, err := layout.Apply(pass1, cfg)
	if err != nil {
		t.Fatalf("pass2: %v", err)
	}
	if pass1 != pass2 {
		t.Errorf("not idempotent:\npass1: %q\npass2: %q", pass1, pass2)
	}
}

func TestSQLCaseIdempotentNever(t *testing.T) {
	sql := "SELECT CASE WHEN x = 1 THEN 'a' ELSE 'b' END FROM t"
	cfg := caseCfg(config.BreakNever, config.BodyIndentIndent, 128)
	pass1, err := layout.Apply(sql, cfg)
	if err != nil {
		t.Fatalf("pass1: %v", err)
	}
	pass2, err := layout.Apply(pass1, cfg)
	if err != nil {
		t.Fatalf("pass2: %v", err)
	}
	if pass1 != pass2 {
		t.Errorf("not idempotent:\npass1: %q\npass2: %q", pass1, pass2)
	}
}

func TestSQLCaseIdempotentAuto(t *testing.T) {
	sql := "SELECT CASE WHEN x = 1 THEN 'a' WHEN x = 2 THEN 'b' ELSE 'c' END FROM t"
	cfg := caseCfg(config.BreakAuto, config.BodyIndentIndent, 30) // short lineLen → always expands
	pass1, err := layout.Apply(sql, cfg)
	if err != nil {
		t.Fatalf("pass1: %v", err)
	}
	pass2, err := layout.Apply(pass1, cfg)
	if err != nil {
		t.Fatalf("pass2: %v", err)
	}
	if pass1 != pass2 {
		t.Errorf("not idempotent:\npass1: %q\npass2: %q", pass1, pass2)
	}
}

// ── CASE in WHERE clause ──────────────────────────────────────────────────────

func TestSQLCaseInWhere(t *testing.T) {
	sql := "SELECT id FROM t WHERE CASE WHEN x = 1 THEN 'a' ELSE 'b' END = 'a'"
	cfg := caseCfg(config.BreakAlways, config.BodyIndentIndent, 128)
	got, err := layout.Apply(sql, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "WHEN x = 1 THEN 'a'") {
		t.Errorf("expected WHEN clause expanded, got:\n%s", got)
	}
	if strings.Count(got, "\n") < 2 {
		t.Errorf("expected expanded CASE with newlines, got:\n%s", got)
	}
}

// ── AND/OR inside CASE is not split ──────────────────────────────────────────

func TestSQLCaseAndOrInsideCaseNotSplit(t *testing.T) {
	// WHERE has two conditions; the second contains CASE with AND inside.
	// The AND inside CASE must not be treated as a WHERE condition split point.
	sql := "SELECT id FROM t WHERE active = TRUE AND CASE WHEN x > 0 AND y > 0 THEN 'pos' ELSE 'other' END = 'pos'"
	cfg := &config.LayoutConfig{
		LineLength: 10, // force content break
		Indent:     config.IndentCfg{Size: 2, Type: config.IndentTypeSpaces},
		Clauses:    config.ClausesCfg{Break: config.BreakAlways, Align: config.AlignSame},
		Union:      config.UnionCfg{BlankLine: config.UnionBlankLinePreserve},
		Content:    config.ContentCfg{Break: config.BreakAlways, Align: config.AlignIndent},
		Case:       config.SQLCaseCfg{Break: config.BreakPreserve},
	}
	got, err := layout.Apply(sql, cfg)
	if err != nil {
		t.Fatal(err)
	}
	// The CASE WHEN ... AND ... should not have been split mid-CASE.
	// The entire CASE expression must appear within one WHERE condition item.
	if !strings.Contains(got, "CASE WHEN x > 0 AND y > 0 THEN") {
		t.Errorf("AND inside CASE was incorrectly split, got:\n%s", got)
	}
}

// ── nested CASE ───────────────────────────────────────────────────────────────

func TestSQLCaseNestedIdempotent(t *testing.T) {
	sql := "SELECT CASE WHEN a = 1 THEN CASE WHEN b = 1 THEN 'ab' ELSE 'a' END ELSE 'other' END FROM t"
	cfg := caseCfg(config.BreakAlways, config.BodyIndentIndent, 128)
	pass1, err := layout.Apply(sql, cfg)
	if err != nil {
		t.Fatalf("pass1: %v", err)
	}
	pass2, err := layout.Apply(pass1, cfg)
	if err != nil {
		t.Fatalf("pass2: %v", err)
	}
	if pass1 != pass2 {
		t.Errorf("nested CASE not idempotent:\npass1: %q\npass2: %q", pass1, pass2)
	}
}

// ── combined: clause break + CASE break ──────────────────────────────────────

func TestSQLCaseCombinedClauseAndCaseBreak(t *testing.T) {
	sql := "SELECT id, CASE WHEN x = 1 THEN 'a' ELSE 'b' END AS result, name FROM t WHERE active = TRUE"
	cfg := &config.LayoutConfig{
		LineLength: 30,
		Indent:     config.IndentCfg{Size: 2, Type: config.IndentTypeSpaces},
		Clauses:    config.ClausesCfg{Break: config.BreakAlways, Align: config.AlignSame},
		Union:      config.UnionCfg{BlankLine: config.UnionBlankLinePreserve},
		Content:    config.ContentCfg{Break: config.BreakAlways, Align: config.AlignIndent},
		Case:       config.SQLCaseCfg{Break: config.BreakAlways, Indent: config.BodyIndentIndent},
	}
	got, err := layout.Apply(sql, cfg)
	if err != nil {
		t.Fatal(err)
	}
	// Clauses break, content breaks, and CASE expands.
	if !strings.Contains(got, "FROM") {
		t.Errorf("expected FROM clause in output, got:\n%s", got)
	}
	if !strings.Contains(got, "WHEN x = 1 THEN 'a'") {
		t.Errorf("expected CASE expanded, got:\n%s", got)
	}
	// Second pass must produce same result.
	pass2, err := layout.Apply(got, cfg)
	if err != nil {
		t.Fatalf("pass2: %v", err)
	}
	if got != pass2 {
		t.Errorf("combined not idempotent:\npass1: %q\npass2: %q", got, pass2)
	}
}
