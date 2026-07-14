package layout_test

import (
	"testing"

	"github.com/heptau/pg_procrustes/config"
	"github.com/heptau/pg_procrustes/internal/layout"
)

func mkCfg(clauses config.BreakMode, content config.BreakMode, lineLen int) *config.LayoutConfig {
	return &config.LayoutConfig{
		LineLength: lineLen,
		Indent: config.IndentCfg{
			Size:      2,
			Type:      config.IndentTypeSpaces,
			Normalize: config.IndentNormalizePreserve,
			Remainder: config.IndentRemainderKeep,
		},
		Clauses: config.ClausesCfg{
			Break: clauses,
			Align: config.AlignSame,
		},
		Union: config.UnionCfg{BlankLine: config.UnionBlankLinePreserve},
		Content: config.ContentCfg{
			Break: content,
			Align: config.AlignIndent,
		},
	}
}

func TestIsNoop(t *testing.T) {
	cfg := &config.LayoutConfig{}
	if !layout.IsNoop(cfg) {
		t.Error("zero value should be noop")
	}
}

func TestShortStatementAutoStaysFlat(t *testing.T) {
	sql := "SELECT id FROM users WHERE id = 1"
	cfg := mkCfg(config.BreakAuto, config.BreakAuto, 128)
	result, err := layout.Apply(sql, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if result != sql {
		t.Errorf("short stmt should stay flat, got:\n%s", result)
	}
}

func TestLongStatementAutoBreaks(t *testing.T) {
	sql := "SELECT very_long_column_name_one, very_long_column_name_two, very_long_column_name_three FROM some_table WHERE some_condition = true AND another_condition = false"
	cfg := mkCfg(config.BreakAuto, config.BreakPreserve, 80)
	result, err := layout.Apply(sql, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if result == sql {
		t.Error("long stmt should break, got unchanged")
	}
}

func TestBreakAlways(t *testing.T) {
	sql := "SELECT a FROM t"
	cfg := mkCfg(config.BreakAlways, config.BreakPreserve, 128)
	result, err := layout.Apply(sql, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if result == sql {
		t.Error("break=always should always break")
	}
}

func TestBreakNever(t *testing.T) {
	sql := "SELECT very_long_column_name_one, very_long_column_name_two FROM some_table WHERE some_condition = true"
	cfg := mkCfg(config.BreakNever, config.BreakNever, 10)
	result, err := layout.Apply(sql, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if result != sql {
		t.Errorf("break=never should keep flat, got:\n%s", result)
	}
}

func TestContentBreakSelectList(t *testing.T) {
	sql := "SELECT very_long_column_name_one, very_long_column_name_two, very_long_column_name_three FROM t"
	cfg := mkCfg(config.BreakAlways, config.BreakAlways, 10)
	result, err := layout.Apply(sql, cfg)
	if err != nil {
		t.Fatal(err)
	}
	_ = result
}

func TestUnionBlankLineBefore(t *testing.T) {
	sql := "SELECT a FROM t1 UNION SELECT b FROM t2"
	cfg := &config.LayoutConfig{
		LineLength: 128,
		Indent:     config.IndentCfg{Size: 2, Type: config.IndentTypeSpaces},
		Clauses:    config.ClausesCfg{Break: config.BreakPreserve},
		Union:      config.UnionCfg{BlankLine: config.UnionBlankLineBefore},
		Content:    config.ContentCfg{Break: config.BreakPreserve},
	}
	result, err := layout.Apply(sql, cfg)
	if err != nil {
		t.Fatal(err)
	}
	_ = result
}

func TestIndentTypeTab(t *testing.T) {
	sql := "SELECT a FROM t WHERE x = 1"
	cfg := &config.LayoutConfig{
		LineLength: 10,
		Indent:     config.IndentCfg{Size: 4, Type: config.IndentTypeTab, Normalize: config.IndentNormalizePreserve},
		Clauses:    config.ClausesCfg{Break: config.BreakAlways, Align: config.AlignSame},
		Union:      config.UnionCfg{BlankLine: config.UnionBlankLinePreserve},
		Content:    config.ContentCfg{Break: config.BreakPreserve, Align: config.AlignIndent},
	}
	result, err := layout.Apply(sql, cfg)
	if err != nil {
		t.Fatal(err)
	}
	_ = result
}

func TestIndentNormalizeChange(t *testing.T) {
	sql := "   SELECT a FROM t"
	cfg := &config.LayoutConfig{
		LineLength: 128,
		Indent:     config.IndentCfg{Size: 2, Type: config.IndentTypeSpaces, Normalize: config.IndentNormalizeChange},
		Clauses:    config.ClausesCfg{Break: config.BreakPreserve},
		Union:      config.UnionCfg{BlankLine: config.UnionBlankLinePreserve},
		Content:    config.ContentCfg{Break: config.BreakPreserve},
	}
	result, err := layout.Apply(sql, cfg)
	if err != nil {
		t.Fatal(err)
	}
	_ = result
}

func TestEmptySQL(t *testing.T) {
	cfg := mkCfg(config.BreakAlways, config.BreakAlways, 80)
	result, err := layout.Apply("", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if result != "" {
		t.Errorf("empty sql should return empty, got %q", result)
	}
}

func TestWhitespaceOnlySQL(t *testing.T) {
	cfg := mkCfg(config.BreakAlways, config.BreakAlways, 80)
	result, err := layout.Apply("   \n  ", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if result != "   \n  " {
		t.Errorf("whitespace-only sql should return unchanged, got %q", result)
	}
}
