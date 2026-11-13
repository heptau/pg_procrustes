package formatter

import (
	"strings"

	pg_query "github.com/pganalyze/pg_query_go/v6"
)

// binaryArithmetic scans tokens and returns the byte offsets at which a binary
// arithmetic operator (+ - * / % ^) starts and ends. An operator is binary only
// when the token before it can end an operand and the token after it can start
// one. Unary signs (x = -1), wildcards (SELECT *, count(*), t.*), %TYPE and
// anything uncertain are left out so spacing rules never touch them.
func binaryArithmetic(tokens []rawToken) (starts, ends map[int]bool) {
	starts = map[int]bool{}
	ends = map[int]bool{}
	for i := 1; i+1 < len(tokens); i++ {
		switch tokens[i].text {
		case "+", "-", "*", "/", "%", "^":
		default:
			continue
		}
		if !canEndOperand(tokens[i-1]) || !canStartOperand(tokens[i+1]) {
			continue
		}
		if tokens[i].text == "%" {
			switch strings.ToLower(tokens[i+1].text) {
			case "type", "rowtype":
				continue
			}
		}
		starts[tokens[i].start] = true
		ends[tokens[i].end] = true
	}
	return starts, ends
}

// reservedValueWords are reserved keywords that nevertheless form a complete operand.
var reservedValueWords = map[string]bool{
	"end": true, "null": true, "true": true, "false": true,
	"current_date": true, "current_time": true, "current_timestamp": true,
	"current_user": true, "session_user": true, "user": true,
	"localtime": true, "localtimestamp": true, "current_role": true,
	"current_catalog": true, "current_schema": true,
}

// reservedOperandStarters are reserved keywords that can begin an operand.
var reservedOperandStarters = map[string]bool{
	"case": true, "array": true, "null": true, "true": true, "false": true,
	"cast":         true, // (CAST(x AS t), which cast_style: operator turns into x::t after this decision)
	"current_date": true, "current_time": true, "current_timestamp": true,
	"current_user": true, "session_user": true, "user": true,
	"localtime": true, "localtimestamp": true, "current_role": true,
	"current_catalog": true, "current_schema": true,
}

// notOperandWords are non-reserved words that act as syntax, not as a value.
var notOperandWords = map[string]bool{
	"between": true, "like": true, "ilike": true, "similar": true, "is": true,
	"isnull": true, "notnull": true, "overlaps": true, "join": true, "inner": true,
	"outer": true, "left": true, "right": true, "full": true, "cross": true,
	"natural": true, "by": true, "set": true, "values": true, "filter": true,
	"over": true, "rows": true, "range": true, "groups": true, "returning": true,
	"return": true, "nulls": true, "within": true, "using": true,
}

func isWordTok(t rawToken) bool {
	c := t.text[0]
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c >= 0x80
}

func isLiteralTok(t rawToken) bool {
	c := t.text[0]
	return (c >= '0' && c <= '9') || c == '\'' || c == '"' || c == '$' || (c == '.' && len(t.text) > 1)
}

// wordIsValue reports whether a word token acts as a value (identifier, type name,
// value keyword) rather than as clause/operator syntax.
func wordIsValue(t rawToken, reservedOK map[string]bool) bool {
	lower := strings.ToLower(t.text)
	if operatorKeywords[lower] || plpgsqlKeywordNames[lower] || notOperandWords[lower] {
		return false
	}
	switch t.kind {
	case pg_query.KeywordKind_RESERVED_KEYWORD:
		return reservedOK[lower]
	default:
		return true
	}
}

func canEndOperand(t rawToken) bool {
	if t.text == "" {
		return false
	}
	if t.text == ")" || t.text == "]" {
		return true
	}
	if isLiteralTok(t) {
		return true
	}
	if isWordTok(t) {
		return wordIsValue(t, reservedValueWords)
	}
	return false
}

func canStartOperand(t rawToken) bool {
	if t.text == "" {
		return false
	}
	switch t.text {
	case "(", "[", "-", "+", "~":
		return true
	}
	if isLiteralTok(t) {
		return true
	}
	if isWordTok(t) {
		return wordIsValue(t, reservedOperandStarters)
	}
	return false
}
