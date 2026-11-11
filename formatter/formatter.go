// Package formatter implements token-level SQL rewriting for pg_procrustes.
package formatter

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	pg_query "github.com/pganalyze/pg_query_go/v6"

	"github.com/heptau/pg_procrustes/config"
	"github.com/heptau/pg_procrustes/internal/layout"
)

// dataTypeNames covers type keywords that pg_query classifies ambiguously
// (COL_NAME_KEYWORD or UNRESERVED_KEYWORD) but are clearly data types.
var dataTypeNames = map[string]bool{
	"int":         true,
	"int2":        true,
	"int4":        true,
	"int8":        true,
	"integer":     true,
	"bigint":      true,
	"smallint":    true,
	"boolean":     true,
	"bool":        true,
	"real":        true,
	"float4":      true,
	"float8":      true,
	"double":      true,
	"precision":   true,
	"text":        true,
	"bytea":       true,
	"char":        true,
	"character":   true,
	"varchar":     true,
	"varying":     true,
	"numeric":     true,
	"decimal":     true,
	"money":       true,
	"date":        true,
	"time":        true,
	"timetz":      true,
	"timestamp":   true,
	"timestamptz": true,
	"interval":    true,
	"json":        true,
	"jsonb":       true,
	"uuid":        true,
	"xml":         true,
	"bit":         true,
	"varbit":      true,
	"serial":      true,
	"bigserial":   true,
	"smallserial": true,
	"inet":        true,
	"cidr":        true,
	"macaddr":     true,
	"macaddr8":    true,
	"point":       true,
	"line":        true,
	"lseg":        true,
	"box":         true,
	"path":        true,
	"polygon":     true,
	"circle":      true,
	"tsvector":    true,
	"tsquery":     true,
	"oid":         true,
	"void":        true,
	"record":      true,
	"anyelement":  true,
	"anyarray":    true,
	"anynonarray": true,
	"anyenum":     true,
	"anyrange":    true,
	"cstring":     true,
	"trigger":     true,
}

// plpgsqlVariableNames covers PL/pgSQL special variables used in trigger and
// procedure bodies. NEW and OLD are UNRESERVED_KEYWORD in the scanner (so they
// would normally follow keywords.case); all others are plain identifiers
// (NO_KEYWORD). All groups are intercepted here so that plpgsql_variables.case
// applies to them uniformly.
var plpgsqlVariableNames = map[string]bool{
	// trigger record variables and ON CONFLICT pseudo-table
	"new":      true,
	"old":      true,
	"excluded": true,
	// common PL/pgSQL status variable
	"found": true,
	// row-level trigger special variables
	"tg_name":         true,
	"tg_when":         true,
	"tg_level":        true,
	"tg_op":           true,
	"tg_relid":        true,
	"tg_relname":      true, // deprecated alias for tg_table_name
	"tg_table_name":   true,
	"tg_table_schema": true,
	"tg_nargs":        true,
	"tg_argv":         true,
	// event trigger special variables
	"tg_event": true,
	"tg_tag":   true,
	// exception-handling variables (EXCEPTION WHEN / RAISE)
	"sqlstate": true,
	"sqlerrm":  true,
	// GET DIAGNOSTICS items
	"row_count":            true,
	"pg_context":           true,
	"pg_exception_detail":  true,
	"pg_exception_hint":    true,
	"pg_exception_context": true,
	// GET STACKED DIAGNOSTICS items (available inside EXCEPTION handlers)
	"returned_sqlstate": true,
	"message_text":      true,
	"pg_datatype_name":  true,
	"pg_routine_oid":    true,
}

// plpgsqlKeywordNames covers PL/pgSQL-only statement keywords that the PostgreSQL
// scanner classifies as NO_KEYWORD (plain identifiers), so they receive no case
// transformation from the keywords/reserved_keywords rules. The plpgsql_keywords
// section lets users control their case independently.
var plpgsqlKeywordNames = map[string]bool{
	// core statement keywords
	"raise":   true,
	"perform": true,
	// conditional / loop control
	"elsif":   true,
	"elseif":  true,
	"foreach": true,
	"reverse": true, // FOR ... REVERSE
	"slice":   true, // FOREACH ... SLICE
	"exit":    true,
	"loop":    true,
	"while":   true,
	// cursor management
	"open": true,
	// diagnostic statement
	"assert": true,
	// RETURN QUERY (a keyword only right after RETURN)
	"query": true,
	// RAISE severity levels
	"debug":   true,
	"info":    true,
	"notice":  true,
	"warning": true,
	// EXCEPTION as RAISE severity (RAISE EXCEPTION '...') and handler keyword
	"exception": true,
}

// raiseLevelNames are the severity words of RAISE, which are ordinary names elsewhere.
var raiseLevelNames = map[string]bool{"debug": true, "info": true, "notice": true, "warning": true}

// parenAfterKeywordIsCall lists keyword-spelled words whose "(" belongs to a call or a type, not
// to the keyword: ARRAY(SELECT …), ROW(1, 2), VARCHAR / CHARACTER VARYING(10).
var parenAfterKeywordIsCall = map[string]bool{
	"array": true, "row": true, "varying": true, "precision": true,
	// keyword-spelled functions: their "(" is a call's
	"coalesce": true, "greatest": true, "least": true, "nullif": true, "extract": true, "trim": true,
	"substring": true, "position": true, "overlay": true, "cast": true, "grouping": true, "normalize": true,
	"xmlattributes": true, "xmlconcat": true, "xmlelement": true, "xmlexists": true, "xmlforest": true,
	"xmlparse": true, "xmlpi": true, "xmlroot": true, "xmlserialize": true, "xmltable": true,
}

// callOnlyKeywords are words spelled like (non-reserved) keywords that are only ever followed by
// "(" as a function call — in PL/pgSQL expressions, which the parser does not see, nothing else
// says so: LEFT(s, 1), REPLACE(s, a, b), FORMAT('x'), JSON_OBJECT('a': 1). Unlike
// parenAfterKeywordIsCall they still count as keywords for casing.
var callOnlyKeywords = map[string]bool{
	"left": true, "right": true, "replace": true, "format": true, "reverse": true, "json": true,
	"json_array": true, "json_arrayagg": true, "json_exists": true, "json_object": true,
	"json_objectagg": true, "json_query": true, "json_scalar": true, "json_serialize": true,
	"json_table": true, "json_value": true, "system_user": true, "xmlcomment": true,
}

// isKeywordClass reports whether c is the class of a keyword or operator word.
func isKeywordClass(c tokenClass) bool {
	return c == classReservedKeyword || c == classKeyword || c == classOperator || c == classPlpgSQLKeyword
}

// startsKeywordWord reports whether tokens[i] is a keyword or operator word (THEN, AND, LOOP, AS, …)
// rather than a name, a function or a type: the AST has not placed it anywhere.
func startsKeywordWord(tokens []rawToken, i int, pos *astPositions) bool {
	t := tokens[i]
	if t.text == "" || !(t.text[0] == '_' || t.text[0] >= 'a' && t.text[0] <= 'z' || t.text[0] >= 'A' && t.text[0] <= 'Z') {
		return false
	}
	lower := strings.ToLower(t.text)
	if literalNames[lower] || dataTypeNames[lower] || parenAfterKeywordIsCall[lower] {
		return false
	}
	at := t.start
	if pos.columns[at] || pos.aliases[at] || pos.tables[at] || pos.schemas[at] || pos.functions[at] ||
		pos.conditionalFunctions[at] || pos.systemFunctions[at] {
		return false
	}
	return t.kind != pg_query.KeywordKind_NO_KEYWORD || plpgsqlKeywordNames[lower] || operatorKeywords[lower]
}

// literalNames are boolean/null literals. They are RESERVED_KEYWORD in the
// scanner but should follow literals.case, not keywords.case.
var literalNames = map[string]bool{
	"true":    true,
	"false":   true,
	"null":    true,
	"unknown": true,
}

// operatorKeywords are logical/comparison operators that appear as keywords in
// the scanner but should follow operators.case, not keywords.case.
var operatorKeywords = map[string]bool{
	"and":      true,
	"or":       true,
	"not":      true,
	"in":       true,
	"is":       true,
	"like":     true,
	"ilike":    true,
	"between":  true,
	"exists":   true,
	"any":      true,
	"all":      true,
	"some":     true,
	"overlaps": true,
	"similar":  true,
	"distinct": true,
}

// ── Type normalization tables ──────────────────────────────────────────────

// singleShortToLong maps single-token short aliases to their canonical long
// form. Multi-word long forms are stored as a single string; applyCase handles
// the casing for the entire string.
var singleShortToLong = map[string]string{
	"int":         "integer",
	"int4":        "integer",
	"int2":        "smallint",
	"int8":        "bigint",
	"bool":        "boolean",
	"float4":      "real",
	"float8":      "double precision",
	"varchar":     "character varying",
	"char":        "character",
	"varbit":      "bit varying",
	"timetz":      "time with time zone",
	"timestamptz": "timestamp with time zone",
}

// singleLongToShort maps long single-word type names to their shortest alias.
var singleLongToShort = map[string]string{
	"integer":   "int",
	"smallint":  "int2",
	"bigint":    "int8",
	"boolean":   "bool",
	"real":      "float4",
	"character": "char",
}

// multiTokenSeq describes a multi-token type sequence and its canonical forms.
type multiTokenSeq struct {
	next      []string // follow-on token texts (lowercase), immediately after first
	longForm  string   // canonical long form (may contain spaces)
	shortForm string   // canonical short form (single token)
}

// multiTokenTypes maps the first data-type token (lowercase) to the sequences
// it can start. Sequences are listed longest-first for greedy matching.
var multiTokenTypes = map[string][]multiTokenSeq{
	"character": {
		{next: []string{"varying"}, longForm: "character varying", shortForm: "varchar"},
	},
	"double": {
		{next: []string{"precision"}, longForm: "double precision", shortForm: "float8"},
	},
	"bit": {
		{next: []string{"varying"}, longForm: "bit varying", shortForm: "varbit"},
	},
	"time": {
		{next: []string{"with", "time", "zone"}, longForm: "time with time zone", shortForm: "timetz"},
		{next: []string{"without", "time", "zone"}, longForm: "time without time zone", shortForm: "time"},
	},
	"timestamp": {
		{next: []string{"with", "time", "zone"}, longForm: "timestamp with time zone", shortForm: "timestamptz"},
		{next: []string{"without", "time", "zone"}, longForm: "timestamp without time zone", shortForm: "timestamp"},
	},
}

// rawToken is a single token from pg_query.Scan with its source byte offsets.
type rawToken struct {
	start, end int
	text       string
	kind       pg_query.KeywordKind
}

type tokenClass int

const (
	classOther tokenClass = iota
	classReservedKeyword
	classKeyword
	classDataType
	classLiteral
	classOperator
	classSchema
	classTable
	classFunction
	classConditionalFunction
	classSystemFunction
	classAlias
	classColumn
	classPlpgSQLVariable // NEW, OLD, FOUND, TG_* — governed by plpgsql_variables.case
	classPlpgSQLKeyword  // RAISE, PERFORM, ELSIF, LOOP… — governed by plpgsql_keywords.case
)

// pendingAlias is a SELECT alias whose byte position must be resolved via
// token scanning after scanning is complete (because ResTarget.Location
// points to the expression start, not the alias name).
type pendingAlias struct {
	exprStart int
	name      string
}

// astPositions holds byte offsets of tokens identified by the parser as
// specific identifier categories.
type astPositions struct {
	schemas              map[int]bool
	tables               map[int]bool
	functions            map[int]bool
	conditionalFunctions map[int]bool
	systemFunctions      map[int]bool
	aliases              map[int]bool
	columns              map[int]bool
	pendingAliases       []pendingAlias
	subselectAliases     map[string]bool // lower-cased aliases of derived tables (FROM (SELECT …) x)
	functionAliases      map[string]bool // lower-cased aliases of table functions (FROM unnest(…) AS a(x, y))
	sortByDefaultLocs    []int
	castFuncLocs         []int
	funcParams           map[string]bool // lower-cased parameter names of CREATE FUNCTION / PROCEDURE
	sql                  string
}

func newASTPositions() *astPositions {
	return &astPositions{
		schemas:              make(map[int]bool),
		tables:               make(map[int]bool),
		functions:            make(map[int]bool),
		conditionalFunctions: make(map[int]bool),
		systemFunctions:      make(map[int]bool),
		aliases:              make(map[int]bool),
		columns:              make(map[int]bool),
		subselectAliases:     make(map[string]bool),
		functionAliases:      make(map[string]bool),
	}
}

// resolveAliases converts pendingAliases into actual positions using the
// scanned token list. ResTarget.Location points to the expression start; we
// scan forward to find the alias NAME token.
func (p *astPositions) resolveAliases(tokens []rawToken) {
	for _, pa := range p.pendingAliases {
		idx := findAliasToken(tokens, pa.exprStart, pa.name)
		if idx >= 0 {
			p.aliases[tokens[idx].start] = true
		}
	}
}

// findAliasToken locates the alias name token in tokens, starting from the
// token at or after exprStart, and returns its index. Returns -1 if not found.
func findAliasToken(tokens []rawToken, exprStart int, aliasName string) int {
	// Find the index of the first token at or after exprStart.
	startIdx := -1
	for i, t := range tokens {
		if t.start >= exprStart {
			startIdx = i
			break
		}
	}
	if startIdx < 0 {
		return -1
	}

	depth := 0
	for i := startIdx; i < len(tokens); i++ {
		text := tokens[i].text
		switch text {
		case "(", "[":
			depth++
		case ")", "]":
			if depth == 0 {
				return -1
			}
			depth--
		case ",", ";":
			if depth == 0 {
				return -1
			}
		default:
			if depth != 0 {
				continue
			}
			lower := strings.ToLower(text)
			if lower == "as" {
				// The next token at depth 0 is the alias.
				// If the very next token is '(' it's a function call, not an alias.
				if j := i + 1; j < len(tokens) && tokens[j].text != "(" {
					return j
				}
				return -1
			}
			// Implicit alias: identifier that matches the expected name,
			// not at the expression start, followed by a list separator or
			// a clause keyword.
			if i > startIdx && (strings.EqualFold(text, aliasName) || unquoteIdent(text) == aliasName) {
				if isFollowedBySeparator(tokens, i) {
					return i
				}
			}
		}
	}
	return -1
}

// resolveRoutineNames registers the name in CREATE | DROP | ALTER | COMMENT ON | GRANT … ON | EXECUTE
// FUNCTION | PROCEDURE [IF EXISTS] [schema.]name as a function when it is spelled like a PL/pgSQL
// keyword (pg_temp.assert, s.raise, s.loop): the statement has no function-call node, and the name
// would be written as the keyword (CREATE FUNCTION pg_temp.ASSERT while its calls stay assert).
func resolveRoutineNames(tokens []rawToken, pos *astPositions) {
	lw := func(i int) string {
		if i < 0 || i >= len(tokens) {
			return ""
		}
		return strings.ToLower(tokens[i].text)
	}
	for i := range tokens {
		if w := lw(i); w != "function" && w != "procedure" && w != "routine" {
			continue
		}
		k := i + 1
		if lw(k) == "if" && lw(k+1) == "exists" {
			k += 2
		}
		for k < len(tokens) {
			name := k
			if plpgsqlKeywordNames[lw(name)] {
				pos.functions[tokens[name].start] = true
			}
			if lw(k+1) != "." {
				break
			}
			k += 2
		}
	}
}

// resolveAliasColumns registers the column names that follow a table alias or a CTE name:
// FROM f(…) AS x(database text, class text), FROM t AS u(a, b), WITH c(a, b) AS (…), CREATE VIEW
// v (a, b) AS …. Their AST nodes carry no location, so the token stream is scanned: the first token
// of each comma-separated item of the parenthesis is a column (a name that is also a keyword —
// database, class, object — would be cased as the keyword). A table name counts only when the
// parenthesis is followed by AS (INSERT INTO t (a, b) and CREATE TABLE t (…) are handled elsewhere).
func resolveAliasColumns(tokens []rawToken, pos *astPositions) {
	for i := 0; i+1 < len(tokens); i++ {
		if tokens[i+1].text != "(" {
			continue
		}
		isAlias := pos.aliases[tokens[i].start]
		isTable := pos.tables[tokens[i].start]
		if !isAlias && !isTable {
			continue
		}
		depth := 0
		closeIdx := -1
		for k := i + 1; k < len(tokens); k++ {
			switch tokens[k].text {
			case "(":
				depth++
			case ")":
				depth--
			}
			if depth == 0 {
				closeIdx = k
				break
			}
		}
		if closeIdx < 0 || !isAlias && (closeIdx+1 >= len(tokens) || !strings.EqualFold(tokens[closeIdx+1].text, "as")) {
			continue
		}
		depth = 0
		expectCol := true
		for k := i + 1; k <= closeIdx; k++ {
			switch t := tokens[k].text; {
			case t == "(":
				depth++
			case t == ")":
				depth--
			case t == "," && depth == 1:
				expectCol = true
			default:
				if depth == 1 && expectCol {
					pos.columns[tokens[k].start] = true
					expectCol = false
				}
			}
		}
	}
}

// resolveOnConflictCols registers the conflict-target column positions from
// ON CONFLICT (col1, col2, …) clauses. InferClause.IndexElem has no Location
// field, so we scan the token stream for the ON CONFLICT ( … ) pattern.
func resolveOnConflictCols(tokens []rawToken, pos *astPositions) {
	for i, tok := range tokens {
		if strings.ToLower(tok.text) != "conflict" {
			continue
		}
		// Must be immediately preceded by ON.
		if i == 0 || strings.ToLower(tokens[i-1].text) != "on" {
			continue
		}
		// Must be followed by '(' — ON CONFLICT ON CONSTRAINT has no paren.
		if i+1 >= len(tokens) || tokens[i+1].text != "(" {
			continue
		}
		// Register the first token of each comma-separated item as a column.
		depth := 1
		expectCol := true
		for k := i + 2; k < len(tokens); k++ {
			t := tokens[k].text
			switch {
			case t == "(":
				depth++
				expectCol = false
			case t == ")":
				depth--
				if depth == 0 {
					goto nextConflict
				}
				expectCol = false
			case t == "," && depth == 1:
				expectCol = true
			default:
				if depth == 1 && expectCol {
					pos.columns[tokens[k].start] = true
					expectCol = false
				}
			}
		}
	nextConflict:
	}
}

// resolveFunctionParams registers the names of CREATE FUNCTION / PROCEDURE parameters
// (and of the RETURNS TABLE (…) columns) as columns: FunctionParameter has no
// Location, so name, type or event would otherwise be upper-cased as keywords.
func resolveFunctionParams(tokens []rawToken, pos *astPositions) {
	if len(pos.funcParams) == 0 {
		return
	}
	lw := func(i int) string {
		if i < 0 || i >= len(tokens) {
			return ""
		}
		return strings.ToLower(tokens[i].text)
	}
	list := func(open int) int { // registers the names of the list opening at open; returns the index after it
		depth, item, k := 1, 0, open+1
		for ; k < len(tokens) && depth > 0; k++ {
			t := tokens[k].text
			switch {
			case t == "(" || t == "[":
				depth++
			case t == ")" || t == "]":
				depth--
			case t == "," && depth == 1:
				item = 0
				continue
			}
			if depth == 1 && t != ")" && t != "]" && t != "," {
				item++
				name := strings.ToLower(t)
				if q := unquoteIdent(t); q != "" {
					name = strings.ToLower(q)
				}
				if item <= 2 && pos.funcParams[name] {
					// A name is followed by its type; a lone token is an unnamed type.
					if k+1 < len(tokens) && tokens[k+1].text != "," && tokens[k+1].text != ")" && tokens[k+1].text != "[" {
						pos.columns[tokens[k].start] = true
						item = 3
					}
				}
			}
		}
		return k
	}
	for i := range tokens {
		if w := lw(i); (w != "function" && w != "procedure") || (lw(i-1) != "create" && lw(i-1) != "replace") {
			continue
		}
		k := i + 1
		for k < len(tokens) && tokens[k].text != "(" && tokens[k].text != ";" {
			k++
		}
		if k >= len(tokens) || tokens[k].text != "(" {
			continue
		}
		k = list(k)
		if lw(k) == "returns" && lw(k+1) == "table" && lw(k+2) == "(" {
			list(k + 2)
		}
	}
}

// resolvePLpgSQLVars registers the variables of a PL/pgSQL body as columns when they are spelled
// like keywords (sql, name, type, key, data, …): the parameters of the function (params), the
// variables of DECLARE sections and the loop variables of FOR x IN / FOREACH x IN. A body's
// expressions (status = sql || name) have no parse tree, so the word would be written as the
// keyword (KEY = SQL || NAME). Only words the scanner knows as non-reserved keywords are
// affected, and nothing the tree has already placed.
func resolvePLpgSQLVars(tokens []rawToken, pos *astPositions, params map[string]bool) {
	if params == nil {
		return // not a PL/pgSQL body (FOR EACH ROW of a trigger is no loop)
	}
	names := collectPLpgSQLVars(tokens, params)
	lw := func(i int) string {
		if i < 0 || i >= len(tokens) {
			return ""
		}
		return strings.ToLower(tokens[i].text)
	}
	if len(names) == 0 {
		return
	}
	markPLpgSQLVars(tokens, pos, names, lw)
}

// plpgsqlVarNames is collectPLpgSQLVars for a body's text.
func plpgsqlVarNames(body string, params map[string]bool) map[string]bool {
	sc, err := pg_query.Scan(body)
	if err != nil {
		return params
	}
	tokens := make([]rawToken, len(sc.Tokens))
	for i, t := range sc.Tokens {
		tokens[i] = rawToken{start: int(t.Start), end: int(t.End), text: body[t.Start:t.End], kind: t.KeywordKind}
	}
	return collectPLpgSQLVars(tokens, params)
}

// collectPLpgSQLVars returns params plus the names declared in the body's DECLARE sections and
// FOR / FOREACH loops.
func collectPLpgSQLVars(tokens []rawToken, params map[string]bool) map[string]bool {
	names := map[string]bool{}
	for n := range params {
		names[n] = true
	}
	lw := func(i int) string {
		if i < 0 || i >= len(tokens) {
			return ""
		}
		return strings.ToLower(tokens[i].text)
	}
	isComment := func(i int) bool {
		t := tokens[i].text
		return strings.HasPrefix(t, "--") || strings.HasPrefix(t, "/*")
	}
	declaring := false
	itemStart := false
	for i := range tokens {
		if isComment(i) {
			continue
		}
		switch w := lw(i); {
		case w == "declare":
			declaring, itemStart = true, true
			continue
		case w == "begin":
			declaring = false
		case w == ";":
			itemStart = declaring
			continue
		case (w == "for" || w == "foreach") && lw(i-1) != "loop" && lw(i+1) != "update" && lw(i+1) != "share" && lw(i+1) != "values":
			// FOR a, b IN …  /  FOREACH a IN ARRAY …  (FOR EACH ROW and FOR TABLE x have no IN
			// right after the names)
			var loopVars []string
			for k := i + 1; k < len(tokens) && k < i+8; k++ {
				if isComment(k) {
					continue
				}
				if tokens[k].text == "(" || tokens[k].text == ";" || lw(k) == "each" {
					break
				}
				if lw(k) == "in" || lw(k) == "slice" {
					for _, v := range loopVars {
						names[v] = true
					}
					break
				}
				if tokens[k].text != "," {
					if len(loopVars) > 0 && lw(k-1) != "," {
						break // two names in a row (FOR ROLE x IN SCHEMA): no loop variables
					}
					loopVars = append(loopVars, lw(k))
				}
			}
		}
		if declaring && itemStart {
			itemStart = false
			if w := lw(i); w != "begin" && w != "declare" && w != "end" && lw(i+1) != "." && lw(i+1) != "(" {
				names[w] = true
			}
		}
	}
	return names
}

func markPLpgSQLVars(tokens []rawToken, pos *astPositions, names map[string]bool, lw func(int) string) {
	for i, t := range tokens {
		if t.kind == pg_query.KeywordKind_NO_KEYWORD || t.kind == pg_query.KeywordKind_RESERVED_KEYWORD || !names[strings.ToLower(t.text)] {
			continue
		}
		if i > 0 && tokens[i-1].text == "." {
			continue
		}
		if i > 1 && tokens[i-1].text == "(" && strings.HasPrefix(lw(i-2), "xml") {
			continue // xmlelement(name x): the keyword
		}
		at := t.start
		if pos.tables[at] && i > 0 && lw(i-1) == "into" && !isInsertTarget(tokens, i) {
			// SELECT … INTO var: the tree sees a new table
			delete(pos.tables, at)
		}
		if pos.columns[at] || pos.aliases[at] || pos.tables[at] || pos.schemas[at] || pos.functions[at] ||
			pos.conditionalFunctions[at] || pos.systemFunctions[at] {
			continue
		}
		pos.columns[at] = true
	}
}

// resolveBodyAliases registers the names after AS in a PL/pgSQL body's statements the parser does
// not see (PERFORM f((SELECT … AS type …)): `expr AS name` followed by `,` or `)`, where name is a
// keyword that is no type (AS type, AS data, AS source). Only a word the tree has not placed.
func resolveBodyAliases(tokens []rawToken, pos *astPositions) {
	for i := 0; i+2 < len(tokens); i++ {
		if !strings.EqualFold(tokens[i].text, "as") {
			continue
		}
		n := tokens[i+1]
		low := strings.ToLower(n.text)
		if n.kind == pg_query.KeywordKind_NO_KEYWORD || n.kind == pg_query.KeywordKind_RESERVED_KEYWORD ||
			dataTypeNames[low] || literalNames[low] || (tokens[i+2].text != "," && tokens[i+2].text != ")") {
			continue
		}
		switch low {
		case "materialized", "not", "enum", "range", "implicit", "assignment", "identity", "restrictive", "permissive", "stored", "operator", "of":
			continue
		}
		at := n.start
		if pos.columns[at] || pos.aliases[at] || pos.tables[at] || pos.schemas[at] || pos.functions[at] ||
			pos.conditionalFunctions[at] || pos.systemFunctions[at] {
			continue
		}
		pos.columns[at] = true
	}
}

// resolveXMLNames registers the names after AS in the arguments of XMLFOREST / XMLATTRIBUTES
// (xmlforest(a AS costs)) as names (AS is required there, so not aliases): they name elements and attributes, and a name that is also a
// keyword (costs, number, type) would be written as the keyword.
func resolveXMLNames(tokens []rawToken, pos *astPositions) {
	var calls []bool // for each open parenthesis: is it the call of xmlforest / xmlattributes
	for i, t := range tokens {
		switch t.text {
		case "(":
			calls = append(calls, i > 0 && (strings.EqualFold(tokens[i-1].text, "xmlforest") || strings.EqualFold(tokens[i-1].text, "xmlattributes")))
		case ")":
			if len(calls) > 0 {
				calls = calls[:len(calls)-1]
			}
		default:
			if len(calls) > 0 && calls[len(calls)-1] && strings.EqualFold(t.text, "as") && i+1 < len(tokens) &&
				tokens[i+1].kind != pg_query.KeywordKind_RESERVED_KEYWORD && !strings.HasPrefix(tokens[i+1].text, `"`) {
				pos.columns[tokens[i+1].start] = true
			}
		}
	}
}

// isInsertTarget reports whether the INTO before tokens[i] belongs to an INSERT (or MERGE … INSERT)
// of the same statement rather than to a SELECT … INTO.
func isInsertTarget(tokens []rawToken, i int) bool {
	for k := i - 1; k >= 0 && tokens[k].text != ";"; k-- {
		if w := strings.ToLower(tokens[k].text); w == "insert" || w == "merge" {
			return true
		}
	}
	return false
}

// resolveIndexCols registers the plain column names of CREATE [UNIQUE] INDEX … (col,
// …) [INCLUDE (col, …)] as columns. IndexElem carries no Location, so without this an
// unreserved-keyword column such as name or status in the list would be upper-cased.
func resolveIndexCols(tokens []rawToken, pos *astPositions) {
	for i, tok := range tokens {
		if strings.ToLower(tok.text) != "index" || i == 0 {
			continue
		}
		if prev := strings.ToLower(tokens[i-1].text); prev != "create" && prev != "unique" {
			continue
		}
		k := i + 1
		for k < len(tokens) && tokens[k].text != ";" && tokens[k].text != "(" {
			k++
		}
		for k < len(tokens) && tokens[k].text == "(" {
			depth, expectCol := 1, true
			k++
			for ; k < len(tokens) && depth > 0; k++ {
				t := tokens[k].text
				switch {
				case t == "(":
					depth++
					expectCol = false
				case t == ")":
					depth--
					expectCol = false
				case t == "," && depth == 1:
					expectCol = true
				default:
					if depth == 1 && expectCol {
						// name(…) is an expression / function call, not a plain column.
						if k+1 >= len(tokens) || tokens[k+1].text != "(" {
							pos.columns[tokens[k].start] = true
						}
						expectCol = false
					}
				}
			}
			// Continue only with INCLUDE (…).
			if k+1 < len(tokens) && strings.ToLower(tokens[k].text) == "include" && tokens[k+1].text == "(" {
				k++
				continue
			}
			break
		}
	}
}

// clauseKeywords are tokens that cannot be alias names and end a SELECT target.
var clauseKeywords = map[string]bool{
	"from": true, "where": true, "group": true, "having": true,
	"order": true, "limit": true, "offset": true, "union": true,
	"intersect": true, "except": true, "fetch": true, "for": true,
	// JOIN clause separators — alias may precede these in FROM/JOIN clauses.
	"join": true, "on": true, "using": true,
	"inner": true, "left": true, "right": true, "full": true,
	"cross": true, "natural": true, "lateral": true,
}

func isFollowedBySeparator(tokens []rawToken, i int) bool {
	if i+1 >= len(tokens) {
		return true
	}
	next := strings.ToLower(tokens[i+1].text)
	return tokens[i+1].text == "," || tokens[i+1].text == ")" ||
		tokens[i+1].text == ";" || clauseKeywords[next]
}

func classify(kind pg_query.KeywordKind, tokenText string) tokenClass {
	lower := strings.ToLower(tokenText)
	switch kind {
	case pg_query.KeywordKind_RESERVED_KEYWORD:
		if dataTypeNames[lower] {
			return classDataType
		}
		return classReservedKeyword
	case pg_query.KeywordKind_UNRESERVED_KEYWORD,
		pg_query.KeywordKind_COL_NAME_KEYWORD,
		pg_query.KeywordKind_TYPE_FUNC_NAME_KEYWORD:
		if dataTypeNames[lower] {
			return classDataType
		}
		return classKeyword
	default:
		return classOther
	}
}

func applyCase(s string, rule config.CaseRule) string {
	// Quoted identifiers are case-sensitive in PostgreSQL: never change their case.
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s
	}
	switch rule {
	case config.CaseUpper:
		return strings.ToUpper(s)
	case config.CaseLower:
		return strings.ToLower(s)
	case config.CaseCapitalize:
		return capitalizeWords(s)
	default:
		return s
	}
}

// applyCaseEx is like applyCase but skips transformation when s (case-insensitively)
// matches one of the exceptions, returning s unchanged in that case.
func applyCaseEx(s string, rule config.CaseRule, exceptions []string) string {
	lower := strings.ToLower(s)
	for _, ex := range exceptions {
		if strings.ToLower(ex) == lower {
			return s
		}
	}
	return applyCase(s, rule)
}

// normalizeType returns the formatted type string and the indices of subsequent
// tokens that should be skipped (consumed as part of a multi-token type).
func normalizeType(tokens []rawToken, i int, skip map[int]bool, form config.TypeForm, caseRule config.CaseRule) (string, []int) {
	text := tokens[i].text
	lower := strings.ToLower(text)

	// nextIndices returns indices of the next n non-skipped tokens after i.
	nextIndices := func(n int) []int {
		out := make([]int, 0, n)
		for j := i + 1; j < len(tokens) && len(out) < n; j++ {
			if !skip[j] {
				out = append(out, j)
			}
		}
		return out
	}

	// matchSeq checks whether subsequent tokens match seq.next and returns
	// their indices if so.
	matchSeq := func(seq multiTokenSeq) ([]int, bool) {
		idxs := nextIndices(len(seq.next))
		if len(idxs) != len(seq.next) {
			return nil, false
		}
		for k, idx := range idxs {
			if strings.ToLower(tokens[idx].text) != seq.next[k] {
				return nil, false
			}
		}
		return idxs, true
	}

	switch form {
	case config.TypeFormShort:
		// Try multi-token collapse first.
		if seqs, ok := multiTokenTypes[lower]; ok {
			for _, seq := range seqs {
				if idxs, ok := matchSeq(seq); ok {
					return applyCase(seq.shortForm, caseRule), idxs
				}
			}
		}
		// Single-token long → short.
		if short, ok := singleLongToShort[lower]; ok {
			return applyCase(short, caseRule), nil
		}
		return applyCase(text, caseRule), nil

	case config.TypeFormLong:
		// Detect multi-token sequences already in long form (e.g. "timestamp with time zone")
		// so follow-on tokens are consumed and casing is applied uniformly.
		if seqs, ok := multiTokenTypes[lower]; ok {
			for _, seq := range seqs {
				if idxs, ok := matchSeq(seq); ok {
					return applyCase(seq.longForm, caseRule), idxs
				}
			}
		}
		// Single-token short → long (possibly multi-word string).
		if long, ok := singleShortToLong[lower]; ok {
			// timestamptz(3) has no long spelling that keeps the precision after the name
			// ("timestamp with time zone(3)" is not SQL; it is timestamp(3) with time zone).
			if next := nextIndices(1); (lower == "timestamptz" || lower == "timetz") && len(next) == 1 && tokens[next[0]].text == "(" {
				return applyCase(text, caseRule), nil
			}
			return applyCase(long, caseRule), nil
		}
		// Already a long single-token form — just apply casing.
		return applyCase(text, caseRule), nil

	case config.TypeFormLongNoSpace:
		// Multi-token sequences whose long form has spaces → collapse to short.
		if seqs, ok := multiTokenTypes[lower]; ok {
			for _, seq := range seqs {
				if strings.Contains(seq.longForm, " ") {
					if idxs, ok := matchSeq(seq); ok {
						return applyCase(seq.shortForm, caseRule), idxs
					}
				}
			}
		}
		// Single-token aliases: use long form only if it has no space.
		if long, ok := singleShortToLong[lower]; ok {
			if !strings.Contains(long, " ") {
				return applyCase(long, caseRule), nil
			}
			// Long form has space → keep short alias.
			return applyCase(text, caseRule), nil
		}
		return applyCase(text, caseRule), nil
	}

	// TypeFormPreserve: apply casing only.
	return applyCase(text, caseRule), nil
}

// isSymbolicOp returns true for symbolic binary operators where exactly one
// space on each side should be enforced when operator_spacing = normalize.
func isSymbolicOp(text string) bool {
	switch text {
	case "=", "!=", "<>", "<", ">", "<=", ">=", "||", ":=", "=>":
		return true
	}
	return false
}

// isDirectionJoinKeyword returns true for keywords that can precede OUTER/JOIN
// (LEFT, RIGHT, FULL).
func isDirectionJoinKeyword(lower string) bool {
	return lower == "left" || lower == "right" || lower == "full"
}

// isAnyJoinKeyword returns true for keywords that can appear before JOIN.
func isAnyJoinKeyword(lower string) bool {
	switch lower {
	case "inner", "left", "right", "full", "cross", "natural", "outer":
		return true
	}
	return false
}

// safeToUnquote returns true if the inner content of a quoted identifier can
// safely be written without the surrounding double-quotes.
func safeToUnquote(inner string) bool {
	if len(inner) == 0 {
		return false
	}
	// Must be [a-z_][a-z0-9_]* — no uppercase, spaces, or special chars.
	for i, ch := range inner {
		if i == 0 {
			if !((ch >= 'a' && ch <= 'z') || ch == '_') {
				return false
			}
		} else {
			if !((ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '_') {
				return false
			}
		}
	}
	// Reject keywords that are not plain identifiers: reserved ones, and the column-name /
	// type-or-function-name classes ("substring"(…), "left", "float") whose unquoted spelling
	// is parsed (and cased) as the keyword, not as a name.
	res, err := pg_query.Scan(inner)
	if err != nil || len(res.Tokens) == 0 {
		return false
	}
	switch res.Tokens[0].KeywordKind {
	case pg_query.KeywordKind_RESERVED_KEYWORD, pg_query.KeywordKind_COL_NAME_KEYWORD, pg_query.KeywordKind_TYPE_FUNC_NAME_KEYWORD:
		return false
	}
	return true
}

// isKeywordName reports whether name, unquoted, is a PostgreSQL keyword of any kind.
func isKeywordName(name string) bool {
	res, err := pg_query.Scan(name)
	return err == nil && len(res.Tokens) == 1 && res.Tokens[0].KeywordKind != pg_query.KeywordKind_NO_KEYWORD
}

// collapseBlankLines replaces runs of excess consecutive newlines. The text of string
// constants that span lines (also inside dollar-quoted bodies) is data and stays as it is.
func collapseBlankLines(s string, mode config.BlankLinesMode) string {
	var max int
	switch mode {
	case config.BlankLinesMax3:
		max = 4 // 3 blank lines = 4 consecutive \n
	case config.BlankLinesMax2:
		max = 3
	case config.BlankLinesMax1:
		max = 2
	default:
		return s
	}
	target := strings.Repeat("\n", max)
	excess := strings.Repeat("\n", max+1)
	collapse := func(part string) string {
		for strings.Contains(part, excess) {
			part = strings.ReplaceAll(part, excess, target)
		}
		return part
	}
	if !strings.Contains(s, excess) {
		return s
	}
	var b strings.Builder
	cur := 0
	for _, r := range multilineStringRanges(s, 0) {
		b.WriteString(collapse(s[cur:r[0]]))
		b.WriteString(s[r[0]:r[1]])
		cur = r[1]
	}
	b.WriteString(collapse(s[cur:]))
	return b.String()
}

// multilineStringRanges returns the byte ranges (offset by base) of the string constants
// of text that contain a line break — '…', E'…', U&'…', B'…', X'…' — in order, looking inside
// dollar-quoted bodies too. Rules that rewrite whitespace must leave these ranges alone.
func multilineStringRanges(text string, base int) [][2]int {
	if !strings.Contains(text, "\n") || !strings.ContainsAny(text, "'$") {
		return nil
	}
	res, err := pg_query.Scan(text)
	if err != nil {
		return nil
	}
	var out [][2]int
	var toks []rawToken // (for bodyIsSQLOrPLpgSQL)
	for _, t := range res.Tokens {
		toks = append(toks, rawToken{start: int(t.Start), end: int(t.End), text: text[t.Start:t.End]})
	}
	for k, t := range res.Tokens {
		tok := text[t.Start:t.End]
		if !strings.Contains(tok, "\n") {
			continue
		}
		if strings.HasPrefix(tok, "$") {
			// A dollar-quoted text that is not the code of a function or DO block (COMMENT ON … IS $$…$$,
			// a text value) is data as a whole; the body of code holds strings that are data.
			prev := ""
			if k > 0 {
				prev = strings.ToLower(toks[k-1].text)
			}
			doLang := k >= 3 && strings.EqualFold(toks[k-2].text, "language") && strings.EqualFold(toks[k-3].text, "do")
			if !((prev == "as" || prev == "do" || doLang) && bodyIsSQLOrPLpgSQL(toks, k)) {
				out = append(out, [2]int{base + int(t.Start), base + int(t.End)})
				continue
			}
			// $tag$ body $tag$: the strings inside the body are data as well
			if i := strings.IndexByte(tok[1:], '$'); i >= 0 {
				open, closeAt := i+2, len(tok)-(i+2)
				if closeAt > open {
					out = append(out, multilineStringRanges(tok[open:closeAt], base+int(t.Start)+open)...)
				}
			}
			continue
		}
		if i := strings.IndexByte(tok, '\''); i >= 0 && i <= 2 && strings.TrimLeft(tok[:i], "eEbBxXuU&nN") == "" {
			out = append(out, [2]int{base + int(t.Start), base + int(t.End)})
		}
	}
	return out
}

// processGap returns the gap string after applying paren spacing, operator
// spacing, and trailing-whitespace rules. nextText is the text of the token
// that follows the gap; prevText is the text of the token that precedes it.
func processGap(gap, prevText, nextText string, arith bool, cfg *config.Config) string {
	hasNL := strings.ContainsRune(gap, '\n')

	// Paren spacing — only when no newline (keep structural formatting intact).
	if !hasNL && cfg.ParenSpacing != config.ParenSpacingPreserve {
		switch cfg.ParenSpacing {
		case config.ParenSpacingRemove:
			if prevText == "(" || nextText == ")" {
				return ""
			}
		case config.ParenSpacingAdd:
			if prevText == "(" && nextText != ")" {
				return " "
			}
			if nextText == ")" && prevText != "(" {
				return " "
			}
		}
	}

	// Operator spacing — single space or no space around symbolic binary ops.
	if !hasNL && cfg.OperatorSpacing == config.OperatorSpacingNormalize {
		if (isSymbolicOp(prevText) || isSymbolicOp(nextText)) &&
			prevText != "(" && nextText != ")" && nextText != "," && nextText != ";" && prevText != "." && nextText != "." {
			// (an operator that is a name — ORDER BY a USING <, OPERATOR(pg_catalog.<) — keeps what
			// the punctuation beside it says)
			return " "
		}
	}
	if !hasNL && cfg.OperatorSpacing == config.OperatorSpacingCompact {
		// (glue is judged on the operators as they are written: != becomes <> with inequality_op: ansi, …)
		finalOp := func(t string) string {
			switch {
			case cfg.InequalityOp == config.InequalityANSI && t == "!=":
				return "<>"
			case cfg.InequalityOp == config.InequalityC && t == "<>":
				return "!="
			}
			return t
		}
		if (isSymbolicOp(prevText) || isSymbolicOp(nextText)) &&
			prevText != "(" && nextText != ")" && glueSafe(finalOp(prevText), finalOp(nextText)) {
			return ""
		}
	}

	// Arithmetic spacing — only around operators proven binary (see binaryArithmetic). A line break
	// beside such an operator goes too in compact mode (unless a comment is next to it): other rules
	// (CAST(x AS t) → x::t) can join the lines, and the blank they leave would be removed only by the
	// next run.
	if arith && hasNL && cfg.ArithmeticSpacing == config.ArithmeticSpacingCompact &&
		!isCommentText(prevText) && !isCommentText(nextText) && isOnlyWhitespace(gap) {
		hasNL = false
	}
	if arith && !hasNL {
		switch cfg.ArithmeticSpacing {
		case config.ArithmeticSpacingNormalize:
			return " "
		case config.ArithmeticSpacingCompact:
			// "a - -1" must not become "a--1" (a line comment).
			// "a - -1" → a--1 would be a comment and "0 % +a" → 0%+a one operator:
			// never glue a symbol onto the operator that precedes it.
			if len(prevText) == 1 && strings.Contains("+-*/%^", prevText) && nextText != "" && strings.ContainsRune("+-*/%^<>=~!@#&|`?", rune(nextText[0])) {
				return " "
			}
			return ""
		}
	}

	// Comma spacing normalization / compaction.
	if !hasNL && cfg.CommaSpacing == config.CommaSpacingNormalize {
		if prevText == "," {
			return " "
		}
		if nextText == "," {
			return ""
		}
	}
	if !hasNL && cfg.CommaSpacing == config.CommaSpacingCompact {
		if prevText == "," || nextText == "," {
			return ""
		}
	}

	// Inline spacing — collapse 2+ horizontal spaces to a single space.
	// Only fires when no other rule returned early, and only on pure-whitespace gaps.
	if !hasNL && cfg.InlineSpacing == config.InlineSpacingNormalize {
		if isOnlyHorizontalWS(gap) && gap != "" && gap != " " { // (a single tab between tokens too)
			gap = " "
		}
	}

	// Trailing whitespace stripping.
	if cfg.TrailingWhitespace == config.TrailingWSStrip {
		gap = stripTrailingFromGap(gap)
	}
	return gap
}

// isOnlyHorizontalWS reports whether s contains only spaces and tabs.
func isOnlyHorizontalWS(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] != ' ' && s[i] != '\t' {
			return false
		}
	}
	return true
}

// semicolonInsertPos returns the byte index in s after which ';' should be
// inserted when adding a trailing semicolon. It skips blank lines and
// comment-only lines (trimmed prefix "--") at the end, and for the first
// real line it inserts before any inline "-- comment" suffix.
// tabIndentAfterLayout reports whether the layout turns the indentation of the lines into tabs
// (indent.type: tab with indent.normalize: change) after the comments were aligned: columns of
// lines indented with tabs cannot be told, so they are not aligned in the first place.
func tabIndentAfterLayout(cfg *config.Config) bool {
	return cfg.Layout.Indent.Type == config.IndentTypeTab && cfg.Layout.Indent.Normalize == config.IndentNormalizeChange
}

// blankIndentTabWidth is the width of a tab when the layout turns the tabs of the lines into
// blanks (indent.type: spaces with indent.normalize: change) after the comments were aligned, else 0.
func blankIndentTabWidth(cfg *config.Config) int {
	if cfg.Layout.Indent.Type != config.IndentTypeTab && cfg.Layout.Indent.Normalize == config.IndentNormalizeChange {
		if n := cfg.Layout.Indent.Size; n > 0 {
			return n
		}
		return 4
	}
	return 0
}

// removeFinalSemicolon takes off the ';' that ends the text (the last token that is code), with the
// blanks before it; a comment after it stays.
func removeFinalSemicolon(s string) string {
	res, err := pg_query.Scan(s)
	if err != nil {
		return s
	}
	for k := len(res.Tokens) - 1; k >= 0; k-- {
		t := res.Tokens[k]
		text := s[t.Start:t.End]
		if strings.HasPrefix(text, "--") || strings.HasPrefix(text, "/*") {
			continue
		}
		if text != ";" {
			return s
		}
		start := int(t.Start)
		for start > 0 && (s[start-1] == ' ' || s[start-1] == '\t') {
			start--
		}
		return s[:start] + s[t.End:]
	}
	return s
}

func semicolonInsertPos(s string) int {
	// Trailing comments of either kind (-- and /* … */, also over several lines) are skipped:
	// the ';' goes right after the last token that is code.
	if res, err := pg_query.Scan(s); err == nil {
		for k := len(res.Tokens) - 1; k >= 0; k-- {
			t := res.Tokens[k]
			text := s[t.Start:t.End]
			if strings.HasPrefix(text, "--") || strings.HasPrefix(text, "/*") {
				continue
			}
			if text == ";" {
				return -1
			}
			return int(t.End) - 1
		}
		return -1
	}
	// Build line start/end offsets (end excludes the '\n').
	type span struct{ start, end int }
	var lines []span
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == '\n' {
			lines = append(lines, span{start, i})
			start = i + 1
		}
	}
	for i := len(lines) - 1; i >= 0; i-- {
		li := lines[i]
		lineStr := s[li.start:li.end]
		trimmedLine := strings.TrimSpace(lineStr)
		if trimmedLine == "" {
			continue
		}
		if strings.HasPrefix(trimmedLine, "--") {
			continue
		}
		// Real content line — find end of content, stopping before inline comment.
		commentOff := strings.Index(lineStr, "--")
		contentEnd := li.end
		if commentOff >= 0 {
			contentEnd = li.start + commentOff
		}
		for j := contentEnd - 1; j >= li.start; j-- {
			if s[j] != ' ' && s[j] != '\t' {
				if s[j] == ';' {
					return -1 // already has a semicolon before the comment
				}
				return j
			}
		}
	}
	return -1
}

// stripTrailingFromGap removes whitespace immediately before each newline
// within a gap string. String literal content never passes through here.
func stripTrailingFromGap(s string) string {
	if !strings.ContainsAny(s, " \t\r\n") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	pending := ""
	for _, ch := range s {
		switch {
		case ch == '\n':
			pending = ""
			b.WriteByte('\n')
		case ch == ' ' || ch == '\t' || ch == '\r':
			pending += string(ch)
		default:
			b.WriteString(pending)
			pending = ""
			b.WriteRune(ch)
		}
	}
	b.WriteString(pending) // trailing content not followed by \n is kept
	return b.String()
}

// formatTolerant normalizes casing and spacing in sql according to cfg without
// checking that sql is valid PostgreSQL (unparsable input falls back to token-level
// rules). Format wraps it with the syntax_errors policy; internal callers that format
// fragments (PL/pgSQL bodies, extracted statements) use it directly.
// Whitespace, comments, and string literals are always preserved verbatim.
func formatTolerant(sql string, cfg *config.Config) (string, error) {
	return formatParsed(sql, cfg, nil)
}

// formatBody is formatTolerant for a PL/pgSQL body: vars are the names of the function's parameters,
// which the body's expressions use (see resolvePLpgSQLVars).
func formatBody(sql string, cfg *config.Config, vars map[string]bool) (string, error) {
	return formatParsedVars(sql, cfg, nil, vars)
}

// formatParsed is formatTolerant with an optional parse result for sql, so callers that
// already parsed the input (to check its syntax) do not make PostgreSQL parse it again.
func formatParsed(sql string, cfg *config.Config, parsed *pg_query.ParseResult) (string, error) {
	return formatParsedVars(sql, cfg, parsed, nil)
}

func formatParsedVars(sql string, cfg *config.Config, parsed *pg_query.ParseResult, vars map[string]bool) (string, error) {
	if strings.TrimSpace(sql) == "" {
		return sql, nil
	}

	scanResult, err := pg_query.Scan(sql)
	if err != nil {
		return "", fmt.Errorf("scan: %w", err)
	}

	// Collect tokens into a slice so we can do index-based lookahead/lookbehind.
	tokens := make([]rawToken, len(scanResult.Tokens))
	for i, t := range scanResult.Tokens {
		tokens[i] = rawToken{
			start: int(t.Start),
			end:   int(t.End),
			text:  sql[t.Start:t.End],
			kind:  t.KeywordKind,
		}
	}

	pos := collectASTPositions(sql, parsed)
	if cfg.Nested && parsed == nil && len(pos.pendingAliases)+len(pos.tables)+len(pos.columns)+len(pos.functions)+len(pos.schemas) == 0 {
		// a PL/pgSQL body is no SQL text as a whole: its statements are parsed one by one
		pos = collectBodyPositions(sql, tokens)
	}
	pos.resolveAliases(tokens)
	resolveSubselectAliases(tokens, pos)
	resolveFunctionAliases(tokens, pos)
	resolveOnConflictCols(tokens, pos)
	resolveAliasColumns(tokens, pos)
	resolveRoutineNames(tokens, pos)
	resolveIndexCols(tokens, pos)
	resolveFunctionParams(tokens, pos)
	resolveDDLColumns(tokens, pos)
	resolvePLpgSQLVars(tokens, pos, vars)
	resolveXMLNames(tokens, pos)
	if cfg.Nested {
		resolveBodyAliases(tokens, pos)
	}

	// ── Pre-processing: compute skip and skipGapBefore maps ──────────────────
	//
	// skip[i]          — don't write token i or the gap before it
	// skipGapBefore[i] — don't write the gap before token i (but write the token)
	// skip[i]        — skip both the gap before token i and token i itself (semicolons)
	// dropText[i]    — write the gap before token i, but write nothing for the token text
	// suppressGap[i] — do not write the gap before token i
	skip := make(map[int]bool)
	dropText := make(map[int]bool)
	suppressGap := make(map[int]bool)
	replaceWith := make(map[int]string)
	insertBefore := make(map[int]string)
	appendASCAtEnd := false

	// nextVisible returns the first index after i that is not in skip or dropText, or -1.
	nextVisible := func(i int) int {
		for j := i + 1; j < len(tokens); j++ {
			if !skip[j] {
				return j
			}
		}
		return -1
	}

	// (semicolons: remove takes off the final ';' of the whole text, in finishWholeText: the ones
	// between statements and inside PL/pgSQL bodies are needed.)

	// JOIN form: short — erase INNER before JOIN; erase OUTER between direction+JOIN.
	if cfg.JoinForm == config.JoinFormShort {
		for i, tok := range tokens {
			if skip[i] {
				continue
			}
			lower := strings.ToLower(tok.text)
			if lower == "inner" {
				if j := nextVisible(i); j >= 0 && strings.ToLower(tokens[j].text) == "join" {
					dropText[i] = true
					suppressGap[j] = true
				}
			}
			if lower == "outer" {
				prevLower := ""
				for k := i - 1; k >= 0; k-- {
					if !skip[k] {
						prevLower = strings.ToLower(tokens[k].text)
						break
					}
				}
				if isDirectionJoinKeyword(prevLower) {
					if j := nextVisible(i); j >= 0 && strings.ToLower(tokens[j].text) == "join" {
						dropText[i] = true
						suppressGap[j] = true
					}
				}
			}
		}
	}

	// AS removal: erase AS tokens followed by an alias.
	if cfg.Aliases.As == config.AliasAsRemove {
		for i, tok := range tokens {
			if skip[i] {
				continue
			}
			if strings.ToLower(tok.text) == "as" && tok.kind == pg_query.KeywordKind_RESERVED_KEYWORD && !isInsertTargetAs(tokens, i) {
				if j := nextVisible(i); j >= 0 && pos.aliases[tokens[j].start] && canOmitAs(tokens[j]) {
					dropText[i] = true
					suppressGap[j] = true
				}
			}
		}
	}

	// order_asc remove: drop explicit ASC tokens.
	if cfg.OrderAsc == config.OrderAscRemove {
		for i, tok := range tokens {
			if strings.ToLower(tok.text) == "asc" && tok.kind == pg_query.KeywordKind_RESERVED_KEYWORD {
				dropText[i] = true
				suppressGap[i] = true // don't write gap before ASC either
				// no suppressGap on next — comma/next token gets natural spacing
			}
		}
	}

	// order_asc add: insert ASC after sort expressions with no explicit direction.
	if cfg.OrderAsc == config.OrderAscAdd {
		termKeywords := map[string]bool{
			"limit": true, "having": true, "union": true, "intersect": true,
			"except": true, "for": true, "lock": true, "fetch": true, "offset": true,
		}
		for _, loc := range pos.sortByDefaultLocs {
			startIdx := -1
			for i, tok := range tokens {
				if tok.start == loc {
					startIdx = i
					break
				}
			}
			if startIdx < 0 {
				continue
			}
			depth := 0
			terminatorIdx := -1
			for j := startIdx + 1; j < len(tokens); j++ {
				if skip[j] || dropText[j] {
					continue
				}
				t := tokens[j].text
				tl := strings.ToLower(t)
				if t == "(" {
					depth++
				} else if t == ")" {
					if depth == 0 {
						terminatorIdx = j
						break
					}
					depth--
				} else if depth == 0 && t == "," {
					terminatorIdx = j
					break
				} else if depth == 0 && t == ";" {
					// statement boundary — insert ASC before the semicolon
					terminatorIdx = j
					break
				} else if depth == 0 && termKeywords[tl] {
					terminatorIdx = j
					break
				}
			}
			if terminatorIdx >= 0 {
				tok := tokens[terminatorIdx].text
				if tok == ";" {
					// insert ASC before the semicolon without suppressing the gap
					insertBefore[terminatorIdx] = " ASC"
				} else {
					insertBefore[terminatorIdx] = " ASC"
					suppressGap[terminatorIdx] = true
				}
			} else {
				appendASCAtEnd = true
			}
		}
	}

	// cast_style operator: convert CAST(x AS t) → x::t
	if cfg.CastStyle == config.CastStyleOperator {
		for _, loc := range pos.castFuncLocs {
			castIdx := -1
			for i, tok := range tokens {
				if tok.start == loc {
					castIdx = i
					break
				}
			}
			if castIdx < 0 {
				continue
			}
			openIdx := nextVisible(castIdx)
			if openIdx < 0 || tokens[openIdx].text != "(" {
				continue
			}
			depth := 1
			asIdx := -1
			closeIdx := -1
			for j := openIdx + 1; j < len(tokens); j++ {
				if skip[j] || dropText[j] {
					continue
				}
				t := tokens[j].text
				if t == "(" {
					depth++
				} else if t == ")" {
					depth--
					if depth == 0 {
						closeIdx = j
						break
					}
				} else if depth == 1 && strings.ToLower(t) == "as" && tokens[j].kind == pg_query.KeywordKind_RESERVED_KEYWORD && asIdx < 0 {
					asIdx = j
				}
			}
			if asIdx < 0 || closeIdx < 0 {
				continue
			}
			typeIdx := -1
			for j := asIdx + 1; j < closeIdx; j++ {
				if !skip[j] && !dropText[j] {
					typeIdx = j
					break
				}
			}
			// Find first argument token inside CAST( ... ) to suppress its gap.
			// When CAST is dropText, the gap before CAST is already written; without
			// suppressing the first arg's gap, processGap sees prevText="<op>" and
			// adds a second space (double space bug).
			firstArgIdx := -1
			for j := openIdx + 1; j < asIdx; j++ {
				if !skip[j] && !dropText[j] {
					firstArgIdx = j
					break
				}
			}
			dropText[castIdx] = true
			dropText[openIdx] = true
			suppressGap[openIdx] = true
			if firstArgIdx >= 0 {
				suppressGap[firstArgIdx] = true
			}
			suppressGap[asIdx] = true
			replaceWith[asIdx] = "::"
			if typeIdx >= 0 {
				suppressGap[typeIdx] = true
			}
			dropText[closeIdx] = true
			suppressGap[closeIdx] = true
		}
	}

	// matchParen returns the index of the ")" that closes the "(" at index k, or -1.
	matchParen := func(k int) int {
		depth := 0
		for m := k; m < len(tokens); m++ {
			switch tokens[m].text {
			case "(":
				depth++
			case ")":
				depth--
				if depth == 0 {
					return m
				}
			}
		}
		return -1
	}
	isQueryStart := func(k int) bool {
		if k < 0 || k >= len(tokens) {
			return false
		}
		switch strings.ToLower(tokens[k].text) {
		case "select", "with", "values", "table":
			return true
		}
		return false
	}

	// not_in not_equals_all: x NOT IN (a, b) → x <> ALL (ARRAY[a, b]);
	// x NOT IN (SELECT …) → x <> ALL (SELECT …). "<> ALL (a, b)" would not be valid SQL.
	if cfg.NotIn == config.NotInNotEqualsAll {
		for i, tok := range tokens {
			if strings.ToLower(tok.text) == "not" && tok.kind == pg_query.KeywordKind_RESERVED_KEYWORD {
				j := nextVisible(i)
				if j < 0 || strings.ToLower(tokens[j].text) != "in" || tokens[j].kind != pg_query.KeywordKind_RESERVED_KEYWORD {
					continue
				}
				open := nextVisible(j)
				if open < 0 || tokens[open].text != "(" {
					continue
				}
				closeIdx := matchParen(open)
				if closeIdx < 0 {
					continue
				}
				if !isQueryStart(nextVisible(open)) {
					replaceWith[open] = "(ARRAY["
					replaceWith[closeIdx] = "])"
				}
				replaceWith[i] = "<>"
				replaceWith[j] = "ALL"
			}
		}
	}

	// not_in not_in: x <> ALL (ARRAY[a, b]) → x NOT IN (a, b); x <> ALL (SELECT …) →
	// x NOT IN (SELECT …). Anything else (<> ALL (array_column)) has no NOT IN form
	// and is left alone.
	if cfg.NotIn == config.NotInNotIn {
		for i, tok := range tokens {
			tl := strings.ToLower(tok.text)
			if (tl == "<>" || tl == "!=") && tok.kind == pg_query.KeywordKind_NO_KEYWORD {
				j := nextVisible(i)
				if j < 0 || strings.ToLower(tokens[j].text) != "all" || tokens[j].kind != pg_query.KeywordKind_RESERVED_KEYWORD {
					continue
				}
				open := nextVisible(j)
				if open < 0 || tokens[open].text != "(" {
					continue
				}
				closeIdx := matchParen(open)
				if closeIdx < 0 {
					continue
				}
				first := nextVisible(open)
				switch {
				case isQueryStart(first):
					// subquery: parentheses stay
				case first >= 0 && strings.ToLower(tokens[first].text) == "array" && nextVisible(first) >= 0 &&
					tokens[nextVisible(first)].text == "[" && closeIdx >= 2 && tokens[closeIdx-1].text == "]" &&
					matchBracket(tokens, nextVisible(first)) == closeIdx-1:
					br := nextVisible(first)
					dropText[first] = true
					suppressGap[first] = true
					dropText[br] = true
					suppressGap[br] = true
					dropText[closeIdx-1] = true
					suppressGap[closeIdx-1] = true
					if nb := nextVisible(br); nb >= 0 && nb != closeIdx-1 {
						suppressGap[nb] = true
					}
				default:
					continue
				}
				replaceWith[i] = "NOT IN"
				dropText[j] = true
				suppressGap[j] = true
			}
		}
	}

	// schema_qualification remove_public: drop "public" + "." prefix.
	if cfg.SchemaQual == config.SchemaQualRemovePublic {
		for i, tok := range tokens {
			if skip[i] || dropText[i] {
				continue
			}
			if pos.schemas[tok.start] && strings.ToLower(tok.text) == "public" {
				dotIdx := nextVisible(i)
				if dotIdx < 0 || tokens[dotIdx].text != "." {
					continue
				}
				nameIdx := nextVisible(dotIdx)
				dropText[i] = true
				suppressGap[dotIdx] = true
				dropText[dotIdx] = true
				if nameIdx >= 0 {
					suppressGap[nameIdx] = true
				}
			}
		}
	}

	// ── Main loop ─────────────────────────────────────────────────────────────
	var buf strings.Builder
	buf.Grow(len(sql) + len(sql)/4)

	cur := 0
	prevTokenText := ""    // raw text of previous written token
	prevIsCall := false    // previous token is a function name (for call_paren_spacing)
	prevIsKeyword := false // previous token is a keyword / operator word (for keyword_paren_spacing)
	arithStart, arithEnd := map[int]bool{}, map[int]bool{}
	if cfg.ArithmeticSpacing == config.ArithmeticSpacingNormalize || cfg.ArithmeticSpacing == config.ArithmeticSpacingCompact {
		arithStart, arithEnd = binaryArithmetic(tokens)
	}

	for i, tok := range tokens {
		if skip[i] {
			cur = tok.end
			continue
		}

		// Write the inter-token gap (whitespace / comments).
		// Always call processGap even for empty gaps so spacing rules can add spaces.
		{
			gap := ""
			if tok.start > cur {
				gap = sql[cur:tok.start]
			}
			if !suppressGap[i] {
				g := processGap(gap, prevTokenText, tok.text, arithStart[tok.start] || arithEnd[cur], cfg)
				if prevIsCall && tok.text == "(" && !strings.Contains(g, "/*") && !strings.Contains(g, "--") {
					switch cfg.CallParenSpacing {
					case config.ParenSpacingAdd:
						g = " "
					case config.ParenSpacingRemove:
						g = ""
					}
				}
				if cfg.KeywordParenSpacing != config.ParenSpacingPreserve && cfg.KeywordParenSpacing != "" &&
					!strings.Contains(g, "/*") && !strings.Contains(g, "--") {
					switch {
					case prevIsKeyword && tok.text == "(": // IF (a), AND (b), IN (1, 2), VALUES (…)
						// (add keeps a line break: the keyword and its "(" are already apart)
						if cfg.KeywordParenSpacing == config.ParenSpacingAdd {
							if !strings.Contains(g, "\n") {
								g = " "
							}
						} else {
							g = ""
						}
					case cfg.KeywordParenSpacing == config.ParenSpacingAdd && prevTokenText == ")" && !strings.Contains(g, "\n") &&
						startsKeywordWord(tokens, i, pos):
						g = " " // (a) AND (b), IF (a) THEN
					}
				}
				buf.WriteString(g)
			}
			// "a"where: once the quotes are removed the two words would fuse.
			if gap == "" && strings.HasPrefix(prevTokenText, `"`) && tok.text != "" &&
				(tok.text[0] == '_' || tok.text[0] >= 'a' && tok.text[0] <= 'z' || tok.text[0] >= 'A' && tok.text[0] <= 'Z' || tok.text[0] >= '0' && tok.text[0] <= '9') {
				ensureSeparator(&buf)
			}
		}

		// insertBefore: emit string after gap but before token.
		if s, ok := insertBefore[i]; ok {
			buf.WriteString(s)
		}

		// dropText: write gap but not the token; advance cur and continue.
		if dropText[i] {
			cur = tok.end
			continue
		}

		tokenText := tok.text
		lower := strings.ToLower(tokenText)

		// ── Quoted identifier removal ─────────────────────────────────────────
		if cfg.QuotedIdents == config.QuotedIdentRemoveSafe &&
			len(tokenText) >= 2 && tokenText[0] == '"' && tokenText[len(tokenText)-1] == '"' {
			inner := tokenText[1 : len(tokenText)-1]
			located := pos.aliases[tok.start] || pos.columns[tok.start] || pos.tables[tok.start] || pos.schemas[tok.start]
			// A keyword-spelled name ("year", "name") the AST did not place (inside a PL/pgSQL
			// expression, say), or spelled like a type or a literal ("date", "text"), would be read — and
			// cased — as that on the next run: its quotes stay.
			// (a name after AS that no tree located — the name in XMLFOREST(a AS "x") — keeps its quotes: the next run would
			// read it as a column and case it)
			if safeToUnquote(inner) && (located || !isKeywordName(inner) && !dataTypeNames[inner] && !literalNames[inner] && !strings.EqualFold(prevTokenText, "as")) {
				// select"a" → SELECT a, not SELECTa: keep a separator after a word.
				// An unquoted alias still follows aliases.as / aliases.case; other
				// identifiers follow their own section's case rule when the AST located them.
				switch {
				case pos.aliases[tok.start]:
					if cfg.Aliases.As == config.AliasAsAdd && strings.ToLower(prevTokenText) != "as" {
						ensureSeparator(&buf)
						buf.WriteString(applyCase("as", cfg.ReservedKeywords.Case))
						buf.WriteByte(' ')
					}
					inner = applyCaseEx(inner, cfg.Aliases.Case, cfg.Aliases.Exceptions)
				case pos.columns[tok.start]:
					inner = applyCaseEx(inner, cfg.Columns.Case, cfg.Columns.Exceptions)
				case pos.tables[tok.start]:
					inner = applyCaseEx(inner, cfg.Tables.Case, cfg.Tables.Exceptions)
				case pos.schemas[tok.start]:
					inner = applyCaseEx(inner, cfg.Schemas.Case, cfg.Schemas.Exceptions)
				case prevTokenText == "." && !pos.functions[tok.start]:
					// (a name after a dot that no tree located — COMMENT ON COLUMN t."c" — is read as a
					// column once it is unquoted)
					inner = applyCaseEx(inner, cfg.Columns.Case, cfg.Columns.Exceptions)
				}
				ensureSeparator(&buf)
				buf.WriteString(inner)
				prevTokenText = tokenText
				prevIsCall, prevIsKeyword = false, false
				cur = tok.end
				continue
			}
		}

		// ── Token classification ──────────────────────────────────────────────
		var class tokenClass
		switch {
		case literalNames[lower]:
			class = classLiteral
		case operatorKeywords[lower] && tok.kind != pg_query.KeywordKind_NO_KEYWORD:
			class = classOperator
		case pos.systemFunctions[tok.start]:
			class = classSystemFunction
		case pos.conditionalFunctions[tok.start]:
			class = classConditionalFunction
		case pos.functions[tok.start]:
			class = classFunction
		case pos.schemas[tok.start]:
			class = classSchema
		case pos.tables[tok.start]:
			class = classTable
		case pos.aliases[tok.start]:
			class = classAlias
		case pos.columns[tok.start]:
			class = classColumn
		case prevTokenText == "." && pos.schemas[tok.start]:
			class = classSchema
		case prevTokenText == "." && pos.tables[tok.start]:
			class = classTable
		case prevTokenText == "." && pos.functions[tok.start]:
			class = classFunction
		case prevTokenText == ".":
			class = classColumn
		default:
			class = classify(tok.kind, tokenText)
			// "trigger" is a pseudo-type only after RETURNS/::/etc.; after these
			// DDL words it is the TRIGGER keyword (CREATE TRIGGER, DROP TRIGGER, ...).
			if class == classDataType && lower == "trigger" && isTriggerKeywordContext(prevTokenText) {
				class = classKeyword
			}
			if class == classOther && dataTypeNames[lower] {
				class = classDataType
			}
		}

		// PL/pgSQL / DML special pseudo-references (NEW, OLD, EXCLUDED, FOUND, TG_*)
		// override the class determined above when plpgsql_variables.case is configured.
		// Schema-, table-, and alias-level AST identifications still win, but column-level
		// does not: NEW.col, OLD.col and EXCLUDED.col are all ColumnRef in the AST yet
		// semantically they are pseudo-table references, not real columns.
		if cfg.PLpgSQLVariables.Case != "" && plpgsqlVariableNames[lower] &&
			class != classSchema && class != classTable && class != classAlias {
			class = classPlpgSQLVariable
		}

		// PL/pgSQL-only statement keywords (RAISE, PERFORM, ELSIF, LOOP, …) that the
		// scanner classifies as NO_KEYWORD. Functions are excluded from the override
		// because some names (reverse, notice, …) coincide with real PostgreSQL functions.
		// The RAISE levels (debug, info, notice, warning) are keywords only right after RAISE: anywhere
		// else they are names (a record variable `info`, `info.client_id`).
		if cfg.PLpgSQLKeywords.Case != "" && plpgsqlKeywordNames[lower] && prevTokenText != "." &&
			class != classSchema && class != classTable && class != classAlias && class != classFunction &&
			(!raiseLevelNames[lower] || strings.EqualFold(prevTokenText, "raise")) &&
			(lower != "query" || (strings.EqualFold(prevTokenText, "return") && i+1 < len(tokens) && tokens[i+1].text != ";")) {
			class = classPlpgSQLKeyword
		}

		// replaceWith: if this token has a replacement, write it and skip normal output.
		if s, ok := replaceWith[i]; ok {
			buf.WriteString(s)
			prevTokenText = tokenText
			// (a replacement that is a keyword phrase, `NOT IN` for `<> ALL`, is the keyword before its "(")
			prevIsCall, prevIsKeyword = false, s != "" && strings.Trim(s, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz ") == ""
			cur = tok.end
			continue
		}

		// ── AS insertion (add mode) ───────────────────────────────────────────
		if class == classAlias && cfg.Aliases.As == config.AliasAsAdd {
			if strings.ToLower(prevTokenText) != "as" {
				collapsePaddingBeforeAS(&buf)
				ensureSeparator(&buf)
				buf.WriteString(applyCase("as", cfg.ReservedKeywords.Case))
				buf.WriteByte(' ')
			}
		}

		// ── JOIN form: long — insert INNER/OUTER before JOIN ──────────────────
		if lower == "join" && cfg.JoinForm == config.JoinFormLong && !joinIsOptionName(tokens, i) {
			prevLower := strings.ToLower(prevTokenText)
			// INNER / OUTER are ordinary keywords, not reserved ones: take the case
			// of the neighbouring JOIN token, or the next pass recases them.
			kw := cfg.Keywords.Case
			if tok.kind == pg_query.KeywordKind_RESERVED_KEYWORD {
				kw = cfg.ReservedKeywords.Case
			}
			switch {
			case isDirectionJoinKeyword(prevLower):
				// LEFT/RIGHT/FULL → insert OUTER
				ensureSeparator(&buf)
				buf.WriteString(applyCase("outer", kw))
				buf.WriteByte(' ')
			case !isAnyJoinKeyword(prevLower):
				// bare JOIN → insert INNER
				ensureSeparator(&buf)
				buf.WriteString(applyCase("inner", kw))
				buf.WriteByte(' ')
			}
		}

		// ── Token output ──────────────────────────────────────────────────────
		switch class {
		case classReservedKeyword:
			buf.WriteString(applyCaseEx(tokenText, cfg.ReservedKeywords.Case, cfg.ReservedKeywords.Exceptions))
		case classKeyword:
			buf.WriteString(applyCaseEx(tokenText, cfg.Keywords.Case, cfg.Keywords.Exceptions))
		case classDataType:
			if cfg.DataTypes.Form == config.TypeFormPreserve {
				buf.WriteString(applyCaseEx(tokenText, cfg.DataTypes.Case, cfg.DataTypes.Exceptions))
			} else {
				out, consumed := normalizeType(tokens, i, skip, cfg.DataTypes.Form, cfg.DataTypes.Case)
				// Apply exceptions after normalization (normalizeType returns the canonical form).
				out = applyCaseEx(out, cfg.DataTypes.Case, cfg.DataTypes.Exceptions)
				buf.WriteString(out)
				for _, idx := range consumed {
					skip[idx] = true
				}
			}
		case classLiteral:
			buf.WriteString(applyCaseEx(tokenText, cfg.Literals.Case, cfg.Literals.Exceptions))
		case classOperator:
			buf.WriteString(applyCaseEx(tokenText, cfg.Operators.Case, cfg.Operators.Exceptions))
		case classSchema:
			buf.WriteString(applyCaseEx(tokenText, cfg.Schemas.Case, cfg.Schemas.Exceptions))
		case classTable:
			buf.WriteString(applyCaseEx(tokenText, cfg.Tables.Case, cfg.Tables.Exceptions))
		case classFunction:
			if cfg.BuiltinFunctions.Case != "" && isBuiltinFunction(tokenText) {
				buf.WriteString(applyCaseEx(tokenText, cfg.BuiltinFunctions.Case, cfg.BuiltinFunctions.Exceptions))
			} else {
				buf.WriteString(applyCaseEx(tokenText, cfg.Functions.Case, cfg.Functions.Exceptions))
			}
		case classConditionalFunction:
			buf.WriteString(applyCaseEx(tokenText, cfg.ConditionalFunctions.Case, cfg.ConditionalFunctions.Exceptions))
		case classSystemFunction:
			buf.WriteString(applyCaseEx(tokenText, cfg.SystemFunctions.Case, cfg.SystemFunctions.Exceptions))
		case classAlias:
			buf.WriteString(applyCaseEx(tokenText, cfg.Aliases.Case, cfg.Aliases.Exceptions))
		case classColumn:
			buf.WriteString(applyCaseEx(tokenText, cfg.Columns.Case, cfg.Columns.Exceptions))
		case classPlpgSQLVariable:
			buf.WriteString(applyCaseEx(tokenText, cfg.PLpgSQLVariables.Case, cfg.PLpgSQLVariables.Exceptions))
		case classPlpgSQLKeyword:
			buf.WriteString(applyCaseEx(tokenText, cfg.PLpgSQLKeywords.Case, cfg.PLpgSQLKeywords.Exceptions))
		default:
			// Inequality operator normalization.
			switch {
			case tokenText == "<>" && cfg.InequalityOp == config.InequalityC:
				buf.WriteString("!=")
			case tokenText == "!=" && cfg.InequalityOp == config.InequalityANSI:
				buf.WriteString("<>")
			case isDollarQuoted(tokenText):
				// Only format dollar-quoted bodies that are function/procedure code
				// (preceded by AS) or anonymous DO blocks. In all other contexts
				// (COMMENT ON … IS $$…$$, string values in expressions, etc.) the
				// content is arbitrary text and must not be re-cased or reformatted.
				prevLower := strings.ToLower(prevTokenText)
				// (DO LANGUAGE plpgsql $$ … $$: the language comes between DO and the body)
				doLang := i >= 3 && strings.EqualFold(tokens[i-2].text, "language") && strings.EqualFold(tokens[i-3].text, "do")
				if (prevLower == "as" || prevLower == "do" || doLang) && bodyIsSQLOrPLpgSQL(tokens, i) {
					out, err := formatDollarQuoted(tokenText, cfg, pos.funcParams)
					if err != nil {
						buf.WriteString(tokenText)
					} else {
						buf.WriteString(out)
					}
				} else {
					buf.WriteString(tokenText)
				}
			case cfg.Comments.SpaceAfterMarker == config.CommentSpaceAdd && strings.HasPrefix(tokenText, "--"):
				buf.WriteString(spaceAfterLineCommentMarker(tokenText))
			default:
				buf.WriteString(tokenText)
			}
		}

		prevTokenText = tokenText
		prevIsCall = pos.functions[tok.start] || pos.conditionalFunctions[tok.start]
		prevIsKeyword = !prevIsCall && isKeywordClass(class) && !parenAfterKeywordIsCall[lower] && !callOnlyKeywords[lower]
		cur = tok.end
	}

	if cur < len(sql) {
		buf.WriteString(processGap(sql[cur:], prevTokenText, "", false, cfg))
	}

	result := buf.String()

	// ── Post-processing ───────────────────────────────────────────────────────

	if appendASCAtEnd {
		result = strings.TrimRight(result, " \t\r") + " ASC"
	}

	if cfg.TrailingWhitespace == config.TrailingWSStrip {
		result = strings.TrimRight(result, " \t\r")
	}

	result = finishWholeText(result, cfg, true)

	// layout post-processing
	if !layout.IsNoop(&cfg.Layout) {
		result, err = layout.Apply(result, layoutConfig(cfg))
		if err != nil {
			return result, fmt.Errorf("layout: %w", err)
		}
	}

	if (cfg.CommaSpacing == config.CommaSpacingCompact || cfg.OperatorSpacing == config.OperatorSpacingCompact) && !layout.IsNoop(&cfg.Layout) {
		result = compactGaps(result, cfg.CommaSpacing == config.CommaSpacingCompact, cfg.OperatorSpacing == config.OperatorSpacingCompact)
	}
	if cfg.Comments.AlignTrailing == config.CommentAlignAlign {
		result = alignTrailingComments(result, tabIndentAfterLayout(cfg), blankIndentTabWidth(cfg))
	}
	if cfg.Comments.AlignTrailing == config.CommentAlignNone {
		result = collapseTrailingCommentGap(result)
	}

	return result, nil
}

// collectASTPositions parses sql and returns byte offsets for each identifier
// category. On parse error returns empty maps (scanner-only fallback).
func collectASTPositions(sql string, result *pg_query.ParseResult) *astPositions {
	pos := newASTPositions()
	pos.sql = sql
	if result == nil {
		var err error
		result, err = pg_query.Parse(sql)
		if err != nil {
			return pos
		}
	}
	for _, stmt := range result.Stmts {
		if stmt.Stmt != nil {
			walkNode(stmt.Stmt, pos)
		}
	}
	return pos
}

// bodyStatementStarters are the words that open an SQL statement of a PL/pgSQL body.
var bodyStatementStarters = map[string]bool{
	"select": true, "insert": true, "update": true, "delete": true, "with": true, "merge": true,
	"create": true, "alter": true, "drop": true, "grant": true, "revoke": true, "truncate": true,
	"comment": true, "lock": true, "refresh": true, "reindex": true, "explain": true, "copy": true, "prepare": true, "perform": true,
}

// collectBodyPositions is collectASTPositions for the text of a PL/pgSQL body, which PostgreSQL
// does not parse as a whole: every SQL statement in it (from its first word to the ';', or to the
// LOOP of a FOR … IN query) is parsed on its own, and what the trees say is shifted to the
// statement's place in the body. A statement that does not parse (it holds PL/pgSQL syntax) says nothing.
func collectBodyPositions(sql string, tokens []rawToken) *astPositions {
	pos := newASTPositions()
	pos.sql = sql
	lw := func(i int) string {
		if i < 0 || i >= len(tokens) {
			return ""
		}
		return strings.ToLower(tokens[i].text)
	}
	prevCode := func(i int) string {
		for i--; i >= 0; i-- {
			if t := tokens[i].text; !strings.HasPrefix(t, "--") && !strings.HasPrefix(t, "/*") {
				return strings.ToLower(t)
			}
		}
		return ""
	}
	depth := 0
	var covered [][2]int // the statements whose tree was read
	for i := 0; i < len(tokens); i++ {
		switch tokens[i].text {
		case "(", "[":
			depth++
			continue
		case ")", "]":
			if depth > 0 {
				depth--
			}
			continue
		}
		if depth > 0 || !bodyStatementStarters[lw(i)] {
			continue
		}
		prev := prevCode(i)
		switch prev {
		case "", ";", "begin", "then", "else", "loop", "in", "query", "for", "exception":
		default:
			continue
		}
		// the end: the next ';' (or the LOOP after FOR … IN query) outside parentheses
		first, end, last, d := i, -1, len(tokens)-1, 0
	scan:
		for k := i; k < len(tokens); k++ {
			switch t := tokens[k].text; {
			case t == "(" || t == "[":
				d++
			case t == ")" || t == "]":
				if d > 0 {
					d--
				}
			case d == 0 && t == ";":
				end, last = tokens[k].start, k
				break scan
			case d == 0 && prev == "in" && strings.EqualFold(t, "loop"):
				end, last = tokens[k].start, k
				break scan
			}
		}
		if end < 0 {
			end = len(sql)
		}
		i = last
		off := tokens[first].start
		chunk := sql[off:end]
		if lw(first) == "perform" {
			chunk = "select " + chunk[len("perform"):] // PERFORM q reads as SELECT q (same length)
		}
		if _, err := pg_query.Parse(chunk); err != nil {
			continue
		}
		pos.mergeShifted(collectASTPositions(chunk, nil), off)
		covered = append(covered, [2]int{off, end})
	}
	resolveBodyCalls(tokens, pos, covered)
	return pos
}

// callContextWords are what stands before a function call in a PL/pgSQL expression that the parser
// did not see (v := abs(x) + f(y), IF lower(s) = 'a' THEN, RETURN round(x)): an operator, an opening
// parenthesis, a comma or a keyword that is followed by an expression. A name before "(" after
// anything else may be a table (INSERT INTO t (a, b)), a cursor or a type, so it is left as written.
var callContextWords = map[string]bool{
	":=": true, "=": true, "(": true, ",": true, "+": true, "-": true, "*": true, "/": true, "||": true,
	"<": true, ">": true, "<=": true, ">=": true, "<>": true, "!=": true, "and": true, "or": true,
	"not": true, "then": true, "return": true, "if": true, "elsif": true, "elseif": true, "while": true,
	"when": true, "perform": true,
}

// resolveBodyCalls registers as functions the names followed by "(" in the expressions of a PL/pgSQL
// body that no statement tree covers, so that functions.case applies to them like to those of the
// SQL statements around them.
func resolveBodyCalls(tokens []rawToken, pos *astPositions, covered [][2]int) {
next:
	for i := 1; i+1 < len(tokens); i++ {
		t := tokens[i]
		for _, c := range covered {
			if t.start >= c[0] && t.start < c[1] {
				continue next // a statement the parser read: its tree says what is a call
			}
		}
		if t.kind != pg_query.KeywordKind_NO_KEYWORD || tokens[i+1].text != "(" || tokens[i+1].start != t.end {
			continue
		}
		if t.text == "" || !(t.text[0] == '_' || t.text[0] >= 'a' && t.text[0] <= 'z' || t.text[0] >= 'A' && t.text[0] <= 'Z') {
			continue
		}
		lower := strings.ToLower(t.text)
		if literalNames[lower] || dataTypeNames[lower] || plpgsqlKeywordNames[lower] || plpgsqlVariableNames[lower] || parenAfterKeywordIsCall[lower] {
			continue
		}
		prev := i - 1
		for prev > 0 && (strings.HasPrefix(tokens[prev].text, "--") || strings.HasPrefix(tokens[prev].text, "/*")) {
			prev-- // (a comment between the operator and the call)
		}
		if !callContextWords[strings.ToLower(tokens[prev].text)] {
			continue
		}
		at := t.start
		if pos.columns[at] || pos.aliases[at] || pos.tables[at] || pos.schemas[at] || pos.functions[at] ||
			pos.conditionalFunctions[at] || pos.systemFunctions[at] {
			continue
		}
		pos.functions[at] = true
	}
}

// walkPseudoRefs walks nodes like walkNode, but the names given (pseudo-variables of the
// statement: VALUE of a domain, OLD and NEW of a trigger) are not taken for tables or columns.
func walkPseudoRefs(nodes []*pg_query.Node, pos *astPositions, names ...string) {
	tmp := newASTPositions()
	tmp.sql = pos.sql
	for _, n := range nodes {
		walkNode(n, tmp)
	}
	isName := func(at int) bool {
		for _, nm := range names {
			if at >= 0 && at+len(nm) <= len(tmp.sql) && strings.EqualFold(tmp.sql[at:at+len(nm)], nm) &&
				(at+len(nm) == len(tmp.sql) || !isIdentByte(tmp.sql[at+len(nm)])) {
				return true
			}
		}
		return false
	}
	for _, m := range []map[int]bool{tmp.columns, tmp.tables, tmp.schemas} {
		for at := range m {
			if isName(at) {
				delete(m, at)
			}
		}
	}
	pos.mergeShifted(tmp, 0)
}

func isIdentByte(c byte) bool {
	return c == '_' || c == '$' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= 0x80
}

// mergeShifted adds what src (the positions in a part of the text) says, shifted by off.
func (p *astPositions) mergeShifted(src *astPositions, off int) {
	shift := func(dst, from map[int]bool) {
		for k, v := range from {
			dst[k+off] = v
		}
	}
	shift(p.schemas, src.schemas)
	shift(p.tables, src.tables)
	shift(p.functions, src.functions)
	shift(p.conditionalFunctions, src.conditionalFunctions)
	shift(p.systemFunctions, src.systemFunctions)
	shift(p.aliases, src.aliases)
	shift(p.columns, src.columns)
	for _, pa := range src.pendingAliases {
		p.pendingAliases = append(p.pendingAliases, pendingAlias{exprStart: pa.exprStart + off, name: pa.name})
	}
	for _, l := range src.sortByDefaultLocs {
		p.sortByDefaultLocs = append(p.sortByDefaultLocs, l+off)
	}
	for _, l := range src.castFuncLocs {
		p.castFuncLocs = append(p.castFuncLocs, l+off)
	}
	for n := range src.subselectAliases {
		p.subselectAliases[n] = true
	}
	for n := range src.functionAliases {
		p.functionAliases[n] = true
	}
	if len(src.funcParams) > 0 {
		if p.funcParams == nil {
			p.funcParams = map[string]bool{}
		}
		for n := range src.funcParams {
			p.funcParams[n] = true
		}
	}
}

func walkNode(node *pg_query.Node, pos *astPositions) {
	if node == nil {
		return
	}
	many := func(nodes []*pg_query.Node) {
		for _, n := range nodes {
			walkNode(n, pos)
		}
	}

	switch n := node.Node.(type) {

	// ── DDL ──────────────────────────────────────────────────────────────────

	case *pg_query.Node_CreateStmt:
		cs := n.CreateStmt
		if cs.Relation != nil {
			addTableRangeVar(cs.Relation, pos)
		}
		many(cs.TableElts)
		many(cs.Constraints)

	case *pg_query.Node_AlterTableStmt:
		if n.AlterTableStmt.Relation != nil {
			addTableRangeVar(n.AlterTableStmt.Relation, pos)
		}
		many(n.AlterTableStmt.Cmds)

	case *pg_query.Node_AlterTableCmd:
		walkNode(n.AlterTableCmd.Def, pos)

	case *pg_query.Node_Constraint:
		// CHECK (value > 0), DEFAULT f(x): the expression reads like any other
		walkNode(n.Constraint.RawExpr, pos)
		many(n.Constraint.Exclusions) // EXCLUDE USING gist (f(a) WITH =)

	case *pg_query.Node_ColumnDef:
		cd := n.ColumnDef
		if cd.Location >= 0 {
			pos.columns[int(cd.Location)] = true
		}
		many(cd.Constraints)

	case *pg_query.Node_CreateFunctionStmt:
		cf := n.CreateFunctionStmt
		for _, p := range cf.Parameters {
			if fp := p.GetFunctionParameter(); fp != nil && fp.Name != "" {
				if pos.funcParams == nil {
					pos.funcParams = map[string]bool{}
				}
				pos.funcParams[strings.ToLower(fp.Name)] = true
			}
		}
		many(cf.Parameters)
		many(cf.Options)
		walkNode(cf.SqlBody, pos) // RETURN expr / BEGIN ATOMIC … END

	case *pg_query.Node_IndexStmt:
		ix := n.IndexStmt
		if ix.Relation != nil {
			addTableRangeVar(ix.Relation, pos)
		}
		many(ix.IndexParams)
		walkNode(ix.WhereClause, pos)

	case *pg_query.Node_IndexElem:
		walkNode(n.IndexElem.Expr, pos) // CREATE INDEX … ((lower(name)))

	case *pg_query.Node_CreateTableAsStmt:
		if into := n.CreateTableAsStmt.Into; into != nil {
			addTableRangeVar(into.Rel, pos) // CREATE TABLE t (a, b) AS …: lets resolveAliasColumns see the list
		}
		walkNode(n.CreateTableAsStmt.Query, pos)

	case *pg_query.Node_ViewStmt:
		addTableRangeVar(n.ViewStmt.View, pos) // CREATE VIEW v (a, b) AS …
		walkNode(n.ViewStmt.Query, pos)

	// ── DML ──────────────────────────────────────────────────────────────────

	case *pg_query.Node_SelectStmt:
		ss := n.SelectStmt
		if ss.WithClause != nil {
			for _, cte := range ss.WithClause.Ctes {
				walkNode(cte, pos)
			}
		}
		many(ss.TargetList)
		many(ss.FromClause)
		walkNode(ss.WhereClause, pos)
		many(ss.GroupClause)
		walkNode(ss.HavingClause, pos)
		many(ss.SortClause)
		many(ss.WindowClause)
		for _, row := range ss.ValuesLists { // VALUES (now(), …)
			if l := row.GetList(); l != nil {
				many(l.Items)
			}
		}
		walkNode(ss.LimitCount, pos)
		walkNode(ss.LimitOffset, pos)
		if ss.Larg != nil {
			walkNode(&pg_query.Node{Node: &pg_query.Node_SelectStmt{SelectStmt: ss.Larg}}, pos)
		}
		if ss.Rarg != nil {
			walkNode(&pg_query.Node{Node: &pg_query.Node_SelectStmt{SelectStmt: ss.Rarg}}, pos)
		}

	case *pg_query.Node_CommonTableExpr:
		cte := n.CommonTableExpr
		if cte.Location >= 0 && cte.Ctename != "" {
			pos.tables[int(cte.Location)] = true // WITH data AS (…): data is a name, not a keyword
		}
		walkNode(cte.Ctequery, pos)

	case *pg_query.Node_InsertStmt:
		is := n.InsertStmt
		if is.WithClause != nil {
			for _, cte := range is.WithClause.Ctes {
				walkNode(cte, pos)
			}
		}
		if is.Relation != nil {
			addTableRangeVar(is.Relation, pos)
		}
		many(is.Cols)
		walkNode(is.SelectStmt, pos)
		many(is.ReturningList)
		if is.OnConflictClause != nil {
			walkSetTargetList(is.OnConflictClause.TargetList, pos)
			walkNode(is.OnConflictClause.WhereClause, pos)
		}

	case *pg_query.Node_UpdateStmt:
		us := n.UpdateStmt
		if us.WithClause != nil {
			for _, cte := range us.WithClause.Ctes {
				walkNode(cte, pos)
			}
		}
		if us.Relation != nil {
			addTableRangeVar(us.Relation, pos)
		}
		walkSetTargetList(us.TargetList, pos)
		walkNode(us.WhereClause, pos)
		many(us.FromClause)
		many(us.ReturningList)

	case *pg_query.Node_DeleteStmt:
		ds := n.DeleteStmt
		if ds.WithClause != nil {
			for _, cte := range ds.WithClause.Ctes {
				walkNode(cte, pos)
			}
		}
		if ds.Relation != nil {
			addTableRangeVar(ds.Relation, pos)
		}
		walkNode(ds.WhereClause, pos)
		many(ds.UsingClause)
		many(ds.ReturningList)

	// ── Statements that wrap or combine queries ──────────────────────────────

	case *pg_query.Node_ExplainStmt:
		walkNode(n.ExplainStmt.Query, pos)
	case *pg_query.Node_PrepareStmt:
		walkNode(n.PrepareStmt.Query, pos)
	case *pg_query.Node_DeclareCursorStmt:
		walkNode(n.DeclareCursorStmt.Query, pos)
	case *pg_query.Node_CopyStmt:
		walkNode(n.CopyStmt.Query, pos)
	case *pg_query.Node_MergeStmt:
		ms := n.MergeStmt
		if ms.WithClause != nil {
			for _, cte := range ms.WithClause.Ctes {
				walkNode(cte, pos)
			}
		}
		if ms.Relation != nil {
			addTableRangeVar(ms.Relation, pos)
		}
		walkNode(ms.SourceRelation, pos)
		walkNode(ms.JoinCondition, pos)
		for _, w := range ms.MergeWhenClauses {
			if wc := w.GetMergeWhenClause(); wc != nil {
				walkNode(wc.Condition, pos)
				walkSetTargetList(wc.TargetList, pos)
				many(wc.Values)
			}
		}
		many(ms.ReturningList)

	// ── Expressions ──────────────────────────────────────────────────────────

	case *pg_query.Node_RangeVar:
		addTableRangeVar(n.RangeVar, pos)

	case *pg_query.Node_ColumnRef:
		cr := n.ColumnRef
		if cr.Location >= 0 {
			pos.columns[int(cr.Location)] = true
		}

	case *pg_query.Node_ResTarget:
		rt := n.ResTarget
		if rt.Name != "" && rt.Location >= 0 {
			if rt.Val == nil {
				// INSERT column list: Name is the column itself (no expression).
				pos.columns[int(rt.Location)] = true
			} else {
				// SELECT target alias: expression + alias name.
				// ResTarget.Location points to the expression start, not the alias
				// name. Collect as pending; Format() resolves positions after scanning.
				pos.pendingAliases = append(pos.pendingAliases, pendingAlias{
					exprStart: int(rt.Location),
					name:      rt.Name,
				})
			}
		}
		walkNode(rt.Val, pos)

	case *pg_query.Node_FuncCall:
		fc := n.FuncCall
		if fc.Location >= 0 {
			if len(fc.Funcname) > 1 {
				// Schema-qualified: pg_catalog.func() → schema + function positions.
				// The function name follows "schema." only when the text really spells it: the
				// parser reports pg_catalog.name for syntax such as SIMILAR TO / EXTRACT / TRIM /
				// AT TIME ZONE, where "location + length of the schema + 1" is whatever follows the
				// keyword (a SIMILAR TO 'AB' string constant lost its capitals that way) and the
				// token at the location is a keyword (EXTRACT, AT), not a schema name.
				if prefix := funcNameStr(fc.Funcname[0]); prefix != "" && pos.spellsQualified(int(fc.Location), prefix) {
					pos.schemas[int(fc.Location)] = true
					pos.functions[int(fc.Location)+len(prefix)+1] = true
				}
			} else {
				pos.functions[int(fc.Location)] = true
			}
		}
		many(fc.Args)
		many(fc.AggOrder)
		walkNode(fc.AggFilter, pos)
		if fc.Over != nil { // f(…) OVER (ORDER BY g(x) ROWS BETWEEN … AND h(2) FOLLOWING)
			walkNode(&pg_query.Node{Node: &pg_query.Node_WindowDef{WindowDef: fc.Over}}, pos)
		}

	case *pg_query.Node_JoinExpr:
		je := n.JoinExpr
		walkNode(je.Larg, pos)
		walkNode(je.Rarg, pos)
		walkNode(je.Quals, pos)

	case *pg_query.Node_RangeSubselect:
		rs := n.RangeSubselect
		walkNode(rs.Subquery, pos)
		if rs.Alias != nil && rs.Alias.Aliasname != "" {
			pos.subselectAliases[strings.ToLower(rs.Alias.Aliasname)] = true
		}

	case *pg_query.Node_RangeFunction:
		rf := n.RangeFunction
		for _, f := range rf.Functions {
			if l := f.GetList(); l != nil { // each entry is a list: [function call, column definitions]
				for _, it := range l.Items {
					walkNode(it, pos)
				}
			} else {
				walkNode(f, pos)
			}
		}
		if rf.Alias != nil && rf.Alias.Aliasname != "" {
			pos.functionAliases[strings.ToLower(rf.Alias.Aliasname)] = true
		}

	case *pg_query.Node_SubLink:
		walkNode(n.SubLink.Testexpr, pos) // (a, f(x)) = ANY (SELECT …), f(x) IN (SELECT …)
		walkNode(n.SubLink.Subselect, pos)

	case *pg_query.Node_BoolExpr:
		many(n.BoolExpr.Args)

	case *pg_query.Node_AExpr:
		ae := n.AExpr
		if ae.Kind == pg_query.A_Expr_Kind_AEXPR_NULLIF && ae.Location >= 0 {
			pos.conditionalFunctions[int(ae.Location)] = true
		}
		walkNode(ae.Lexpr, pos)
		walkNode(ae.Rexpr, pos)

	case *pg_query.Node_TypeCast:
		tc := n.TypeCast
		loc := int(tc.Location)
		if loc >= 0 && loc+4 <= len(pos.sql) && strings.EqualFold(pos.sql[loc:loc+4], "cast") {
			pos.castFuncLocs = append(pos.castFuncLocs, loc)
		}
		walkNode(tc.Arg, pos)

	case *pg_query.Node_CaseExpr:
		ce := n.CaseExpr
		walkNode(ce.Arg, pos)
		many(ce.Args)
		walkNode(ce.Defresult, pos)

	case *pg_query.Node_CaseWhen:
		cw := n.CaseWhen
		walkNode(cw.Expr, pos)
		walkNode(cw.Result, pos)

	case *pg_query.Node_CoalesceExpr:
		ce := n.CoalesceExpr
		if ce.Location >= 0 {
			pos.conditionalFunctions[int(ce.Location)] = true
		}
		many(ce.Args)

	case *pg_query.Node_MinMaxExpr: // GREATEST / LEAST
		mm := n.MinMaxExpr
		if mm.Location >= 0 {
			pos.conditionalFunctions[int(mm.Location)] = true
		}
		many(mm.Args)

	// CURRENT_DATE, CURRENT_TIMESTAMP, SESSION_USER, CURRENT_USER, etc.
	case *pg_query.Node_SqlvalueFunction:
		sf := n.SqlvalueFunction
		if sf.Location >= 0 {
			pos.systemFunctions[int(sf.Location)] = true
		}

	case *pg_query.Node_SortBy:
		sb := n.SortBy
		if sb.SortbyDir == pg_query.SortByDir_SORTBY_DEFAULT && sb.Node != nil {
			// SortBy.Location is often -1; use the node's location instead.
			nodeLoc := nodeLocation(sb.Node)
			if nodeLoc >= 0 {
				pos.sortByDefaultLocs = append(pos.sortByDefaultLocs, nodeLoc)
			}
		}
		walkNode(sb.Node, pos)

	case *pg_query.Node_WindowDef:
		wd := n.WindowDef
		many(wd.PartitionClause)
		many(wd.OrderClause)
		walkNode(wd.StartOffset, pos)
		walkNode(wd.EndOffset, pos)

	case *pg_query.Node_GroupingSet:
		many(n.GroupingSet.Content)

	case *pg_query.Node_DefElem:
		walkNode(n.DefElem.Arg, pos)

	case *pg_query.Node_CreateDomainStmt:
		// VALUE in a domain's CHECK is the keyword, not a column
		walkPseudoRefs(n.CreateDomainStmt.Constraints, pos, "value")

	case *pg_query.Node_CreateTrigStmt:
		// OLD / NEW in the WHEN condition are the record variables, written as keywords
		walkPseudoRefs([]*pg_query.Node{n.CreateTrigStmt.WhenClause}, pos, "old", "new")

	case *pg_query.Node_XmlExpr:
		// XMLFOREST(a AS x), XMLATTRIBUTES(a AS x): the names are no aliases (the AS stays)
		for _, na := range n.XmlExpr.NamedArgs {
			if rt := na.GetResTarget(); rt != nil {
				walkNode(rt.Val, pos)
			} else {
				walkNode(na, pos)
			}
		}
		many(n.XmlExpr.Args)

	default:
		walkChildren(node, pos)
	}
}

// nodeLocation returns the byte offset for a node, or -1 if not available.
func nodeLocation(node *pg_query.Node) int {
	if node == nil {
		return -1
	}
	switch n := node.Node.(type) {
	case *pg_query.Node_ColumnRef:
		return int(n.ColumnRef.Location)
	case *pg_query.Node_FuncCall:
		return int(n.FuncCall.Location)
	case *pg_query.Node_AConst:
		return int(n.AConst.Location)
	case *pg_query.Node_TypeCast:
		return int(n.TypeCast.Location)
	case *pg_query.Node_AExpr:
		return int(n.AExpr.Location)
	case *pg_query.Node_CaseExpr:
		return int(n.CaseExpr.Location)
	case *pg_query.Node_CoalesceExpr:
		return int(n.CoalesceExpr.Location)
	}
	return -1
}

// addTableRangeVar records schema and table name positions from a RangeVar.
// For schema-qualified names (schema.table), the schema goes to pos.schemas
// and the table to pos.tables. Unqualified names go directly to pos.tables.
func addTableRangeVar(rv *pg_query.RangeVar, pos *astPositions) {
	if rv == nil || rv.Location < 0 {
		return
	}
	loc := int(rv.Location)
	if rv.Schemaname != "" && rv.Relname != "" {
		pos.schemas[loc] = true
		pos.tables[tableAfterSchema(pos.sql, loc, rv.Schemaname)] = true
	} else {
		pos.tables[loc] = true
	}
	// Track table alias (e.g. "FROM users u" or "FROM users AS u") so that
	// aliases.as: add/remove applies to table aliases too.
	// Use loc as exprStart so that findAliasToken's startIdx lands on the
	// table (or schema) token; the i > startIdx guard then skips it before
	// finding the alias — setting exprStart past the table name would make
	// startIdx == alias, which i > startIdx would skip entirely.
	if rv.Alias != nil && rv.Alias.Aliasname != "" {
		pos.pendingAliases = append(pos.pendingAliases, pendingAlias{
			exprStart: loc,
			name:      rv.Alias.Aliasname,
		})
	}
}

// tableAfterSchema is the offset of the name that follows the schema at loc and its dot: the schema
// is `"quoted"` (with quotes of its own to count, and "" inside) or as long as its name.
func tableAfterSchema(sql string, loc int, schema string) int {
	end := loc + len(schema)
	if loc >= 0 && loc < len(sql) && sql[loc] == '"' {
		i := loc + 1
		for i < len(sql) {
			if sql[i] == '"' {
				if i+1 < len(sql) && sql[i+1] == '"' {
					i += 2
					continue
				}
				break
			}
			i++
		}
		end = i + 1
	}
	return end + 1 // +1 for the dot
}

// walkSetTargetList walks a SET assignment list (UPDATE … SET or ON CONFLICT DO
// UPDATE SET). ResTarget.Name is the assignment target column — not a SELECT
// alias — so it is added directly to pos.columns instead of pendingAliases.
func walkSetTargetList(nodes []*pg_query.Node, pos *astPositions) {
	for _, n := range nodes {
		if n == nil {
			continue
		}
		if rt, ok := n.Node.(*pg_query.Node_ResTarget); ok {
			r := rt.ResTarget
			if r.Name != "" && r.Location >= 0 {
				pos.columns[int(r.Location)] = true
			}
			walkNode(r.Val, pos)
		} else {
			walkNode(n, pos)
		}
	}
}

// funcNameStr returns the string value of a funcname String node, or "".
func funcNameStr(n *pg_query.Node) string {
	if s, ok := n.Node.(*pg_query.Node_String_); ok {
		return s.String_.Sval
	}
	return ""
}

// isDollarQuoted returns true if s is a PostgreSQL dollar-quoted string literal.
func isDollarQuoted(s string) bool {
	return len(s) >= 4 && s[0] == '$'
}

// applyBodyIndent applies the BodyIndentMode to a multi-line block of content.
// applySectionIndent is applyBodyIndent for the text after a keyword (DECLARE, BEGIN): what
// follows the keyword on its own line (`DECLARE x int;`) stays where it is.
func applySectionIndent(content, indentUnit string, mode config.BodyIndentMode) string {
	nl := strings.IndexByte(content, '\n')
	switch {
	case nl < 0 && strings.TrimSpace(content) != "":
		return content // all on the line of the keyword
	case nl > 0 && strings.TrimSpace(content[:nl]) != "":
		return content[:nl] + applyBodyIndent(content[nl:], indentUnit, mode)
	}
	return applyBodyIndent(content, indentUnit, mode)
}

func applyBodyIndent(content, indentUnit string, mode config.BodyIndentMode) string {
	if mode == config.BodyIndentPreserve || mode == "" || content == "" {
		return content
	}
	lines := strings.Split(content, "\n")
	inside := linesInsideTokens(content, len(lines)) // (data: string constants, nested bodies, block comments)
	var code []string
	for i, line := range lines {
		if !inside[i] {
			code = append(code, line)
		}
	}
	minIndent := findMinIndent(code)
	result := make([]string, len(lines))
	for i, line := range lines {
		switch {
		case inside[i]:
			result[i] = line
		case strings.TrimSpace(line) == "":
			result[i] = ""
		case mode == config.BodyIndentNone:
			result[i] = trimIndent(line, minIndent)
		default:
			// mode == BodyIndentIndent: normalise to exactly one indent level —
			// strip existing indent first, then add indentUnit.
			result[i] = indentUnit + trimIndent(line, minIndent)
		}
	}
	return strings.Join(result, "\n")
}

var subqueryOpen = regexp.MustCompile(`(?i)\(\s*(?:select|with|values)\b`)

// reindentContinuation replaces the indent oldIndent that starts the lines of seg after its first
// one by newIndent (the lines of a condition that moves with the keyword line it belongs to).
func reindentContinuation(seg, oldIndent, newIndent string) string {
	nl := strings.IndexByte(seg, '\n')
	if nl < 0 {
		return seg
	}
	inside := linesInsideTokens(seg, strings.Count(seg, "\n")+1)
	lines := strings.Split(seg, "\n")
	// (only with a subquery in the condition: its lines, laid out from the line that opens it, have to keep
	// their distance to that line; other lines stay where they are, left of the keyword or not)
	spacesOnly := strings.Trim(oldIndent, " ") == "" && strings.Trim(newIndent, " ") == "" && subqueryOpen.MatchString(seg)
	for i := 1; i < len(lines); i++ {
		last := i == len(lines)-1 // (the indent of the keyword that ends the segment)
		if !inside[i] && strings.HasPrefix(lines[i], oldIndent) && (last && lines[i] != "" || strings.TrimSpace(lines[i]) != "") {
			lines[i] = newIndent + lines[i][len(oldIndent):]
		} else if !inside[i] && !last && spacesOnly && strings.TrimSpace(lines[i]) != "" {
			// a line that stood left of the old indent (hand-aligned `AND`) moves by the same distance, or
			// the lines of a subquery opened in the condition would move without it and meet it differently
			// on the next run
			if d := len(newIndent) - len(oldIndent); d > 0 {
				lines[i] = strings.Repeat(" ", d) + lines[i]
			} else if t := strings.TrimLeft(lines[i], " "); len(lines[i])-len(t) > 0 {
				lines[i] = lines[i][min(-d, len(lines[i])-len(t)):]
			}
		}
	}
	return strings.Join(lines, "\n")
}

// linesInsideTokens reports, for each of the n lines of text, whether the line starts inside a
// token that spans lines (a string constant, a nested dollar-quoted body, a block comment): its
// blanks are data, not indentation.
func linesInsideTokens(text string, n int) []bool {
	inside := make([]bool, n)
	res, err := pg_query.Scan(text)
	if err != nil {
		return inside
	}
	lineOf := func(off int) int { return strings.Count(text[:off], "\n") }
	for _, t := range res.Tokens {
		tok := text[t.Start:t.End]
		if !strings.Contains(tok, "\n") {
			continue
		}
		first := lineOf(int(t.Start))
		for k := 1; k <= strings.Count(tok, "\n"); k++ {
			if first+k < n {
				inside[first+k] = true
			}
		}
	}
	return inside
}

// applyBodySectionBlanks ensures the correct number of leading/trailing newlines
// in a section body (the text between two block keywords such as BEGIN and END).
// "before" controls the gap after the opening keyword; "after" controls the gap
// before the closing keyword.
func applyBodySectionBlanks(content string, before, after config.BlankLineAction) string {
	switch before {
	case config.BlankLineAdd:
		stripped := strings.TrimLeft(content, "\n")
		content = "\n\n" + stripped
	case config.BlankLineRemove:
		stripped := strings.TrimLeft(content, "\n")
		content = "\n" + stripped
	}
	switch after {
	case config.BlankLineAdd:
		stripped := strings.TrimRight(content, "\n")
		content = stripped + "\n\n"
	case config.BlankLineRemove:
		stripped := strings.TrimRight(content, "\n")
		content = stripped + "\n"
	}
	return content
}

func findMinIndent(lines []string) string {
	// the number of blanks the least indented line has, as a string of that many (tabs and blanks mix:
	// `        x` and `\ty` lose one character each, whatever it is — see trimIndent)
	minLen := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		j := 0
		for j < len(line) && (line[j] == ' ' || line[j] == '\t') {
			j++
		}
		if minLen < 0 || j < minLen {
			minLen = j
		}
	}
	if minLen <= 0 {
		return ""
	}
	return strings.Repeat(" ", minLen)
}

// trimIndent takes len(indent) blanks (of either kind) off the start of line.
func trimIndent(line, indent string) string {
	n := 0
	for n < len(indent) && n < len(line) && (line[n] == ' ' || line[n] == '\t') {
		n++
	}
	return line[n:]
}

var leadingBlankLines = regexp.MustCompile(`^(?:[ \t]*\n)+`)

func applyDollarBoundary(body string, afterOpen, beforeClose config.BlankLineAction) string {
	if strings.TrimSpace(body) == "" && (afterOpen == config.BlankLineAdd || afterOpen == config.BlankLineRemove ||
		beforeClose == config.BlankLineAdd || beforeClose == config.BlankLineRemove) {
		// nothing in it: one line break, or a blank line when one is to be added
		if afterOpen == config.BlankLineAdd || beforeClose == config.BlankLineAdd {
			return "\n\n"
		}
		return "\n"
	}
	switch afterOpen {
	case config.BlankLineAdd:
		stripped := leadingBlankLines.ReplaceAllString(body, "") // (a line of blanks after the delimiter is a blank line)
		body = "\n\n" + stripped
	case config.BlankLineRemove:
		stripped := leadingBlankLines.ReplaceAllString(body, "")
		body = "\n" + stripped
	}
	switch beforeClose {
	case config.BlankLineAdd:
		stripped := strings.TrimRight(body, " \t\n") // (blanks in front of the closing delimiter too: a line that holds only them is a blank line)
		body = stripped + "\n\n"
	case config.BlankLineRemove:
		stripped := strings.TrimRight(body, " \t\n")
		body = stripped + "\n"
	}
	return body
}

func indentUnitFromCfg(cfg *config.Config) string {
	lc := cfg.Layout
	if lc.Indent.Type == config.IndentTypeTab {
		return "\t"
	}
	size := lc.Indent.Size
	if size <= 0 {
		size = 4
	}
	return strings.Repeat(" ", size)
}

// formatDollarQuoted formats the SQL/PL/pgSQL body inside a dollar-quoted literal.
func formatDollarQuoted(s string, cfg *config.Config, vars map[string]bool) (string, error) {
	delimEnd := strings.Index(s[1:], "$")
	if delimEnd < 0 {
		return s, nil
	}
	delimiter := s[:delimEnd+2]
	body := s[len(delimiter) : len(s)-len(delimiter)]

	dqCfg := cfg.Layout.DollarQuote

	// Apply casing/spacing without layout first.
	cfgInner := *cfg
	cfgInner.Layout = config.LayoutConfig{}
	// Trailing comments are aligned once the layout has moved the lines (below); aligned here
	// they would carry the padding of lines the layout then separates.
	cfgInner.Comments.AlignTrailing = config.CommentAlignPreserve
	// A body is a fragment of the file: whole-file settings must not touch its edges
	// (trailing_newline: strip would glue the closing delimiter to the last line).
	cfgInner.TrailingNewline = config.TrailingNewlinePreserve
	// The ';' after the END of a body is end_semicolon when that is set; semicolons gives way to it.
	if es := cfg.Layout.DollarQuote.PLpgSQL.EndSemicolon; es == config.EndSemicolonAdd || es == config.EndSemicolonRemove {
		cfgInner.Semicolons = config.SemicolonPreserve
	}
	// Nor does the blank line between statements: inside a body it would also separate the last
	// statement from END (the plpgsql blank-line settings are layout.dollar_quote.plpgsql).
	cfgInner.BlankLinesBetweenStatements = "preserve"
	// A PL/pgSQL body written on one line (do $$ begin perform 1; end $$) stays compact: there
	// is no structure to lay out, and breaking its statements would leave it half-formatted.
	oneLineBody := isPLpgSQLBody(body) && !strings.Contains(strings.TrimSpace(body), "\n")
	if oneLineBody {
		cfgInner.StatementBreak = config.StatementBreakPreserve
	}
	if isPLpgSQLBody(body) {
		body = normalizeAssignments(body, cfg.PLpgSQLAssignment)
	}
	cfgInner.Nested = true
	cfgInner.OwnLineParen = cfg.OwnLineParen || cfg.Layout.ParenIndent.Open == config.ParenOpenOwnLine
	vars = plpgsqlVarNames(body, vars)
	formatted, err := formatBody(body, &cfgInner, vars)
	if err != nil {
		formatted = body
	}

	var resultBody string
	if oneLineBody {
		resultBody = formatted
	} else if isPLpgSQLBody(formatted) {
		// PL/pgSQL body: apply layout per-statement, then apply dollar_quote.plpgsql config.
		resultBody, err = applyLayoutPerStatement(formatted, cfg, vars)
		if err != nil {
			resultBody = formatted
		}
		resultBody = applyPLpgSQLDollarQuoteConfig(resultBody, cfg)
	} else {
		// Plain SQL body: apply layout per-statement, then apply dollar_quote.sql config.
		resultBody, err = applyLayoutPerStatement(formatted, cfg, vars)
		if err != nil {
			resultBody = formatted
		}
		indentUnit := indentUnitFromCfg(cfg)
		resultBody = applyBodyIndent(resultBody, indentUnit, dqCfg.SQL.BodyIndent)
		resultBody = applyBodySectionBlanks(resultBody, dqCfg.SQL.BlankLineBefore, dqCfg.SQL.BlankLineAfter)
	}

	// Apply dollar boundary (newline_after_open, newline_before_close).
	resultBody = applyDollarBoundary(resultBody, dqCfg.NewlineAfterOpen, dqCfg.NewlineBeforeClose)

	// The restructuring above can leave trailing blanks behind (e.g. a line that is
	// followed by an inserted blank line); strip them, but never inside tokens.
	if cfg.TrailingWhitespace == config.TrailingWSStrip {
		resultBody = stripTrailingOutsideTokens(resultBody)
	}
	if (cfg.CommaSpacing == config.CommaSpacingCompact || cfg.OperatorSpacing == config.OperatorSpacingCompact) && !layout.IsNoop(&cfg.Layout) {
		resultBody = compactGaps(resultBody, cfg.CommaSpacing == config.CommaSpacingCompact, cfg.OperatorSpacing == config.OperatorSpacingCompact)
	}
	if cfg.Comments.AlignTrailing == config.CommentAlignAlign {
		resultBody = alignTrailingComments(resultBody, tabIndentAfterLayout(cfg), blankIndentTabWidth(cfg))
	}
	if cfg.Comments.AlignTrailing == config.CommentAlignNone {
		resultBody = collapseTrailingCommentGap(resultBody)
	}

	// Keep a single-line trailing gap before the closing delimiter (`end $$`),
	// which Format() trims; otherwise the last keyword is glued to it (`END$$`).
	if trimmed := strings.TrimRight(body, " \t"); trimmed != body && !strings.HasSuffix(strings.TrimRight(body, " \t"), "\n") &&
		resultBody != "" && !strings.ContainsAny(resultBody[len(resultBody)-1:], " \t\n") {
		resultBody += " "
	}

	return delimiter + resultBody + delimiter, nil
}

// applyPLpgSQLDollarQuoteConfig applies dollar_quote.plpgsql config to a PL/pgSQL body.
// When all config values are zero/preserve (the default), this is a no-op.
func applyPLpgSQLDollarQuoteConfig(body string, cfg *config.Config) string {
	plCfg := cfg.Layout.DollarQuote.PLpgSQL

	// Check if any non-preserve action is requested.
	isPreserve := func(bim config.BodyIndentMode) bool {
		return bim == "" || bim == config.BodyIndentPreserve
	}
	isPreserveWE := func(we config.WhenEmptyMode) bool {
		return we == "" || we == config.WhenEmptyPreserve
	}
	isPreserveES := func(es config.EndSemicolonMode) bool {
		return es == "" || es == config.EndSemicolonPreserve
	}

	isPreserveBL := func(bla config.BlankLineAction) bool {
		return bla == "" || bla == config.BlankLinePreserve
	}
	allPreserve := isPreserve(plCfg.KeywordIndent) &&
		isPreserve(plCfg.Declare.Indent) &&
		isPreserveBL(plCfg.Declare.BlankLineBefore) &&
		isPreserveBL(plCfg.Declare.BlankLineAfter) &&
		isPreserve(plCfg.BeginBody.Indent) &&
		isPreserveBL(plCfg.BeginBody.BlankLineBefore) &&
		isPreserveBL(plCfg.BeginBody.BlankLineAfter) &&
		isPreserveWE(plCfg.DeclareEmpty) &&
		isPreserveES(plCfg.EndSemicolon) &&
		controlFlowIsNoop(plCfg.ControlFlow)

	if allPreserve {
		return body
	}

	// Simple regex-free parser: find DECLARE, BEGIN, END keywords at the start
	// of a line (with optional leading whitespace), handling nested blocks.
	// This is a conservative implementation that preserves content on error.
	result, ok := restructurePLpgSQL(body, cfg)
	if !ok {
		return body
	}
	return result
}

// restructurePLpgSQL parses and reconstructs a PL/pgSQL body applying config.
// Returns (result, true) on success, ("", false) if parsing fails.
func restructurePLpgSQL(body string, cfg *config.Config) (string, bool) {
	plCfg := cfg.Layout.DollarQuote.PLpgSQL
	indentUnit := indentUnitFromCfg(cfg)

	// Use pg_query scanner to find DECLARE, BEGIN, END tokens at depth 0.
	scanResult, err := pg_query.Scan(body)
	if err != nil {
		return "", false
	}

	type section int
	const (
		secLeading section = iota
		secDeclare
		secBegin
		secException
		secAfterEnd
	)

	state := secLeading
	blockDepth := 0

	var (
		leadingEnd         int
		hasDeclare         bool
		declareKwStart     int
		declareKwEnd       int
		declareBodyStart   int
		beginKwStart       int
		beginKwEnd         int
		beginBodyStart     int
		hasException       bool
		exceptionKwStart   int
		exceptionKwEnd     int
		exceptionBodyStart int
		endKwStart         int
		endKwEnd           int
		hasSemi            bool
		trailingStart      int
	)

	allToks := scanResult.Tokens
	prevText := ""
	for i := 0; i < len(allToks); i++ {
		tok := allToks[i]
		tokStart := int(tok.Start)
		tokEnd := int(tok.End)
		text := strings.ToUpper(body[tokStart:tokEnd])
		prev := prevText
		prevText = text

		// peekNext returns the text of the next token without advancing i.
		peekNextTok := func() string {
			if i+1 < len(allToks) {
				nt := allToks[i+1]
				return strings.ToUpper(body[nt.Start:nt.End])
			}
			return ""
		}

		switch state {
		case secLeading:
			if text == "DECLARE" {
				hasDeclare = true
				declareKwStart = tokStart
				declareKwEnd = tokEnd
				leadingEnd = tokStart
				declareBodyStart = tokEnd
				state = secDeclare
			} else if text == "BEGIN" {
				beginKwStart = tokStart
				beginKwEnd = tokEnd
				leadingEnd = tokStart
				beginBodyStart = tokEnd
				blockDepth = 1
				state = secBegin
			}

		case secDeclare:
			if text == "BEGIN" {
				beginKwStart = tokStart
				beginKwEnd = tokEnd
				beginBodyStart = tokEnd
				blockDepth = 1
				state = secBegin
			}

		case secBegin:
			if text == "BEGIN" || text == "CASE" || text == "IF" || text == "LOOP" {
				blockDepth++
			} else if text == "END" {
				next := peekNextTok()
				if next == "IF" || next == "LOOP" || next == "CASE" {
					// END IF / END LOOP / END CASE — closing a nested control structure.
					blockDepth--
					i++ // consume the IF, LOOP, or CASE token
				} else {
					blockDepth--
					if blockDepth == 0 {
						endKwStart = tokStart
						endKwEnd = tokEnd
						trailingStart = tokEnd
						state = secAfterEnd
					}
				}
			} else if text == "EXCEPTION" && blockDepth == 1 && prev != "RAISE" { // RAISE EXCEPTION is a statement, not the section
				hasException = true
				exceptionKwStart = tokStart
				exceptionKwEnd = tokEnd
				exceptionBodyStart = tokEnd
				blockDepth = 0
				state = secException
			}

		case secException:
			if text == "BEGIN" || text == "CASE" || text == "IF" || text == "LOOP" {
				blockDepth++
			} else if text == "END" {
				next := peekNextTok()
				if next == "IF" || next == "LOOP" || next == "CASE" {
					if blockDepth > 0 {
						blockDepth--
					}
					i++ // consume IF, LOOP, or CASE token
				} else if blockDepth == 0 {
					endKwStart = tokStart
					endKwEnd = tokEnd
					trailingStart = tokEnd
					state = secAfterEnd
				} else {
					blockDepth--
				}
			}

		case secAfterEnd:
			if body[tokStart:tokEnd] == ";" {
				hasSemi = true
				trailingStart = tokEnd
			}
		}
	}

	if state != secAfterEnd {
		return "", false
	}

	// Determine keyword indent.
	kwIndent := ""
	switch plCfg.KeywordIndent {
	case config.BodyIndentIndent:
		kwIndent = indentUnit
	case config.BodyIndentNone:
		kwIndent = ""
	default: // preserve: extract from original
		// Use the original indent of the BEGIN keyword line.
		line := body[leadingEnd:beginKwStart]
		lastNL := strings.LastIndexByte(line, '\n')
		if lastNL >= 0 {
			segment := line[lastNL+1:]
			j := 0
			for j < len(segment) && (segment[j] == ' ' || segment[j] == '\t') {
				j++
			}
			kwIndent = segment[:j]
		} else if !hasDeclare {
			// BEGIN is the first keyword: its indent is the blanks of the last line before it
			lead := body[:leadingEnd]
			if nl := strings.LastIndexByte(lead, '\n'); nl >= 0 && strings.TrimSpace(lead[nl+1:]) == "" {
				kwIndent = lead[nl+1:] // (on a line of its own; after `$$ ` it has no indent)
			}
		}
	}

	var sb strings.Builder

	// trimHWS strips trailing horizontal whitespace (spaces/tabs) from the builder
	// so that kwIndent fully controls keyword indentation on every pass.
	trimHWS := func() {
		s := sb.String()
		i := len(s)
		for i > 0 && (s[i-1] == ' ' || s[i-1] == '\t') {
			i--
		}
		if i < len(s) {
			sb.Reset()
			sb.Grow(i + 64)
			sb.WriteString(s[:i])
		}
	}

	// Leading gap (whitespace before DECLARE or BEGIN).
	sb.WriteString(body[:leadingEnd])

	// DECLARE section.
	if hasDeclare {
		declBody := body[declareBodyStart:beginKwStart]
		trimmedDecl := strings.TrimSpace(declBody)
		showDeclare := true
		switch plCfg.DeclareEmpty {
		case config.WhenEmptyRemove:
			if trimmedDecl == "" {
				showDeclare = false
			}
		case config.WhenEmptyAdd:
			// always show (already true)
		}
		if showDeclare {
			trimHWS()
			sb.WriteString(kwIndent)
			sb.WriteString(body[declareKwStart:declareKwEnd]) // preserve original casing
			decl := applySectionIndent(declBody, indentUnit, plCfg.Declare.Indent)
			decl = applyBodySectionBlanks(decl, plCfg.Declare.BlankLineBefore, plCfg.Declare.BlankLineAfter)
			sb.WriteString(decl)
		}
	}

	// BEGIN keyword.
	trimHWS()
	sb.WriteString(kwIndent)
	sb.WriteString(body[beginKwStart:beginKwEnd])

	// Begin body.
	var beginBody string
	if hasException {
		beginBody = body[beginBodyStart:exceptionKwStart]
	} else {
		beginBody = body[beginBodyStart:endKwStart]
	}
	bb := applySectionIndent(beginBody, indentUnit, plCfg.BeginBody.Indent)
	bb = applyBodySectionBlanks(bb, plCfg.BeginBody.BlankLineBefore, plCfg.BeginBody.BlankLineAfter)
	if !controlFlowIsNoop(plCfg.ControlFlow) {
		bb = reformatControlFlow(bb, indentUnit, plCfg.ControlFlow, cfg.Layout.LineLength, plCfg.BeginBody.Indent == config.BodyIndentNone)
	}
	sb.WriteString(bb)

	// EXCEPTION section.
	if hasException {
		trimHWS()
		sb.WriteString(kwIndent)
		sb.WriteString(body[exceptionKwStart:exceptionKwEnd])
		eb := body[exceptionBodyStart:endKwStart]
		if !controlFlowIsNoop(plCfg.ControlFlow) {
			eb = reformatControlFlow(eb, indentUnit, plCfg.ControlFlow, cfg.Layout.LineLength, plCfg.BeginBody.Indent == config.BodyIndentNone)
		}
		sb.WriteString(eb)
	}

	// END keyword.
	trimHWS()
	sb.WriteString(kwIndent)
	sb.WriteString(body[endKwStart:endKwEnd])

	// Semicolon after END.
	switch plCfg.EndSemicolon {
	case config.EndSemicolonAdd:
		sb.WriteString(";")
	case config.EndSemicolonRemove:
		// don't write
	default: // preserve
		if hasSemi {
			sb.WriteString(";")
		}
	}

	// Trailing gap.
	sb.WriteString(body[trailingStart:])

	return sb.String(), true
}

// isPLpgSQLBody returns true when the dollar-quoted body looks like a PL/pgSQL block
// (starts with BEGIN or DECLARE) rather than a plain SQL statement.
func isPLpgSQLBody(body string) bool {
	lower := strings.ToLower(strings.TrimSpace(body))
	// comments in front of the block (a /*DOCU … */ header) are no part of it
	for {
		switch {
		case strings.HasPrefix(lower, "--"):
			nl := strings.IndexByte(lower, '\n')
			if nl < 0 {
				return false
			}
			lower = strings.TrimSpace(lower[nl+1:])
		case strings.HasPrefix(lower, "/*"):
			end := strings.Index(lower, "*/")
			if end < 0 {
				return false
			}
			lower = strings.TrimSpace(lower[end+2:])
		default:
			return strings.HasPrefix(lower, "begin") || strings.HasPrefix(lower, "declare")
		}
	}
}

// sqlClauseStarters are top-level keywords that begin a SQL statement in PL/pgSQL.
var sqlClauseStarters = map[string]bool{
	"select": true, "insert": true, "update": true, "delete": true,
	"with": true, "perform": true, "merge": true,
}

// withStartsQuery reports whether a WITH that follows the word prev opens a query (a CTE list)
// rather than being the WITH of CREATE VIEW v WITH (security_barrier), CREATE TABLE … WITH (…),
// … WITH NO DATA, WITH CHECK OPTION, WITH ORDINALITY: those follow a name or a keyword of the statement.
func withStartsQuery(prev string) bool {
	switch prev {
	case "", ";", "begin", "then", "else", "loop", "in", "as", "query", "for", "exception", "return":
		return true
	}
	return false
}

// applyLayoutPerStatement applies layout to individual SQL statements inside a
// PL/pgSQL body. It scans for SQL clause starters (SELECT, INSERT, UPDATE, DELETE,
// WITH, PERFORM) at paren depth 0 and applies layout.Apply() to each one, while
// leaving block-structure tokens (BEGIN, END, DECLARE, RAISE, variable assignments,
// etc.) completely untouched.
//
// RETURN QUERY SELECT: "RETURN QUERY" is output as prefix verbatim; layout is
// applied to the "SELECT …" portion and subsequent lines are re-indented.
func applyLayoutPerStatement(body string, cfg *config.Config, vars map[string]bool) (string, error) {
	if layout.IsNoop(&cfg.Layout) {
		return body, nil
	}
	scanResult, err := pg_query.Scan(body)
	if err != nil {
		return body, nil
	}

	var result strings.Builder
	parenDepth := 0
	lastEnd := 0         // position in body up to which we have already written output
	sqlStart := -1       // byte position where the current SQL clause keyword starts
	inPrivilege := false // inside a GRANT / REVOKE statement
	forInSQL := false    // true when tracking a FOR r IN SELECT...LOOP cursor query
	prevLower := ""      // last non-paren token seen at paren depth 0 (lowercase)
	condStart := -1      // start of an IF / ELSIF / WHILE condition being scanned
	condEnd := ""        // keyword that ends that condition (then | loop)

	// cfgNoLayout is used for a re-format pass on each extracted SQL statement.
	// The outer Format(body, ...) call fails to parse the PL/pgSQL body (pg_query.Parse
	// rejects it), so collectASTPositions returns empty maps and alias detection never runs.
	// Individual SQL statements extracted here ARE parseable, so we re-run the formatter
	// on each one (without layout, to avoid double-breaking) to pick up alias detection,
	// casing, and other formatter rules that require a successful parse.
	cfgNoLayout := *cfg
	cfgNoLayout.Layout = config.LayoutConfig{}
	cfgNoLayout.Nested = true // (a statement of a body: resolveBodyAliases)
	cfgNoLayout.OwnLineParen = cfg.OwnLineParen || cfg.Layout.ParenIndent.Open == config.ParenOpenOwnLine
	// (trailing comments are aligned after the layout, on the whole body)
	cfgNoLayout.Comments.AlignTrailing = config.CommentAlignPreserve
	cfgNoLayout.Semicolons = config.SemicolonPreserve
	cfgNoLayout.TrailingNewline = config.TrailingNewlinePreserve // a statement inside a body is not a file

	// applyLayoutToSQL returns the laid-out statement; drop is the number of blanks at the end of
	// prefix that the result replaces (layout.clauses.align: right pads the first keyword).
	applyLayoutToSQL := func(prefix, sqlText string) (out string, ok bool, drop int) {
		baseIndent := lastLineIndent(prefix)
		lc := bodyLayoutConfig(cfg)
		if lc.Clauses.Align == config.AlignRight && strings.TrimSpace(prefix[strings.LastIndexByte(prefix, '\n')+1:]) != "" &&
			!caseBranchMovesBody(body[:lastEnd+len(prefix)], body[lastEnd+len(prefix):], cfg) {
			// RETURN QUERY SELECT …: the statement does not start its line, so there is no
			// edge to pad its keyword to
			lc.Clauses.Align = config.AlignSame
		}
		if lc.Clauses.Align == config.AlignRight {
			// The blanks in front of the statement may be the river padding of a former run:
			// the body indent is what they share with the lines below.
			base := baseIndent
			sqlLines := strings.Split(sqlText, "\n")
			sqlInside := linesInsideTokens(sqlText, len(sqlLines)) // (lines of a string constant are data)
			for li, ln := range sqlLines[1:] {
				if strings.TrimSpace(ln) == "" || sqlInside[li+1] {
					continue
				}
				ind := ln[:len(ln)-len(strings.TrimLeft(ln, " \t"))]
				n := 0
				for n < len(base) && n < len(ind) && base[n] == ind[n] {
					n++
				}
				base = base[:n]
			}
			drop = len(baseIndent) - len(base)
			baseIndent = base
		}

		// Strip the body's base indent from continuation lines so that
		// reindentLines adds it back exactly once (without this, Phase 2
		// sees FROM users already at 4sp, preserves it, then reindentLines
		// prepends another 4sp giving double indentation).
		rawText := sqlText
		sqlText = stripLeadingIndent(sqlText, baseIndent)

		// Phase 1: apply full formatter rules (alias detection, casing) now that we have
		// a valid SQL statement that pg_query.Parse can handle.
		preFormatted, preErr := formatBody(sqlText, &cfgNoLayout, vars)
		preChanged := preErr == nil && preFormatted != sqlText
		if preChanged {
			sqlText = preFormatted
		}

		// A statement that follows another one on the same line (`PERFORM f(1); PERFORM f(2);`) has no
		// line of its own to take an indent from: broken up, its lines would land at the margin and
		// move on every run. It keeps its place; only the token rules above apply to it.
		before := body[:lastEnd+len(prefix)] // (prefix is body[lastEnd:start of the statement])
		if lastLine := before[strings.LastIndexByte(before, '\n')+1:]; strings.HasSuffix(strings.TrimRight(lastLine, " \t"), ";") || endsWithOpenerWord(lastLine) && !caseBranchMovesBody(before, body[len(before):], cfg) {
			if cfg.Layout.ParenIndent.Open == config.ParenOpenOwnLine {
				// paren_indent.open: own_line is no line breaking: the "(" that ends its line goes to a line of
				// its own here too (the token pass may just have joined it to its keyword)
				ownLine := &config.LayoutConfig{ParenIndent: config.ParenIndentCfg{Open: config.ParenOpenOwnLine}}
				if !preChanged {
					// only the move: on the lines as they stand (their indent need not be the body's base indent
					// a re-indentation could restore: tabs against blanks)
					if moved, err := layout.Apply(rawText, ownLine); err == nil && moved != rawText {
						return moved, true, 0
					}
				} else if moved, err := layout.Apply(sqlText, ownLine); err == nil && moved != sqlText {
					sqlText = moved
				}
			}
			if !preChanged {
				return "", false, 0
			}
			return reindentLines(sqlText, baseIndent), true, 0
		}

		// Phase 2: apply layout (clause breaking, indentation normalization).
		formatted, fmtErr := layout.Apply(sqlText, lc)
		layoutChanged := fmtErr == nil && formatted != sqlText

		if !preChanged && !layoutChanged {
			return "", false, 0
		}
		if !layoutChanged {
			// Only formatter rules changed (e.g. AS added), no clause restructuring.
			return reindentLines(sqlText, baseIndent), true, 0
		}
		return reindentLines(formatted, baseIndent), true, drop
	}

	for ti, tok := range scanResult.Tokens {
		tokStart := int(tok.Start)
		tokEnd := int(tok.End)
		text := body[tokStart:tokEnd]

		switch text {
		case "(", "[":
			parenDepth++
			continue
		case ")", "]":
			if parenDepth > 0 {
				parenDepth--
			}
			continue
		}

		if parenDepth > 0 {
			continue
		}
		if strings.HasPrefix(text, "--") || strings.HasPrefix(text, "/*") {
			continue // (a comment between `IN` and `SELECT` does not make the SELECT another kind of word)
		}

		lower := strings.ToLower(text)

		// IF / ELSIF / WHILE condition: not a statement, but a subquery in it (exists (select …))
		// is laid out like any other (layout.subquery.break).
		if sqlStart < 0 && condStart < 0 && (lower == "if" || lower == "elsif" || lower == "elseif" || lower == "while") &&
			(prevLower == "" || prevLower == ";" || prevLower == "begin" || prevLower == "then" || prevLower == "else" || prevLower == "loop") {
			condStart, condEnd = tokStart, "then"
			if lower == "while" {
				condEnd = "loop"
			}
			prevLower = lower
			continue
		}
		if condStart >= 0 && lower == condEnd {
			prefix := body[lastEnd:condStart]
			head := prefix[strings.LastIndexByte(prefix, '\n')+1:]
			text := head + body[condStart:tokStart]
			if out := layout.ExpandSubqueries(text, bodyLayoutConfig(cfg)); out != text {
				result.WriteString(prefix[:len(prefix)-len(head)])
				result.WriteString(out)
			} else {
				result.WriteString(body[lastEnd:tokStart])
			}
			lastEnd = tokStart
			condStart = -1
			prevLower = lower
			continue
		}

		if sqlStart < 0 && (lower == "grant" || lower == "revoke") {
			inPrivilege = true // GRANT SELECT, UPDATE ON …: the privileges are no statements
		}
		if sqlStart < 0 && bodyOpaqueDDL(body, scanResult.Tokens, ti, prevLower) {
			inPrivilege = true // CREATE RULE … ON UPDATE TO t DO …, CREATE TRIGGER … BEFORE UPDATE ON t: the words are no statements
		}
		if sqlStart < 0 && !inPrivilege && (sqlClauseStarters[lower] && (lower != "with" || withStartsQuery(prevLower)) ||
			bodyDDLStarts(body, scanResult.Tokens, ti, prevLower)) {
			// Found the start of a SQL statement; remember position.
			// If SELECT follows IN (FOR r IN SELECT...LOOP), mark it as a cursor query
			// so we terminate at LOOP rather than the first unrelated ';'.
			sqlStart = tokStart
			forInSQL = (lower == "select" || lower == "with" || lower == "update" || lower == "delete" || lower == "insert" || lower == "values") && prevLower == "in"
			prevLower = lower
			continue
		}
		prevLower = lower

		if lower == "loop" && forInSQL && sqlStart >= 0 {
			// End of a FOR r IN SELECT...LOOP cursor query.
			// Trim trailing whitespace/newlines (and any stray semicolons from the source)
			// to get just the SQL, preserving the gap (the "\n\tindent" before LOOP)
			// so LOOP stays on its own indented line.
			prefix := body[lastEnd:sqlStart]
			rawBlock := body[sqlStart:tokStart]
			sqlText := strings.TrimRight(rawBlock, " \t\n;")
			gapBeforeLoop := strings.TrimLeft(rawBlock[len(sqlText):], ";")
			if reformatted, ok, drop := applyLayoutToSQL(prefix, sqlText); ok {
				result.WriteString(prefix[:len(prefix)-drop])
				result.WriteString(reformatted)
				result.WriteString(gapBeforeLoop)
			} else {
				result.WriteString(body[lastEnd:tokStart])
			}
			lastEnd = tokStart
			sqlStart = -1
			forInSQL = false
			continue
		}

		if text == ";" && sqlStart >= 0 && !forInSQL {
			// End of a SQL statement we were tracking.
			// Everything from lastEnd to sqlStart is prefix (BEGIN, RETURN QUERY, etc.)
			prefix := body[lastEnd:sqlStart]
			sqlText := strings.TrimRight(body[sqlStart:tokStart], " \t\n")
			// (a statement whose own `;` was lost — in a comment — runs on into the END of its block:
			// it is no statement to lay out, and neither is it once end_semicolon has put a `;` there)
			if reformatted, ok, drop := applyLayoutToSQL(prefix, sqlText); ok && !runsIntoBlockEnd(sqlText) {
				result.WriteString(prefix[:len(prefix)-drop])
				result.WriteString(reformatted)
				result.WriteByte(';')
			} else {
				result.WriteString(body[lastEnd:tokEnd])
			}
			lastEnd = tokEnd
			sqlStart = -1
			forInSQL = false
			continue
		}

		if text == ";" && sqlStart < 0 {
			inPrivilege = false
			// Non-SQL statement ending (RAISE, assignment, END, etc.): output verbatim.
			result.WriteString(body[lastEnd:tokEnd])
			lastEnd = tokEnd
			continue
		}
	}

	// Flush any remaining body content.
	result.WriteString(body[lastEnd:])
	return result.String(), nil
}

// runsIntoBlockEnd reports whether the SQL text ends in an END that closes no CASE of its own (it is the
// END of the block the statement stands in).
func runsIntoBlockEnd(sqlText string) bool {
	res, err := pg_query.Scan(sqlText)
	if err != nil || len(res.Tokens) == 0 {
		return false
	}
	depth := 0
	last := ""
	for _, t := range res.Tokens {
		w := strings.ToLower(sqlText[t.Start:t.End])
		switch w {
		case "case":
			depth++
		case "end":
			depth--
		}
		last = w
	}
	return last == "end" && depth < 0
}

// endsWithOpenerWord reports whether a line of a PL/pgSQL body ends with THEN, ELSE or LOOP followed by
// nothing else (a statement that follows it is on the line of its opener: `if a then select …;`).
func endsWithOpenerWord(line string) bool {
	t := strings.ToLower(strings.TrimSpace(line))
	for _, w := range []string{"then", "else", "loop"} {
		if t == w || strings.HasSuffix(t, " "+w) || strings.HasSuffix(t, "\t"+w) {
			return true
		}
	}
	return false
}

// caseBranchMovesBody reports whether text (a PL/pgSQL body up to a statement that follows THEN) ends in the
// `WHEN … THEN` of a CASE branch whose body case.*.body_break: always puts on a line of its own afterwards:
// the statement is laid out as the line-start statement it will be (kept where it stands, the next run
// found it on its own line).
func caseBranchMovesBody(text, rest string, cfg *config.Config) bool {
	t := strings.ToLower(strings.TrimRight(text, " \t\n"))
	if !strings.HasSuffix(t, "then") {
		return false
	}
	// the keyword that opened this THEN: the last WHEN / IF / ELSIF before it
	m := regexp.MustCompile(`\b(when|if|elsif|elseif)\b`).FindAllStringSubmatch(t, -1)
	if len(m) == 0 || m[len(m)-1][1] != "when" {
		return false
	}
	c := cfg.Layout.DollarQuote.PLpgSQL.ControlFlow.Case
	if c.Simple.BodyBreak == config.BreakAlways || c.Searched.BodyBreak == config.BreakAlways {
		return true
	}
	if c.Simple.BodyBreak == config.BreakAuto || c.Searched.BodyBreak == config.BreakAuto {
		// auto breaks the body of a branch of more than one statement
		end := branchEnd.FindStringIndex(strings.ToLower(rest))
		if end == nil {
			return false
		}
		return strings.Count(stringConst.ReplaceAllString(rest[:end[0]], "''"), ";") > 1
	}
	return false
}

var (
	branchEnd   = regexp.MustCompile(`(?im)^[ \t]*(?:when|else|elsif|elseif|if|case|end)\b|\bloop\b`) // (the next control-flow event: the body of the branch is the text up to it)
	stringConst = regexp.MustCompile(`'(?:[^']|'')*'`)
)

// bodyOpaqueDDL reports whether the token ti opens a CREATE / ALTER of a rule, trigger, policy or
// default privileges: statements with SQL words (UPDATE, DELETE, INSERT, SELECT) that do not start
// a statement of their own.
func bodyOpaqueDDL(body string, toks []*pg_query.ScanToken, ti int, prevLower string) bool {
	switch prevLower {
	case "", ";", "begin", "then", "else", "loop", "exception":
	default:
		return false
	}
	word := func(k int) string {
		if k < 0 || k >= len(toks) {
			return ""
		}
		return strings.ToLower(body[toks[k].Start:toks[k].End])
	}
	if w := word(ti); w != "create" && w != "alter" {
		return false
	}
	for k := ti + 1; k < len(toks) && k < ti+6; k++ {
		switch word(k) {
		case "rule", "trigger", "policy", "privileges":
			return true
		}
	}
	return false
}

// bodyDDLStarts reports whether the token ti opens a DDL statement of a PL/pgSQL body that the layout
// handles like at the top level (CREATE [UNIQUE] INDEX, CREATE [TEMP | UNLOGGED …] TABLE, ALTER TABLE,
// DROP …); CREATE FUNCTION and the like, which hold bodies of their own, are left as they are.
func bodyDDLStarts(body string, toks []*pg_query.ScanToken, ti int, prevLower string) bool {
	switch prevLower {
	case "", ";", "begin", "then", "else", "loop", "exception":
	default:
		return false
	}
	word := func(k int) string {
		if k < 0 || k >= len(toks) {
			return ""
		}
		return strings.ToLower(body[toks[k].Start:toks[k].End])
	}
	switch word(ti) {
	case "drop":
		return true
	case "alter":
		return word(ti+1) == "table"
	case "create":
		for k := ti + 1; k < len(toks) && k < ti+6; k++ {
			switch word(k) {
			case "table", "index":
				return true
			case "unique", "temp", "temporary", "unlogged", "global", "local", "or", "replace", "concurrently":
			default:
				return false
			}
		}
	}
	return false
}

// lastLineIndent returns the leading whitespace of the last line in s.
func lastLineIndent(s string) string {
	lastNL := strings.LastIndexByte(s, '\n')
	var line string
	if lastNL < 0 {
		line = s
	} else {
		line = s[lastNL+1:]
	}
	j := 0
	for j < len(line) && (line[j] == ' ' || line[j] == '\t') {
		j++
	}
	return line[:j]
}

// stripLeadingIndent removes indent from the start of every non-first line of sql.
// This undoes the body's base indentation before layout/normalize so that
// reindentLines can re-add it exactly once.
func stripLeadingIndent(sql, indent string) string {
	if indent == "" {
		return sql
	}
	lines := strings.SplitAfter(sql, "\n")
	inside := linesInsideTokens(sql, len(lines)) // (the lines of a string constant are data)
	for i := 1; i < len(lines); i++ {
		if !inside[i] {
			lines[i] = strings.TrimPrefix(lines[i], indent)
		}
	}
	return strings.Join(lines, "")
}

// reindentLines prepends baseIndent to every line in sql except the first and
// empty lines — used to restore PL/pgSQL body indentation after layout breaks clauses.
func reindentLines(sql, baseIndent string) string {
	if baseIndent == "" {
		return sql
	}
	lines := strings.SplitAfter(sql, "\n")
	inside := linesInsideTokens(sql, len(lines)) // (the lines of a string constant are data)
	var sb strings.Builder
	for i, line := range lines {
		if i == 0 || strings.TrimRight(line, "\n") == "" || inside[i] {
			sb.WriteString(line)
		} else {
			sb.WriteString(baseIndent)
			sb.WriteString(line)
		}
	}
	return sb.String()
}

// controlFlowIsNoop returns true when all ControlFlowCfg settings are preserve/empty.
func controlFlowIsNoop(cfg config.ControlFlowCfg) bool {
	isPreserveBIM := func(m config.BodyIndentMode) bool {
		return m == "" || m == config.BodyIndentPreserve
	}
	isPreserveBLA := func(a config.BlankLineAction) bool {
		return a == "" || a == config.BlankLinePreserve
	}
	isPreserveBM := func(m config.BreakMode) bool {
		return m == "" || m == config.BreakPreserve
	}
	caseBranchNoop := func(b config.CaseBranchCfg) bool {
		return isPreserveBIM(b.WhenIndent) &&
			isPreserveBM(b.ThenBreak) &&
			isPreserveBIM(b.ThenIndent) &&
			isPreserveBM(b.BodyBreak) &&
			isPreserveBIM(b.BodyIndent) &&
			isPreserveBLA(b.BlankLineBefore) &&
			isPreserveBLA(b.BlankLineAfter)
	}
	return isPreserveBIM(cfg.If.BodyIndent) &&
		isPreserveBLA(cfg.If.BlankLineBefore) &&
		isPreserveBLA(cfg.If.BlankLineAfter) &&
		isPreserveBIM(cfg.Loop.BodyIndent) &&
		isPreserveBLA(cfg.Loop.BlankLineBefore) &&
		isPreserveBLA(cfg.Loop.BlankLineAfter) &&
		caseBranchNoop(cfg.Case.Simple) &&
		caseBranchNoop(cfg.Case.Searched)
}

// reformatControlFlow applies indentation and blank-line rules to IF/ELSIF/ELSE/
// END IF, LOOP/END LOOP and CASE/END CASE constructs within a PL/pgSQL begin body.
// lineLength is used for auto-break decisions within CASE branches.
// It is called after applyBodyIndent so all lines already carry their base indent.
func reformatControlFlow(body, indentUnit string, cfg config.ControlFlowCfg, lineLength int, flush bool) string {
	if controlFlowIsNoop(cfg) {
		return body
	}

	scanResult, err := pg_query.Scan(body)
	if err != nil {
		return body
	}

	// Depth 1 is one indent unit, or no indent at all when the body of BEGIN is flush left
	// (begin_body.indent: none); each further level adds one unit.
	indentAt := func(depth int) string {
		if flush && depth > 0 {
			depth--
		}
		return strings.Repeat(indentUnit, depth)
	}

	// cfEvent types
	type cfEventKind int
	const (
		evNone     cfEventKind = iota
		evIF                   // IF keyword (not in END IF)
		evTHEN                 // THEN keyword (after IF or ELSIF)
		evELSIF                // ELSIF keyword
		evELSE                 // ELSE keyword
		evENDIF                // END IF
		evLOOP                 // LOOP keyword (bare, FOR...LOOP, WHILE...LOOP)
		evENDLOOP              // END LOOP
		evCASE                 // CASE keyword (PL/pgSQL statement)
		evWHEN                 // WHEN keyword inside PL/pgSQL CASE
		evCASETHEN             // THEN keyword after WHEN (inside CASE)
		evCASEELSE             // ELSE keyword inside PL/pgSQL CASE
		evENDCASE              // END CASE
	)

	type cfFrame struct {
		kind       cfEventKind // evIF, evLOOP, or evCASE
		lastEvent  cfEventKind // tracks last event in frame for THEN/WHEN detection
		caseSimple bool        // true = simple CASE (CASE expr WHEN), false = searched (CASE WHEN)
	}

	// Tokenise once into a flat list.
	type scanTok struct {
		start, end int
		text       string
	}
	toks := make([]scanTok, len(scanResult.Tokens))
	for i, t := range scanResult.Tokens {
		toks[i] = scanTok{int(t.Start), int(t.End), strings.ToUpper(body[t.Start:t.End])}
	}

	// Build event list.
	type event struct {
		kind        cfEventKind
		start, end  int  // byte span of the keyword(s) in body
		extraIndent int  // nesting depth used by depth formulas
		caseSimple  bool // true = simple CASE, false = searched (only set on CASE-related events)
	}
	var events []event

	parenDepth := 0
	caseDepth := 0
	var stack []cfFrame
	prevKw := "" // previous keyword-level token (upper)

	// We'll do two passes: first collect events, then reconstruct.
	// Lookahead: for END, we need to see next token.
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		switch t.text {
		case "(", "[":
			parenDepth++
			continue
		case ")", "]":
			if parenDepth > 0 {
				parenDepth--
			}
			continue
		}
		if parenDepth > 0 {
			continue
		}

		// Helper: peek at next non-paren token.
		peekNext := func() string {
			for j := i + 1; j < len(toks); j++ {
				if toks[j].text == "(" || toks[j].text == "[" {
					continue
				}
				return toks[j].text
			}
			return ""
		}

		// isPLCASE returns true when the CASE keyword introduces a PL/pgSQL
		// statement rather than a SQL expression. We check the preceding keyword.
		isPLCASE := func(prev string) bool {
			switch prev {
			case "", ";", "BEGIN", "THEN", "ELSE", "ELSIF", "LOOP", "ENDIF", "ENDLOOP", "ENDCASE":
				return true
			}
			return false
		}

		switch t.text {
		case "CASE":
			if isPLCASE(prevKw) {
				// Peek past CASE to determine simple vs searched form.
				isSimple := peekNext() != "WHEN"
				stack = append(stack, cfFrame{kind: evCASE, lastEvent: evCASE, caseSimple: isSimple})
				events = append(events, event{kind: evCASE, start: t.start, end: t.end, extraIndent: len(stack) - 1, caseSimple: isSimple})
			} else if prevKw != "END" {
				caseDepth++
			}
		case "END":
			next := peekNext()
			if next == "IF" {
				if len(stack) > 0 && stack[len(stack)-1].kind == evIF {
					stack = stack[:len(stack)-1]
				}
				endStart := t.start
				i++
				endEnd := toks[i].end
				events = append(events, event{kind: evENDIF, start: endStart, end: endEnd, extraIndent: len(stack)})
				prevKw = "ENDIF"
				continue
			} else if next == "LOOP" {
				if len(stack) > 0 && stack[len(stack)-1].kind == evLOOP {
					stack = stack[:len(stack)-1]
				}
				endStart := t.start
				i++
				endEnd := toks[i].end
				events = append(events, event{kind: evENDLOOP, start: endStart, end: endEnd, extraIndent: len(stack)})
				prevKw = "ENDLOOP"
				continue
			} else if next == "CASE" {
				isSimple := false
				if len(stack) > 0 && stack[len(stack)-1].kind == evCASE {
					isSimple = stack[len(stack)-1].caseSimple
					stack = stack[:len(stack)-1]
				}
				endStart := t.start
				i++
				endEnd := toks[i].end
				events = append(events, event{kind: evENDCASE, start: endStart, end: endEnd, extraIndent: len(stack), caseSimple: isSimple})
				prevKw = "ENDCASE"
				continue
			} else {
				if caseDepth > 0 {
					caseDepth--
				}
			}
		case "IF":
			if prevKw != "END" && caseDepth == 0 {
				stack = append(stack, cfFrame{kind: evIF, lastEvent: evIF})
				events = append(events, event{kind: evIF, start: t.start, end: t.end, extraIndent: len(stack) - 1})
			}
		case "WHEN":
			if caseDepth == 0 && len(stack) > 0 && stack[len(stack)-1].kind == evCASE {
				fr := stack[len(stack)-1]
				stack[len(stack)-1].lastEvent = evWHEN
				events = append(events, event{kind: evWHEN, start: t.start, end: t.end, extraIndent: len(stack) - 1, caseSimple: fr.caseSimple})
			}
		case "THEN":
			if caseDepth == 0 && len(stack) > 0 {
				fr := stack[len(stack)-1]
				if fr.kind == evIF && (fr.lastEvent == evIF || fr.lastEvent == evELSIF) {
					stack[len(stack)-1].lastEvent = evTHEN
					events = append(events, event{kind: evTHEN, start: t.start, end: t.end, extraIndent: len(stack)})
				} else if fr.kind == evCASE && fr.lastEvent == evWHEN {
					stack[len(stack)-1].lastEvent = evCASETHEN
					events = append(events, event{kind: evCASETHEN, start: t.start, end: t.end, extraIndent: len(stack) - 1, caseSimple: fr.caseSimple})
				}
			}
		case "ELSIF":
			if caseDepth == 0 && len(stack) > 0 && stack[len(stack)-1].kind == evIF {
				stack[len(stack)-1].lastEvent = evELSIF
				events = append(events, event{kind: evELSIF, start: t.start, end: t.end, extraIndent: len(stack) - 1})
			}
		case "ELSE":
			if caseDepth == 0 && len(stack) > 0 {
				fr := stack[len(stack)-1]
				if fr.kind == evIF {
					stack[len(stack)-1].lastEvent = evELSE
					events = append(events, event{kind: evELSE, start: t.start, end: t.end, extraIndent: len(stack) - 1})
				} else if fr.kind == evCASE {
					stack[len(stack)-1].lastEvent = evCASEELSE
					events = append(events, event{kind: evCASEELSE, start: t.start, end: t.end, extraIndent: len(stack) - 1, caseSimple: fr.caseSimple})
				}
			}
		case "LOOP":
			if caseDepth == 0 {
				stack = append(stack, cfFrame{kind: evLOOP})
				events = append(events, event{kind: evLOOP, start: t.start, end: t.end, extraIndent: len(stack)})
			}
		}
		prevKw = t.text
	}

	// If no events, return unchanged.
	if len(events) == 0 {
		return body
	}

	// Validate: stack should be empty at end (balanced structures).
	// If not, bail out to avoid mangling malformed code.
	if len(stack) != 0 {
		return body
	}

	// normalizeBodyLines normalizes non-blank lines in seg to the given target indent.
	// Strips the minimum existing indent then applies target, making the operation idempotent.
	// Only operates in "indent" mode; "none" and "preserve" are no-ops here.
	normalizeBodyLines := func(seg string, targetStr string, mode config.BodyIndentMode) string {
		if mode != config.BodyIndentIndent || targetStr == "" {
			return seg
		}
		lines := strings.Split(seg, "\n")
		inside := linesInsideTokens(seg, len(lines)) // (the lines of a string constant are data)
		// A segment starts right after an opener keyword: its first line is the rest of the keyword's
		// line (IF a THEN RETURN 1; ELSE …), not a line of its own, and keeps its place.
		var codeLines []string
		for i, line := range lines[1:] {
			if !inside[i+1] {
				codeLines = append(codeLines, line)
			}
		}
		minInd := findMinIndent(codeLines)
		for i, line := range lines {
			if i == 0 || strings.TrimSpace(line) == "" || inside[i] {
				continue
			}
			stripped := trimIndent(line, minInd)
			lines[i] = targetStr + stripped
		}
		return strings.Join(lines, "\n")
	}

	// applyBlankBefore adjusts the number of blank lines before a keyword.
	// preceding ends with the keyword's indent (e.g. "\n  " for a 2-space indented keyword).
	applyBlankBefore := func(preceding string, action config.BlankLineAction) string {
		if action == config.BlankLinePreserve || action == "" {
			return preceding
		}
		lastNL := strings.LastIndexByte(preceding, '\n')
		if lastNL < 0 {
			// the keyword is on the same line as the content before it (`if a then x := 1; end if;`):
			// there is no line of its own to put blank lines before
			return preceding
		}
		beforeLastNL := preceding[:lastNL] // content lines
		kwIndent := preceding[lastNL+1:]   // leading spaces of the keyword line
		content := strings.TrimRight(beforeLastNL, "\n")
		switch action {
		case config.BlankLineRemove:
			return content + "\n" + kwIndent
		case config.BlankLineAdd:
			return content + "\n\n" + kwIndent
		}
		return preceding
	}

	// applyBlankAfter adjusts the leading newlines of the text that follows a keyword.
	applyBlankAfter := func(following string, action config.BlankLineAction) string {
		if action == config.BlankLinePreserve || action == "" {
			return following
		}
		// a comment after the keyword on its line (`ELSE -- why`) stays there: the blank lines
		// are those after it
		head := ""
		if nl := strings.IndexByte(following, '\n'); nl >= 0 {
			if t := strings.TrimSpace(following[:nl]); strings.HasPrefix(t, "--") || strings.HasPrefix(t, "/*") {
				head, following = following[:nl], following[nl:]
			}
		}
		if head == "" && !strings.HasPrefix(strings.TrimLeft(following, " \t"), "\n") && strings.TrimSpace(following) != "" {
			return following // a statement on the line of the keyword (`then x := 1;`): no blank lines to adjust
		}
		if t := strings.TrimLeft(following, " \t"); strings.HasPrefix(t, "\n") {
			following = t // (blanks left behind on the keyword's line)
		}
		stripped := leadingBlankLines.ReplaceAllString(following, "") // (a line of blanks is a blank line)
		if strings.TrimSpace(following) == "" {
			// nothing but blanks up to the next keyword: its indent is what stays
			stripped = following[strings.LastIndexByte(following, '\n')+1:]
		}
		switch action {
		case config.BlankLineRemove:
			return head + "\n" + stripped
		case config.BlankLineAdd:
			return head + "\n\n" + stripped
		}
		return head + following
	}

	// branchCfgFor returns the appropriate CaseBranchCfg based on whether the CASE is simple.
	branchCfgFor := func(isSimple bool) config.CaseBranchCfg {
		if isSimple {
			return cfg.Case.Simple
		}
		return cfg.Case.Searched
	}

	// kwDepthForEvent returns the absolute indent depth for a keyword event.
	// depth * indentUnit is the keyword's target leading whitespace.
	kwDepthForEvent := func(ev event) int {
		switch ev.kind {
		case evIF, evELSIF, evELSE, evENDIF, evENDLOOP:
			return ev.extraIndent + 1
		case evLOOP:
			// LOOP.extraIndent = len(stack) after push (includes the LOOP frame).
			return ev.extraIndent
		case evCASE, evENDCASE:
			return ev.extraIndent + 1
		case evWHEN, evCASEELSE:
			bc := branchCfgFor(ev.caseSimple)
			if bc.WhenIndent == config.BodyIndentIndent {
				return ev.extraIndent + 2
			}
			return ev.extraIndent + 1
		default:
			return 0 // evTHEN, evCASETHEN — stays on same line as IF/WHEN
		}
	}

	// bodyDepthForOpener returns the absolute indent depth for body content
	// that follows an opener keyword (THEN, ELSE, LOOP, CASETHEN, CASEELSE).
	bodyDepthForOpener := func(ev event) int {
		switch ev.kind {
		case evTHEN:
			return ev.extraIndent + 1
		case evELSE:
			return ev.extraIndent + 2
		case evLOOP:
			return ev.extraIndent + 1
		case evCASETHEN, evCASEELSE:
			bc := branchCfgFor(ev.caseSimple)
			if bc.WhenIndent == config.BodyIndentIndent {
				return ev.extraIndent + 3
			}
			return ev.extraIndent + 2
		default:
			return 0
		}
	}

	// collapseToSameLine strips leading whitespace/newlines from s and prepends a single space.
	// Used to move body content onto the same line as the preceding keyword.
	collapseToSameLine := func(s string) string {
		if strings.Contains(s, "--") {
			return s // a line comment ends its line: what follows may not be joined onto it
		}
		stripped := strings.TrimLeft(s, " \t\n")
		if stripped == "" {
			return ""
		}
		return " " + stripped
	}

	// Pre-pass: for each evCASETHEN and evCASEELSE, compute break decisions.
	type caseOpenerDecision struct {
		// For evCASETHEN: controls line break before THEN.
		breakBefore    bool // insert \n+indent before THEN
		collapseBefore bool // force THEN onto same line as WHEN (strip stray \n)
		// For both evCASETHEN and evCASEELSE: controls body position after keyword.
		breakAfter    bool // body on new line after keyword
		collapseAfter bool // body on same line as keyword
	}
	caseDecisions := make(map[int]caseOpenerDecision)

	for ei, ev := range events {
		if ev.kind != evCASETHEN && ev.kind != evCASEELSE {
			continue
		}
		bc := branchCfgFor(ev.caseSimple)

		// Determine the body segment (after this keyword, before next event).
		bodyStart := ev.end
		bodyEnd := len(body)
		if ei+1 < len(events) {
			bodyEnd = events[ei+1].start
		}
		bodySeg := body[bodyStart:bodyEnd]
		isMulti := strings.Count(bodySeg, ";") > 1

		var dec caseOpenerDecision

		if ev.kind == evCASETHEN {
			// Compute breakBefore / collapseBefore for THEN.
			whenEv := events[ei-1] // always evWHEN immediately before evCASETHEN
			valSeg := body[whenEv.end:ev.start]
			thenBreak := bc.ThenBreak
			if strings.Contains(valSeg, "--") {
				thenBreak = config.BreakPreserve // WHEN x -- why \n THEN …: THEN stays on the line after the comment
			}
			switch thenBreak {
			case config.BreakNever:
				dec.collapseBefore = true
			case config.BreakAlways:
				dec.breakBefore = true
			case config.BreakAuto:
				if isMulti {
					dec.breakBefore = true
				} else {
					// Measure: indent + "WHEN" + val + " THEN" + " " + firstBodyLine
					whenDepth := kwDepthForEvent(whenEv)
					firstLine := strings.TrimLeft(strings.SplitN(strings.TrimLeft(bodySeg, " \t\n"), "\n", 2)[0], " \t")
					lineW := whenDepth*len(indentUnit) + 4 + len(strings.TrimSpace(valSeg)) + 5 + 1 + len(firstLine)
					if lineW > lineLength {
						dec.breakBefore = true
					} else {
						dec.collapseBefore = true
					}
				}
				// preserve: neither breakBefore nor collapseBefore — leave original whitespace
			}
		}

		// Compute breakAfter / collapseAfter (body position after THEN or ELSE).
		switch bc.BodyBreak {
		case config.BreakNever:
			dec.collapseAfter = true
		case config.BreakAlways:
			dec.breakAfter = true
		case config.BreakAuto:
			if isMulti {
				dec.breakAfter = true
			} else if ev.kind == evCASETHEN && dec.breakBefore {
				// Single long stmt: THEN is on new line, body continues on THEN line.
				dec.collapseAfter = true
			} else if ev.kind == evCASETHEN {
				// Single stmt fits: body stays on THEN line (already collapsed with THEN).
				dec.collapseAfter = true
			} else {
				// evCASEELSE auto: check line width.
				elseDepth := kwDepthForEvent(ev)
				firstLine := strings.TrimLeft(strings.SplitN(strings.TrimLeft(bodySeg, " \t\n"), "\n", 2)[0], " \t")
				lineW := elseDepth*len(indentUnit) + 4 + 1 + len(firstLine) // "ELSE" + " " + first
				if lineW > lineLength {
					dec.breakAfter = true
				} else {
					dec.collapseAfter = true
				}
			}
			// preserve: neither — leave original whitespace
		}

		caseDecisions[ei] = dec
	}

	// Reconstruct body by processing events and segments between them.
	// For each event:
	//   1. Take segment body[pos:ev.start] (includes trailing keyword indent)
	//   2. Apply blankAfter from previous opener, normalize body content to target depth
	//   3. Replace trailing keyword indent with computed target (for idempotence)
	//   4. Apply blankBefore adjustment
	//   5. Write segment and keyword text
	var out strings.Builder
	out.Grow(len(body) + len(body)/4)
	pos := 0
	// the indent an IF / ELSIF line had and got: the lines of its condition (up to THEN) move with it
	var condOld, condNew string
	condPending := false

	for ei, ev := range events {
		segText := body[pos:ev.start]
		if condPending && (ev.kind == evTHEN || ev.kind == evCASETHEN) && condOld != condNew {
			segText = reindentContinuation(segText, condOld, condNew)
		}
		condPending = false

		// Determine body-indent mode and opener depth from the previous event.
		// For CASE openers (evCASETHEN, evCASEELSE), caseDecisions overrides behaviour.
		var segMode config.BodyIndentMode
		var segBodyDepth int
		var prevOpener cfEventKind
		var prevOpenerEI int
		if ei > 0 {
			prevEv := events[ei-1]
			prevOpenerEI = ei - 1
			switch prevEv.kind {
			case evTHEN:
				segMode = cfg.If.BodyIndent
				segBodyDepth = bodyDepthForOpener(prevEv)
				prevOpener = evTHEN
			case evELSE:
				segMode = cfg.If.BodyIndent
				segBodyDepth = bodyDepthForOpener(prevEv)
				prevOpener = evELSE
			case evLOOP:
				segMode = cfg.Loop.BodyIndent
				segBodyDepth = bodyDepthForOpener(prevEv)
				prevOpener = evLOOP
			case evCASETHEN:
				prevOpener = evCASETHEN
				dec := caseDecisions[prevOpenerEI]
				if dec.breakAfter {
					// Body on new line — normalize indentation.
					bc := branchCfgFor(prevEv.caseSimple)
					segMode = bc.BodyIndent
					segBodyDepth = bodyDepthForOpener(prevEv)
				}
				// collapseAfter or preserve: segMode stays "" (no normalization).
			case evCASEELSE:
				prevOpener = evCASEELSE
				dec := caseDecisions[prevOpenerEI]
				if dec.breakAfter {
					bc := branchCfgFor(prevEv.caseSimple)
					segMode = bc.BodyIndent
					segBodyDepth = bodyDepthForOpener(prevEv)
				}
			}
		}

		// Apply blankAfter / collapse from the previous opener keyword.
		if prevOpener == evTHEN || prevOpener == evELSE {
			segText = applyBlankAfter(segText, cfg.If.BlankLineAfter)
		} else if prevOpener == evLOOP {
			segText = applyBlankAfter(segText, cfg.Loop.BlankLineAfter)
		} else if prevOpener == evCASETHEN || prevOpener == evCASEELSE {
			dec := caseDecisions[prevOpenerEI]
			bc := branchCfgFor(events[prevOpenerEI].caseSimple)
			if dec.collapseAfter {
				segText = collapseToSameLine(segText)
			} else if dec.breakAfter {
				// Force body onto new line. A body that already starts on a line of its own keeps the
				// indent it has; one that follows the keyword gets the indent of its depth.
				rest := strings.TrimLeft(segText, " \t")
				var stripped string
				if strings.HasPrefix(rest, "\n") {
					stripped = strings.TrimLeft(rest, "\n")
				} else {
					target := indentAt(bodyDepthForOpener(events[prevOpenerEI]))
					// the lines below the first move with it: they stood relative to the line of the keyword
					kwStart := events[prevOpenerEI].start
					lineStart := strings.LastIndexByte(body[:kwStart], '\n') + 1
					oldBase := body[lineStart:kwStart]
					oldBase = oldBase[:len(oldBase)-len(strings.TrimLeft(oldBase, " \t"))]
					if oldBase != target {
						rest = reindentContinuation(rest, oldBase, target)
					}
					stripped = target + rest
				}
				if bc.BlankLineAfter == config.BlankLineAdd {
					segText = "\n\n" + stripped
				} else {
					segText = "\n" + stripped
				}
			}
			// preserve: leave segText unchanged
		}

		// Split segText into body lines and the trailing keyword indent (last line).
		lastNL := strings.LastIndexByte(segText, '\n')
		var bodyContent, kwIndentStr string
		if lastNL >= 0 {
			bodyContent = segText[:lastNL+1]
			kwIndentStr = segText[lastNL+1:]
		} else {
			bodyContent = segText
			kwIndentStr = ""
		}

		if segMode != "" && segMode != config.BodyIndentPreserve {
			targetBodyStr := indentAt(segBodyDepth)
			bodyContent = normalizeBodyLines(bodyContent, targetBodyStr, segMode)
		}

		// Replace only the leading whitespace of the keyword line with the target indent.
		// Special case for evCASETHEN: apply then_break decision instead of kwDepth.
		if ev.kind == evCASETHEN {
			dec := caseDecisions[ei]
			bc := branchCfgFor(ev.caseSimple)
			if dec.breakBefore {
				// Force line break before THEN.
				var thenDepth int
				if ei > 0 && events[ei-1].kind == evWHEN {
					thenDepth = kwDepthForEvent(events[ei-1])
					if bc.ThenIndent == config.BodyIndentIndent {
						thenDepth++
					}
				}
				bodyContent = strings.TrimRight(bodyContent, " \t\n") + "\n"
				kwIndentStr = indentAt(thenDepth)
			} else if dec.collapseBefore && lastNL >= 0 {
				// Collapse val+THEN onto same line (remove stray newlines in val segment).
				// Preserve a single space before THEN so "WHEN val THEN" reads correctly.
				all := bodyContent + kwIndentStr
				bodyContent = strings.TrimRight(all, " \t\n")
				kwIndentStr = " "
			}
			// preserve or no-newline case: leave as-is
		} else {
			kwDepth := kwDepthForEvent(ev)
			if kwDepth > 0 && lastNL >= 0 { // (a keyword in the middle of a line has no indent to set)
				rest := strings.TrimLeft(kwIndentStr, " \t")
				oldIndent := kwIndentStr[:len(kwIndentStr)-len(rest)]
				kwIndentStr = indentAt(kwDepth) + rest
				if ev.kind == evIF || ev.kind == evELSIF || ev.kind == evWHEN {
					condOld, condNew, condPending = oldIndent, indentAt(kwDepth), true
				}
			}
		}
		segText = bodyContent + kwIndentStr

		// Apply blankBefore for keywords that begin a new clause.
		switch ev.kind {
		case evIF, evELSIF, evELSE, evENDIF:
			segText = applyBlankBefore(segText, cfg.If.BlankLineBefore)
		case evENDLOOP:
			segText = applyBlankBefore(segText, cfg.Loop.BlankLineBefore)
		case evCASE, evWHEN, evCASEELSE, evENDCASE:
			bc := branchCfgFor(ev.caseSimple)
			segText = applyBlankBefore(segText, bc.BlankLineBefore)
		}

		out.WriteString(segText)
		out.WriteString(body[ev.start:ev.end])
		pos = ev.end
	}

	// Flush remaining body after the last event.
	if pos < len(body) {
		remaining := body[pos:]
		if len(events) > 0 {
			lastEvIdx := len(events) - 1
			lastEv := events[lastEvIdx]
			switch lastEv.kind {
			case evTHEN, evELSE:
				remaining = applyBlankAfter(remaining, cfg.If.BlankLineAfter)
				if cfg.If.BodyIndent != "" && cfg.If.BodyIndent != config.BodyIndentPreserve {
					target := indentAt(bodyDepthForOpener(lastEv))
					remaining = normalizeBodyLines(remaining, target, cfg.If.BodyIndent)
				}
			case evLOOP:
				remaining = applyBlankAfter(remaining, cfg.Loop.BlankLineAfter)
				if cfg.Loop.BodyIndent != "" && cfg.Loop.BodyIndent != config.BodyIndentPreserve {
					target := indentAt(bodyDepthForOpener(lastEv))
					remaining = normalizeBodyLines(remaining, target, cfg.Loop.BodyIndent)
				}
			case evCASETHEN, evCASEELSE:
				dec := caseDecisions[lastEvIdx]
				bc := branchCfgFor(lastEv.caseSimple)
				if dec.collapseAfter {
					remaining = collapseToSameLine(remaining)
				} else if dec.breakAfter {
					remaining = applyBlankAfter(remaining, bc.BlankLineAfter)
					if bc.BodyIndent != "" && bc.BodyIndent != config.BodyIndentPreserve {
						target := indentAt(bodyDepthForOpener(lastEv))
						remaining = normalizeBodyLines(remaining, target, bc.BodyIndent)
					}
				}
			}
		}
		out.WriteString(remaining)
	}

	return out.String()
}

// FormatFile reads the file at path, formats it, and returns the result.
func FormatFile(path string, cfg *config.Config) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return Format(string(data), cfg)
}

// isTriggerKeywordContext reports whether a "trigger" token preceded by prev is
// the TRIGGER keyword rather than the pseudo-type returned by trigger functions.
func isTriggerKeywordContext(prev string) bool {
	switch strings.ToLower(prev) {
	case "create", "drop", "alter", "replace", "on", "disable", "enable", "constraint", "event", "user", "system":
		return true
	}
	return false
}

// resolveSubselectAliases registers the alias token that follows a derived table
// — FROM/JOIN/LATERAL ( SELECT … ) alias — in pos.aliases. RangeSubselect nodes
// carry no location, so the alias is found by matching the closing paren of a
// "( SELECT|WITH|VALUES" group against the alias names reported by the AST.
func resolveSubselectAliases(tokens []rawToken, pos *astPositions) {
	if len(pos.subselectAliases) == 0 {
		return
	}
	isComment := func(t rawToken) bool { return strings.HasPrefix(t.text, "--") || strings.HasPrefix(t.text, "/*") }
	next := func(i int) int { // index of the next token that is not a comment
		for i++; i < len(tokens) && isComment(tokens[i]); i++ {
		}
		return i
	}
	for i := 0; i+1 < len(tokens); i++ {
		if tokens[i].text != "(" {
			continue
		}
		n := next(i)
		if n >= len(tokens) {
			break
		}
		switch strings.ToLower(tokens[n].text) {
		case "select", "with", "values":
		default:
			continue
		}
		if i > 0 {
			p := i - 1
			for p > 0 && isComment(tokens[p]) {
				p--
			}
			switch strings.ToLower(tokens[p].text) {
			case "from", "join", "lateral", ",", "using":
			default:
				continue
			}
		}
		depth := 0
		for j := i; j < len(tokens); j++ {
			switch tokens[j].text {
			case "(":
				depth++
			case ")":
				depth--
			}
			if depth != 0 {
				continue
			}
			k := next(j)
			if k < len(tokens) && strings.EqualFold(tokens[k].text, "as") {
				k = next(k)
			}
			if k < len(tokens) && pos.subselectAliases[strings.ToLower(tokens[k].text)] {
				pos.aliases[tokens[k].start] = true
			}
			break
		}
	}
}

// resolveFunctionAliases registers the alias of a table function — FROM/JOIN unnest(…) [AS] a(x, y)
// — in pos.aliases: the name that follows the call's closing parenthesis (and an optional AS)
// and is one of the aliases the AST reports for table functions.
func resolveFunctionAliases(tokens []rawToken, pos *astPositions) {
	if len(pos.functionAliases) == 0 {
		return
	}
	var opens []int // indexes of the "(" not closed yet
	for i := 0; i+1 < len(tokens); i++ {
		switch tokens[i].text {
		case "(":
			opens = append(opens, i)
			continue
		case ")":
		default:
			continue
		}
		open := -1
		if n := len(opens); n > 0 {
			open = opens[n-1]
			opens = opens[:n-1]
		}
		// JOIN … USING (i) AS x names the join, not a table function; its AS is required.
		if open > 0 && strings.EqualFold(tokens[open-1].text, "using") {
			continue
		}
		k := i + 1
		if strings.EqualFold(tokens[k].text, "as") {
			k++
		}
		if k < len(tokens) && pos.functionAliases[strings.ToLower(tokens[k].text)] {
			// DISTINCT ON (u.id) u.id: the name after the ")" is a column reference (qualified,
			// or the key list of DISTINCT ON), not the alias of a table function.
			if k+1 < len(tokens) && tokens[k+1].text == "." {
				continue
			}
			if k == i+1 && open > 0 && strings.EqualFold(tokens[open-1].text, "on") {
				continue
			}
			pos.aliases[tokens[k].start] = true
		}
	}
}

// isInsertTargetAs reports whether the AS at tokens[i] follows the table of INSERT INTO name:
// unlike UPDATE / DELETE / MERGE, INSERT INTO t AS alias needs its AS.
func isInsertTargetAs(tokens []rawToken, i int) bool {
	j := i - 1
	if j < 0 {
		return false
	}
	for j >= 2 && tokens[j-1].text == "." { // schema.table
		j -= 2
	}
	return j >= 2 && strings.EqualFold(tokens[j-1].text, "into") && strings.EqualFold(tokens[j-2].text, "insert")
}

// canOmitAs reports whether the alias token can follow an expression without AS.
// Keywords that PostgreSQL does not allow as bare labels (month, year, over, filter, …)
// parse only with AS, so removing it would produce invalid SQL. Only keyword tokens
// can be affected, so ordinary identifiers skip the parser, and the answer for a
// keyword is cached because it depends on the name alone.
func canOmitAs(tok rawToken) bool {
	if tok.text == "" {
		return false
	}
	if tok.text[0] == '"' || tok.kind == pg_query.KeywordKind_NO_KEYWORD {
		return true
	}
	key := strings.ToLower(tok.text)
	if v, ok := bareLabelCache.Load(key); ok {
		return v.(bool)
	}
	// (`SELECT 1 isnull` parses too, but as the postfix operator: the word must come back as the
	// name of the column, or dropping the AS would change the query)
	ok := false
	if res, err := pg_query.Parse("SELECT 1 " + tok.text); err == nil && len(res.Stmts) == 1 {
		if sel := res.Stmts[0].Stmt.GetSelectStmt(); sel != nil && len(sel.TargetList) == 1 {
			if rt := sel.TargetList[0].GetResTarget(); rt != nil && strings.EqualFold(rt.Name, tok.text) {
				ok = true
			}
		}
	}
	bareLabelCache.Store(key, ok)
	return ok
}

// bareLabelCache memoises canOmitAs for keyword aliases (lower-cased name → bool).
var bareLabelCache sync.Map

// layoutConfig returns the layout config with the paren_spacing hint filled in.
// bodyLayoutConfig is layoutConfig for the statements of a PL/pgSQL body.
func bodyLayoutConfig(cfg *config.Config) *config.LayoutConfig {
	lc := layoutConfig(cfg)
	pl := cfg.Layout.DollarQuote.PLpgSQL
	notPreserve := func(m config.BodyIndentMode) bool { return m != "" && m != config.BodyIndentPreserve }
	lc.IndentSetLater = !controlFlowIsNoop(pl.ControlFlow) || notPreserve(pl.BeginBody.Indent) || notPreserve(pl.Declare.Indent) ||
		notPreserve(pl.KeywordIndent) || notPreserve(cfg.Layout.DollarQuote.SQL.BodyIndent)
	return lc
}

func layoutConfig(cfg *config.Config) *config.LayoutConfig {
	lc := cfg.Layout
	lc.ParenSpacing = cfg.ParenSpacing
	lc.CommaSpacing = cfg.CommaSpacing
	lc.KeywordParenSpacing = cfg.KeywordParenSpacing
	lc.Case.ParenSpacing = cfg.ParenSpacing
	lc.KeepStatementIndent = !cfg.Nested
	return &lc
}

// ensureSeparator writes a space when buf ends in a word character, so a keyword or
// identifier written next cannot fuse with the previous token (SELECTa, !aAS).
// collapsePaddingBeforeAS reduces the blanks (hand-aligned column padding) that stand before an
// inserted AS to one, as the next run does to the AS that is written then. Blanks that open a line
// are indentation and stay.
func collapsePaddingBeforeAS(buf *strings.Builder) {
	out := buf.String()
	i := len(out)
	for i > 0 && (out[i-1] == ' ' || out[i-1] == '\t') {
		i--
	}
	if len(out)-i < 2 || i == 0 || out[i-1] == '\n' {
		return
	}
	buf.Reset()
	buf.WriteString(out[:i])
	buf.WriteByte(' ')
}

func ensureSeparator(buf *strings.Builder) {
	out := buf.String()
	if out == "" {
		return
	}
	if c := out[len(out)-1]; c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= 0x80 || c == '!' || c == '"' || c == '\'' || c == ')' || c == ']' {
		buf.WriteByte(' ')
	}
}

// resolveDDLColumns registers column names the AST walk cannot locate:
//   - ALTER/RENAME/DROP/ADD COLUMN [IF [NOT] EXISTS] name, and the new name in
//     RENAME COLUMN old TO new;
//   - the column list of CREATE [UNIQUE] INDEX … ON tbl [USING m] (a, b DESC, …).
//
// Without this, names that are unreserved keywords (label, event, name, …) are
// upper-cased as keywords.
func resolveDDLColumns(tokens []rawToken, pos *astPositions) {
	isWord := func(t string) bool {
		return t != "" && (t[0] == '_' || t[0] >= 'a' && t[0] <= 'z' || t[0] >= 'A' && t[0] <= 'Z' || t[0] == '"')
	}
	lowerAt := func(i int) string {
		if i < 0 || i >= len(tokens) {
			return ""
		}
		return strings.ToLower(tokens[i].text)
	}
	for i := range tokens {
		switch lowerAt(i) {
		case "column":
			k := i + 1
			if lowerAt(k) == "if" {
				k++
				if lowerAt(k) == "not" {
					k++
				}
				if lowerAt(k) == "exists" {
					k++
				}
			}
			if k >= len(tokens) || !isWord(tokens[k].text) || lowerAt(k+1) == "." {
				continue // not a bare column name (e.g. COMMENT ON COLUMN t.c)
			}
			pos.columns[tokens[k].start] = true
			if i > 0 && lowerAt(i-1) == "rename" && lowerAt(k+1) == "to" && k+2 < len(tokens) && isWord(tokens[k+2].text) {
				pos.columns[tokens[k+2].start] = true
			}
		case "index":
			// CREATE [UNIQUE] INDEX [CONCURRENTLY] [IF NOT EXISTS] [name] ON [ONLY] tbl [USING m] ( elems )
			if i == 0 || (lowerAt(i-1) != "create" && lowerAt(i-1) != "unique") {
				continue
			}
			k := i + 1
			for k < len(tokens) && lowerAt(k) != "on" && tokens[k].text != ";" && tokens[k].text != "(" {
				k++
			}
			for k < len(tokens) && tokens[k].text != "(" && tokens[k].text != ";" {
				k++
			}
			if k >= len(tokens) || tokens[k].text != "(" {
				continue
			}
			depth, expect := 1, true
			for k++; k < len(tokens) && depth > 0; k++ {
				switch t := tokens[k].text; {
				case t == "(":
					depth++
					expect = false
				case t == ")":
					depth--
				case t == "," && depth == 1:
					expect = true
				default:
					if depth == 1 && expect {
						if isWord(t) && lowerAt(k+1) != "(" {
							pos.columns[tokens[k].start] = true
						}
						expect = false
					}
				}
			}
		}
	}
}

// capitalizeWords upper-cases the first letter of each space-separated word and
// lower-cases the rest. Underscores are left alone: capitalize never renames an
// identifier, it only changes the case of its letters.
func capitalizeWords(s string) string {
	words := strings.Split(strings.ToLower(s), " ")
	for i, w := range words {
		if w == "" {
			continue
		}
		r, size := utf8.DecodeRuneInString(w)
		words[i] = string(unicode.ToUpper(r)) + w[size:]
	}
	return strings.Join(words, " ")
}

// stripTrailingOutsideTokens removes trailing blanks before newlines in the gaps
// between tokens of sql, leaving string literals and comments untouched.
func stripTrailingOutsideTokens(sql string) string {
	// Scanning is comparatively expensive; most bodies have nothing to strip.
	if !strings.Contains(sql, " \n") && !strings.Contains(sql, "\t\n") && !strings.Contains(sql, " \r") && !strings.Contains(sql, "\t\r") {
		return sql
	}
	res, err := pg_query.Scan(sql)
	if err != nil || len(res.Tokens) == 0 {
		return sql
	}
	var b strings.Builder
	cur := 0
	for _, t := range res.Tokens {
		start, end := int(t.Start), int(t.End)
		if start < cur || end > len(sql) {
			return sql
		}
		b.WriteString(stripTrailingFromGap(sql[cur:start]))
		b.WriteString(sql[start:end])
		cur = end
	}
	b.WriteString(stripTrailingFromGap(sql[cur:]))
	return b.String()
}

// unquoteIdent returns the identifier named by a double-quoted token ("a""b" → a"b),
// or "" when text is not a quoted identifier.
func unquoteIdent(text string) string {
	if len(text) < 2 || text[0] != '"' || text[len(text)-1] != '"' {
		return ""
	}
	return strings.ReplaceAll(text[1:len(text)-1], `""`, `"`)
}

// finishWholeText applies the settings that act on the text as a whole: blank-line
// limits, the missing final semicolon (only when addSemicolon) and the trailing newline.
func finishWholeText(result string, cfg *config.Config, addSemicolon bool) string {
	if cfg.BlankLines != config.BlankLinesPreserve {
		result = collapseBlankLines(result, cfg.BlankLines)
	}

	if addSemicolon && cfg.Semicolons == config.SemicolonRemove {
		result = removeFinalSemicolon(result)
	}

	if addSemicolon && cfg.Semicolons == config.SemicolonAdd {
		trimmed := strings.TrimRight(result, " \t\r\n")
		if !strings.HasSuffix(trimmed, ";") {
			// Insert ';' before any trailing comment lines / inline comments,
			// preserving trailing newlines.
			if idx := semicolonInsertPos(result); idx >= 0 {
				result = result[:idx+1] + ";" + result[idx+1:]
			}
		}
	}

	if cfg.StatementBreak == config.StatementBreakAlways {
		result = breakSameLineStatements(result)
	}
	if n, err := cfg.BlankLinesBetween(); err == nil && n >= 0 {
		result = separateStatements(result, n)
	}

	switch cfg.TrailingNewline {
	case config.TrailingNewlineStrip:
		result = strings.TrimRight(result, "\n")
	case config.TrailingNewlineAdd:
		if !strings.HasSuffix(result, "\n") {
			result += "\n"
		}
	}
	return result
}

// matchBracket returns the index of the "]" that closes the "[" at index k, or -1.
func matchBracket(tokens []rawToken, k int) int {
	depth := 0
	for m := k; m < len(tokens); m++ {
		switch tokens[m].text {
		case "[":
			depth++
		case "]":
			depth--
			if depth == 0 {
				return m
			}
		}
	}
	return -1
}

// spellsQualified reports whether the source at loc reads `prefix.` (the schema of a
// schema-qualified function call, as written, any case).
func (p *astPositions) spellsQualified(loc int, prefix string) bool {
	end := loc + len(prefix)
	if p.sql == "" {
		return true // no source to check against: keep the position the parser gave
	}
	return loc >= 0 && end < len(p.sql) && p.sql[end] == '.' && strings.EqualFold(p.sql[loc:end], prefix)
}

// bodyIsSQLOrPLpgSQL reports whether the dollar-quoted body at tokens[i] may be formatted: it is
// unless the statement names another language (LANGUAGE plpythonu, plperl, pltcl, c, …), whose
// code must come out exactly as it went in. A statement without LANGUAGE (an anonymous DO
// block) is PL/pgSQL.
func bodyIsSQLOrPLpgSQL(tokens []rawToken, i int) bool {
	lo, hi := i, i
	for lo > 0 && tokens[lo-1].text != ";" {
		lo--
	}
	for hi < len(tokens) && tokens[hi].text != ";" {
		hi++
	}
	for k := lo; k+1 < hi; k++ {
		if strings.EqualFold(tokens[k].text, "language") {
			lang := strings.ToLower(strings.Trim(tokens[k+1].text, `'"`))
			return lang == "sql" || lang == "plpgsql"
		}
	}
	return true
}

func isCommentText(t string) bool { return strings.HasPrefix(t, "--") || strings.HasPrefix(t, "/*") }

func isOnlyWhitespace(s string) bool { return strings.TrimSpace(s) == "" }

// joinIsOptionName reports whether the JOIN at tokens[i] is a name, not a join
// (CREATE OPERATOR (… JOIN = f), ALTER OPERATOR … SET (JOIN = f)).
func joinIsOptionName(tokens []rawToken, i int) bool {
	for j := i + 1; j < len(tokens); j++ {
		if strings.TrimSpace(tokens[j].text) == "" {
			continue
		}
		return tokens[j].text == "="
	}
	return false
}
