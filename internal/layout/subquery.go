package layout

import (
	"regexp"
	"strings"

	pg_query "github.com/pganalyze/pg_query_go/v6"

	"github.com/heptau/pg_procrustes/config"
)

// Subqueries — (SELECT …), (WITH …), (VALUES …) — are laid out in two steps around the
// clause layout of the enclosing statement:
//
//  1. flattenSubqueries joins every outermost subquery onto one line first, so the
//     enclosing layout sees an ordinary parenthesised item whatever the input looked like
//     (this is what keeps the result idempotent);
//  2. expandSubqueries then opens the subqueries that layout.subquery.break asks for:
//     "(" stays on its line, the body is laid out with the same rules one level deeper and
//     ")" gets a line of its own at the indent of the line that holds the "(".
//
// A subquery with a -- comment (or a multi-line string) inside is left as written.

func subqueryMode(cfg *config.LayoutConfig) config.BreakMode {
	if cfg.Subquery.Break == "" {
		return config.BreakPreserve
	}
	return cfg.Subquery.Break
}

type subqueryParen struct {
	open, close int // token indexes of "(" and its ")"
}

// findSubqueries returns the outermost parenthesised subqueries of tokens that can be
// re-laid out.
func findSubqueries(tokens []rawTok) []subqueryParen {
	var out []subqueryParen
	for i := 0; i < len(tokens); i++ {
		if tokens[i].text != "(" || i+1 >= len(tokens) {
			continue
		}
		n := i + 1
		for n < len(tokens)-1 && isCommentTok(tokens[n].text) {
			n++
		}
		switch strings.ToLower(tokens[n].text) {
		case "select", "with", "values":
		default:
			continue
		}
		depth, closeIdx := 0, -1
		for j := i; j < len(tokens); j++ {
			switch tokens[j].text {
			case "(":
				depth++
			case ")":
				depth--
			}
			if depth == 0 {
				closeIdx = j
				break
			}
		}
		if closeIdx < 0 {
			return out
		}
		// -- comments are fine (inlineContent keeps the line break after them); a string or block
		// comment that spans lines is not something to re-indent
		plain := true
		for j := i + 1; j < closeIdx; j++ {
			if strings.Contains(tokens[j].text, "\n") {
				plain = false
				break
			}
		}
		if plain {
			out = append(out, subqueryParen{i, closeIdx})
		}
		i = closeIdx
	}
	return out
}

func flattenSubqueries(sql string, cfg *config.LayoutConfig) string {
	return flattenSubqueriesHang(sql, cfg, false)
}

// flattenSubqueriesHang is flattenSubqueries that, with hang, puts the line that follows a -- comment in a
// subquery one level below the line the subquery opens on (nothing indents it afterwards, as the clause
// layout does for the statements of a file).
// parenKeywords are the words whose "(" keyword_paren_spacing speaks about.
var parenKeywords = map[string]bool{"as": true, "in": true, "values": true, "on": true, "exists": true, "any": true,
	"all": true, "some": true, "from": true, "join": true, "where": true, "and": true, "or": true, "not": true,
	"select": true, "set": true, "using": true, "union": true, "intersect": true, "except": true, "then": true,
	"else": true, "when": true, "having": true, "lateral": true}

// pos0 is the index of the first token that starts at or after the text offset pos.
func pos0(tokens []rawTok, pos int) int {
	for i, t := range tokens {
		if t.start >= pos {
			return i
		}
	}
	return len(tokens)
}

func flattenSubqueriesHang(sql string, cfg *config.LayoutConfig, hang bool) string {
	if subqueryMode(cfg) == config.BreakPreserve {
		return sql
	}
	res, err := pg_query.Scan(sql)
	if err != nil {
		return sql
	}
	tokens := make([]rawTok, len(res.Tokens))
	for i, t := range res.Tokens {
		tokens[i] = rawTok{start: int(t.Start), end: int(t.End), text: sql[t.Start:t.End]}
	}
	subs := findSubqueries(tokens)
	if len(subs) == 0 {
		return sql
	}
	var sb strings.Builder
	pos := 0
	for _, sq := range subs {
		if prev := sq.open - 1; cfg.KeywordParenSpacing == config.ParenSpacingRemove && prev >= pos0(tokens, pos) &&
			parenKeywords[strings.ToLower(tokens[prev].text)] && strings.Contains(gapBetween(sql, tokens, prev, sq.open), "\n") {
			// keyword_paren_spacing: remove joins the "(" of a subquery that is joined to one line to its keyword
			sb.WriteString(sql[pos:tokens[prev].end])
			sb.WriteString("(")
		} else {
			sb.WriteString(sql[pos:tokens[sq.open].end])
		}
		lineHang := ""
		if hang {
			ls := strings.LastIndexByte(sql[:tokens[sq.open].start], '\n') + 1
			lead := sql[ls:tokens[sq.open].start]
			lineHang = lead[:len(lead)-len(strings.TrimLeft(lead, " \t"))] + indentUnitStr(cfg)
		}
		inner := inlineContentLines(sql, tokens, sq.open+1, sq.close, cfg.ParenSpacing, false, lineHang)
		if cfg.ParenSpacing == config.ParenSpacingAdd && inner != "" && sq.close-1 > sq.open && !strings.HasPrefix(tokens[sq.close-1].text, "--") {
			inner = " " + inner + " " // (paren_spacing: add — the blanks inside the parentheses stay when the subquery is joined)
		}
		sb.WriteString(inner)
		if sq.close-1 > sq.open && strings.HasPrefix(tokens[sq.close-1].text, "--") {
			// the ")" must not end up inside the comment: it goes to a line of its own, under the line of its "("
			ls := strings.LastIndexByte(sql[:tokens[sq.open].start], '\n') + 1
			lead := sql[ls:tokens[sq.open].start]
			sb.WriteByte('\n')
			sb.WriteString(lead[:len(lead)-len(strings.TrimLeft(lead, " \t"))])
		}
		pos = tokens[sq.close].start
	}
	sb.WriteString(sql[pos:])
	return sb.String()
}

func expandSubqueries(sql string, cfg *config.LayoutConfig, indentUnit string) string {
	mode := subqueryMode(cfg)
	if mode == config.BreakPreserve || mode == config.BreakNever {
		return sql
	}
	res, err := pg_query.Scan(sql)
	if err != nil {
		return sql
	}
	tokens := make([]rawTok, len(res.Tokens))
	for i, t := range res.Tokens {
		tokens[i] = rawTok{start: int(t.Start), end: int(t.End), text: sql[t.Start:t.End]}
	}
	subs := findSubqueries(tokens)
	if len(subs) == 0 {
		return sql
	}

	var sb strings.Builder
	pos := 0
	lastIndent, expanded := "", false
	for _, sq := range subs {
		openEnd := tokens[sq.open].end
		if mode == config.BreakAuto && !subqueryLineTooLong(sql, tokens[sq.open].start, tokens[sq.close].end, cfg.LineLength, cfg.ParenIndent.Open == config.ParenOpenOwnLine, indentUnit, cfg.CommaSpacing == config.CommaSpacingCompact, cfg.IndentSetLater || cfg.ParenIndent.Mode != config.ParenIndentPreserve && cfg.ParenIndent.Mode != "") {
			continue
		}
		inner := strings.TrimSpace(sql[openEnd:tokens[sq.close].start])
		if inner == "" {
			continue
		}
		sb.WriteString(afterExpanded(sql[pos:openEnd], lastIndent, expanded && cfg.Clauses.Align != config.AlignRight))
		indent := holderIndent(sb.String())
		bodyIndent := indent + indentUnit
		if strings.Contains(inner, "\n") {
			// the lines a -- comment kept apart: take the indent this expansion gave them off again
			inner = dedentLines(inner, len(bodyIndent))
		}
		body := applyCore(inner, cfg, indentUnit)
		sb.WriteByte('\n')
		for i, ln := range strings.Split(body, "\n") {
			if i > 0 {
				sb.WriteByte('\n')
			}
			if strings.TrimSpace(ln) != "" {
				sb.WriteString(bodyIndent)
				sb.WriteString(ln)
			}
		}
		sb.WriteByte('\n')
		sb.WriteString(indent)
		pos = tokens[sq.close].start
		lastIndent, expanded = indent, true
	}
	sb.WriteString(afterExpanded(sql[pos:], lastIndent, expanded && cfg.Clauses.Align != config.AlignRight))
	return sb.String()
}

// afterExpanded is the text that follows an expanded subquery (`) _(x) union select …`, up to the
// next subquery): a set operation on the line of the closing parenthesis goes to a line of its own
// at once, where the clause layout puts it on the next run (it keeps it inline only while it cannot
// see that the line before has been broken; not with clauses.align: right, which indents it by the
// width of its river).
func afterExpanded(seg, indent string, expanded bool) string {
	if !expanded {
		return seg
	}
	nl := strings.IndexByte(seg, '\n')
	if nl < 0 {
		nl = len(seg)
	}
	if loc := setOpWord.FindStringIndex(seg[:nl]); loc != nil {
		return seg[:loc[0]] + "\n" + indent + seg[loc[0]+1:]
	}
	return seg
}

// subqueryLineTooLong reports whether the line holding the subquery at [start, end) is
// longer than lineLength. A set operation (UNION, INTERSECT, EXCEPT) is a line break of its own
// (the clause layout puts it there), so the line is measured between the neighbouring ones; and a
// `(` that paren_indent.open: own_line has put on a line of its own stands where the line before
// it ends (`FROM` + `(`) — the decision must not depend on which of these the text happens to be
// in, or the next run would take the opposite one.
func subqueryLineTooLong(sql string, start, end, lineLength int, ownLineParen bool, indentUnit string, commaCompact, setLater bool) bool {
	ls := strings.LastIndexByte(sql[:start], '\n') + 1
	front := sql[ls:start]
	if ownLineParen && strings.TrimSpace(front) == "" && ls > 0 {
		pl := strings.LastIndexByte(sql[:ls-1], '\n') + 1
		if prev := strings.TrimRight(sql[pl:ls-1], " \t"); strings.TrimSpace(prev) != "" && !strings.Contains(prev, "--") && !strings.HasSuffix(prev, "(") {
			front = prev + " " + strings.TrimLeft(front, " \t")
			front = strings.TrimRight(front, " \t") + " "
		}
	}
	if loc := setOpWord.FindAllStringIndex(front, -1); len(loc) > 0 {
		front = front[loc[len(loc)-1][0]+1:] // from the operator on
	}
	rest := restOfLine(sql, end)
	// what hangs on the subquery is its alias (`) AS t (a, b)`) and a `,` / `;`: the rest of the line
	// (the next join, WHERE …) goes to lines of its own, or stays, depending on the clause layout;
	// and what stands before it is measured after the last `)` (an earlier subquery that was
	// expanded, ending a line the other run put in front of this one)
	if m := aliasTail.FindString(rest); (m != "" && !aliasIsKeyword(m)) || strings.TrimSpace(rest) == "" {
		rest = m
	} else {
		rest = ""
	}
	if i := strings.LastIndexByte(front, ')'); i >= 0 {
		front = front[i+1:]
	}
	if setLater {
		// paren_indent sets the indentation of the lines afterwards: the blanks in front of this one
		// are not the ones it ends up with (a second run would measure those)
		front = strings.TrimLeft(front, " \t")
	}
	front = collapseBlanks(front)
	rest = collapseBlanks(rest)
	// (a tab counts as the indent unit it is converted to: indent.normalize: change turns the
	// tabs of the lines into blanks after this decision was taken, which made the next run see
	// longer lines)
	tab := len(indentUnit)
	if tab == 0 || strings.Contains(indentUnit, "\t") {
		tab = 4
	}
	front = strings.ReplaceAll(front, "\t", strings.Repeat(" ", tab))
	// (a subquery inside is joined to one line on its own, without the blank after its `(`)
	// (blank runs inside count as one: a CASE inside is reformatted afterwards, and that takes them off)
	sub := collapseBlanks(innerSubqueryOpen.ReplaceAllString(sql[start:end], "(${1}"))
	if commaCompact {
		// comma_spacing: compact takes the blank after a comma off in the token pass; a text that
		// has it (lines joined behind a comma) and one that has not are the same line to measure
		front, sub, rest = unspaceCommas(front), unspaceCommas(sub), unspaceCommas(rest)
	}
	return len(front)+len(sub)+len(rest) > lineLength
}

// unspaceCommas removes the blanks after the commas of s outside string constants.
func unspaceCommas(s string) string {
	if !strings.Contains(s, ", ") {
		return s
	}
	var b strings.Builder
	inStr := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		b.WriteByte(c)
		switch {
		case c == '\'':
			inStr = !inStr
		case c == ',' && !inStr:
			for i+1 < len(s) && s[i+1] == ' ' {
				i++
			}
		}
	}
	return b.String()
}

// aliasIsKeyword reports whether the word the alias pattern matched is a keyword that starts
// the next clause or set operation (it goes to a line of its own, so it is no part of the line
// of the subquery).
func aliasIsKeyword(m string) bool {
	w := strings.ToLower(strings.TrimRight(strings.TrimSpace(m), ",;"))
	if i := strings.IndexAny(w, " \t\n("); i >= 0 {
		w = w[:i]
	}
	switch w {
	case "union", "intersect", "except", "from", "where", "group", "order", "having", "limit",
		"offset", "window", "join", "inner", "left", "right", "full", "cross", "natural", "on",
		"using", "for", "fetch", "returning", "values", "select":
		return true
	}
	return false
}

var innerSubqueryOpen = regexp.MustCompile(`(?i)\(\s+(select\b|with\b|values\b)`)

var aliasTail = regexp.MustCompile(`(?i)^\s*(?:as\s+)?(?:"[^"]+"|[a-z_][a-z0-9_$]*)?\s*(?:\([^()]*\))?\s*[,;]?`)

// collapseBlanks reduces the runs of blanks inside s (not those that open it) to one.
func collapseBlanks(s string) string {
	t := strings.TrimLeft(s, " \t")
	return s[:len(s)-len(t)] + strings.Join(strings.Fields(t), " ") + func() string {
		if strings.HasSuffix(t, " ") {
			return " "
		}
		return ""
	}()
}

var setOpWord = regexp.MustCompile(`(?i)[ \t](union|intersect|except)\b`)

// restOfLine is the text of sql after end up to the end of its line.
func restOfLine(sql string, end int) string {
	if i := strings.IndexByte(sql[end:], '\n'); i >= 0 {
		return sql[end : end+i]
	}
	return sql[end:]
}

// holderIndent is the indent of the last line of text; a line that starts with ", " (leading
// comma) counts as indented by the comma's two columns, so its items line up.
func holderIndent(text string) string {
	line := text[strings.LastIndexByte(text, '\n')+1:]
	trimmed := strings.TrimLeft(line, " \t")
	indent := line[:len(line)-len(trimmed)]
	if strings.HasPrefix(trimmed, ", ") {
		indent += "  "
	}
	return indent
}

// hasSubquery reports whether tokens[from:to] contain a parenthesised subquery.
func hasSubquery(tokens []rawTok, from, to int) bool {
	for i := from; i+1 < to && i+1 < len(tokens); i++ {
		if tokens[i].text != "(" {
			continue
		}
		n := i + 1
		for n < len(tokens)-1 && isCommentTok(tokens[n].text) {
			n++
		}
		switch strings.ToLower(tokens[n].text) {
		case "select", "with", "values":
			return true
		}
	}
	return false
}

// ExpandSubqueries applies layout.subquery.break to the subqueries of a fragment that is not
// a statement on its own — the condition of a PL/pgSQL IF / ELSIF / WHILE. text should start
// with the line the fragment begins on, so a subquery opens at that line's indent.
func ExpandSubqueries(text string, cfg *config.LayoutConfig) string {
	if cfg == nil || subqueryMode(cfg) == config.BreakPreserve {
		return text
	}
	return expandSubqueries(flattenSubqueriesHang(text, cfg, true), cfg, indentUnitStr(cfg))
}

// dedentLines removes up to n leading blanks from every line of text after the first.
func dedentLines(text string, n int) string {
	lines := strings.Split(text, "\n")
	for i := 1; i < len(lines); i++ {
		k := 0
		for k < n && k < len(lines[i]) && (lines[i][k] == ' ' || lines[i][k] == '\t') {
			k++
		}
		lines[i] = lines[i][k:]
	}
	return strings.Join(lines, "\n")
}
