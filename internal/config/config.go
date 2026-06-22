package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type CaseRule string

const (
	CaseUpper    CaseRule = "upper"
	CaseLower    CaseRule = "lower"
	CasePreserve CaseRule = "preserve"
)

// TypeForm controls canonical form of data type names.
type TypeForm string

const (
	// TypeFormPreserve leaves the type name unchanged.
	TypeFormPreserve TypeForm = "preserve"
	// TypeFormLong expands to the full SQL standard name: "character varying", "double precision", …
	TypeFormLong TypeForm = "long"
	// TypeFormShort collapses to the shortest alias: "varchar", "float8", "int", …
	TypeFormShort TypeForm = "short"
	// TypeFormLongNoSpace uses the long single-word form where possible, short where the long form
	// would contain a space: "integer", "bigint", "varchar" (not "character varying"), "timestamptz".
	TypeFormLongNoSpace TypeForm = "long_no_space"
)

// AliasAs controls the AS keyword before column/expression aliases.
type AliasAs string

const (
	AliasAsAdd      AliasAs = "add"      // always insert AS when missing
	AliasAsPreserve AliasAs = "preserve" // leave as-is
	AliasAsRemove   AliasAs = "remove"   // always remove AS
)

// TrailingWS controls trailing whitespace at the end of lines.
type TrailingWS string

const (
	TrailingWSStrip    TrailingWS = "strip"
	TrailingWSPreserve TrailingWS = "preserve"
)

// SemicolonMode controls trailing semicolons on statements.
type SemicolonMode string

const (
	SemicolonPreserve SemicolonMode = "preserve"
	SemicolonAdd      SemicolonMode = "add"
	SemicolonRemove   SemicolonMode = "remove"
)

// InequalityOp normalises the inequality operator.
type InequalityOp string

const (
	InequalityPreserve InequalityOp = "preserve"
	InequalityANSI     InequalityOp = "ansi" // <>
	InequalityC        InequalityOp = "c"    // !=
)

// JoinForm controls whether JOIN keywords are written in short or long form.
type JoinForm string

const (
	JoinFormPreserve JoinForm = "preserve"
	JoinFormShort    JoinForm = "short" // JOIN, LEFT JOIN, RIGHT JOIN, FULL JOIN
	JoinFormLong     JoinForm = "long"  // INNER JOIN, LEFT OUTER JOIN, RIGHT OUTER JOIN, FULL OUTER JOIN
)

// OperatorSpacingMode controls spaces around symbolic binary operators.
type OperatorSpacingMode string

const (
	OperatorSpacingPreserve  OperatorSpacingMode = "preserve"
	OperatorSpacingNormalize OperatorSpacingMode = "normalize" // exactly one space on each side
)

// BlankLinesMode limits consecutive blank lines between statements.
type BlankLinesMode string

const (
	BlankLinesPreserve BlankLinesMode = "preserve"
	BlankLinesMax3     BlankLinesMode = "max_3"
	BlankLinesMax2     BlankLinesMode = "max_2"
	BlankLinesMax1     BlankLinesMode = "max_1"
)

// ParenSpacingMode controls spaces immediately inside parentheses.
type ParenSpacingMode string

const (
	ParenSpacingPreserve ParenSpacingMode = "preserve"
	ParenSpacingAdd      ParenSpacingMode = "add"
	ParenSpacingRemove   ParenSpacingMode = "remove"
)

// QuotedIdentMode controls removal of unnecessary double-quote wrappers.
type QuotedIdentMode string

const (
	QuotedIdentPreserve   QuotedIdentMode = "preserve"
	QuotedIdentRemoveSafe QuotedIdentMode = "remove_safe" // remove when safe (no spaces, all lowercase, not a reserved keyword)
)

// TrailingNewlineMode controls the trailing newline at end of output.
type TrailingNewlineMode string

const (
	TrailingNewlinePreserve TrailingNewlineMode = "preserve"
	TrailingNewlineAdd      TrailingNewlineMode = "add"
	TrailingNewlineStrip    TrailingNewlineMode = "strip"
)

// CommaSpacingMode controls spaces around commas.
type CommaSpacingMode string

const (
	CommaSpacingPreserve  CommaSpacingMode = "preserve"
	CommaSpacingNormalize CommaSpacingMode = "normalize"
)

// OrderAscMode controls explicit ASC in ORDER BY clauses.
type OrderAscMode string

const (
	OrderAscPreserve OrderAscMode = "preserve"
	OrderAscAdd      OrderAscMode = "add"
	OrderAscRemove   OrderAscMode = "remove"
)

// CastStyleMode controls CAST(x AS t) vs x::t form.
type CastStyleMode string

const (
	CastStylePreserve CastStyleMode = "preserve"
	CastStyleOperator CastStyleMode = "operator"
)

// NotInMode controls NOT IN vs <> ALL form.
type NotInMode string

const (
	NotInPreserve     NotInMode = "preserve"
	NotInNotIn        NotInMode = "not_in"
	NotInNotEqualsAll NotInMode = "not_equals_all"
)

// SchemaQualMode controls schema qualification of object names.
type SchemaQualMode string

const (
	SchemaQualPreserve     SchemaQualMode = "preserve"
	SchemaQualRemovePublic SchemaQualMode = "remove_public" // remove "public." prefix
)

type BreakMode string

const (
	BreakPreserve BreakMode = "preserve"
	BreakNever    BreakMode = "never"
	BreakAlways   BreakMode = "always"
	BreakAuto     BreakMode = "auto"
)

type AlignMode string

const (
	AlignSame   AlignMode = "same"
	AlignIndent AlignMode = "indent"
)

type IndentTypeVal string

const (
	IndentTypeSpaces IndentTypeVal = "spaces"
	IndentTypeTab    IndentTypeVal = "tab"
)

type IndentNormalizeVal string

const (
	IndentNormalizePreserve IndentNormalizeVal = "preserve"
	IndentNormalizeChange   IndentNormalizeVal = "change"
)

type IndentRemainderVal string

const (
	IndentRemainderKeep   IndentRemainderVal = "keep"
	IndentRemainderAdd    IndentRemainderVal = "add"
	IndentRemainderRemove IndentRemainderVal = "remove"
	IndentRemainderRound  IndentRemainderVal = "round"
)

type UnionBlankLine string

const (
	UnionBlankLinePreserve UnionBlankLine = "preserve"
	UnionBlankLineNone     UnionBlankLine = "none"
	UnionBlankLineBefore   UnionBlankLine = "before"
	UnionBlankLineAfter    UnionBlankLine = "after"
	UnionBlankLineBoth     UnionBlankLine = "both"
)

type ClauseRule struct {
	Break BreakMode `yaml:"break"`
	Align AlignMode `yaml:"align"`
}

type ContentRule struct {
	Break BreakMode `yaml:"break"`
	Align AlignMode `yaml:"align"`
}

type IndentCfg struct {
	Size      int                `yaml:"size"`
	Type      IndentTypeVal      `yaml:"type"`
	Normalize IndentNormalizeVal `yaml:"normalize"`
	Remainder IndentRemainderVal `yaml:"remainder"`
}

type ClausesCfg struct {
	Break      BreakMode  `yaml:"break"`
	Align      AlignMode  `yaml:"align"`
	Into       ClauseRule `yaml:"into"`
	From       ClauseRule `yaml:"from"`
	Join       ClauseRule `yaml:"join"`
	Where      ClauseRule `yaml:"where"`
	GroupBy    ClauseRule `yaml:"group_by"`
	Having     ClauseRule `yaml:"having"`
	OrderBy    ClauseRule `yaml:"order_by"`
	Limit      ClauseRule `yaml:"limit"`
	Offset     ClauseRule `yaml:"offset"`
	Values     ClauseRule `yaml:"values"`
	OnConflict ClauseRule `yaml:"on_conflict"`
	Set        ClauseRule `yaml:"set"`
	Using      ClauseRule `yaml:"using"`
	Returning  ClauseRule `yaml:"returning"`
	With       ClauseRule `yaml:"with"`
	Exception  ClauseRule `yaml:"exception"`
}

type ContentCfg struct {
	Break      BreakMode   `yaml:"break"`
	Align      AlignMode   `yaml:"align"`
	SelectList ContentRule `yaml:"select_list"`
	WhereConds ContentRule `yaml:"where_conds"`
	JoinOn     ContentRule `yaml:"join_on"`
	GroupList  ContentRule `yaml:"group_list"`
	OrderList  ContentRule `yaml:"order_list"`
	SetList    ContentRule `yaml:"set_list"`
	ValuesList ContentRule `yaml:"values_list"`
}

type UnionCfg struct {
	BlankLine UnionBlankLine `yaml:"blank_line"`
}

// BlankLineAction controls whether a blank line is added, removed, or preserved.
type BlankLineAction string

const (
	BlankLinePreserve BlankLineAction = "preserve"
	BlankLineAdd      BlankLineAction = "add"
	BlankLineRemove   BlankLineAction = "remove"
)

// BodyIndentMode controls indentation of content lines inside a dollar-quoted body.
type BodyIndentMode string

const (
	BodyIndentPreserve BodyIndentMode = "preserve" // keep as-is
	BodyIndentNone     BodyIndentMode = "none"     // remove leading indent (start at column 0)
	BodyIndentIndent   BodyIndentMode = "indent"   // add one indent level
)

// WhenEmptyMode controls whether an optional block (e.g. DECLARE) is kept when empty.
type WhenEmptyMode string

const (
	WhenEmptyPreserve WhenEmptyMode = "preserve"
	WhenEmptyAdd      WhenEmptyMode = "add"    // always emit even if empty
	WhenEmptyRemove   WhenEmptyMode = "remove" // remove if empty
)

// EndSemicolonMode controls whether END has a trailing semicolon.
type EndSemicolonMode string

const (
	EndSemicolonPreserve EndSemicolonMode = "preserve"
	EndSemicolonAdd      EndSemicolonMode = "add"
	EndSemicolonRemove   EndSemicolonMode = "remove"
)

type ControlFlowItemCfg struct {
	BodyIndent      BodyIndentMode  `yaml:"body_indent"`
	BlankLineBefore BlankLineAction `yaml:"blank_line_before"`
	BlankLineAfter  BlankLineAction `yaml:"blank_line_after"`
}

// CaseBranchCfg configures one form of PL/pgSQL CASE statement.
// Simple form:   CASE expr WHEN val  THEN ...
// Searched form: CASE       WHEN cond THEN ...
type CaseBranchCfg struct {
	WhenIndent      BodyIndentMode  `yaml:"when_indent"`       // none | indent  — WHEN/ELSE vs CASE level
	ThenBreak       BreakMode       `yaml:"then_break"`        // preserve | never | always | auto
	ThenIndent      BodyIndentMode  `yaml:"then_indent"`       // none | indent  — THEN indent when broken
	BodyBreak       BreakMode       `yaml:"body_break"`        // preserve | never | always | auto
	BodyIndent      BodyIndentMode  `yaml:"body_indent"`       // none | indent  — body indent when on new line
	BlankLineBefore BlankLineAction `yaml:"blank_line_before"` // blank line before WHEN/ELSE/END CASE
	BlankLineAfter  BlankLineAction `yaml:"blank_line_after"`  // blank line after WHEN THEN / ELSE (unused when body on same line)
}

type CaseCfg struct {
	Simple   CaseBranchCfg `yaml:"simple"`   // CASE expr WHEN val THEN ...
	Searched CaseBranchCfg `yaml:"searched"` // CASE WHEN cond THEN ...
}

type ControlFlowCfg struct {
	If   ControlFlowItemCfg `yaml:"if"`
	Loop ControlFlowItemCfg `yaml:"loop"`
	Case CaseCfg            `yaml:"case"`
}

type SQLDollarQuoteConfig struct {
	BodyIndent      BodyIndentMode  `yaml:"body_indent"`
	BlankLineBefore BlankLineAction `yaml:"blank_line_before"` // blank line after opening $$
	BlankLineAfter  BlankLineAction `yaml:"blank_line_after"`  // blank line before closing $$
}

type PLpgSQLSectionCfg struct {
	Indent          BodyIndentMode  `yaml:"indent"`
	BlankLineBefore BlankLineAction `yaml:"blank_line_before"`
	BlankLineAfter  BlankLineAction `yaml:"blank_line_after"`
}

type PLpgSQLDollarQuoteConfig struct {
	KeywordIndent BodyIndentMode    `yaml:"keyword_indent"`     // indent of DECLARE/BEGIN/END keywords
	Declare       PLpgSQLSectionCfg `yaml:"declare"`            // content between DECLARE and BEGIN
	DeclareEmpty  WhenEmptyMode     `yaml:"declare_when_empty"` // keep DECLARE when section is empty
	BeginBody     PLpgSQLSectionCfg `yaml:"begin_body"`         // content between BEGIN and EXCEPTION/END
	EndSemicolon  EndSemicolonMode  `yaml:"end_semicolon"`
	ControlFlow   ControlFlowCfg    `yaml:"control_flow"`
}

type DollarQuoteCfg struct {
	NewlineAfterOpen   BlankLineAction          `yaml:"newline_after_open"`   // \n after opening $$
	NewlineBeforeClose BlankLineAction          `yaml:"newline_before_close"` // \n before closing $$
	SQL                SQLDollarQuoteConfig     `yaml:"sql"`
	PLpgSQL            PLpgSQLDollarQuoteConfig `yaml:"plpgsql"`
}

// SQLCaseCfg controls formatting of SQL CASE expressions (SELECT, WHERE, etc.).
// This is distinct from PL/pgSQL CASE statements (layout.dollar_quote.plpgsql.control_flow.case).
type SQLCaseCfg struct {
	Break  BreakMode      `yaml:"break"`  // preserve | never | always | auto
	Indent BodyIndentMode `yaml:"indent"` // preserve | none | indent  (depth of WHEN/ELSE relative to CASE)
}

type LayoutConfig struct {
	LineLength  int            `yaml:"line_length"`
	Indent      IndentCfg      `yaml:"indent"`
	Clauses     ClausesCfg     `yaml:"clauses"`
	Union       UnionCfg       `yaml:"union"`
	Content     ContentCfg     `yaml:"content"`
	Case        SQLCaseCfg     `yaml:"case"`
	DollarQuote DollarQuoteCfg `yaml:"dollar_quote"`
}

// DefaultLayout returns the default LayoutConfig (all fields set to preserve).
func DefaultLayout() LayoutConfig { return defaultLayout() }

func defaultLayout() LayoutConfig {
	return LayoutConfig{
		LineLength: 128,
		Indent: IndentCfg{
			Size:      3,
			Type:      IndentTypeSpaces,
			Normalize: IndentNormalizePreserve,
			Remainder: IndentRemainderKeep,
		},
		Clauses: ClausesCfg{
			Break: BreakPreserve,
			Align: AlignSame,
		},
		Union: UnionCfg{BlankLine: UnionBlankLinePreserve},
		Content: ContentCfg{
			Break: BreakPreserve,
			Align: AlignIndent,
		},
		Case: SQLCaseCfg{
			Break:  BreakPreserve,
			Indent: BodyIndentIndent,
		},
		DollarQuote: DollarQuoteCfg{
			NewlineAfterOpen:   BlankLinePreserve,
			NewlineBeforeClose: BlankLinePreserve,
			SQL: SQLDollarQuoteConfig{
				BodyIndent:      BodyIndentPreserve,
				BlankLineBefore: BlankLinePreserve,
				BlankLineAfter:  BlankLinePreserve,
			},
			PLpgSQL: PLpgSQLDollarQuoteConfig{
				KeywordIndent: BodyIndentPreserve,
				Declare: PLpgSQLSectionCfg{
					Indent:          BodyIndentPreserve,
					BlankLineBefore: BlankLinePreserve,
					BlankLineAfter:  BlankLinePreserve,
				},
				DeclareEmpty: WhenEmptyPreserve,
				BeginBody: PLpgSQLSectionCfg{
					Indent:          BodyIndentPreserve,
					BlankLineBefore: BlankLinePreserve,
					BlankLineAfter:  BlankLinePreserve,
				},
				EndSemicolon: EndSemicolonPreserve,
				ControlFlow: ControlFlowCfg{
					If:   ControlFlowItemCfg{BodyIndent: BodyIndentPreserve, BlankLineBefore: BlankLinePreserve, BlankLineAfter: BlankLinePreserve},
					Loop: ControlFlowItemCfg{BodyIndent: BodyIndentPreserve, BlankLineBefore: BlankLinePreserve, BlankLineAfter: BlankLinePreserve},
					Case: CaseCfg{
						Simple: CaseBranchCfg{
							WhenIndent: BodyIndentPreserve, ThenBreak: BreakPreserve, ThenIndent: BodyIndentPreserve,
							BodyBreak: BreakPreserve, BodyIndent: BodyIndentPreserve,
							BlankLineBefore: BlankLinePreserve, BlankLineAfter: BlankLinePreserve,
						},
						Searched: CaseBranchCfg{
							WhenIndent: BodyIndentPreserve, ThenBreak: BreakPreserve, ThenIndent: BodyIndentPreserve,
							BodyBreak: BreakPreserve, BodyIndent: BodyIndentPreserve,
							BlankLineBefore: BlankLinePreserve, BlankLineAfter: BlankLinePreserve,
						},
					},
				},
			},
		},
	}
}

type Section struct {
	Case CaseRule `yaml:"case"`
}

type DataTypesSection struct {
	Case CaseRule `yaml:"case"`
	Form TypeForm `yaml:"form"`
}

type AliasSection struct {
	Case CaseRule `yaml:"case"`
	As   AliasAs  `yaml:"as"`
}

type Config struct {
	ReservedKeywords     Section             `yaml:"reserved_keywords"`
	Keywords             Section             `yaml:"keywords"`
	DataTypes            DataTypesSection    `yaml:"data_types"`
	Literals             Section             `yaml:"literals"`
	Operators            Section             `yaml:"operators"`
	Schemas              Section             `yaml:"schemas"`
	Tables               Section             `yaml:"tables"`
	Functions            Section             `yaml:"functions"`
	ConditionalFunctions Section             `yaml:"conditional_functions"`
	SystemFunctions      Section             `yaml:"system_functions"`
	Aliases              AliasSection        `yaml:"aliases"`
	Columns              Section             `yaml:"columns"`
	TrailingWhitespace   TrailingWS          `yaml:"trailing_whitespace"`
	Semicolons           SemicolonMode       `yaml:"semicolons"`
	InequalityOp         InequalityOp        `yaml:"inequality_op"`
	JoinForm             JoinForm            `yaml:"join_form"`
	OperatorSpacing      OperatorSpacingMode `yaml:"operator_spacing"`
	BlankLines           BlankLinesMode      `yaml:"blank_lines"`
	ParenSpacing         ParenSpacingMode    `yaml:"paren_spacing"`
	QuotedIdents         QuotedIdentMode     `yaml:"quoted_identifiers"`
	TrailingNewline      TrailingNewlineMode `yaml:"trailing_newline"`
	CommaSpacing         CommaSpacingMode    `yaml:"comma_spacing"`
	OrderAsc             OrderAscMode        `yaml:"order_asc"`
	CastStyle            CastStyleMode       `yaml:"cast_style"`
	NotIn                NotInMode           `yaml:"not_in"`
	SchemaQual           SchemaQualMode      `yaml:"schema_qualification"`
	Layout               LayoutConfig        `yaml:"layout"`
}

func defaultConfig() *Config {
	return &Config{
		ReservedKeywords:     Section{Case: CaseUpper},
		Keywords:             Section{Case: CaseUpper},
		DataTypes:            DataTypesSection{Case: CaseLower, Form: TypeFormLong},
		Literals:             Section{Case: CaseUpper},
		Operators:            Section{Case: CaseUpper},
		Schemas:              Section{Case: CaseLower},
		Tables:               Section{Case: CaseLower},
		Functions:            Section{Case: CaseLower},
		ConditionalFunctions: Section{Case: CaseUpper},
		SystemFunctions:      Section{Case: CaseUpper},
		Aliases:              AliasSection{Case: CaseLower, As: AliasAsAdd},
		Columns:              Section{Case: CaseLower},
		TrailingWhitespace:   TrailingWSStrip,
		Semicolons:           SemicolonPreserve,
		InequalityOp:         InequalityC,
		JoinForm:             JoinFormPreserve,
		OperatorSpacing:      OperatorSpacingNormalize,
		BlankLines:           BlankLinesPreserve,
		ParenSpacing:         ParenSpacingRemove,
		QuotedIdents:         QuotedIdentRemoveSafe,
		TrailingNewline:      TrailingNewlinePreserve,
		CommaSpacing:         CommaSpacingNormalize,
		OrderAsc:             OrderAscPreserve,
		CastStyle:            CastStylePreserve,
		NotIn:                NotInPreserve,
		SchemaQual:           SchemaQualPreserve,
		Layout:               defaultLayout(),
	}
}

// Load reads a YAML config file. If path is empty, searches for .procrustes.yaml
// walking up from the current directory. Returns defaults if no file is found.
func Load(path string) (*Config, error) {
	if path == "" {
		found, err := findConfig()
		if err != nil || found == "" {
			return defaultConfig(), nil
		}
		path = found
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return defaultConfig(), nil
		}
		return nil, err
	}

	cfg := defaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	if err := validate(cfg); err != nil {
		return nil, fmt.Errorf("invalid config %s: %w", path, err)
	}
	return cfg, nil
}

// Validate checks that all config fields have recognised values.
func Validate(cfg *Config) error { return validate(cfg) }

func validate(cfg *Config) error {
	cases := []CaseRule{
		cfg.ReservedKeywords.Case, cfg.Keywords.Case, cfg.DataTypes.Case,
		cfg.Literals.Case, cfg.Operators.Case,
		cfg.Schemas.Case, cfg.Tables.Case,
		cfg.Functions.Case, cfg.ConditionalFunctions.Case, cfg.SystemFunctions.Case,
		cfg.Aliases.Case, cfg.Columns.Case,
	}
	for _, v := range cases {
		if v != CaseUpper && v != CaseLower && v != CasePreserve {
			return fmt.Errorf("case must be %q, %q or %q, got %q", CaseUpper, CaseLower, CasePreserve, v)
		}
	}

	switch cfg.DataTypes.Form {
	case TypeFormPreserve, TypeFormLong, TypeFormShort, TypeFormLongNoSpace, "":
	default:
		return fmt.Errorf("data_types.form must be preserve|long|short|long_no_space, got %q", cfg.DataTypes.Form)
	}

	switch cfg.Aliases.As {
	case AliasAsAdd, AliasAsPreserve, AliasAsRemove, "":
	default:
		return fmt.Errorf("aliases.as must be add|preserve|remove, got %q", cfg.Aliases.As)
	}

	switch cfg.TrailingWhitespace {
	case TrailingWSStrip, TrailingWSPreserve, "":
	default:
		return fmt.Errorf("trailing_whitespace must be strip|preserve, got %q", cfg.TrailingWhitespace)
	}

	switch cfg.Semicolons {
	case SemicolonPreserve, SemicolonAdd, SemicolonRemove, "":
	default:
		return fmt.Errorf("semicolons must be preserve|add|remove, got %q", cfg.Semicolons)
	}

	switch cfg.InequalityOp {
	case InequalityPreserve, InequalityANSI, InequalityC, "":
	default:
		return fmt.Errorf("inequality_op must be preserve|ansi|c, got %q", cfg.InequalityOp)
	}

	switch cfg.JoinForm {
	case JoinFormPreserve, JoinFormShort, JoinFormLong, "":
	default:
		return fmt.Errorf("join_form must be preserve|short|long, got %q", cfg.JoinForm)
	}

	switch cfg.OperatorSpacing {
	case OperatorSpacingPreserve, OperatorSpacingNormalize, "":
	default:
		return fmt.Errorf("operator_spacing must be preserve|normalize, got %q", cfg.OperatorSpacing)
	}

	switch cfg.BlankLines {
	case BlankLinesPreserve, BlankLinesMax3, BlankLinesMax2, BlankLinesMax1, "":
	default:
		return fmt.Errorf("blank_lines must be preserve|max_3|max_2|max_1, got %q", cfg.BlankLines)
	}

	switch cfg.ParenSpacing {
	case ParenSpacingPreserve, ParenSpacingAdd, ParenSpacingRemove, "":
	default:
		return fmt.Errorf("paren_spacing must be preserve|add|remove, got %q", cfg.ParenSpacing)
	}

	switch cfg.QuotedIdents {
	case QuotedIdentPreserve, QuotedIdentRemoveSafe, "":
	default:
		return fmt.Errorf("quoted_identifiers must be preserve|remove_safe, got %q", cfg.QuotedIdents)
	}

	switch cfg.TrailingNewline {
	case TrailingNewlinePreserve, TrailingNewlineAdd, TrailingNewlineStrip, "":
	default:
		return fmt.Errorf("trailing_newline must be preserve|add|strip, got %q", cfg.TrailingNewline)
	}

	switch cfg.CommaSpacing {
	case CommaSpacingPreserve, CommaSpacingNormalize, "":
	default:
		return fmt.Errorf("comma_spacing must be preserve|normalize, got %q", cfg.CommaSpacing)
	}

	switch cfg.OrderAsc {
	case OrderAscPreserve, OrderAscAdd, OrderAscRemove, "":
	default:
		return fmt.Errorf("order_asc must be preserve|add|remove, got %q", cfg.OrderAsc)
	}

	switch cfg.CastStyle {
	case CastStylePreserve, CastStyleOperator, "":
	default:
		return fmt.Errorf("cast_style must be preserve|operator, got %q", cfg.CastStyle)
	}

	switch cfg.NotIn {
	case NotInPreserve, NotInNotIn, NotInNotEqualsAll, "":
	default:
		return fmt.Errorf("not_in must be preserve|not_in|not_equals_all, got %q", cfg.NotIn)
	}

	switch cfg.SchemaQual {
	case SchemaQualPreserve, SchemaQualRemovePublic, "":
	default:
		return fmt.Errorf("schema_qualification must be preserve|remove_public, got %q", cfg.SchemaQual)
	}

	switch cfg.Layout.Clauses.Break {
	case BreakPreserve, BreakNever, BreakAlways, BreakAuto, "":
	default:
		return fmt.Errorf("layout.clauses.break must be preserve|never|always|auto, got %q", cfg.Layout.Clauses.Break)
	}
	switch cfg.Layout.Content.Break {
	case BreakPreserve, BreakNever, BreakAlways, BreakAuto, "":
	default:
		return fmt.Errorf("layout.content.break must be preserve|never|always|auto, got %q", cfg.Layout.Content.Break)
	}
	switch cfg.Layout.Union.BlankLine {
	case UnionBlankLinePreserve, UnionBlankLineNone, UnionBlankLineBefore, UnionBlankLineAfter, UnionBlankLineBoth, "":
	default:
		return fmt.Errorf("layout.union.blank_line must be preserve|none|before|after|both, got %q", cfg.Layout.Union.BlankLine)
	}
	switch cfg.Layout.Indent.Normalize {
	case IndentNormalizePreserve, IndentNormalizeChange, "":
	default:
		return fmt.Errorf("layout.indent.normalize must be preserve|change, got %q", cfg.Layout.Indent.Normalize)
	}

	validateCaseBranch := func(b CaseBranchCfg, prefix string) error {
		switch b.WhenIndent {
		case BodyIndentPreserve, BodyIndentNone, BodyIndentIndent, "":
		default:
			return fmt.Errorf("%s.when_indent must be preserve|none|indent, got %q", prefix, b.WhenIndent)
		}
		for _, pair := range []struct {
			v    BreakMode
			name string
		}{
			{b.ThenBreak, "then_break"},
			{b.BodyBreak, "body_break"},
		} {
			switch pair.v {
			case BreakPreserve, BreakNever, BreakAlways, BreakAuto, "":
			default:
				return fmt.Errorf("%s.%s must be preserve|never|always|auto, got %q", prefix, pair.name, pair.v)
			}
		}
		switch b.ThenIndent {
		case BodyIndentPreserve, BodyIndentNone, BodyIndentIndent, "":
		default:
			return fmt.Errorf("%s.then_indent must be preserve|none|indent, got %q", prefix, b.ThenIndent)
		}
		switch b.BodyIndent {
		case BodyIndentPreserve, BodyIndentNone, BodyIndentIndent, "":
		default:
			return fmt.Errorf("%s.body_indent must be preserve|none|indent, got %q", prefix, b.BodyIndent)
		}
		for _, pair := range []struct {
			v    BlankLineAction
			name string
		}{
			{b.BlankLineBefore, "blank_line_before"},
			{b.BlankLineAfter, "blank_line_after"},
		} {
			switch pair.v {
			case BlankLinePreserve, BlankLineAdd, BlankLineRemove, "":
			default:
				return fmt.Errorf("%s.%s must be preserve|add|remove, got %q", prefix, pair.name, pair.v)
			}
		}
		return nil
	}
	if err := validateCaseBranch(cfg.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Simple, "layout.dollar_quote.plpgsql.control_flow.case.simple"); err != nil {
		return err
	}
	if err := validateCaseBranch(cfg.Layout.DollarQuote.PLpgSQL.ControlFlow.Case.Searched, "layout.dollar_quote.plpgsql.control_flow.case.searched"); err != nil {
		return err
	}

	switch cfg.Layout.Case.Break {
	case BreakPreserve, BreakNever, BreakAlways, BreakAuto, "":
	default:
		return fmt.Errorf("layout.case.break must be preserve|never|always|auto, got %q", cfg.Layout.Case.Break)
	}
	switch cfg.Layout.Case.Indent {
	case BodyIndentPreserve, BodyIndentNone, BodyIndentIndent, "":
	default:
		return fmt.Errorf("layout.case.indent must be preserve|none|indent, got %q", cfg.Layout.Case.Indent)
	}

	return nil
}

func findConfig() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		candidate := filepath.Join(dir, ".procrustes.yaml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", nil
}
