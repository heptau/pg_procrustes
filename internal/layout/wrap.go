package layout

import (
	"strings"

	pg_query "github.com/pganalyze/pg_query_go/v6"
)

// wrapLongItem breaks a single-line list item that does not fit into lineLen: the
// first parenthesised group with at least two top-level commas (function arguments,
// an IN list, a row) gets one element per line and its ")" on a line of its own;
// elements that are still too long are wrapped the same way, one level deeper.
// indent is the indentation of the item itself (the caller writes it for the first
// line). Items with comments, CASE, subqueries or no such group are returned as is.
func wrapLongItem(item, indent, unit string, lineLen int) string {
	if lineLen <= 0 || strings.ContainsRune(item, '\n') || runeLen(indent)+runeLen(item) <= lineLen {
		return item
	}
	if strings.Contains(item, "--") || strings.Contains(item, "/*") {
		return item
	}
	res, err := pg_query.Scan(item)
	if err != nil {
		return item
	}
	toks := res.Tokens
	text := func(i int) string { return item[toks[i].Start:toks[i].End] }
	for i := range toks {
		if strings.EqualFold(text(i), "case") {
			return item
		}
	}
	for open := range toks {
		if text(open) != "(" {
			continue
		}
		// Match the closing parenthesis and collect the top-level commas.
		depth, closeIdx := 0, -1
		var commas []int
		for k := open; k < len(toks); k++ {
			switch text(k) {
			case "(", "[":
				depth++
			case ")", "]":
				depth--
				if depth == 0 {
					closeIdx = k
				}
			case ",":
				if depth == 1 {
					commas = append(commas, k)
				}
			}
			if closeIdx >= 0 {
				break
			}
		}
		if closeIdx < 0 {
			return item
		}
		if len(commas) == 0 {
			continue // no elements to separate here; a deeper group may have some
		}
		if open+1 < closeIdx {
			switch strings.ToLower(text(open + 1)) {
			case "select", "with", "values", "table":
				return item // a subquery is not a list of elements
			}
		}
		// Elements between the parentheses.
		var parts []string
		from := int(toks[open].End)
		for _, c := range commas {
			parts = append(parts, strings.TrimSpace(item[from:toks[c].Start]))
			from = int(toks[c].End)
		}
		parts = append(parts, strings.TrimSpace(item[from:toks[closeIdx].Start]))
		inner := indent + unit
		var sb strings.Builder
		sb.WriteString(item[:toks[open].End])
		for k, p := range parts {
			sb.WriteByte('\n')
			sb.WriteString(inner)
			p = wrapLongItem(p, inner, unit, lineLen)
			sb.WriteString(p)
			if k < len(parts)-1 {
				sb.WriteByte(',')
			}
		}
		sb.WriteByte('\n')
		sb.WriteString(indent)
		sb.WriteString(item[toks[closeIdx].Start:])
		return sb.String()
	}
	return item
}
