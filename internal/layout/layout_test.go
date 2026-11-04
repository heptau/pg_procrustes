package layout_test

import (
	"strings"
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

func TestUnionPreserveWithClauseBreakAlways(t *testing.T) {
	cfg := &config.LayoutConfig{
		LineLength: 128,
		Indent:     config.IndentCfg{Size: 3, Type: config.IndentTypeSpaces},
		Clauses:    config.ClausesCfg{Break: config.BreakAlways},
		Union:      config.UnionCfg{BlankLine: config.UnionBlankLinePreserve},
		Content:    config.ContentCfg{Break: config.BreakPreserve},
	}
	for _, sql := range []string{
		"SELECT a FROM u UNION SELECT b FROM v;",
		"SELECT a FROM u WHERE x = 1 UNION ALL SELECT 1;",
	} {
		got, err := layout.Apply(sql, cfg)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(got, "uUNION") || strings.Contains(got, "1UNION") {
			t.Fatalf("UNION glued to previous token:\n%s", got)
		}
		again, err := layout.Apply(got, cfg)
		if err != nil {
			t.Fatalf("second pass failed: %v\n%s", err, got)
		}
		if again != got {
			t.Fatalf("not idempotent:\n%s\n---\n%s", got, again)
		}
	}
}

func TestLineCommentsKeepCommaAndIdempotent(t *testing.T) {
	cfg := mkCfg(config.BreakAlways, config.BreakAlways, 128)
	for _, sql := range []string{
		"SELECT a -- first\n, b -- second\nFROM t;",
		"SELECT a, -- note\n b FROM t;",
		"INSERT INTO t (a, -- x\n b) VALUES (1, 2);",
	} {
		got, err := layout.Apply(sql, cfg)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(got, "-- first,") || strings.Contains(got, "-- x,") {
			t.Fatalf("comma swallowed by line comment:\n%s", got)
		}
		again, err := layout.Apply(got, cfg)
		if err != nil || again != got {
			t.Fatalf("not idempotent (err=%v):\n%s\n---\n%s", err, got, again)
		}
	}
}

func TestPrefixedSelectStartsOnNewLine(t *testing.T) {
	cfg := mkCfg(config.BreakAlways, config.BreakAlways, 128)
	got, err := layout.Apply("CREATE VIEW v AS SELECT id FROM t WHERE a = 1;", cfg)
	if err != nil {
		t.Fatal(err)
	}
	want := "CREATE VIEW v AS\nSELECT\n  id\nFROM\n  t\nWHERE\n  a = 1;"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	again, err := layout.Apply(got, cfg)
	if err != nil || again != got {
		t.Fatalf("not idempotent (err=%v):\n%s", err, again)
	}
}

func TestNonQueryKeywordsAreNotClauses(t *testing.T) {
	cfg := mkCfg(config.BreakAlways, config.BreakAlways, 128)
	for _, sql := range []string{
		"ALTER TABLE t ALTER COLUMN a SET NOT NULL;",
		"ALTER TABLE t ALTER COLUMN a TYPE integer USING a::integer;",
		"CREATE TABLE x (a integer) WITH (fillfactor = 70);",
		"SET search_path = public;",
		"CREATE TRIGGER tr BEFORE INSERT OR UPDATE ON t FOR EACH ROW EXECUTE FUNCTION f();",
		"CREATE TABLE t2 (a integer REFERENCES o(id) ON UPDATE CASCADE);",
	} {
		got, err := layout.Apply(sql, cfg)
		if err != nil {
			t.Fatal(err)
		}
		if got != sql {
			t.Errorf("statement was rewrapped:\n%s\n---\n%s", sql, got)
		}
	}
	got, err := layout.Apply("SELECT * FROM t WHERE a = 1 FOR UPDATE;", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "FOR UPDATE") {
		t.Errorf("FOR UPDATE split across lines:\n%s", got)
	}
}

func TestMergeClauses(t *testing.T) {
	cfg := mkCfg(config.BreakAlways, config.BreakAlways, 128)
	sql := "MERGE INTO t AS tgt USING s ON tgt.id = s.id AND tgt.ok WHEN MATCHED THEN UPDATE SET a = CASE WHEN s.x THEN 1 ELSE 2 END, b = 3 WHEN NOT MATCHED THEN INSERT (id) VALUES (s.id);"
	got, err := layout.Apply(sql, cfg)
	if err != nil {
		t.Fatal(err)
	}
	want := "MERGE INTO t AS tgt\n" +
		"USING\n  s\n" +
		"ON\n  tgt.id = s.id\n  AND tgt.ok\n" +
		"WHEN MATCHED THEN UPDATE SET a = CASE WHEN s.x THEN 1 ELSE 2 END, b = 3\n" +
		"WHEN NOT MATCHED THEN INSERT (id) VALUES (s.id);"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	again, err := layout.Apply(got, cfg)
	if err != nil || again != got {
		t.Fatalf("not idempotent (err=%v):\n%s", err, again)
	}
}

func TestPerClauseOverrideAloneIsNotNoop(t *testing.T) {
	cfg := mkCfg(config.BreakPreserve, config.BreakPreserve, 128)
	cfg.Clauses.Where = config.ClauseRule{Break: config.BreakAlways}
	if layout.IsNoop(cfg) {
		t.Fatal("clauses.where.break: always must not be treated as a no-op")
	}
	got, err := layout.Apply("SELECT a FROM t WHERE x = 1;", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "\nWHERE") {
		t.Fatalf("WHERE override ignored:\n%s", got)
	}

	cfg = mkCfg(config.BreakPreserve, config.BreakPreserve, 128)
	cfg.Content.WhenActions = config.ContentRule{Break: config.BreakAlways}
	if layout.IsNoop(cfg) {
		t.Fatal("content.when_actions.break: always must not be treated as a no-op")
	}
}

func TestMergeSeparateSettings(t *testing.T) {
	sql := "MERGE INTO t AS tgt USING s ON tgt.id = s.id AND tgt.ok WHEN MATCHED AND s.x > 0 THEN UPDATE SET a = 1 WHEN NOT MATCHED THEN INSERT (id) VALUES (s.id);"
	cfg := mkCfg(config.BreakPreserve, config.BreakPreserve, 128)
	cfg.Clauses.When = config.ClauseRule{Break: config.BreakAlways}
	cfg.Clauses.MergeOn = config.ClauseRule{Break: config.BreakAlways}
	cfg.Content.MergeOn = config.ContentRule{Break: config.BreakAlways}
	cfg.Content.WhenActions = config.ContentRule{Break: config.BreakAlways}
	got, err := layout.Apply(sql, cfg)
	if err != nil {
		t.Fatal(err)
	}
	want := "MERGE INTO t AS tgt USING s\n" +
		"ON\n  tgt.id = s.id\n  AND tgt.ok\n" +
		"WHEN MATCHED AND s.x > 0 THEN\n  UPDATE SET a = 1\n" +
		"WHEN NOT MATCHED THEN\n  INSERT (id) VALUES (s.id);"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	if again, err := layout.Apply(got, cfg); err != nil || again != got {
		t.Fatalf("not idempotent (err=%v):\n%s", err, again)
	}

	// when_actions does not inherit content.break: WHEN branches stay on one line.
	cfg = mkCfg(config.BreakAlways, config.BreakAlways, 128)
	got, err = layout.Apply(sql, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "WHEN MATCHED AND s.x > 0 THEN UPDATE SET a = 1\n") {
		t.Fatalf("WHEN action should stay inline by default:\n%s", got)
	}
}
