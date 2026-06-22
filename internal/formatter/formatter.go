package formatter

import (
	"fmt"
	"os"
	"strings"

	pg_query "github.com/pganalyze/pg_query_go/v6"

	"github.com/heptau/pg_procrustes/internal/config"
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
	sortByDefaultLocs    []int
	castFuncLocs         []int
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
				for j := i + 1; j < len(tokens); j++ {
					if tokens[j].text == "(" {
						break // function call after AS, not an alias
					}
					return j
				}
				return -1
			}
			// Implicit alias: identifier that matches the expected name,
			// not at the expression start, followed by a list separator or
			// a clause keyword.
			if i > startIdx && strings.EqualFold(text, aliasName) {
				if isFollowedBySeparator(tokens, i) {
					return i
				}
			}
		}
	}
	return -1
}

// clauseKeywords are tokens that cannot be alias names and end a SELECT target.
var clauseKeywords = map[string]bool{
	"from": true, "where": true, "group": true, "having": true,
	"order": true, "limit": true, "offset": true, "union": true,
	"intersect": true, "except": true, "fetch": true, "for": true,
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
	switch rule {
	case config.CaseUpper:
		return strings.ToUpper(s)
	case config.CaseLower:
		return strings.ToLower(s)
	default:
		return s
	}
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
		// Single-token short → long (possibly multi-word string).
		if long, ok := singleShortToLong[lower]; ok {
			return applyCase(long, caseRule), nil
		}
		// Already a long/multi-token form — just apply casing.
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
	case "=", "!=", "<>", "<", ">", "<=", ">=", "||":
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
	// Reject if it would be a reserved keyword.
	res, err := pg_query.Scan(inner)
	if err != nil || len(res.Tokens) == 0 {
		return false
	}
	return res.Tokens[0].KeywordKind != pg_query.KeywordKind_RESERVED_KEYWORD
}

// collapseBlankLines replaces runs of excess consecutive newlines.
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
	for strings.Contains(s, excess) {
		s = strings.ReplaceAll(s, excess, target)
	}
	return s
}

// processGap returns the gap string after applying paren spacing, operator
// spacing, and trailing-whitespace rules. nextText is the text of the token
// that follows the gap; prevText is the text of the token that precedes it.
func processGap(gap, prevText, nextText string, cfg *config.Config) string {
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

	// Operator spacing — single space on each side of symbolic binary ops.
	if !hasNL && cfg.OperatorSpacing == config.OperatorSpacingNormalize {
		if (isSymbolicOp(prevText) || isSymbolicOp(nextText)) &&
			prevText != "(" && nextText != ")" {
			return " "
		}
	}

	// Comma spacing normalization.
	if !hasNL && cfg.CommaSpacing == config.CommaSpacingNormalize {
		if prevText == "," {
			return " "
		}
		if nextText == "," {
			return ""
		}
	}

	// Trailing whitespace stripping.
	if cfg.TrailingWhitespace == config.TrailingWSStrip {
		gap = stripTrailingFromGap(gap)
	}
	return gap
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

// Format normalizes casing in sql according to cfg.
// Whitespace, comments, and string literals are always preserved verbatim.
func Format(sql string, cfg *config.Config) (string, error) {
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

	pos := collectASTPositions(sql)
	pos.resolveAliases(tokens)

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

	// Semicolons: skip ';' tokens (and their preceding gap, which is usually empty).
	if cfg.Semicolons == config.SemicolonRemove {
		for i, tok := range tokens {
			if tok.text == ";" {
				skip[i] = true
			}
		}
	}

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
			if strings.ToLower(tok.text) == "as" && tok.kind == pg_query.KeywordKind_RESERVED_KEYWORD {
				if j := nextVisible(i); j >= 0 && pos.aliases[tokens[j].start] {
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
				} else if depth == 0 && termKeywords[tl] {
					terminatorIdx = j
					break
				}
			}
			if terminatorIdx >= 0 {
				insertBefore[terminatorIdx] = " ASC"
				suppressGap[terminatorIdx] = true
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
			dropText[castIdx] = true
			dropText[openIdx] = true
			suppressGap[openIdx] = true
			suppressGap[asIdx] = true
			replaceWith[asIdx] = "::"
			if typeIdx >= 0 {
				suppressGap[typeIdx] = true
			}
			dropText[closeIdx] = true
			suppressGap[closeIdx] = true
		}
	}

	// not_in not_equals_all: NOT IN → <> ALL
	if cfg.NotIn == config.NotInNotEqualsAll {
		for i, tok := range tokens {
			if strings.ToLower(tok.text) == "not" && tok.kind == pg_query.KeywordKind_RESERVED_KEYWORD {
				if j := nextVisible(i); j >= 0 && strings.ToLower(tokens[j].text) == "in" && tokens[j].kind == pg_query.KeywordKind_RESERVED_KEYWORD {
					replaceWith[i] = "<>"
					replaceWith[j] = "ALL"
				}
			}
		}
	}

	// not_in not_in: <> ALL or != ALL → NOT IN
	if cfg.NotIn == config.NotInNotIn {
		for i, tok := range tokens {
			tl := strings.ToLower(tok.text)
			if (tl == "<>" || tl == "!=") && tok.kind == pg_query.KeywordKind_NO_KEYWORD {
				if j := nextVisible(i); j >= 0 && strings.ToLower(tokens[j].text) == "all" && tokens[j].kind == pg_query.KeywordKind_RESERVED_KEYWORD {
					if k := nextVisible(j); k >= 0 && tokens[k].text == "(" {
						replaceWith[i] = "NOT IN"
						dropText[j] = true
						suppressGap[j] = true
					}
				}
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
	prevTokenText := "" // raw text of previous written token

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
				buf.WriteString(processGap(gap, prevTokenText, tok.text, cfg))
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
			if safeToUnquote(inner) {
				buf.WriteString(inner)
				prevTokenText = tokenText
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
			if class == classOther && dataTypeNames[lower] {
				class = classDataType
			}
		}

		// replaceWith: if this token has a replacement, write it and skip normal output.
		if s, ok := replaceWith[i]; ok {
			buf.WriteString(s)
			prevTokenText = tokenText
			cur = tok.end
			continue
		}

		// ── AS insertion (add mode) ───────────────────────────────────────────
		if class == classAlias && cfg.Aliases.As == config.AliasAsAdd {
			if strings.ToLower(prevTokenText) != "as" {
				buf.WriteString(applyCase("as", cfg.ReservedKeywords.Case))
				buf.WriteByte(' ')
			}
		}

		// ── JOIN form: long — insert INNER/OUTER before JOIN ──────────────────
		if lower == "join" && cfg.JoinForm == config.JoinFormLong {
			prevLower := strings.ToLower(prevTokenText)
			kw := cfg.ReservedKeywords.Case
			switch {
			case isDirectionJoinKeyword(prevLower):
				// LEFT/RIGHT/FULL → insert OUTER
				buf.WriteString(applyCase("outer", kw))
				buf.WriteByte(' ')
			case !isAnyJoinKeyword(prevLower):
				// bare JOIN → insert INNER
				buf.WriteString(applyCase("inner", kw))
				buf.WriteByte(' ')
			}
		}

		// ── Token output ──────────────────────────────────────────────────────
		switch class {
		case classReservedKeyword:
			buf.WriteString(applyCase(tokenText, cfg.ReservedKeywords.Case))
		case classKeyword:
			buf.WriteString(applyCase(tokenText, cfg.Keywords.Case))
		case classDataType:
			if cfg.DataTypes.Form == config.TypeFormPreserve {
				buf.WriteString(applyCase(tokenText, cfg.DataTypes.Case))
			} else {
				out, consumed := normalizeType(tokens, i, skip, cfg.DataTypes.Form, cfg.DataTypes.Case)
				buf.WriteString(out)
				for _, idx := range consumed {
					skip[idx] = true
				}
			}
		case classLiteral:
			buf.WriteString(applyCase(tokenText, cfg.Literals.Case))
		case classOperator:
			buf.WriteString(applyCase(tokenText, cfg.Operators.Case))
		case classSchema:
			buf.WriteString(applyCase(tokenText, cfg.Schemas.Case))
		case classTable:
			buf.WriteString(applyCase(tokenText, cfg.Tables.Case))
		case classFunction:
			buf.WriteString(applyCase(tokenText, cfg.Functions.Case))
		case classConditionalFunction:
			buf.WriteString(applyCase(tokenText, cfg.ConditionalFunctions.Case))
		case classSystemFunction:
			buf.WriteString(applyCase(tokenText, cfg.SystemFunctions.Case))
		case classAlias:
			buf.WriteString(applyCase(tokenText, cfg.Aliases.Case))
		case classColumn:
			buf.WriteString(applyCase(tokenText, cfg.Columns.Case))
		default:
			// Inequality operator normalization.
			switch {
			case tokenText == "<>" && cfg.InequalityOp == config.InequalityC:
				buf.WriteString("!=")
			case tokenText == "!=" && cfg.InequalityOp == config.InequalityANSI:
				buf.WriteString("<>")
			case isDollarQuoted(tokenText):
				out, err := formatDollarQuoted(tokenText, cfg)
				if err != nil {
					buf.WriteString(tokenText)
				} else {
					buf.WriteString(out)
				}
			default:
				buf.WriteString(tokenText)
			}
		}

		prevTokenText = tokenText
		cur = tok.end
	}

	if cur < len(sql) {
		buf.WriteString(processGap(sql[cur:], prevTokenText, "", cfg))
	}

	result := buf.String()

	// ── Post-processing ───────────────────────────────────────────────────────

	if appendASCAtEnd {
		result = strings.TrimRight(result, " \t\r") + " ASC"
	}

	if cfg.TrailingWhitespace == config.TrailingWSStrip {
		result = strings.TrimRight(result, " \t\r")
	}

	if cfg.BlankLines != config.BlankLinesPreserve {
		result = collapseBlankLines(result, cfg.BlankLines)
	}

	if cfg.Semicolons == config.SemicolonAdd {
		trimmed := strings.TrimRight(result, " \t\r\n")
		if !strings.HasSuffix(trimmed, ";") {
			// Insert ';' after the last non-whitespace character, preserving trailing newlines.
			idx := strings.LastIndexFunc(result, func(r rune) bool {
				return r != ' ' && r != '\t' && r != '\r' && r != '\n'
			})
			if idx >= 0 {
				result = result[:idx+1] + ";" + result[idx+1:]
			}
		}
	}

	// trailing_newline
	if cfg.TrailingNewline == config.TrailingNewlineStrip {
		result = strings.TrimRight(result, "\n")
	} else if cfg.TrailingNewline == config.TrailingNewlineAdd {
		if !strings.HasSuffix(result, "\n") {
			result += "\n"
		}
	}

	// layout post-processing
	if !layout.IsNoop(&cfg.Layout) {
		result, err = layout.Apply(result, &cfg.Layout)
		if err != nil {
			return result, fmt.Errorf("layout: %w", err)
		}
	}

	return result, nil
}

// collectASTPositions parses sql and returns byte offsets for each identifier
// category. On parse error returns empty maps (scanner-only fallback).
func collectASTPositions(sql string) *astPositions {
	pos := newASTPositions()
	pos.sql = sql
	result, err := pg_query.Parse(sql)
	if err != nil {
		return pos
	}
	for _, stmt := range result.Stmts {
		if stmt.Stmt != nil {
			walkNode(stmt.Stmt, pos)
		}
	}
	return pos
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

	case *pg_query.Node_ColumnDef:
		cd := n.ColumnDef
		if cd.Location >= 0 {
			pos.columns[int(cd.Location)] = true
		}
		many(cd.Constraints)

	case *pg_query.Node_CreateFunctionStmt:
		cf := n.CreateFunctionStmt
		many(cf.Parameters)
		many(cf.Options)

	case *pg_query.Node_IndexStmt:
		ix := n.IndexStmt
		if ix.Relation != nil {
			addTableRangeVar(ix.Relation, pos)
		}
		many(ix.IndexParams)

	case *pg_query.Node_CreateTableAsStmt:
		walkNode(n.CreateTableAsStmt.Query, pos)

	case *pg_query.Node_ViewStmt:
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
		if ss.Larg != nil {
			walkNode(&pg_query.Node{Node: &pg_query.Node_SelectStmt{SelectStmt: ss.Larg}}, pos)
		}
		if ss.Rarg != nil {
			walkNode(&pg_query.Node{Node: &pg_query.Node_SelectStmt{SelectStmt: ss.Rarg}}, pos)
		}

	case *pg_query.Node_CommonTableExpr:
		cte := n.CommonTableExpr
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
			many(is.OnConflictClause.TargetList)
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
		many(us.TargetList)
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
				pos.schemas[int(fc.Location)] = true
				if prefix := funcNameStr(fc.Funcname[0]); prefix != "" {
					pos.functions[int(fc.Location)+len(prefix)+1] = true
				}
			} else {
				pos.functions[int(fc.Location)] = true
			}
		}
		many(fc.Args)
		many(fc.AggOrder)
		walkNode(fc.AggFilter, pos)

	case *pg_query.Node_JoinExpr:
		je := n.JoinExpr
		walkNode(je.Larg, pos)
		walkNode(je.Rarg, pos)
		walkNode(je.Quals, pos)

	case *pg_query.Node_RangeSubselect:
		rs := n.RangeSubselect
		walkNode(rs.Subquery, pos)

	case *pg_query.Node_SubLink:
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
		pos.tables[loc+len(rv.Schemaname)+1] = true // +1 for the dot
	} else {
		pos.tables[loc] = true
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
func applyBodyIndent(content, indentUnit string, mode config.BodyIndentMode) string {
	if mode == config.BodyIndentPreserve || mode == "" || content == "" {
		return content
	}
	lines := strings.Split(content, "\n")
	if mode == config.BodyIndentNone {
		minIndent := findMinIndent(lines)
		result := make([]string, len(lines))
		for i, line := range lines {
			if strings.TrimSpace(line) == "" {
				result[i] = ""
			} else {
				result[i] = strings.TrimPrefix(line, minIndent)
			}
		}
		return strings.Join(result, "\n")
	}
	// mode == BodyIndentIndent: normalise to exactly one indent level —
	// strip existing indent first, then add indentUnit.
	minIndent := findMinIndent(lines)
	result := make([]string, len(lines))
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			result[i] = ""
		} else {
			result[i] = indentUnit + strings.TrimPrefix(line, minIndent)
		}
	}
	return strings.Join(result, "\n")
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
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			return line[:minLen]
		}
	}
	return ""
}

func applyDollarBoundary(body string, afterOpen, beforeClose config.BlankLineAction) string {
	switch afterOpen {
	case config.BlankLineAdd:
		if !strings.HasPrefix(body, "\n") {
			body = "\n" + body
		}
	case config.BlankLineRemove:
		body = strings.TrimLeft(body, "\n\r")
		// default: preserve
	}
	switch beforeClose {
	case config.BlankLineAdd:
		if !strings.HasSuffix(body, "\n") {
			body = body + "\n"
		}
	case config.BlankLineRemove:
		body = strings.TrimRight(body, "\n\r")
		// default: preserve
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
func formatDollarQuoted(s string, cfg *config.Config) (string, error) {
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
	formatted, err := Format(body, &cfgInner)
	if err != nil {
		formatted = body
	}

	var resultBody string
	if isPLpgSQLBody(formatted) {
		// PL/pgSQL body: apply layout per-statement, then apply dollar_quote.plpgsql config.
		resultBody, err = applyLayoutPerStatement(formatted, cfg)
		if err != nil {
			resultBody = formatted
		}
		resultBody = applyPLpgSQLDollarQuoteConfig(resultBody, cfg)
	} else {
		// Plain SQL body: apply layout per-statement, then apply dollar_quote.sql config.
		resultBody, err = applyLayoutPerStatement(formatted, cfg)
		if err != nil {
			resultBody = formatted
		}
		indentUnit := indentUnitFromCfg(cfg)
		resultBody = applyBodyIndent(resultBody, indentUnit, dqCfg.SQL.BodyIndent)
		resultBody = applyBodySectionBlanks(resultBody, dqCfg.SQL.BlankLineBefore, dqCfg.SQL.BlankLineAfter)
	}

	// Apply dollar boundary (newline_after_open, newline_before_close).
	resultBody = applyDollarBoundary(resultBody, dqCfg.NewlineAfterOpen, dqCfg.NewlineBeforeClose)

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
	for i := 0; i < len(allToks); i++ {
		tok := allToks[i]
		tokStart := int(tok.Start)
		tokEnd := int(tok.End)
		text := strings.ToUpper(body[tokStart:tokEnd])

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
			} else if text == "EXCEPTION" && blockDepth == 1 {
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
		}
	}

	var sb strings.Builder

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
			sb.WriteString(kwIndent)
			sb.WriteString(body[declareKwStart:declareKwEnd]) // preserve original casing
			decl := applyBodyIndent(declBody, indentUnit, plCfg.Declare.Indent)
			decl = applyBodySectionBlanks(decl, plCfg.Declare.BlankLineBefore, plCfg.Declare.BlankLineAfter)
			sb.WriteString(decl)
		}
	}

	// BEGIN keyword.
	sb.WriteString(kwIndent)
	sb.WriteString(body[beginKwStart:beginKwEnd])

	// Begin body.
	var beginBody string
	if hasException {
		beginBody = body[beginBodyStart:exceptionKwStart]
	} else {
		beginBody = body[beginBodyStart:endKwStart]
	}
	bb := applyBodyIndent(beginBody, indentUnit, plCfg.BeginBody.Indent)
	bb = applyBodySectionBlanks(bb, plCfg.BeginBody.BlankLineBefore, plCfg.BeginBody.BlankLineAfter)
	if !controlFlowIsNoop(plCfg.ControlFlow) {
		bb = reformatControlFlow(bb, indentUnit, plCfg.ControlFlow, cfg.Layout.LineLength)
	}
	sb.WriteString(bb)

	// EXCEPTION section.
	if hasException {
		sb.WriteString(kwIndent)
		sb.WriteString(body[exceptionKwStart:exceptionKwEnd])
		eb := body[exceptionBodyStart:endKwStart]
		if !controlFlowIsNoop(plCfg.ControlFlow) {
			eb = reformatControlFlow(eb, indentUnit, plCfg.ControlFlow, cfg.Layout.LineLength)
		}
		sb.WriteString(eb)
	}

	// END keyword.
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
	return strings.HasPrefix(lower, "begin") || strings.HasPrefix(lower, "declare")
}

// sqlClauseStarters are top-level keywords that begin a SQL statement in PL/pgSQL.
var sqlClauseStarters = map[string]bool{
	"select": true, "insert": true, "update": true, "delete": true,
	"with": true, "perform": true,
}

// applyLayoutPerStatement applies layout to individual SQL statements inside a
// PL/pgSQL body. It scans for SQL clause starters (SELECT, INSERT, UPDATE, DELETE,
// WITH, PERFORM) at paren depth 0 and applies layout.Apply() to each one, while
// leaving block-structure tokens (BEGIN, END, DECLARE, RAISE, variable assignments,
// etc.) completely untouched.
//
// RETURN QUERY SELECT: "RETURN QUERY" is output as prefix verbatim; layout is
// applied to the "SELECT …" portion and subsequent lines are re-indented.
func applyLayoutPerStatement(body string, cfg *config.Config) (string, error) {
	if layout.IsNoop(&cfg.Layout) {
		return body, nil
	}
	scanResult, err := pg_query.Scan(body)
	if err != nil {
		return body, nil
	}

	var result strings.Builder
	parenDepth := 0
	lastEnd := 0   // position in body up to which we have already written output
	sqlStart := -1 // byte position where the current SQL clause keyword starts

	for _, tok := range scanResult.Tokens {
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

		lower := strings.ToLower(text)

		if sqlStart < 0 && sqlClauseStarters[lower] {
			// Found the start of a SQL statement; remember position.
			sqlStart = tokStart
			continue
		}

		if text == ";" && sqlStart >= 0 {
			// End of a SQL statement we were tracking.
			// Everything from lastEnd to sqlStart is prefix (BEGIN, RETURN QUERY, etc.)
			prefix := body[lastEnd:sqlStart]
			sqlText := strings.TrimRight(body[sqlStart:tokStart], " \t")
			baseIndent := lastLineIndent(prefix)

			formatted, fmtErr := layout.Apply(sqlText, &cfg.Layout)
			if fmtErr != nil || formatted == sqlText {
				// No change or error: output verbatim.
				result.WriteString(body[lastEnd:tokEnd])
			} else {
				result.WriteString(prefix)
				result.WriteString(reindentLines(formatted, baseIndent))
				result.WriteByte(';')
			}
			lastEnd = tokEnd
			sqlStart = -1
			continue
		}

		if text == ";" && sqlStart < 0 {
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

// reindentLines prepends baseIndent to every line in sql except the first and
// empty lines — used to restore PL/pgSQL body indentation after layout breaks clauses.
func reindentLines(sql, baseIndent string) string {
	if baseIndent == "" {
		return sql
	}
	lines := strings.SplitAfter(sql, "\n")
	var sb strings.Builder
	for i, line := range lines {
		if i == 0 || strings.TrimRight(line, "\n") == "" {
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
func reformatControlFlow(body, indentUnit string, cfg config.ControlFlowCfg, lineLength int) string {
	if controlFlowIsNoop(cfg) {
		return body
	}

	scanResult, err := pg_query.Scan(body)
	if err != nil {
		return body
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
					break
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
		minInd := findMinIndent(lines)
		for i, line := range lines {
			if strings.TrimSpace(line) == "" {
				continue
			}
			stripped := strings.TrimPrefix(line, minInd)
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
			// keyword is on the same line as previous content
			switch action {
			case config.BlankLineRemove:
				return "\n" + preceding
			case config.BlankLineAdd:
				return "\n\n" + preceding
			}
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
		stripped := strings.TrimLeft(following, "\n")
		switch action {
		case config.BlankLineRemove:
			return "\n" + stripped
		case config.BlankLineAdd:
			return "\n\n" + stripped
		}
		return following
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
			switch bc.ThenBreak {
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

	for ei, ev := range events {
		segText := body[pos:ev.start]

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
				// Force body onto new line — strip all leading whitespace then add \n.
				stripped := strings.TrimLeft(segText, " \t\n")
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
			targetBodyStr := strings.Repeat(indentUnit, segBodyDepth)
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
				kwIndentStr = strings.Repeat(indentUnit, thenDepth)
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
			if kwDepth > 0 {
				rest := strings.TrimLeft(kwIndentStr, " \t")
				kwIndentStr = strings.Repeat(indentUnit, kwDepth) + rest
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
					target := strings.Repeat(indentUnit, bodyDepthForOpener(lastEv))
					remaining = normalizeBodyLines(remaining, target, cfg.If.BodyIndent)
				}
			case evLOOP:
				remaining = applyBlankAfter(remaining, cfg.Loop.BlankLineAfter)
				if cfg.Loop.BodyIndent != "" && cfg.Loop.BodyIndent != config.BodyIndentPreserve {
					target := strings.Repeat(indentUnit, bodyDepthForOpener(lastEv))
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
						target := strings.Repeat(indentUnit, bodyDepthForOpener(lastEv))
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
