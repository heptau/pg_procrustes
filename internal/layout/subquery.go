package layout

import (
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
		sb.WriteString(sql[pos:tokens[sq.open].end])
		sb.WriteString(inlineContent(sql, tokens, sq.open+1, sq.close, cfg.ParenSpacing))
		if sq.close-1 > sq.open && strings.HasPrefix(tokens[sq.close-1].text, "--") {
			sb.WriteByte('\n') // the ")" must not end up inside the comment
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
	for _, sq := range subs {
		openEnd := tokens[sq.open].end
		if mode == config.BreakAuto && !subqueryLineTooLong(sql, tokens[sq.open].start, tokens[sq.close].end, cfg.LineLength) {
			continue
		}
		inner := strings.TrimSpace(sql[openEnd:tokens[sq.close].start])
		if inner == "" {
			continue
		}
		sb.WriteString(sql[pos:openEnd])
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
	}
	sb.WriteString(sql[pos:])
	return sb.String()
}

// subqueryLineTooLong reports whether the line holding the subquery at [start, end) is
// longer than lineLength.
func subqueryLineTooLong(sql string, start, end, lineLength int) bool {
	ls := strings.LastIndexByte(sql[:start], '\n') + 1
	le := end
	if i := strings.IndexByte(sql[end:], '\n'); i >= 0 {
		le = end + i
	} else {
		le = len(sql)
	}
	return len(sql[ls:le]) > lineLength
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
	return expandSubqueries(flattenSubqueries(text, cfg), cfg, indentUnitStr(cfg))
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
