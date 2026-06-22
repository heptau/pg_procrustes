package formatter_test

import (
	"strings"
	"testing"

	"github.com/heptau/pg_procrustes/internal/config"
	"github.com/heptau/pg_procrustes/internal/formatter"
)

var preserve = config.CasePreserve

func cfg(kw, dt config.CaseRule) *config.Config {
	return cfgFull(kw, dt, preserve)
}

// dtCfg builds a config focused on data type form testing.
func dtCfg(form config.TypeForm, cas config.CaseRule) *config.Config {
	return &config.Config{
		ReservedKeywords:     config.Section{Case: upper},
		Keywords:             config.Section{Case: upper},
		DataTypes:            config.DataTypesSection{Case: cas, Form: form},
		Literals:             config.Section{Case: upper},
		Operators:            config.Section{Case: upper},
		Schemas:              config.Section{Case: preserve},
		Tables:               config.Section{Case: preserve},
		Functions:            config.Section{Case: preserve},
		ConditionalFunctions: config.Section{Case: upper},
		SystemFunctions:      config.Section{Case: upper},
		Aliases:              config.AliasSection{Case: preserve, As: config.AliasAsPreserve},
		Columns:              config.Section{Case: preserve},
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
	}
}

func cfgFull(kw, dt, col config.CaseRule) *config.Config {
	return &config.Config{
		ReservedKeywords:     config.Section{Case: kw},
		Keywords:             config.Section{Case: kw},
		DataTypes:            config.DataTypesSection{Case: dt, Form: config.TypeFormPreserve},
		Literals:             config.Section{Case: kw},
		Operators:            config.Section{Case: kw},
		Schemas:              config.Section{Case: preserve},
		Tables:               config.Section{Case: preserve},
		Functions:            config.Section{Case: preserve},
		ConditionalFunctions: config.Section{Case: kw},
		SystemFunctions:      config.Section{Case: kw},
		Aliases:              config.AliasSection{Case: preserve, As: config.AliasAsPreserve},
		Columns:              config.Section{Case: col},
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
	}
}

var upper = config.CaseUpper
var lower = config.CaseLower

func TestKeywordsUpper(t *testing.T) {
	got, err := formatter.Format("select * from users where id = 1", cfg(upper, upper))
	want := "SELECT * FROM users WHERE id = 1"
	assertFormat(t, got, want, err)
}

func TestKeywordsLower(t *testing.T) {
	got, err := formatter.Format("SELECT * FROM users WHERE id = 1", cfg(lower, lower))
	want := "select * from users where id = 1"
	assertFormat(t, got, want, err)
}

func TestDataTypeUpper(t *testing.T) {
	got, err := formatter.Format("CREATE TABLE t (col1 integer, col2 text, col3 boolean)", cfg(upper, upper))
	want := "CREATE TABLE t (col1 INTEGER, col2 TEXT, col3 BOOLEAN)"
	assertFormat(t, got, want, err)
}

func TestDataTypeLower(t *testing.T) {
	got, err := formatter.Format("CREATE TABLE t (col1 INTEGER, col2 TEXT)", cfg(lower, lower))
	want := "create table t (col1 integer, col2 text)"
	assertFormat(t, got, want, err)
}

func TestMixedKeywordUpperDataTypeLower(t *testing.T) {
	got, err := formatter.Format("create table t (col1 INTEGER)", cfg(upper, lower))
	want := "CREATE TABLE t (col1 integer)"
	assertFormat(t, got, want, err)
}

func TestPreservesStringLiteral(t *testing.T) {
	got, err := formatter.Format("SELECT 'Hello World' FROM t", cfg(upper, upper))
	want := "SELECT 'Hello World' FROM t"
	assertFormat(t, got, want, err)
}

func TestPreservesIdentifiers(t *testing.T) {
	got, err := formatter.Format("SELECT myColumn FROM myTable", cfg(upper, upper))
	want := "SELECT myColumn FROM myTable"
	assertFormat(t, got, want, err)
}

func TestPreservesWhitespace(t *testing.T) {
	input := "select\n  *\n  from t"
	got, err := formatter.Format(input, cfg(upper, upper))
	want := "SELECT\n  *\n  FROM t"
	assertFormat(t, got, want, err)
}

func TestMultiStatement(t *testing.T) {
	got, err := formatter.Format("select 1; select 2;", cfg(upper, upper))
	want := "SELECT 1; SELECT 2;"
	assertFormat(t, got, want, err)
}

func TestVarchar(t *testing.T) {
	got, err := formatter.Format("CREATE TABLE t (col varchar(255))", cfg(upper, upper))
	want := "CREATE TABLE t (col VARCHAR(255))"
	assertFormat(t, got, want, err)
}

func TestDoublePrecision(t *testing.T) {
	got, err := formatter.Format("CREATE TABLE t (val double precision)", cfg(upper, upper))
	want := "CREATE TABLE t (val DOUBLE PRECISION)"
	assertFormat(t, got, want, err)
}

func TestPreservesLineComment(t *testing.T) {
	input := "SELECT -- pick everything\n * FROM t"
	got, err := formatter.Format(input, cfg(upper, upper))
	want := "SELECT -- pick everything\n * FROM t"
	assertFormat(t, got, want, err)
}

func TestStringLiteralPreserved(t *testing.T) {
	// Keywords inside single-quoted strings must never be changed.
	got, err := formatter.Format("SELECT 'select from where insert' FROM t", cfg(upper, upper))
	want := "SELECT 'select from where insert' FROM t"
	assertFormat(t, got, want, err)
}

func TestDollarQuotedBody(t *testing.T) {
	input := `CREATE OR REPLACE FUNCTION f() RETURNS trigger AS $$
begin
  return new;
end;
$$ LANGUAGE plpgsql`
	got, err := formatter.Format(input, cfg(upper, upper))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "BEGIN") || !strings.Contains(got, "RETURN") || !strings.Contains(got, "END") {
		t.Errorf("dollar-quoted body keywords not uppercased:\n%s", got)
	}
}

func TestDollarQuotedBodyNamedDelimiter(t *testing.T) {
	input := `CREATE FUNCTION f() RETURNS void AS $body$
begin
  return;
end;
$body$ LANGUAGE plpgsql`
	got, err := formatter.Format(input, cfg(upper, upper))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "BEGIN") || !strings.Contains(got, "RETURN") {
		t.Errorf("named dollar-quoted body keywords not uppercased:\n%s", got)
	}
}

func TestColumnsUppercase(t *testing.T) {
	got, err := formatter.Format(
		"CREATE TABLE t (user_id integer, email_addr text)",
		cfgFull(upper, upper, upper),
	)
	want := "CREATE TABLE t (USER_ID INTEGER, EMAIL_ADDR TEXT)"
	assertFormat(t, got, want, err)
}

func TestColumnsLowercase(t *testing.T) {
	got, err := formatter.Format(
		"SELECT UserId, EmailAddr FROM t WHERE UserId = 1",
		cfgFull(upper, upper, lower),
	)
	want := "SELECT userid, emailaddr FROM t WHERE userid = 1"
	assertFormat(t, got, want, err)
}

func TestColumnsPreserve(t *testing.T) {
	got, err := formatter.Format(
		"SELECT UserId, EmailAddr FROM t",
		cfgFull(upper, upper, preserve),
	)
	want := "SELECT UserId, EmailAddr FROM t"
	assertFormat(t, got, want, err)
}

func TestColumnsDDLColumnNameIsKeyword(t *testing.T) {
	// "name" is a pg_query keyword but used as a column name — must follow columns.case
	got, err := formatter.Format(
		"CREATE TABLE t (name text)",
		cfgFull(upper, upper, lower),
	)
	want := "CREATE TABLE t (name TEXT)"
	assertFormat(t, got, want, err)
}

func TestColumnsDotAccess(t *testing.T) {
	// field access after dot must follow columns.case
	got, err := formatter.Format(
		"SELECT old.Value, new.Name FROM t",
		cfgFull(upper, upper, lower),
	)
	want := "SELECT old.value, new.name FROM t"
	assertFormat(t, got, want, err)
}

func fullCfg(res, kw, dt, lit, op, tbl, fn, cfn, sfn, alias, col config.CaseRule) *config.Config {
	return &config.Config{
		ReservedKeywords:     config.Section{Case: res},
		Keywords:             config.Section{Case: kw},
		DataTypes:            config.DataTypesSection{Case: dt, Form: config.TypeFormPreserve},
		Literals:             config.Section{Case: lit},
		Operators:            config.Section{Case: op},
		Schemas:              config.Section{Case: preserve},
		Tables:               config.Section{Case: tbl},
		Functions:            config.Section{Case: fn},
		ConditionalFunctions: config.Section{Case: cfn},
		SystemFunctions:      config.Section{Case: sfn},
		Aliases:              config.AliasSection{Case: alias, As: config.AliasAsPreserve},
		Columns:              config.Section{Case: col},
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
	}
}

func TestLiteralsUpper(t *testing.T) {
	got, err := formatter.Format("SELECT true, false, null FROM t",
		fullCfg(upper, upper, lower, upper, upper, preserve, preserve, upper, upper, preserve, preserve))
	want := "SELECT TRUE, FALSE, NULL FROM t"
	assertFormat(t, got, want, err)
}

func TestLiteralsLower(t *testing.T) {
	got, err := formatter.Format("SELECT TRUE, FALSE, NULL FROM t",
		fullCfg(upper, upper, lower, lower, upper, preserve, preserve, upper, upper, preserve, preserve))
	want := "SELECT true, false, null FROM t"
	assertFormat(t, got, want, err)
}

func TestOperatorsUpper(t *testing.T) {
	got, err := formatter.Format("SELECT * FROM t WHERE col1 = 1 and col2 is not null or col3 between 1 and 10",
		fullCfg(upper, upper, lower, upper, upper, preserve, preserve, upper, upper, preserve, lower))
	want := "SELECT * FROM t WHERE col1 = 1 AND col2 IS NOT NULL OR col3 BETWEEN 1 AND 10"
	assertFormat(t, got, want, err)
}

func TestOperatorsLower(t *testing.T) {
	got, err := formatter.Format("SELECT * FROM t WHERE col1 IN (1,2) AND col2 LIKE '%foo%'",
		fullCfg(upper, upper, lower, upper, lower, preserve, preserve, upper, upper, preserve, lower))
	want := "SELECT * FROM t WHERE col1 in (1,2) and col2 like '%foo%'"
	assertFormat(t, got, want, err)
}

func TestTablesCase(t *testing.T) {
	got, err := formatter.Format("SELECT * FROM MyTable WHERE MyTable.id = 1",
		fullCfg(upper, upper, lower, upper, upper, lower, preserve, upper, upper, preserve, lower))
	want := "SELECT * FROM mytable WHERE mytable.id = 1"
	assertFormat(t, got, want, err)
}

func TestFunctionsCase(t *testing.T) {
	// functions (FuncCall) → lower, conditional → upper, system → upper
	got, err := formatter.Format("SELECT now(), COALESCE(col1, 0), CURRENT_DATE FROM t",
		fullCfg(upper, upper, lower, upper, upper, preserve, lower, upper, upper, preserve, lower))
	want := "SELECT now(), COALESCE(col1, 0), CURRENT_DATE FROM t"
	assertFormat(t, got, want, err)
}

func TestConditionalFunctionsLower(t *testing.T) {
	got, err := formatter.Format("SELECT COALESCE(col1, 0), GREATEST(col1, col2), NULLIF(col1, 0) FROM t",
		fullCfg(upper, upper, lower, upper, upper, preserve, preserve, lower, upper, preserve, lower))
	want := "SELECT coalesce(col1, 0), greatest(col1, col2), nullif(col1, 0) FROM t"
	assertFormat(t, got, want, err)
}

func TestSystemFunctionsLower(t *testing.T) {
	got, err := formatter.Format("SELECT CURRENT_DATE, CURRENT_TIMESTAMP, SESSION_USER FROM t",
		fullCfg(upper, upper, lower, upper, upper, preserve, preserve, upper, lower, preserve, lower))
	want := "SELECT current_date, current_timestamp, session_user FROM t"
	assertFormat(t, got, want, err)
}

func TestSchemasCase(t *testing.T) {
	got, err := formatter.Format("SELECT * FROM Public.MyTable",
		&config.Config{
			ReservedKeywords:     config.Section{Case: upper},
			Keywords:             config.Section{Case: upper},
			DataTypes:            config.DataTypesSection{Case: lower, Form: config.TypeFormPreserve},
			Literals:             config.Section{Case: upper},
			Operators:            config.Section{Case: upper},
			Schemas:              config.Section{Case: lower},
			Tables:               config.Section{Case: lower},
			Functions:            config.Section{Case: lower},
			ConditionalFunctions: config.Section{Case: upper},
			SystemFunctions:      config.Section{Case: upper},
			Aliases:              config.AliasSection{Case: lower, As: config.AliasAsPreserve},
			Columns:              config.Section{Case: lower},
			TrailingWhitespace:   config.TrailingWSPreserve,
		})
	want := "SELECT * FROM public.mytable"
	assertFormat(t, got, want, err)
}

func TestWithCTE(t *testing.T) {
	got, err := formatter.Format(
		"WITH cte AS (SELECT id FROM users) SELECT id FROM cte",
		cfg(upper, lower),
	)
	want := "WITH cte AS (SELECT id FROM users) SELECT id FROM cte"
	assertFormat(t, got, want, err)
}

func TestWindowDef(t *testing.T) {
	got, err := formatter.Format(
		"SELECT row_number() OVER (PARTITION BY dept ORDER BY salary) FROM t",
		cfg(upper, lower),
	)
	want := "SELECT row_number() OVER (PARTITION BY dept ORDER BY salary) FROM t"
	assertFormat(t, got, want, err)
}

func TestReturning(t *testing.T) {
	got, err := formatter.Format(
		"INSERT INTO users (name) VALUES ('alice') RETURNING id, name",
		cfgFull(upper, lower, lower),
	)
	want := "INSERT INTO users (name) VALUES ('alice') RETURNING id, name"
	assertFormat(t, got, want, err)
}

// ── Type form tests ───────────────────────────────────────────────────────

func TestTypeFormLongSingleAlias(t *testing.T) {
	// int → integer, bool → boolean
	got, err := formatter.Format("CREATE TABLE t (a int, b bool, c float4, c2 float8)",
		dtCfg(config.TypeFormLong, lower))
	want := "CREATE TABLE t (a integer, b boolean, c real, c2 double precision)"
	assertFormat(t, got, want, err)
}

func TestTypeFormLongVarchar(t *testing.T) {
	// varchar → character varying
	got, err := formatter.Format("CREATE TABLE t (a varchar(100), b varbit(8))",
		dtCfg(config.TypeFormLong, lower))
	want := "CREATE TABLE t (a character varying(100), b bit varying(8))"
	assertFormat(t, got, want, err)
}

func TestTypeFormLongTimestamp(t *testing.T) {
	got, err := formatter.Format("CREATE TABLE t (a timestamptz, b timetz)",
		dtCfg(config.TypeFormLong, lower))
	want := "CREATE TABLE t (a timestamp with time zone, b time with time zone)"
	assertFormat(t, got, want, err)
}

func TestTypeFormLongCasing(t *testing.T) {
	// upper casing with long form
	got, err := formatter.Format("CREATE TABLE t (a varchar(50), b int)",
		dtCfg(config.TypeFormLong, upper))
	want := "CREATE TABLE t (a CHARACTER VARYING(50), b INTEGER)"
	assertFormat(t, got, want, err)
}

func TestTypeFormShortSingle(t *testing.T) {
	// integer → int, boolean → bool, real → float4
	got, err := formatter.Format("CREATE TABLE t (a integer, b boolean, c real, d character)",
		dtCfg(config.TypeFormShort, lower))
	want := "CREATE TABLE t (a int, b bool, c float4, d char)"
	assertFormat(t, got, want, err)
}

func TestTypeFormShortMultiToken(t *testing.T) {
	// character varying → varchar, double precision → float8
	got, err := formatter.Format("CREATE TABLE t (a character varying(100), b double precision)",
		dtCfg(config.TypeFormShort, lower))
	want := "CREATE TABLE t (a varchar(100), b float8)"
	assertFormat(t, got, want, err)
}

func TestTypeFormShortTimestamp(t *testing.T) {
	got, err := formatter.Format("CREATE TABLE t (a timestamp with time zone, b time with time zone)",
		dtCfg(config.TypeFormShort, lower))
	want := "CREATE TABLE t (a timestamptz, b timetz)"
	assertFormat(t, got, want, err)
}

func TestTypeFormLongNoSpace(t *testing.T) {
	// single-word → long no space; multi-word long forms → short
	got, err := formatter.Format(
		"CREATE TABLE t (a int, b varchar(100), c double precision, d bit varying, e timestamptz)",
		dtCfg(config.TypeFormLongNoSpace, lower))
	want := "CREATE TABLE t (a integer, b varchar(100), c float8, d varbit, e timestamptz)"
	assertFormat(t, got, want, err)
}

func TestTypeFormPreserve(t *testing.T) {
	// nothing changes when form=preserve
	got, err := formatter.Format("CREATE TABLE t (a int4, b character varying(10))",
		dtCfg(config.TypeFormPreserve, lower))
	want := "CREATE TABLE t (a int4, b character varying(10))"
	assertFormat(t, got, want, err)
}

// ── AS keyword tests ──────────────────────────────────────────────────────

func TestAliasAsAdd(t *testing.T) {
	// AS is missing → should be inserted
	got, err := formatter.Format("SELECT id myid, name myname FROM t",
		&config.Config{
			ReservedKeywords:     config.Section{Case: upper},
			Keywords:             config.Section{Case: upper},
			DataTypes:            config.DataTypesSection{Case: lower, Form: config.TypeFormPreserve},
			Literals:             config.Section{Case: upper},
			Operators:            config.Section{Case: upper},
			Schemas:              config.Section{Case: preserve},
			Tables:               config.Section{Case: preserve},
			Functions:            config.Section{Case: preserve},
			ConditionalFunctions: config.Section{Case: upper},
			SystemFunctions:      config.Section{Case: upper},
			Aliases:              config.AliasSection{Case: preserve, As: config.AliasAsAdd},
			Columns:              config.Section{Case: preserve},
			TrailingWhitespace:   config.TrailingWSPreserve,
		})
	want := "SELECT id AS myid, name AS myname FROM t"
	assertFormat(t, got, want, err)
}

func TestAliasAsPresent(t *testing.T) {
	// AS already present → do not duplicate
	got, err := formatter.Format("SELECT id AS myid FROM t",
		&config.Config{
			ReservedKeywords:     config.Section{Case: upper},
			Keywords:             config.Section{Case: upper},
			DataTypes:            config.DataTypesSection{Case: lower, Form: config.TypeFormPreserve},
			Literals:             config.Section{Case: upper},
			Operators:            config.Section{Case: upper},
			Schemas:              config.Section{Case: preserve},
			Tables:               config.Section{Case: preserve},
			Functions:            config.Section{Case: preserve},
			ConditionalFunctions: config.Section{Case: upper},
			SystemFunctions:      config.Section{Case: upper},
			Aliases:              config.AliasSection{Case: preserve, As: config.AliasAsAdd},
			Columns:              config.Section{Case: preserve},
			TrailingWhitespace:   config.TrailingWSPreserve,
		})
	want := "SELECT id AS myid FROM t"
	assertFormat(t, got, want, err)
}

func TestAliasAsRemove(t *testing.T) {
	got, err := formatter.Format("SELECT id AS myid, name AS myname FROM t",
		&config.Config{
			ReservedKeywords:     config.Section{Case: upper},
			Keywords:             config.Section{Case: upper},
			DataTypes:            config.DataTypesSection{Case: lower, Form: config.TypeFormPreserve},
			Literals:             config.Section{Case: upper},
			Operators:            config.Section{Case: upper},
			Schemas:              config.Section{Case: preserve},
			Tables:               config.Section{Case: preserve},
			Functions:            config.Section{Case: preserve},
			ConditionalFunctions: config.Section{Case: upper},
			SystemFunctions:      config.Section{Case: upper},
			Aliases:              config.AliasSection{Case: preserve, As: config.AliasAsRemove},
			Columns:              config.Section{Case: preserve},
			TrailingWhitespace:   config.TrailingWSPreserve,
		})
	want := "SELECT id myid, name myname FROM t"
	assertFormat(t, got, want, err)
}

// ── Trailing whitespace tests ─────────────────────────────────────────────

func TestTrailingWhitespaceStrip(t *testing.T) {
	input := "SELECT id   \nFROM t   \nWHERE id = 1  "
	got, err := formatter.Format(input,
		&config.Config{
			ReservedKeywords:     config.Section{Case: upper},
			Keywords:             config.Section{Case: upper},
			DataTypes:            config.DataTypesSection{Case: lower, Form: config.TypeFormPreserve},
			Literals:             config.Section{Case: upper},
			Operators:            config.Section{Case: upper},
			Schemas:              config.Section{Case: preserve},
			Tables:               config.Section{Case: preserve},
			Functions:            config.Section{Case: preserve},
			ConditionalFunctions: config.Section{Case: upper},
			SystemFunctions:      config.Section{Case: upper},
			Aliases:              config.AliasSection{Case: preserve, As: config.AliasAsPreserve},
			Columns:              config.Section{Case: preserve},
			TrailingWhitespace:   config.TrailingWSStrip,
		})
	want := "SELECT id\nFROM t\nWHERE id = 1"
	assertFormat(t, got, want, err)
}

func TestTrailingWhitespacePreservesStringContent(t *testing.T) {
	// Spaces inside string literals must never be stripped.
	input := "SELECT 'hello   ' FROM t   \n"
	got, err := formatter.Format(input,
		&config.Config{
			ReservedKeywords:     config.Section{Case: upper},
			Keywords:             config.Section{Case: upper},
			DataTypes:            config.DataTypesSection{Case: lower, Form: config.TypeFormPreserve},
			Literals:             config.Section{Case: upper},
			Operators:            config.Section{Case: upper},
			Schemas:              config.Section{Case: preserve},
			Tables:               config.Section{Case: preserve},
			Functions:            config.Section{Case: preserve},
			ConditionalFunctions: config.Section{Case: upper},
			SystemFunctions:      config.Section{Case: upper},
			Aliases:              config.AliasSection{Case: preserve, As: config.AliasAsPreserve},
			Columns:              config.Section{Case: preserve},
			TrailingWhitespace:   config.TrailingWSStrip,
		})
	want := "SELECT 'hello   ' FROM t\n"
	assertFormat(t, got, want, err)
}

func TestEmptyInput(t *testing.T) {
	got, err := formatter.Format("", cfg(upper, upper))
	assertFormat(t, got, "", err)
}

// p3cfg builds a config with all Phase-3 options set to the given values and
// everything else at safe preserve/upper defaults.
func p3cfg(semi config.SemicolonMode, ineq config.InequalityOp, join config.JoinForm,
	ops config.OperatorSpacingMode, bl config.BlankLinesMode,
	paren config.ParenSpacingMode, qi config.QuotedIdentMode) *config.Config {
	return &config.Config{
		ReservedKeywords:     config.Section{Case: upper},
		Keywords:             config.Section{Case: upper},
		DataTypes:            config.DataTypesSection{Case: lower, Form: config.TypeFormPreserve},
		Literals:             config.Section{Case: upper},
		Operators:            config.Section{Case: upper},
		Schemas:              config.Section{Case: preserve},
		Tables:               config.Section{Case: preserve},
		Functions:            config.Section{Case: preserve},
		ConditionalFunctions: config.Section{Case: upper},
		SystemFunctions:      config.Section{Case: upper},
		Aliases:              config.AliasSection{Case: preserve, As: config.AliasAsPreserve},
		Columns:              config.Section{Case: preserve},
		TrailingWhitespace:   config.TrailingWSPreserve,
		Semicolons:           semi,
		InequalityOp:         ineq,
		JoinForm:             join,
		OperatorSpacing:      ops,
		BlankLines:           bl,
		ParenSpacing:         paren,
		QuotedIdents:         qi,
		TrailingNewline:      config.TrailingNewlinePreserve,
		CommaSpacing:         config.CommaSpacingPreserve,
		OrderAsc:             config.OrderAscPreserve,
		CastStyle:            config.CastStylePreserve,
		NotIn:                config.NotInPreserve,
		SchemaQual:           config.SchemaQualPreserve,
	}
}

// ── Semicolons ────────────────────────────────────────────────────────────────

func TestSemicolonPreserve(t *testing.T) {
	got, err := formatter.Format("SELECT 1; SELECT 2;",
		p3cfg(config.SemicolonPreserve, config.InequalityPreserve, config.JoinFormPreserve,
			config.OperatorSpacingPreserve, config.BlankLinesPreserve,
			config.ParenSpacingPreserve, config.QuotedIdentPreserve))
	assertFormat(t, got, "SELECT 1; SELECT 2;", err)
}

func TestSemicolonRemove(t *testing.T) {
	got, err := formatter.Format("SELECT 1; SELECT 2;",
		p3cfg(config.SemicolonRemove, config.InequalityPreserve, config.JoinFormPreserve,
			config.OperatorSpacingPreserve, config.BlankLinesPreserve,
			config.ParenSpacingPreserve, config.QuotedIdentPreserve))
	assertFormat(t, got, "SELECT 1 SELECT 2", err)
}

func TestSemicolonAdd(t *testing.T) {
	got, err := formatter.Format("SELECT 1",
		p3cfg(config.SemicolonAdd, config.InequalityPreserve, config.JoinFormPreserve,
			config.OperatorSpacingPreserve, config.BlankLinesPreserve,
			config.ParenSpacingPreserve, config.QuotedIdentPreserve))
	assertFormat(t, got, "SELECT 1;", err)
}

func TestSemicolonAddAlreadyPresent(t *testing.T) {
	got, err := formatter.Format("SELECT 1;",
		p3cfg(config.SemicolonAdd, config.InequalityPreserve, config.JoinFormPreserve,
			config.OperatorSpacingPreserve, config.BlankLinesPreserve,
			config.ParenSpacingPreserve, config.QuotedIdentPreserve))
	assertFormat(t, got, "SELECT 1;", err)
}

// ── Inequality operator ───────────────────────────────────────────────────────

func TestInequalityC(t *testing.T) {
	got, err := formatter.Format("SELECT * FROM t WHERE a <> b",
		p3cfg(config.SemicolonPreserve, config.InequalityC, config.JoinFormPreserve,
			config.OperatorSpacingPreserve, config.BlankLinesPreserve,
			config.ParenSpacingPreserve, config.QuotedIdentPreserve))
	assertFormat(t, got, "SELECT * FROM t WHERE a != b", err)
}

func TestInequalityANSI(t *testing.T) {
	got, err := formatter.Format("SELECT * FROM t WHERE a != b",
		p3cfg(config.SemicolonPreserve, config.InequalityANSI, config.JoinFormPreserve,
			config.OperatorSpacingPreserve, config.BlankLinesPreserve,
			config.ParenSpacingPreserve, config.QuotedIdentPreserve))
	assertFormat(t, got, "SELECT * FROM t WHERE a <> b", err)
}

func TestInequalityPreserve(t *testing.T) {
	got, err := formatter.Format("SELECT * FROM t WHERE a != b AND c <> d",
		p3cfg(config.SemicolonPreserve, config.InequalityPreserve, config.JoinFormPreserve,
			config.OperatorSpacingPreserve, config.BlankLinesPreserve,
			config.ParenSpacingPreserve, config.QuotedIdentPreserve))
	assertFormat(t, got, "SELECT * FROM t WHERE a != b AND c <> d", err)
}

// ── JOIN form ─────────────────────────────────────────────────────────────────

func TestJoinFormShortInner(t *testing.T) {
	got, err := formatter.Format("SELECT * FROM a INNER JOIN b ON a.id = b.id",
		p3cfg(config.SemicolonPreserve, config.InequalityPreserve, config.JoinFormShort,
			config.OperatorSpacingPreserve, config.BlankLinesPreserve,
			config.ParenSpacingPreserve, config.QuotedIdentPreserve))
	assertFormat(t, got, "SELECT * FROM a JOIN b ON a.id = b.id", err)
}

func TestJoinFormShortLeftOuter(t *testing.T) {
	got, err := formatter.Format("SELECT * FROM a LEFT OUTER JOIN b ON a.id = b.id",
		p3cfg(config.SemicolonPreserve, config.InequalityPreserve, config.JoinFormShort,
			config.OperatorSpacingPreserve, config.BlankLinesPreserve,
			config.ParenSpacingPreserve, config.QuotedIdentPreserve))
	assertFormat(t, got, "SELECT * FROM a LEFT JOIN b ON a.id = b.id", err)
}

func TestJoinFormLongBareJoin(t *testing.T) {
	got, err := formatter.Format("SELECT * FROM a JOIN b ON a.id = b.id",
		p3cfg(config.SemicolonPreserve, config.InequalityPreserve, config.JoinFormLong,
			config.OperatorSpacingPreserve, config.BlankLinesPreserve,
			config.ParenSpacingPreserve, config.QuotedIdentPreserve))
	assertFormat(t, got, "SELECT * FROM a INNER JOIN b ON a.id = b.id", err)
}

func TestJoinFormLongLeftJoin(t *testing.T) {
	got, err := formatter.Format("SELECT * FROM a LEFT JOIN b ON a.id = b.id",
		p3cfg(config.SemicolonPreserve, config.InequalityPreserve, config.JoinFormLong,
			config.OperatorSpacingPreserve, config.BlankLinesPreserve,
			config.ParenSpacingPreserve, config.QuotedIdentPreserve))
	assertFormat(t, got, "SELECT * FROM a LEFT OUTER JOIN b ON a.id = b.id", err)
}

// ── Operator spacing ──────────────────────────────────────────────────────────

func TestOperatorSpacingNormalize(t *testing.T) {
	got, err := formatter.Format("SELECT * FROM t WHERE a=1 AND b>=2",
		p3cfg(config.SemicolonPreserve, config.InequalityPreserve, config.JoinFormPreserve,
			config.OperatorSpacingNormalize, config.BlankLinesPreserve,
			config.ParenSpacingPreserve, config.QuotedIdentPreserve))
	assertFormat(t, got, "SELECT * FROM t WHERE a = 1 AND b >= 2", err)
}

func TestOperatorSpacingPreserve(t *testing.T) {
	got, err := formatter.Format("SELECT * FROM t WHERE a=1",
		p3cfg(config.SemicolonPreserve, config.InequalityPreserve, config.JoinFormPreserve,
			config.OperatorSpacingPreserve, config.BlankLinesPreserve,
			config.ParenSpacingPreserve, config.QuotedIdentPreserve))
	assertFormat(t, got, "SELECT * FROM t WHERE a=1", err)
}

// ── Blank lines ───────────────────────────────────────────────────────────────

func TestBlankLinesMax1(t *testing.T) {
	input := "SELECT 1;\n\n\n\nSELECT 2;"
	got, err := formatter.Format(input,
		p3cfg(config.SemicolonPreserve, config.InequalityPreserve, config.JoinFormPreserve,
			config.OperatorSpacingPreserve, config.BlankLinesMax1,
			config.ParenSpacingPreserve, config.QuotedIdentPreserve))
	assertFormat(t, got, "SELECT 1;\n\nSELECT 2;", err)
}

func TestBlankLinesMax2(t *testing.T) {
	input := "SELECT 1;\n\n\n\nSELECT 2;"
	got, err := formatter.Format(input,
		p3cfg(config.SemicolonPreserve, config.InequalityPreserve, config.JoinFormPreserve,
			config.OperatorSpacingPreserve, config.BlankLinesMax2,
			config.ParenSpacingPreserve, config.QuotedIdentPreserve))
	assertFormat(t, got, "SELECT 1;\n\n\nSELECT 2;", err)
}

func TestBlankLinesPreserve(t *testing.T) {
	input := "SELECT 1;\n\n\n\nSELECT 2;"
	got, err := formatter.Format(input,
		p3cfg(config.SemicolonPreserve, config.InequalityPreserve, config.JoinFormPreserve,
			config.OperatorSpacingPreserve, config.BlankLinesPreserve,
			config.ParenSpacingPreserve, config.QuotedIdentPreserve))
	assertFormat(t, got, "SELECT 1;\n\n\n\nSELECT 2;", err)
}

// ── Paren spacing ─────────────────────────────────────────────────────────────

func TestParenSpacingRemove(t *testing.T) {
	got, err := formatter.Format("SELECT count( * ) FROM t WHERE ( a = 1 )",
		p3cfg(config.SemicolonPreserve, config.InequalityPreserve, config.JoinFormPreserve,
			config.OperatorSpacingPreserve, config.BlankLinesPreserve,
			config.ParenSpacingRemove, config.QuotedIdentPreserve))
	assertFormat(t, got, "SELECT count(*) FROM t WHERE (a = 1)", err)
}

func TestParenSpacingAdd(t *testing.T) {
	got, err := formatter.Format("SELECT count(*) FROM t WHERE (a = 1)",
		p3cfg(config.SemicolonPreserve, config.InequalityPreserve, config.JoinFormPreserve,
			config.OperatorSpacingPreserve, config.BlankLinesPreserve,
			config.ParenSpacingAdd, config.QuotedIdentPreserve))
	assertFormat(t, got, "SELECT count( * ) FROM t WHERE ( a = 1 )", err)
}

// ── Quoted identifiers ────────────────────────────────────────────────────────

func TestQuotedIdentRemoveSafe(t *testing.T) {
	got, err := formatter.Format(`SELECT "user_id", "name" FROM t`,
		p3cfg(config.SemicolonPreserve, config.InequalityPreserve, config.JoinFormPreserve,
			config.OperatorSpacingPreserve, config.BlankLinesPreserve,
			config.ParenSpacingPreserve, config.QuotedIdentRemoveSafe))
	assertFormat(t, got, "SELECT user_id, name FROM t", err)
}

func TestQuotedIdentRemoveSafeKeepsKeyword(t *testing.T) {
	// "select" is a reserved keyword — must stay quoted.
	got, err := formatter.Format(`SELECT "select" FROM t`,
		p3cfg(config.SemicolonPreserve, config.InequalityPreserve, config.JoinFormPreserve,
			config.OperatorSpacingPreserve, config.BlankLinesPreserve,
			config.ParenSpacingPreserve, config.QuotedIdentRemoveSafe))
	assertFormat(t, got, `SELECT "select" FROM t`, err)
}

func TestQuotedIdentPreserve(t *testing.T) {
	got, err := formatter.Format(`SELECT "user_id" FROM t`,
		p3cfg(config.SemicolonPreserve, config.InequalityPreserve, config.JoinFormPreserve,
			config.OperatorSpacingPreserve, config.BlankLinesPreserve,
			config.ParenSpacingPreserve, config.QuotedIdentPreserve))
	assertFormat(t, got, `SELECT "user_id" FROM t`, err)
}

// ── Trailing newline ──────────────────────────────────────────────────────────

func newFeatureCfg() *config.Config {
	return &config.Config{
		ReservedKeywords:     config.Section{Case: upper},
		Keywords:             config.Section{Case: upper},
		DataTypes:            config.DataTypesSection{Case: lower, Form: config.TypeFormPreserve},
		Literals:             config.Section{Case: upper},
		Operators:            config.Section{Case: upper},
		Schemas:              config.Section{Case: preserve},
		Tables:               config.Section{Case: preserve},
		Functions:            config.Section{Case: preserve},
		ConditionalFunctions: config.Section{Case: upper},
		SystemFunctions:      config.Section{Case: upper},
		Aliases:              config.AliasSection{Case: preserve, As: config.AliasAsPreserve},
		Columns:              config.Section{Case: preserve},
		TrailingWhitespace:   config.TrailingWSStrip,
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
	}
}

func TestTrailingNewlineAdd(t *testing.T) {
	c := newFeatureCfg()
	c.TrailingNewline = config.TrailingNewlineAdd
	got, err := formatter.Format("SELECT 1", c)
	assertFormat(t, got, "SELECT 1\n", err)
}

func TestTrailingNewlineStrip(t *testing.T) {
	c := newFeatureCfg()
	c.TrailingNewline = config.TrailingNewlineStrip
	got, err := formatter.Format("SELECT 1\n", c)
	assertFormat(t, got, "SELECT 1", err)
}

func TestTrailingNewlinePreserveWithNewline(t *testing.T) {
	c := newFeatureCfg()
	c.TrailingNewline = config.TrailingNewlinePreserve
	got, err := formatter.Format("SELECT 1\n", c)
	assertFormat(t, got, "SELECT 1\n", err)
}

// ── Comma spacing ─────────────────────────────────────────────────────────────

func TestCommaSpacingNormalize(t *testing.T) {
	c := newFeatureCfg()
	c.CommaSpacing = config.CommaSpacingNormalize
	got, err := formatter.Format("SELECT a,b,c FROM t", c)
	assertFormat(t, got, "SELECT a, b, c FROM t", err)
}

func TestCommaSpacingNoSpaceBefore(t *testing.T) {
	c := newFeatureCfg()
	c.CommaSpacing = config.CommaSpacingNormalize
	got, err := formatter.Format("SELECT a ,b FROM t", c)
	assertFormat(t, got, "SELECT a, b FROM t", err)
}

// ── order_asc ─────────────────────────────────────────────────────────────────

func TestOrderAscRemove(t *testing.T) {
	c := newFeatureCfg()
	c.OrderAsc = config.OrderAscRemove
	got, err := formatter.Format("SELECT * FROM t ORDER BY a ASC, b DESC", c)
	assertFormat(t, got, "SELECT * FROM t ORDER BY a, b DESC", err)
}

func TestOrderAscAdd(t *testing.T) {
	c := newFeatureCfg()
	c.OrderAsc = config.OrderAscAdd
	got, err := formatter.Format("SELECT * FROM t ORDER BY a, b DESC", c)
	assertFormat(t, got, "SELECT * FROM t ORDER BY a ASC, b DESC", err)
}

func TestOrderAscAddAtEnd(t *testing.T) {
	c := newFeatureCfg()
	c.OrderAsc = config.OrderAscAdd
	got, err := formatter.Format("SELECT * FROM t ORDER BY a", c)
	assertFormat(t, got, "SELECT * FROM t ORDER BY a ASC", err)
}

// ── cast_style ────────────────────────────────────────────────────────────────

func TestCastStyleOperator(t *testing.T) {
	c := newFeatureCfg()
	c.CastStyle = config.CastStyleOperator
	got, err := formatter.Format("SELECT CAST(x AS integer) FROM t", c)
	assertFormat(t, got, "SELECT x::integer FROM t", err)
}

func TestCastStyleOperatorPreserveDoubleColon(t *testing.T) {
	c := newFeatureCfg()
	c.CastStyle = config.CastStyleOperator
	got, err := formatter.Format("SELECT x::integer FROM t", c)
	assertFormat(t, got, "SELECT x::integer FROM t", err)
}

// ── not_in ────────────────────────────────────────────────────────────────────

func TestNotInNotEqualsAll(t *testing.T) {
	c := newFeatureCfg()
	c.NotIn = config.NotInNotEqualsAll
	got, err := formatter.Format("SELECT * FROM t WHERE a NOT IN (SELECT id FROM u)", c)
	assertFormat(t, got, "SELECT * FROM t WHERE a <> ALL (SELECT id FROM u)", err)
}

func TestNotInNotIn(t *testing.T) {
	c := newFeatureCfg()
	c.NotIn = config.NotInNotIn
	got, err := formatter.Format("SELECT * FROM t WHERE a <> ALL (SELECT id FROM u)", c)
	assertFormat(t, got, "SELECT * FROM t WHERE a NOT IN (SELECT id FROM u)", err)
}

// ── Schema qualification ──────────────────────────────────────────────────────

func TestSchemaQualRemovePublicTable(t *testing.T) {
	c := newFeatureCfg()
	c.SchemaQual = config.SchemaQualRemovePublic
	got, err := formatter.Format("SELECT * FROM public.users", c)
	assertFormat(t, got, "SELECT * FROM users", err)
}

func TestSchemaQualRemovePublicFunction(t *testing.T) {
	c := newFeatureCfg()
	c.SchemaQual = config.SchemaQualRemovePublic
	got, err := formatter.Format("SELECT public.my_func(x) FROM t", c)
	assertFormat(t, got, "SELECT my_func(x) FROM t", err)
}

func TestSchemaQualRemovePublicMultiple(t *testing.T) {
	c := newFeatureCfg()
	c.SchemaQual = config.SchemaQualRemovePublic
	got, err := formatter.Format("SELECT * FROM public.orders JOIN public.users ON o.user_id = u.id", c)
	assertFormat(t, got, "SELECT * FROM orders JOIN users ON o.user_id = u.id", err)
}

func TestSchemaQualPreservesOtherSchemas(t *testing.T) {
	c := newFeatureCfg()
	c.SchemaQual = config.SchemaQualRemovePublic
	got, err := formatter.Format("SELECT * FROM myschema.users", c)
	assertFormat(t, got, "SELECT * FROM myschema.users", err)
}

func TestSchemaQualPreserve(t *testing.T) {
	c := newFeatureCfg()
	c.SchemaQual = config.SchemaQualPreserve
	got, err := formatter.Format("SELECT * FROM public.users", c)
	assertFormat(t, got, "SELECT * FROM public.users", err)
}

func assertFormat(t *testing.T, got, want string, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("\ngot:  %q\nwant: %q", got, want)
	}
}
