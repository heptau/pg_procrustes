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

func TestLineCommentsNeverSwallowCode(t *testing.T) {
	cfg := mkCfg(config.BreakAlways, config.BreakAlways, 128)
	cfg.Case.Break = config.BreakNever
	for _, sql := range []string{
		"SELECT CASE -- c1\n WHEN a > 1 THEN 'x' -- c2\n ELSE 'z' END AS k FROM t;",
		"INSERT INTO t VALUES (1, 2) -- one\n, (3, 4) -- two\n;",
		"SELECT a FROM t -- tail\n;",
	} {
		got, err := layout.Apply(sql, cfg)
		if err != nil {
			t.Fatal(err)
		}
		// Every line comment must end its line: nothing but the comment may follow "--" on
		// that line, and the statement must still end with a real ';'.
		for _, ln := range strings.Split(got, "\n") {
			if i := strings.Index(ln, "--"); i >= 0 {
				rest := ln[i:]
				if strings.Contains(rest, ";") || strings.Contains(rest, "WHEN") || strings.Contains(rest, "END") {
					t.Errorf("code swallowed by a comment in %q:\n%s", ln, got)
				}
			}
		}
		if again, err := layout.Apply(got, cfg); err != nil || again != got {
			t.Errorf("not idempotent (err=%v):\n%s\n---\n%s", err, got, again)
		}
	}
}

func TestCommaPositionStart(t *testing.T) {
	cfg := mkCfg(config.BreakAlways, config.BreakAlways, 128)
	cfg.Indent.Size = 3
	cfg.Content.CommaPosition = config.CommaStart
	got, err := layout.Apply("SELECT a, b, count(*) AS n FROM t GROUP BY a, b;", cfg)
	if err != nil {
		t.Fatal(err)
	}
	want := "SELECT\n   a\n , b\n , count(*) AS n\nFROM\n   t\nGROUP BY\n   a\n , b;"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	if again, err := layout.Apply(got, cfg); err != nil || again != got {
		t.Fatalf("not idempotent (err=%v):\n%s", err, again)
	}
}

func TestLogicalOperatorPositionEndKeepsBetweenIntact(t *testing.T) {
	cfg := mkCfg(config.BreakAlways, config.BreakAlways, 128)
	cfg.Content.LogicalOperatorPosition = config.LogicalOperatorEnd
	got, err := layout.Apply("SELECT 1 FROM t WHERE a = 1 AND b BETWEEN 1 AND 5 OR c = 3;", cfg)
	if err != nil {
		t.Fatal(err)
	}
	want := "SELECT\n  1\nFROM\n  t\nWHERE\n  a = 1 AND\n  b BETWEEN 1 AND 5 OR\n  c = 3;"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	if again, err := layout.Apply(got, cfg); err != nil || again != got {
		t.Fatalf("not idempotent (err=%v):\n%s", err, again)
	}
}

func TestJoinOnPositionNewline(t *testing.T) {
	cfg := mkCfg(config.BreakAlways, config.BreakAlways, 128)
	cfg.Clauses.Join.Align = config.AlignIndent
	cfg.Content.JoinOnPosition = config.JoinOnNewline
	got, err := layout.Apply("SELECT 1 FROM a JOIN b ON a.id = b.id AND a.x = 1 LEFT JOIN c USING (id);", cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"\n  JOIN b\n    ON a.id = b.id\n    AND a.x = 1\n"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
	if again, err := layout.Apply(got, cfg); err != nil || again != got {
		t.Fatalf("not idempotent (err=%v):\n%s", err, again)
	}
}

func TestOperatorPositionEndMovesCommentWithOperator(t *testing.T) {
	cfg := mkCfg(config.BreakAlways, config.BreakAlways, 128)
	cfg.Content.LogicalOperatorPosition = config.LogicalOperatorEnd
	got, err := layout.Apply("SELECT 1 FROM t WHERE a = 1 -- one\n AND b = 2 -- two\n;", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "a = 1 AND -- one\n") {
		t.Fatalf("comment should stay on the line of its condition:\n%s", got)
	}
	if again, err := layout.Apply(got, cfg); err != nil || again != got {
		t.Fatalf("not idempotent (err=%v):\n%s\n---\n%s", err, got, again)
	}
}

func ddlCfg(create config.BreakMode, align config.ColumnAlignMode, alter config.BreakMode) *config.LayoutConfig {
	cfg := mkCfg(config.BreakPreserve, config.BreakPreserve, 128)
	cfg.Indent.Size = 3
	cfg.DDL.CreateTable = config.CreateTableCfg{Break: create, Align: align}
	cfg.DDL.AlterTable = config.AlterTableCfg{Break: alter}
	return cfg
}

func assertIdempotent(t *testing.T, cfg *config.LayoutConfig, got string) {
	t.Helper()
	if again, err := layout.Apply(got, cfg); err != nil || again != got {
		t.Fatalf("not idempotent (err=%v):\n%s\n---\n%s", err, got, again)
	}
}

func TestCreateTableBreakAndAlign(t *testing.T) {
	sql := "CREATE TABLE IF NOT EXISTS public.t (id bigint NOT NULL, code character varying(64) NOT NULL DEFAULT 'x', note text, CONSTRAINT pk PRIMARY KEY (id)) WITH (fillfactor = 70);"
	for _, tc := range []struct {
		align config.ColumnAlignMode
		want  string
	}{
		{config.ColumnAlignPreserve, "CREATE TABLE IF NOT EXISTS public.t (\n   id bigint NOT NULL,\n   code character varying(64) NOT NULL DEFAULT 'x',\n   note text,\n   CONSTRAINT pk PRIMARY KEY (id)\n) WITH (fillfactor = 70);"},
		{config.ColumnAlignTypes, "CREATE TABLE IF NOT EXISTS public.t (\n   id   bigint NOT NULL,\n   code character varying(64) NOT NULL DEFAULT 'x',\n   note text,\n   CONSTRAINT pk PRIMARY KEY (id)\n) WITH (fillfactor = 70);"},
		{config.ColumnAlignAll, "CREATE TABLE IF NOT EXISTS public.t (\n   id   bigint                NOT NULL,\n   code character varying(64) NOT NULL DEFAULT 'x',\n   note text,\n   CONSTRAINT pk PRIMARY KEY (id)\n) WITH (fillfactor = 70);"},
	} {
		cfg := ddlCfg(config.BreakAlways, tc.align, config.BreakPreserve)
		got, err := layout.Apply(sql, cfg)
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Errorf("align %s:\ngot:\n%s\nwant:\n%s", tc.align, got, tc.want)
		}
		assertIdempotent(t, cfg, got)
	}
}

func TestCreateTableCommaStartCommentsAndNoSemicolon(t *testing.T) {
	cfg := ddlCfg(config.BreakAlways, config.ColumnAlignTypes, config.BreakPreserve)
	cfg.Content.CommaPosition = config.CommaStart
	sql := "create table x (a int, -- first\n bb text, ccc date)"
	got, err := layout.Apply(sql, cfg)
	if err != nil {
		t.Fatal(err)
	}
	want := "create table x (\n   a   int -- first\n , bb  text\n , ccc date\n)"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	assertIdempotent(t, cfg, got)
}

func TestCreateTableNeverAndAutoAndPreserve(t *testing.T) {
	sql := "CREATE TABLE t (\n   a int,\n   b text\n);"
	got, err := layout.Apply(sql, ddlCfg(config.BreakNever, config.ColumnAlignPreserve, config.BreakPreserve))
	if err != nil || got != "CREATE TABLE t (a int, b text);" {
		t.Fatalf("never: %q %v", got, err)
	}
	long := "CREATE TABLE t (a int, b text);"
	cfg := ddlCfg(config.BreakAuto, config.ColumnAlignPreserve, config.BreakPreserve)
	if got, _ := layout.Apply(long, cfg); got != long {
		t.Fatalf("auto must keep a short statement: %q", got)
	}
	cfg.LineLength = 10
	if got, _ := layout.Apply(long, cfg); !strings.Contains(got, "\n   a int,\n   b text\n);") {
		t.Fatalf("auto must break a long statement: %q", got)
	}
	// preserve: untouched.
	if got, _ := layout.Apply(long, ddlCfg(config.BreakPreserve, config.ColumnAlignAll, config.BreakPreserve)); got != long {
		t.Fatalf("preserve changed the statement: %q", got)
	}
	// A line comment makes "never" back off instead of commenting out the rest.
	withComment := "CREATE TABLE t (a int, -- c\n b text);"
	if got, _ := layout.Apply(withComment, ddlCfg(config.BreakNever, config.ColumnAlignPreserve, config.BreakPreserve)); got != withComment {
		t.Fatalf("never must not collapse a statement with a line comment: %q", got)
	}
}

func TestAlterTableBreak(t *testing.T) {
	multi := "ALTER TABLE public.t ADD COLUMN a int, DROP COLUMN b, ALTER COLUMN c SET NOT NULL;"
	single := "ALTER TABLE t RENAME TO u;"
	cfg := ddlCfg(config.BreakPreserve, config.ColumnAlignPreserve, config.BreakAuto)
	got, err := layout.Apply(multi, cfg)
	if err != nil {
		t.Fatal(err)
	}
	want := "ALTER TABLE public.t\n   ADD COLUMN a int,\n   DROP COLUMN b,\n   ALTER COLUMN c SET NOT NULL;"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	assertIdempotent(t, cfg, got)
	if got, _ := layout.Apply(single, cfg); got != single {
		t.Fatalf("auto must keep a single short action on one line: %q", got)
	}
	cfg.DDL.AlterTable.Break = config.BreakAlways
	if got, _ := layout.Apply(single, cfg); got != "ALTER TABLE t\n   RENAME TO u;" {
		t.Fatalf("always must break even a single action: %q", got)
	}
	// ALTER TABLE ALL IN TABLESPACE is not an action list.
	all := "ALTER TABLE ALL IN TABLESPACE a SET TABLESPACE b;"
	if got, _ := layout.Apply(all, cfg); got != all {
		t.Fatalf("ALL IN TABLESPACE changed: %q", got)
	}
}

func TestDDLOptionsAreNotNoop(t *testing.T) {
	cfg := mkCfg(config.BreakPreserve, config.BreakPreserve, 128)
	if !layout.IsNoop(cfg) {
		t.Fatal("defaults must be a no-op")
	}
	cfg.DDL.CreateTable.Break = config.BreakAlways
	if layout.IsNoop(cfg) {
		t.Fatal("ddl.create_table.break must not be a no-op")
	}
}

func TestWindowClauseBreaks(t *testing.T) {
	cfg := caseCfg(config.BreakPreserve, config.BodyIndentIndent, 80)
	cfg.Clauses.Break = config.BreakAlways
	cfg.Content.Break = config.BreakAlways
	sql := "select sum(x) over w from t window w as (partition by a), v as (order by b) order by 1"
	want := "select\n  sum(x) over w\nfrom\n  t\nwindow\n  w as (partition by a),\n  v as (order by b)\norder by\n  1"
	assertLayoutFormat(t, sql, cfg, want)
	assertSubqueryIdempotent(t, sql, cfg)
}

func TestWindowClauseNotAClauseOutsideQuery(t *testing.T) {
	cfg := caseCfg(config.BreakPreserve, config.BodyIndentIndent, 80)
	cfg.Clauses.Break = config.BreakAlways
	sql := "select sum(x) over (partition by a) from t"
	want := "select sum(x) over (partition by a)\nfrom t"
	assertLayoutFormat(t, sql, cfg, want)
}

// Words that look like clauses but are not: privileges, IS DISTINCT FROM, COPY … FROM, policy commands.
func TestNotClausesOutsideQueries(t *testing.T) {
	cfg := caseCfg(config.BreakPreserve, config.BodyIndentIndent, 80)
	cfg.Clauses.Break = config.BreakAlways
	cfg.Content.Break = config.BreakAlways
	for _, sql := range []string{
		"grant select, insert, update on t to r1, r2",
		"revoke select on t from r1",
		"alter default privileges in schema s grant select on tables to r",
		"copy t from stdin",
		"create policy p on t for select using (a = 1)",
	} {
		assertLayoutFormat(t, sql, cfg, sql)
	}
	assertLayoutFormat(t, "select a from t where c is not distinct from d", cfg,
		"select\n  a\nfrom\n  t\nwhere\n  c is not distinct from d")
}

// A name after a dot (a.join, s.from, t.where) is an identifier even when it is spelled like a
// clause keyword: it must not start a clause (the layout broke the line after the dot, and the
// second pass then saw something else than the first).
func TestKeywordAfterDotIsNoClause(t *testing.T) {
	cfg := caseCfg(config.BreakPreserve, config.BodyIndentIndent, 128)
	cfg.Clauses.Break = config.BreakAlways
	for in, want := range map[string]string{
		"select from a.join":                "select\nfrom a.join",
		"select a.from from t":              "select a.from\nfrom t",
		"select x from t where a.where = 1": "select x\nfrom t\nwhere a.where = 1",
	} {
		got, err := layout.Apply(in, cfg)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("%q:\n got %q\nwant %q", in, got, want)
		}
		if again, _ := layout.Apply(got, cfg); again != got {
			t.Errorf("%q: not idempotent: %q → %q", in, got, again)
		}
	}
}

// The lines of a quoted identifier that spans several lines are its name: the layout must not
// indent them (like it leaves multi-line string constants alone).
func TestMultiLineQuotedIdentifierIsData(t *testing.T) {
	cfg := caseCfg(config.BreakPreserve, config.BodyIndentIndent, 128)
	cfg.Clauses.Break = config.BreakAlways
	cfg.Content.Break = config.BreakAlways
	in := "select x, \"a\n  b\" from \"t\nu\" where \"c\nd\" = 1"
	got, err := layout.Apply(in, cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"\"a\n  b\"", "\"t\nu\"", "\"c\nd\""} {
		if !strings.Contains(got, id) {
			t.Errorf("identifier %q was changed:\n%s", id, got)
		}
	}
	if again, _ := layout.Apply(got, cfg); again != got {
		t.Errorf("not idempotent:\n%s\n---\n%s", got, again)
	}
}

// A blank line below a comment in a broken content item stays empty: the indent that was put
// in front of it was taken off again by the next run (not idempotent).
func TestBlankLineAfterCommentHasNoIndent(t *testing.T) {
	cfg := caseCfg(config.BreakPreserve, config.BodyIndentIndent, 128)
	cfg.Clauses.Break = config.BreakAlways
	cfg.Content.Break = config.BreakAlways
	for _, in := range []string{"select 1 from t where --\n\na = 1", ";select --\n--\n\n-0"} {
		got, err := layout.Apply(in, cfg)
		if err != nil {
			t.Fatal(err)
		}
		for _, ln := range strings.Split(got, "\n") {
			if ln != "" && strings.TrimSpace(ln) == "" {
				t.Errorf("%q: line of blanks %q in\n%s", in, ln, got)
			}
		}
		if again, _ := layout.Apply(got, cfg); again != got {
			t.Errorf("%q: not idempotent:\n%q\n%q", in, got, again)
		}
	}
}

// Tabs in front of the first line below a comment, for an indent made of spaces, must not make
// the next run place the item differently (not idempotent).
func TestTabAfterCommentInSpaceIndentIsStable(t *testing.T) {
	cfg := caseCfg(config.BreakPreserve, config.BodyIndentIndent, 128)
	cfg.Clauses.Break = config.BreakAlways
	cfg.Content.Break = config.BreakAlways
	for _, in := range []string{"select -- c\n\ta\n+ 1", "select--\n\t!\n0"} {
		got, err := layout.Apply(in, cfg)
		if err != nil {
			t.Fatal(err)
		}
		if again, _ := layout.Apply(got, cfg); again != got {
			t.Errorf("%q: not idempotent:\n%q\n%q", in, got, again)
		}
	}
}
