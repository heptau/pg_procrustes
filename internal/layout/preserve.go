package layout

import (
	"strings"

	"github.com/heptau/pg_procrustes/config"
)

// Detection helpers for the "preserve" value of the position / align options:
// they look at how the input is written and keep that style. When the input gives
// no signal they fall back to the classic style (comma and operator at the end /
// start of the line as before: trailing comma, leading AND/OR, ON inline, items
// indented under the keyword).

// tokenStartsLine reports whether only blanks precede the token on its line.
func tokenStartsLine(sql string, t rawTok) bool {
	j := t.start - 1
	for j >= 0 && (sql[j] == ' ' || sql[j] == '\t') {
		j--
	}
	return j < 0 || sql[j] == '\n'
}

// tokenEndsLine reports whether only blanks follow the token on its line.
func tokenEndsLine(sql string, t rawTok) bool {
	j := t.end
	for j < len(sql) && (sql[j] == ' ' || sql[j] == '\t' || sql[j] == '\r') {
		j++
	}
	return j >= len(sql) || sql[j] == '\n'
}

// lineIndent is the leading blanks of the line the offset is on.
func lineIndent(sql string, off int) string {
	ls := strings.LastIndexByte(sql[:off], '\n') + 1
	j := ls
	for j < len(sql) && (sql[j] == ' ' || sql[j] == '\t') {
		j++
	}
	return sql[ls:j]
}

// resolveCommaStart: true when the comma opens the line.
func resolveCommaStart(mode config.CommaPosition, sql string, tokens []rawTok, from, to int) bool {
	switch mode {
	case config.CommaStart:
		return true
	case config.CommaPreserve, "":
		depth := 0
		for i := from; i < to && i < len(tokens); i++ {
			switch t := tokens[i].text; {
			case t == "(" || t == "[":
				depth++
			case t == ")" || t == "]":
				if depth > 0 {
					depth--
				}
			case t == "," && depth == 0 && tokenStartsLine(sql, tokens[i]):
				return true
			}
		}
	}
	return false
}

// resolveOperatorEnd: true when AND / OR end the line.
func resolveOperatorEnd(mode config.LogicalOperatorPosition, sql string, tokens []rawTok, from, to int) bool {
	switch mode {
	case config.LogicalOperatorEnd:
		return true
	case config.LogicalOperatorPreserve, "":
		depth := 0
		for i := from; i < to && i < len(tokens); i++ {
			switch t := strings.ToLower(tokens[i].text); {
			case t == "(" || t == "[":
				depth++
			case t == ")" || t == "]":
				if depth > 0 {
					depth--
				}
			case depth == 0 && (t == "and" || t == "or") && tokenEndsLine(sql, tokens[i]):
				return true
			}
		}
	}
	return false
}

// resolveJoinNewline: true when ON goes on its own line below JOIN t.
func resolveJoinNewline(mode config.JoinOnPosition, sql string, tokens []rawTok, from, to int) bool {
	on := findTopLevelOn(tokens, from, to)
	if on < 0 {
		return false
	}
	switch mode {
	case config.JoinOnNewline:
		return true
	case config.JoinOnPreserve, "":
		return tokenStartsLine(sql, tokens[on])
	}
	return false
}

// resolveContentAlign turns "preserve" into same / indent by comparing the indent of
// the first content line that starts a line with the indent of the clause keyword.
func resolveContentAlign(mode config.AlignMode, sql string, tokens []rawTok, kwTok, from, to int) config.AlignMode {
	if mode != config.AlignPreserve {
		return mode
	}
	if !tokenStartsLine(sql, tokens[kwTok]) {
		return config.AlignIndent // keyword shares its line with another statement: no reliable indent
	}
	kwIndent := lineIndent(sql, tokens[kwTok].start)
	isInsert := strings.EqualFold(tokens[kwTok].text, "insert")
	for i := from; i < to && i < len(tokens); i++ {
		if isInsert && tokens[i].text == "(" {
			// the column list's "(" is not an item; where it stands (paren_indent.open) says
			// nothing about the style of the items
			continue
		}
		if tokenStartsLine(sql, tokens[i]) {
			if lineIndent(sql, tokens[i].start) == kwIndent {
				return config.AlignSame
			}
			return config.AlignIndent
		}
	}
	return config.AlignIndent
}

// hasNewlineAfter reports whether the text between the token and the next one holds a newline.
func hasNewlineAfter(sql string, tokens []rawTok, i, to int) bool {
	if i+1 >= to || i+1 >= len(tokens) {
		return false
	}
	return strings.Contains(sql[tokens[i].end:tokens[i+1].start], "\n")
}
