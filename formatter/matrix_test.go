package formatter_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	pg_query "github.com/pganalyze/pg_query_go/v6"

	"github.com/heptau/pg_procrustes/config"
	"github.com/heptau/pg_procrustes/formatter"
)

// matrixVariant mutates a default config. All variants must keep the SQL
// semantically identical (same AST fingerprint), parseable and idempotent.
type matrixVariant struct {
	name  string
	apply func(c *config.Config)
}

var matrixVariants = []matrixVariant{
	{"call_paren_add_builtin_upper", func(c *config.Config) {
		c.CallParenSpacing = config.ParenSpacingAdd
		c.BuiltinFunctions.Case = config.CaseUpper
	}},
	{"call_paren_remove_builtin_capitalize", func(c *config.Config) {
		c.CallParenSpacing = config.ParenSpacingRemove
		c.BuiltinFunctions.Case = config.CaseCapitalize
	}},
	{"keyword_paren_add", func(c *config.Config) {
		c.KeywordParenSpacing = config.ParenSpacingAdd
		c.CallParenSpacing = config.ParenSpacingRemove
		c.Layout.Clauses.Break = config.BreakAlways
		c.Layout.ParenIndent.Open = config.ParenOpenOwnLine
	}},
	{"keyword_paren_remove", func(c *config.Config) {
		c.KeywordParenSpacing = config.ParenSpacingRemove
		c.Layout.Clauses.Break = config.BreakAlways
		c.Layout.Content.Break = config.BreakAuto
	}},
	{"subquery_always", func(c *config.Config) {
		c.Layout.Clauses.Break = config.BreakAlways
		c.Layout.Content.Break = config.BreakAuto
		c.Layout.Subquery.Break = config.BreakAlways
	}},
	{"subquery_always_start_commas", func(c *config.Config) {
		c.Layout.Clauses.Break = config.BreakAlways
		c.Layout.Content.Break = config.BreakAlways
		c.Layout.Content.CommaPosition = config.CommaStart
		c.Layout.Subquery.Break = config.BreakAlways
	}},
	{"subquery_auto_never", func(c *config.Config) {
		c.Layout.LineLength = 40
		c.Layout.Subquery.Break = config.BreakAuto
	}},
	{"paren_open_own_line", func(c *config.Config) {
		c.Layout.Clauses.Break = config.BreakAlways
		c.Layout.Content.Break = config.BreakAuto
		c.Layout.Subquery.Break = config.BreakAlways
		c.Layout.ParenIndent.Open = config.ParenOpenOwnLine
	}},
	{"paren_open_own_line_ddl", func(c *config.Config) {
		c.Layout.DDL.CreateTable.Break = config.BreakAlways
		c.Layout.DDL.CreateIndex.Break = config.BreakAlways
		c.Layout.Content.WrapExpressions = config.WrapWrap
		c.Layout.LineLength = 60
		c.Layout.ParenIndent.Open = config.ParenOpenOwnLine
	}},
	{"subquery_never", func(c *config.Config) { c.Layout.Subquery.Break = config.BreakNever }},
	{"clauses_always", func(c *config.Config) { c.Layout.Clauses.Break = config.BreakAlways }},
	{"content_always", func(c *config.Config) { c.Layout.Content.Break = config.BreakAlways }},
	{"clauses_content_always", func(c *config.Config) {
		c.Layout.Clauses.Break = config.BreakAlways
		c.Layout.Content.Break = config.BreakAlways
	}},
	{"always_first_inline", func(c *config.Config) {
		c.Layout.Clauses.Break = config.BreakAlways
		c.Layout.Content.Break = config.BreakAlways
		c.Layout.Content.FirstItem = config.FirstItemInline
	}},
	{"always_align_indent", func(c *config.Config) {
		c.Layout.Clauses.Break = config.BreakAlways
		c.Layout.Clauses.Align = config.AlignIndent
		c.Layout.Content.Break = config.BreakAlways
	}},
	{"always_river", func(c *config.Config) {
		c.Layout.Clauses.Break = config.BreakAlways
		c.Layout.Clauses.Align = config.AlignRight
		c.Layout.Content.Break = config.BreakAlways
		c.Layout.Content.FirstItem = config.FirstItemInline
	}},
	{"always_tabs", func(c *config.Config) {
		c.Layout.Clauses.Break = config.BreakAlways
		c.Layout.Content.Break = config.BreakAlways
		c.Layout.Indent.Type = config.IndentTypeTab
	}},
	{"auto_short_lines", func(c *config.Config) {
		c.Layout.LineLength = 30
		c.Layout.Clauses.Break = config.BreakAuto
		c.Layout.Content.Break = config.BreakAuto
	}},
	{"never", func(c *config.Config) {
		c.Layout.Clauses.Break = config.BreakNever
		c.Layout.Content.Break = config.BreakNever
	}},
	{"union_both", func(c *config.Config) {
		c.Layout.Clauses.Break = config.BreakAlways
		c.Layout.Union.BlankLine = config.UnionBlankLineBoth
	}},
	{"case_always", func(c *config.Config) { c.Layout.Case.Break = config.BreakAlways }},
	{"case_never", func(c *config.Config) { c.Layout.Case.Break = config.BreakNever }},
	{"all_layout", func(c *config.Config) {
		c.Layout.Clauses.Break = config.BreakAlways
		c.Layout.Content.Break = config.BreakAlways
		c.Layout.Case.Break = config.BreakAlways
		c.Layout.Union.BlankLine = config.UnionBlankLineBoth
	}},
	{"operators_compact", func(c *config.Config) { c.OperatorSpacing = config.OperatorSpacingCompact }},
	{"arith_normalize", func(c *config.Config) { c.ArithmeticSpacing = config.ArithmeticSpacingNormalize }},
	{"arith_compact", func(c *config.Config) { c.ArithmeticSpacing = config.ArithmeticSpacingCompact }},
	{"comma_compact", func(c *config.Config) { c.CommaSpacing = config.CommaSpacingCompact }},
	{"paren_add", func(c *config.Config) { c.ParenSpacing = config.ParenSpacingAdd }},
	{"inline_normalize", func(c *config.Config) { c.InlineSpacing = config.InlineSpacingNormalize }},
	{"aliases_no_as", func(c *config.Config) { c.Aliases.As = config.AliasAsRemove }},
	{"join_long", func(c *config.Config) { c.JoinForm = config.JoinFormLong }},
	{"cast_operator", func(c *config.Config) { c.CastStyle = config.CastStyleOperator }},
	{"statement_break_clauses", func(c *config.Config) {
		c.StatementBreak = config.StatementBreakAlways
		c.Layout.Clauses.Break = config.BreakAlways
	}},
	{"align_none_comments_none", func(c *config.Config) {
		c.Layout.Clauses.Break = config.BreakAlways
		c.Layout.Content.Break = config.BreakAlways
		c.Layout.Content.AlignAliases = config.AlignItemsNone
		c.Layout.Content.AlignSet = config.AlignItemsNone
		c.Comments.AlignTrailing = config.CommentAlignNone
	}},
	{"positions_explicit", func(c *config.Config) {
		c.Layout.Clauses.Break = config.BreakAlways
		c.Layout.Content.Break = config.BreakAlways
		c.Layout.Content.LogicalOperatorPosition = config.LogicalOperatorEnd
		c.Layout.Content.JoinOnPosition = config.JoinOnNewline
		c.Layout.Content.Align = config.AlignSame
	}},
	{"align_aliases_set", func(c *config.Config) {
		c.Layout.Clauses.Break = config.BreakAlways
		c.Layout.Content.Break = config.BreakAlways
		c.Layout.Content.AlignAliases = config.AlignItemsAlign
		c.Layout.Content.AlignSet = config.AlignItemsAlign
	}},
	{"align_aliases_set_comma_start", func(c *config.Config) {
		c.Layout.Clauses.Break = config.BreakAlways
		c.Layout.Content.Break = config.BreakAlways
		c.Layout.Content.CommaPosition = config.CommaStart
		c.Layout.Content.AlignAliases = config.AlignItemsAlign
		c.Layout.Content.AlignSet = config.AlignItemsAlign
	}},
	{"comma_start", func(c *config.Config) {
		c.Layout.Clauses.Break = config.BreakAlways
		c.Layout.Content.Break = config.BreakAlways
		c.Layout.Content.CommaPosition = config.CommaStart
	}},
	{"comma_start_first_inline", func(c *config.Config) {
		c.Layout.Clauses.Break = config.BreakAlways
		c.Layout.Content.Break = config.BreakAlways
		c.Layout.Content.FirstItem = config.FirstItemInline
		c.Layout.Content.CommaPosition = config.CommaStart
	}},
	{"operator_end", func(c *config.Config) {
		c.Layout.Clauses.Break = config.BreakAlways
		c.Layout.Content.Break = config.BreakAlways
		c.Layout.Content.LogicalOperatorPosition = config.LogicalOperatorEnd
	}},
	{"join_on_newline", func(c *config.Config) {
		c.Layout.Clauses.Break = config.BreakAlways
		c.Layout.Clauses.Join.Align = config.AlignIndent
		c.Layout.Content.Break = config.BreakAlways
		c.Layout.Content.JoinOnPosition = config.JoinOnNewline
	}},
	{"positions_combined_tabs", func(c *config.Config) {
		c.Layout.Clauses.Break = config.BreakAlways
		c.Layout.Content.Break = config.BreakAlways
		c.Layout.Content.CommaPosition = config.CommaStart
		c.Layout.Content.LogicalOperatorPosition = config.LogicalOperatorEnd
		c.Layout.Content.JoinOnPosition = config.JoinOnNewline
		c.Layout.Indent.Type = config.IndentTypeTab
	}},
	{"ddl_create_always_align_all", func(c *config.Config) {
		c.Layout.DDL.CreateTable = config.CreateTableCfg{Break: config.BreakAlways, Align: config.ColumnAlignAll}
		c.Layout.DDL.AlterTable.Break = config.BreakAlways
	}},
	{"ddl_create_always_align_types", func(c *config.Config) {
		c.Layout.DDL.CreateTable = config.CreateTableCfg{Break: config.BreakAlways, Align: config.ColumnAlignTypes}
		c.Layout.DDL.AlterTable.Break = config.BreakAuto
	}},
	{"ddl_create_always_align_none", func(c *config.Config) {
		c.Layout.DDL.CreateTable = config.CreateTableCfg{Break: config.BreakAlways, Align: config.ColumnAlignNone}
	}},
	{"ddl_index_always", func(c *config.Config) {
		c.Layout.DDL.CreateIndex.Break = config.BreakAlways
		c.Layout.Clauses.Break = config.BreakAlways
		c.Layout.Content.Break = config.BreakAlways
	}},
	{"ddl_index_never", func(c *config.Config) { c.Layout.DDL.CreateIndex.Break = config.BreakNever }},
	{"ddl_index_auto_narrow", func(c *config.Config) {
		c.Layout.DDL.CreateIndex.Break = config.BreakAuto
		c.Layout.LineLength = 40
	}},
	{"ddl_function_always", func(c *config.Config) {
		c.Layout.DDL.CreateFunction = config.CreateFunctionCfg{Params: config.BreakAlways, Options: config.BreakAlways}
	}},
	{"ddl_function_never", func(c *config.Config) {
		c.Layout.DDL.CreateFunction = config.CreateFunctionCfg{Params: config.BreakNever, Options: config.BreakNever}
	}},
	{"ddl_function_auto_narrow", func(c *config.Config) {
		c.Layout.DDL.CreateFunction = config.CreateFunctionCfg{Params: config.BreakAuto, Options: config.BreakAuto}
		c.Layout.LineLength = 50
		c.Layout.Clauses.Break = config.BreakAlways
	}},
	{"wrap_narrow", func(c *config.Config) {
		c.Layout.LineLength = 40
		c.Layout.Clauses.Break = config.BreakAlways
		c.Layout.Content.Break = config.BreakAuto
		c.Layout.Content.WrapExpressions = config.WrapWrap
	}},
	{"wrap_always_tabs_comma_start", func(c *config.Config) {
		c.Layout.LineLength = 30
		c.Layout.Clauses.Break = config.BreakAlways
		c.Layout.Content.Break = config.BreakAlways
		c.Layout.Content.CommaPosition = config.CommaStart
		c.Layout.Content.WrapExpressions = config.WrapWrap
		c.Layout.Indent.Type = config.IndentTypeTab
	}},
	{"ddl_never", func(c *config.Config) {
		c.Layout.DDL.CreateTable.Break = config.BreakNever
		c.Layout.DDL.AlterTable.Break = config.BreakNever
	}},
	{"ddl_auto_short", func(c *config.Config) {
		c.Layout.LineLength = 30
		c.Layout.DDL.CreateTable = config.CreateTableCfg{Break: config.BreakAuto, Align: config.ColumnAlignTypes}
		c.Layout.DDL.AlterTable.Break = config.BreakAuto
	}},
	{"ddl_comma_start_tabs", func(c *config.Config) {
		c.Layout.DDL.CreateTable = config.CreateTableCfg{Break: config.BreakAlways, Align: config.ColumnAlignAll}
		c.Layout.DDL.AlterTable.Break = config.BreakAlways
		c.Layout.Content.CommaPosition = config.CommaStart
		c.Layout.Indent.Type = config.IndentTypeTab
	}},
	{"ddl_with_everything", func(c *config.Config) {
		c.Layout.Clauses.Break = config.BreakAlways
		c.Layout.Content.Break = config.BreakAlways
		c.Layout.DDL.CreateTable = config.CreateTableCfg{Break: config.BreakAlways, Align: config.ColumnAlignAll}
		c.Layout.DDL.AlterTable.Break = config.BreakAlways
		c.Layout.ParenIndent.Mode = config.ParenIndentIndent
	}},
	{"ddl_more_always", func(c *config.Config) {
		c.Layout.DDL.CreateView.Columns = config.BreakAlways
		c.Layout.DDL.CreateTrigger.Break = config.BreakAlways
		c.Layout.DDL.CreatePolicy.Break = config.BreakAlways
		c.Layout.DDL.Grant.Break = config.BreakAlways
		c.Layout.DDL.CreateType.Break = config.BreakAlways
		c.Layout.DDL.CreateSequence.Break = config.BreakAlways
	}},
	{"ddl_more_never", func(c *config.Config) {
		c.Layout.DDL.CreateView.Columns = config.BreakNever
		c.Layout.DDL.CreateTrigger.Break = config.BreakNever
		c.Layout.DDL.CreatePolicy.Break = config.BreakNever
		c.Layout.DDL.Grant.Break = config.BreakNever
		c.Layout.DDL.CreateType.Break = config.BreakNever
		c.Layout.DDL.CreateSequence.Break = config.BreakNever
	}},
	{"ddl_more_auto_narrow_tabs_comma_start", func(c *config.Config) {
		c.Layout.LineLength = 25
		c.Layout.Clauses.Break = config.BreakAlways
		c.Layout.Content.Break = config.BreakAlways
		c.Layout.Content.CommaPosition = config.CommaStart
		c.Layout.Indent.Type = config.IndentTypeTab
		c.Layout.DDL.CreateView.Columns = config.BreakAuto
		c.Layout.DDL.CreateTrigger.Break = config.BreakAuto
		c.Layout.DDL.CreatePolicy.Break = config.BreakAuto
		c.Layout.DDL.Grant.Break = config.BreakAuto
		c.Layout.DDL.CreateType.Break = config.BreakAuto
		c.Layout.DDL.CreateSequence.Break = config.BreakAuto
	}},
	{"ddl_rest_always", func(c *config.Config) {
		c.Layout.DDL.AlterTable.Constraints = config.BreakAlways
		c.Layout.DDL.CreatePartition.Break = config.BreakAlways
		c.Layout.DDL.Drop.Break = config.BreakAlways
		c.Layout.DDL.CreateDomain.Break = config.BreakAlways
		c.Layout.DDL.CreateExtension.Break = config.BreakAlways
	}},
	{"ddl_rest_never", func(c *config.Config) {
		c.Layout.DDL.CreatePartition.Break = config.BreakNever
		c.Layout.DDL.Drop.Break = config.BreakNever
		c.Layout.DDL.CreateDomain.Break = config.BreakNever
		c.Layout.DDL.CreateExtension.Break = config.BreakNever
	}},
	{"ddl_rest_auto_narrow_tabs_comma_start", func(c *config.Config) {
		c.Layout.LineLength = 25
		c.Layout.Clauses.Break = config.BreakAlways
		c.Layout.Content.Break = config.BreakAlways
		c.Layout.Content.CommaPosition = config.CommaStart
		c.Layout.Indent.Type = config.IndentTypeTab
		c.Layout.DDL.AlterTable = config.AlterTableCfg{Break: config.BreakAlways, Constraints: config.BreakAuto}
		c.Layout.DDL.CreatePartition.Break = config.BreakAuto
		c.Layout.DDL.Drop.Break = config.BreakAuto
		c.Layout.DDL.CreateDomain.Break = config.BreakAuto
		c.Layout.DDL.CreateExtension.Break = config.BreakAuto
	}},
	{"blank_lines_max_1", func(c *config.Config) { c.BlankLines = config.BlankLinesMax1 }},
	{"comments_both", func(c *config.Config) {
		c.Comments = config.CommentsCfg{SpaceAfterMarker: config.CommentSpaceAdd, AlignTrailing: config.CommentAlignAlign}
	}},
	{"comments_with_layout", func(c *config.Config) {
		c.Comments = config.CommentsCfg{SpaceAfterMarker: config.CommentSpaceAdd, AlignTrailing: config.CommentAlignAlign}
		c.Layout.Clauses.Break = config.BreakAlways
		c.Layout.Content.Break = config.BreakAlways
		c.Layout.DDL.CreateTable = config.CreateTableCfg{Break: config.BreakAlways, Align: config.ColumnAlignAll}
	}},
	{"separation_0", func(c *config.Config) { c.BlankLinesBetweenStatements = "0" }},
	{"separation_2_trailing_newline", func(c *config.Config) {
		c.BlankLinesBetweenStatements = "2"
		c.TrailingNewline = config.TrailingNewlineAdd
	}},
	{"line_endings_crlf", func(c *config.Config) { c.LineEndings = config.LineEndingCRLF }},
	{"line_endings_lf_with_layout", func(c *config.Config) {
		c.LineEndings = config.LineEndingLF
		c.Layout.Clauses.Break = config.BreakAlways
	}},
	{"capitalize_all", func(c *config.Config) {
		for _, sec := range []*config.Section{
			&c.ReservedKeywords, &c.Keywords, &c.Literals, &c.Operators, &c.Schemas, &c.Tables,
			&c.Functions, &c.ConditionalFunctions, &c.SystemFunctions, &c.Columns,
		} {
			sec.Case = config.CaseCapitalize
		}
		c.DataTypes.Case = config.CaseCapitalize
		c.Aliases.Case = config.CaseCapitalize
	}},
	{"plpgsql_indent_all", func(c *config.Config) {
		pl := &c.Layout.DollarQuote.PLpgSQL
		pl.KeywordIndent = config.BodyIndentIndent
		pl.Declare.Indent = config.BodyIndentIndent
		pl.BeginBody.Indent = config.BodyIndentIndent
		pl.ControlFlow.If.BodyIndent = config.BodyIndentIndent
		pl.ControlFlow.Loop.BodyIndent = config.BodyIndentIndent
	}},
	{"plpgsql_blank_lines", func(c *config.Config) {
		pl := &c.Layout.DollarQuote.PLpgSQL
		pl.ControlFlow.If.BlankLineBefore = config.BlankLineAdd
		pl.ControlFlow.If.BlankLineAfter = config.BlankLineAdd
		pl.ControlFlow.Loop.BlankLineBefore = config.BlankLineAdd
		pl.ControlFlow.Loop.BlankLineAfter = config.BlankLineAdd
		pl.Declare.BlankLineBefore = config.BlankLineAdd
		pl.Declare.BlankLineAfter = config.BlankLineAdd
		pl.BeginBody.BlankLineBefore = config.BlankLineAdd
		pl.BeginBody.BlankLineAfter = config.BlankLineAdd
	}},
	{"plpgsql_case_expanded", func(c *config.Config) {
		pl := &c.Layout.DollarQuote.PLpgSQL.ControlFlow.Case
		for _, b := range []*config.CaseBranchCfg{&pl.Simple, &pl.Searched} {
			b.WhenIndent = config.BodyIndentIndent
			b.ThenBreak = config.BreakAlways
			b.ThenIndent = config.BodyIndentIndent
			b.BodyBreak = config.BreakAlways
			b.BodyIndent = config.BodyIndentIndent
			b.BlankLineBefore = config.BlankLineAdd
			b.BlankLineAfter = config.BlankLineAdd
		}
	}},
	{"plpgsql_end_semicolon_declare", func(c *config.Config) {
		pl := &c.Layout.DollarQuote.PLpgSQL
		pl.EndSemicolon = config.EndSemicolonAdd
		pl.DeclareEmpty = config.WhenEmptyAdd
	}},
	{"dollar_boundary", func(c *config.Config) {
		dq := &c.Layout.DollarQuote
		dq.NewlineAfterOpen = config.BlankLineAdd
		dq.NewlineBeforeClose = config.BlankLineAdd
		dq.SQL.BodyIndent = config.BodyIndentIndent
		dq.SQL.BlankLineBefore = config.BlankLineAdd
		dq.SQL.BlankLineAfter = config.BlankLineAdd
	}},
	{"dollar_plus_clauses_always", func(c *config.Config) {
		c.Layout.Clauses.Break = config.BreakAlways
		c.Layout.Content.Break = config.BreakAlways
		c.Layout.Case.Break = config.BreakAlways
		c.Layout.DollarQuote.NewlineAfterOpen = config.BlankLineAdd
		c.Layout.DollarQuote.NewlineBeforeClose = config.BlankLineAdd
		c.Layout.DollarQuote.PLpgSQL.KeywordIndent = config.BodyIndentIndent
		c.Layout.DollarQuote.PLpgSQL.BeginBody.Indent = config.BodyIndentIndent
		c.Layout.DollarQuote.PLpgSQL.ControlFlow.If.BodyIndent = config.BodyIndentIndent
	}},
	{"paren_indent", func(c *config.Config) {
		c.Layout.ParenIndent.Mode = config.ParenIndentIndent
		c.Layout.Clauses.Break = config.BreakAlways
		c.Layout.Content.Break = config.BreakAlways
	}},
	{"paren_indent_close_after", func(c *config.Config) {
		c.Layout.ParenIndent.Mode = config.ParenIndentIndent
		c.Layout.ParenIndent.CloseFirst = config.ParenCloseAfter
		c.Layout.Clauses.Break = config.BreakAlways
	}},
	{"indent_normalize_tabs", func(c *config.Config) {
		c.Layout.Indent.Type = config.IndentTypeTab
		c.Layout.Indent.Normalize = config.IndentNormalizeChange
		c.Layout.Clauses.Break = config.BreakAlways
	}},
	{"whitespace_and_blank_lines", func(c *config.Config) {
		c.TrailingWhitespace = config.TrailingWSPreserve
		c.BlankLines = config.BlankLinesMax1
	}},
	{"semicolons_add", func(c *config.Config) { c.Semicolons = config.SemicolonAdd }},
	{"quoted_preserve", func(c *config.Config) { c.QuotedIdents = config.QuotedIdentPreserve }},
	{"capitalize_with_layout", func(c *config.Config) {
		c.ReservedKeywords.Case = config.CaseCapitalize
		c.Keywords.Case = config.CaseCapitalize
		c.Tables.Case = config.CaseCapitalize
		c.Layout.Clauses.Break = config.BreakAlways
	}},
	{"keywords_lower", func(c *config.Config) {
		c.ReservedKeywords.Case = config.CaseLower
		c.Keywords.Case = config.CaseLower
	}},
}

// matrixInline are extra statements exercising constructs the golden inputs miss
// (comments, set operations, CASE, quoted identifiers, arithmetic).
var matrixInline = []string{
	"select coalesce(nullif(trim(both ' ' from first_name), ''), nullif(trim(last_name), ''), 'unknown_value_placeholder') as display_name, id from users where status in ('active', 'pending', 'suspended', 'archived', 'deleted', 'banned', 'review') and (select count(*) from t where a = 1 and b = 2) > 0;",
	"insert into t (a, b, c) values (1, 'a long literal value number one', now()), (2, 'a long literal value number two', now());",
	"create or replace function public.add_user(p_name text, p_age int default 18, variadic tags text[] default '{}') returns table(id bigint, name text) language plpgsql security definer set search_path = public, pg_temp stable parallel safe cost 100 as $$ begin return query select 1, 'a'; end; $$;",
	"create function f(name text, status int, text) returns int language sql immutable as 'select 1'; create function g() returns trigger as $$ begin return new; end $$ language plpgsql;",
	"create unique index concurrently if not exists idx on public.users using btree (lower(email), created_at desc) include (name) where deleted_at is null and active;",
	"create index on t (a); create index i2 on only t using gin (doc jsonb_path_ops) with (fastupdate = off) tablespace fast where a > 1;",
	"create materialized view mv as select a, b from t where x > 1 with no data; create view v as select a from t with local check option;",
	"create view v (a, b, c) with (security_barrier) as select 1, 2, 3; create or replace recursive view r (n) as values (1);",
	"create trigger trg before insert or update of a, b on public.t for each row when (old.* is distinct from new.*) execute function f(1, 'x'); create event trigger et on ddl_command_start when tag in ('create table') execute function g();",
	"create constraint trigger ct after insert on t from r deferrable initially deferred for each row execute procedure f(); create trigger t2 after update on t referencing old table as o new table as n for each statement execute function f();",
	"create policy p on t as permissive for select to r1, r2 using (a = 1) with check (b = 2); create policy q on s for all using (exists (select 1 from t where t.id = s.id));",
	"grant select, insert (a, b) on table t1, t2 to r1, r2 with grant option; grant r1 to r2 with admin option granted by r3; revoke grant option for select on t from public cascade; revoke all on all tables in schema s from r;",
	"create type mood as enum ('sad', 'ok', 'happy'); create type pt as (x int, y int); create type r as range (subtype = int4); create type base (input = i, output = o);",
	"create sequence if not exists s as bigint increment by 2 minvalue 1 no maxvalue start with 5 cache 10 no cycle owned by t.id; alter sequence s restart with 1 cycle; alter sequence s owner to r;",
	"alter table t add constraint c foreign key (a, b) references u (a, b) match full on delete cascade on update set null deferrable initially deferred not valid; alter table t add primary key (id), add constraint u1 unique (x), drop column z;",
	"create table c1 partition of p for values in (1, 2, 3); create table c2 partition of p for values from (1) to (10) partition by range (x) tablespace ts; create table c3 partition of p for values with (modulus 4, remainder 0); create table c4 partition of p default;",
	"drop table if exists a, b, c cascade; drop table a; drop index concurrently if exists i1, i2; drop materialized view mv restrict; drop function f(int);",
	"create domain d as int default 1 not null constraint pos check (value > 0) check (value < 100); create domain e as text collate \"C\" null;",
	"create extension if not exists pgcrypto with schema public version '1.3' cascade; create extension hstore;",
	"insert into t (a, b) values (1, 2) on conflict (a) do update set b = excluded.b where t.b < 3 returning a; insert into t (a) values (1) on conflict on constraint k do nothing;",
	"select 'a\nb  c' as x, 2 from t where y = 'p\n  q' and z in ('m\n    n', 'o') order by 1;",
	"select e'line1\nline2' as a, 'x\n\ty' || b as c from (select 'in\n   side' as b) s where a like '%\n%';",
	"insert into t (a, b) values ('multi\nline  text', 1), ('other\n   row', 2) on conflict (a) do update set b = 'u\n v';",
	"create table t (a text default 'one\n  two', b int check (a <> 'x\n y'), c text);",
	"update t set a = 'a\n   b', c = case when d = 'q\n r' then 's\n  t' else 'u' end where e = 'w\n x';",
	"with c as (select 'cte\n   text' as v) select v, 'out\n  er' as w from c join d on d.k = 'j\n   k';",
	"select coalesce(nullif('long\n   literal', ''), 'x') as a, concat('a\n  b', 'c\n d') as b, 1 from t group by 1 having count(*) > 'h\n i'::int;",
	"select 'trailing   \n\n\tTabbed \nlast' as a, 'x' from t where b = 'u  \n  v  ';",
	"select 'a --not a comment\n/* nor this */ b' as x -- real comment\nfrom t;",
	"select 'p\n\n\n\nq' as a;\n\n\n\n\nselect 2 as b;",
	"create function f() returns text language plpgsql as $$ begin return 'x\n\n\n\ny'; end $$;",
	"with data as (select 1) select * from data;",
	"select a, -- first\n b -- second\n from t where x = 1 -- tail\n and y = 2;",
	"select a from u union select b from v union all select 1;",
	"select a from u where x = 1 union all select 1 order by 1;",
	`select "Id", "My Col" as "X" from "User" u where u."Id" = 1;`,
	"select case when a > 1 then 'x' when a < 0 then 'y' else 'z' end, case b when 1 then 2 end from t;",
	"select a+1, b - 2, c*d, -1, count(*), t.*, a - -1, (a+b)*(c-d) from t where n*2 > 1;",
	"insert into t (a, -- x\n b) values (1, 2) on conflict (a) do update set b = excluded.b returning *;",
	"update t set a = 1, b = b + 1 from u where t.id = u.id and t.x is not null returning t.id;",
	"with recursive r(n) as (select 1 union all select n+1 from r where n < 5) select * from r;",
	"select * from t, lateral (select 1) x join (select 2) y on true left join (select 3) as z on true;",
	"select sum(x) over (partition by a order by b rows between unbounded preceding and current row) as s from t window w as (order by c);",
	"create table if not exists public.t (id serial primary key, name varchar(20) not null default 'x', constraint fk foreign key (id) references o(id) on delete cascade);",
	"create trigger tr before insert or update on t for each row execute function f(); drop trigger tr on t;",
	"select date_trunc('month', o.created_at) as month, sum(x) over (order by y) as over from t o;",
	"select a from t where a in (select b from u) and exists (select 1 from v where v.x = t.a);",
	"create view v as select id, name from t where a = 1; create table x as select a from t; explain select a from t where b = 1;",
	"alter table t alter column a set not null, alter column b set default 1; alter table t set schema s; alter role bob set search_path = x; set search_path = public;",
	"create table x (a int references o(id) on delete cascade on update cascade) with (fillfactor = 70); alter table t alter column a type int using a::int;",
	"select * from t where a = 1 for update; create trigger tr before insert or update on t for each row execute function f();",
	"insert into t values (1) on conflict (id) do update set a = 2; delete from t using u where t.id = u.id;",
	"merge into public.Accounts as tgt using (select id, amount from staging where ok) as src on tgt.id = src.id and tgt.active when matched and src.amount > 0 then update set balance = tgt.balance + src.amount, updated = now() when matched then delete when not matched then insert (id, balance) values (src.id, case when src.amount > 0 then src.amount else 0 end) when not matched by source then do nothing;",
	"merge into t using u on t.id = u.id when matched then update set a = case when u.x then 1 else 2 end -- tail\n when not matched then insert values (u.id, 1);",
	`create function f(p int) returns int language plpgsql as $$
declare
v int := 1;
begin
if p > 1 then
v := v + 1;
elsif p = 0 then
raise notice 'x %', p;
else
perform now();
end if;
return v;
exception when others then
return 0;
end $$;`,
	`create function g(n int) returns setof int language plpgsql as $$
begin
for i in 1..n loop
return next i;
end loop;
while n > 0 loop
n := n - 1;
exit when n = 2;
end loop;
return;
end $$;`,
	`create function h(k text) returns text language plpgsql as $$
declare
r text;
begin
case k
when 'a' then
r := 'x';
when 'b' then
r := 'y';
else
r := 'z';
end case;
case
when k = 'q' then
r := 'w';
else
null;
end case;
return r;
end $$;`,
	`create function n() returns void language plpgsql as $$
begin
begin
insert into t values (1);
exception when unique_violation then
update t set a = 2 where id = 1;
end;
if exists (select 1 from t where a = 1) then
delete from t where a = 1;
end if;
end $$;`,
	`create function q() returns table (id int, v text) language plpgsql as $$
begin
return query select a.id, a.v from t a where a.id > 1 order by 1;
end $$;`,
	`create function c() returns int language plpgsql as $$
begin -- leading comment
if true then -- inside
return 1; /* block */
end if;
return 0;
end $$;`,
	`do $$
begin
perform 1;
if found then
raise exception 'boom';
end if;
end $$;`,
	"create function s() returns int language sql as $$ select a, b from t where x = 1 order by a $$;",
	"select a -- first col\n, b /* second */ , c\nfrom t -- the table\nwhere x = 1 -- cond one\n and y = 2 /* cond two */\n and z = 3 -- cond three\norder by a -- sort\n, b;",
	"select /* lead */ a, b from t /* before where */ where /* inside */ x = 1;",
	"select a,\n -- comment between items\n b,\n /* block between */\n c from t;",
	"select case -- c1\n when a > 1 then 'x' -- c2\n when a < 0 then 'y' /* c3 */\n else 'z' -- c4\n end as k from t;",
	"select a from t -- c1\nunion -- c2\nselect b from u /* c3 */;",
	"select a from t join u -- join comment\n on t.id = u.id -- on comment\n and t.x = u.x -- more\n where t.k = 1;",
	"insert into t (a, b) -- cols\nvalues (1, 2), -- row one\n (3, 4) -- row two\n;",
	"update t set a = 1, -- first\n b = 2 -- second\nwhere id = 3 -- key\n;",
	"with x as ( -- cte comment\n select 1 as a /* inline */ ) -- after cte\nselect * from x;",
	"-- leading comment\nselect 1; /* between statements */ select 2; -- trailing",
	"select a, /* c */ b from t where x in (1, -- one\n 2, /* two */ 3);",
	"select a from t where -- why\n x = 1 and -- more why\n y = 2;",
	"select a --\n, b from t;",
	"insert into t values (1, 2) -- one\n, (3, 4) -- two\n, (5, 6);",
	"select a from t where x = 1 -- one\n or y = 2 -- two\n or (z = 3 -- three\n and w = 4);",
	"select a, b from t -- tail one\n-- tail two\n;",
	"select coalesce(a, -- fallback\n b) as c, f(1, /* two */ 2) from t group by a -- g\n, b having count(*) > 1 -- h\n;",
	"select a from t order by a desc -- d\n limit 5 -- l\n offset 1 -- o\n;",
	"select a from t left join u on t.id = u.id -- lj\n left join v on v.id = u.id -- lj2\n;",
	"select (select max(x) -- inner\n from u) as m from t;",
	"select a from (select 1 as a -- inner\n) s;",
	"select a from t where x between 1 and 2 and y = 3 or z not between 4 and 5;",
	"select 1 from a join b on a.id = b.id and a.x between 1 and 2 join (select 1 as id from c join d on c.id = d.id) e on e.id = a.id and e.id > 0 left join f using (id) cross join g;",
	"select a, b, c from t where a = 1 and (b = 2 or c = 3) and d in (select 1 from u where u.x = 1 and u.y = 2) order by a, b;",
	"select a from t join u on t.id = u.id -- on1\n and t.x = u.x -- on2\n or t.y = u.y -- on3\n where a = 1 -- w1\n and b = 2 -- w2\n;",
	"select a -- c1\n, b -- c2\n, c from t group by a -- g1\n, b -- g2\n, c;",
	"insert into t (a, b, c) values (1, 2, 3), (4, 5, 6) returning a, b;",
	"update t set a = 1, b = 2 where c = 3 and d = 4 returning a, b;",
	"with x as (select 1 as a), y as (select 2 as b) select * from x, y;",
	"create table t (id int primary key, name varchar(20) not null default 'x', note text);",
	"create table if not exists public.accounts (id bigint not null generated always as identity, code character varying(64) not null default 'x', balance numeric(18, 4) not null default 0, created_at timestamp with time zone not null default now(), constraint pk primary key (id), constraint fk foreign key (code) references public.codes (code) on delete cascade) with (fillfactor = 70);",
	"create temp table if not exists tmp (a int, -- first\n b text /* inline */, c int[] default '{}', \"Mixed Case\" boolean not null, check (a > 0 and\n b <> ''));",
	"create unlogged table u (like t including all, extra int);\ncreate table p (id int, d date) partition by range (d);\ncreate table p1 partition of p for values from ('2020-01-01') to ('2021-01-01');",
	"create table x (a int, b int)",
	"alter table public.accounts add column notes text, drop column old_col, alter column code set not null, add constraint c check (a > 0 and b < 5);",
	"alter table only t alter column a type numeric(10, 2) using a::numeric, alter column b set default 1; alter table if exists t rename column a to b; alter table t rename to u; alter table all in tablespace x set tablespace y;",
	"alter table t add column a int, -- one\n add column b int /* two */",
	"-- lead\ncreate table c1 (a int); /* between */ create table c2 (b int, c int);",
	"select a, --first\n  bb, -- second\n  ccc --third\nfrom t; -- tail\n-- standalone\n--- banner ---\nselect 1;   -- one\nselect 2;     -- two\nselect 3;",
	"select 1;\n\n\n\nselect 2; -- c\n-- lead\nselect 3;select 4;\n\n-- end\n",
	"select a,\r\n  b -- c\r\nfrom t\r\nwhere x = 1;\r\n\r\nselect 2;\r\n",
	"create function f() returns int language plpgsql as $$\nbegin\n  return 1; --one\n  -- two\nend $$;\nselect 1;",
	"select f(a => 1, b := 2), x::int, cast(y as text), 'a' || 'b' from t where a != b and c <> d;",
}

func matrixInputs(t *testing.T) map[string]string {
	t.Helper()
	in := map[string]string{}
	files, _ := filepath.Glob("testdata/*/input.sql")
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		in[filepath.Base(filepath.Dir(f))] = string(b)
	}
	for i, s := range matrixInline {
		in[fmt.Sprintf("inline_%02d", i)] = s
	}
	return in
}

// TestConfigMatrix formats every input under every variant and checks that the
// result (1) parses when the input parses, (2) has the same AST fingerprint as
// the input formatted with the default (layout-preserving) config, and (3) is
// idempotent. It catches glued tokens, swallowed commas and similar breakage.
func TestConfigMatrix(t *testing.T) {
	inputs := matrixInputs(t)
	for name, src := range inputs {
		if _, err := pg_query.Parse(src); err != nil {
			continue // PL/pgSQL-only or partial snippets: semantic checks need a parseable input
		}
		base, err := loadStyled("")
		if err != nil {
			t.Fatal(err)
		}
		baseOut, err := formatter.Format(src, base)
		if err != nil {
			t.Fatalf("%s baseline: %v", name, err)
		}
		baseFP, err := pg_query.Fingerprint(baseOut)
		if err != nil {
			t.Fatalf("%s baseline output does not parse: %v\n%s", name, err, baseOut)
		}
		baseShape := plShape(baseOut)
		for _, v := range matrixVariants {
			t.Run(name+"/"+v.name, func(t *testing.T) {
				cfg, _ := loadStyled("")
				v.apply(cfg)
				out, err := formatter.Format(src, cfg)
				if err != nil {
					t.Fatalf("Format: %v", err)
				}
				fp, err := pg_query.Fingerprint(out)
				if err != nil {
					t.Fatalf("output does not parse: %v\n--- output ---\n%s", err, out)
				}
				if fp != baseFP {
					t.Errorf("AST fingerprint changed\n--- input ---\n%s\n--- output ---\n%s", src, out)
				}
				if got, want := stringLiterals(out), stringLiterals(src); !reflect.DeepEqual(got, want) {
					t.Errorf("string literals changed (AST fingerprints ignore constant values)\n--- input ---\n%s\n--- output ---\n%s\n--- literals before ---\n%q\n--- after ---\n%q", src, out, want, got)
				}
				if shape := plShape(out); baseShape != "" && shape != baseShape { // an unparseable baseline (e.g. `;;` repaired by layout) has nothing to compare
					t.Errorf("PL/pgSQL structure changed\n--- input ---\n%s\n--- output ---\n%s", src, out)
				}
				out2, err := formatter.Format(out, cfg)
				if err != nil {
					t.Fatalf("second pass: %v\n--- first output ---\n%s", err, out)
				}
				if out2 != out {
					t.Errorf("not idempotent\n--- first ---\n%s\n--- second ---\n%s", out, out2)
				}
			})
		}
	}
}

// stringLiterals returns the sorted '…' string constants of sql (dollar-quoted bodies are
// formatted on purpose and left out). Layout and casing rules must never touch their text.
func stringLiterals(sql string) []string {
	res, err := pg_query.Scan(sql)
	if err != nil {
		return nil
	}
	var out []string
	for _, t := range res.Tokens {
		text := sql[t.Start:t.End]
		if strings.HasPrefix(text, "$") {
			continue
		}
		if i := strings.IndexByte(text, '\''); i >= 0 && i <= 2 && strings.TrimLeft(text[:i], "eEbBxXuU&nN") == "" {
			out = append(out, strings.ReplaceAll(text, "\r\n", "\n")) // line_endings: crlf rewrites line breaks on purpose
		}
	}
	sort.Strings(out)
	return out
}

var wsRE = regexp.MustCompile(`\s+`)

// plShape returns a normalised structural description of the PL/pgSQL functions in
// sql: statement structure and expressions with line numbers dropped and embedded SQL
// replaced by its AST fingerprint, so casing/spacing/layout differences vanish but any
// change in control flow or statements shows up. Empty when sql has no PL/pgSQL function.
func plShape(sql string) string {
	raw, err := pg_query.ParsePlPgSqlToJSON(sql)
	if err != nil || strings.TrimSpace(raw) == "" {
		return ""
	}
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return ""
	}
	var norm func(any) any
	norm = func(x any) any {
		switch t := x.(type) {
		case map[string]any:
			out := map[string]any{}
			for k, val := range t {
				switch k {
				case "lineno", "stmtid", "location":
					continue
				case "query":
					if q, ok := val.(string); ok {
						out[k] = normQuery(q)
						continue
					}
				}
				out[k] = norm(val)
			}
			return out
		case string:
			// identifiers and type names are case-insensitive and carry stray padding
			return strings.ToLower(strings.TrimSpace(t))
		case []any:
			out := make([]any, len(t))
			for i, e := range t {
				out[i] = norm(e)
			}
			return out
		}
		return x
	}
	b, _ := json.Marshal(norm(v))
	return string(b)
}

// normQuery fingerprints an embedded query/expression/assignment, falling back to
// lower-cased whitespace-free text when it cannot be parsed standalone.
func normQuery(q string) string {
	cands := []string{q, "SELECT " + q}
	if i := strings.Index(q, ":="); i >= 0 {
		cands = append(cands, "SELECT "+q[i+2:])
	}
	for _, c := range cands {
		if fp, err := pg_query.Fingerprint(c); err == nil {
			return "fp:" + fp
		}
	}
	return "txt:" + strings.ToLower(wsRE.ReplaceAllString(q, ""))
}
