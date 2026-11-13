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
	if runeLen(indent)+runeLen(unpadded(item)) <= lineLen {
		// it only is too long for the blanks of a hand-aligned list (`'a'   , f(x)`); a wrapped item
		// loses them, so the next run would find it short enough and join it again
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
		if t := text(i); strings.EqualFold(t, "case") || strings.EqualFold(t, "select") {
			return item // CASE and subqueries are laid out by their own rules
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
		if open > 0 && strings.EqualFold(text(open-1), "over") {
			// A window specification is not a list: PARTITION BY / ORDER BY / the frame go on
			// a line each (the commas inside them are part of those clauses).
			if segs := windowSegments(item, toks[open+1:closeIdx]); len(segs) >= 2 {
				inner := indent + unit
				var sb strings.Builder
				sb.WriteString(item[:toks[open].End])
				for _, seg := range segs {
					sb.WriteByte('\n')
					sb.WriteString(inner)
					sb.WriteString(seg)
				}
				sb.WriteByte('\n')
				sb.WriteString(indent)
				sb.WriteString(item[toks[closeIdx].Start:])
				return sb.String()
			}
			continue
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
			parts = append(parts, trimSQLSpace(item[from:toks[c].Start]))
			from = int(toks[c].End)
		}
		parts = append(parts, trimSQLSpace(item[from:toks[closeIdx].Start]))
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

// windowSegments splits the tokens of a window specification (without its parentheses) into
// its PARTITION BY, ORDER BY and frame (ROWS / RANGE / GROUPS …) parts, each as text.
func windowSegments(item string, toks []*pg_query.ScanToken) []string {
	text := func(i int) string { return strings.ToLower(item[toks[i].Start:toks[i].End]) }
	var starts []int
	depth := 0
	for i := range toks {
		switch text(i) {
		case "(", "[":
			depth++
			continue
		case ")", "]":
			depth--
			continue
		}
		if depth != 0 {
			continue
		}
		switch text(i) {
		case "partition", "order":
			if i+1 < len(toks) && text(i+1) == "by" {
				starts = append(starts, i)
			}
		case "rows", "range", "groups":
			starts = append(starts, i)
		}
	}
	if len(starts) == 0 {
		return nil
	}
	var segs []string
	// anything before the first keyword is an existing window name: (w partition by …)
	if int(toks[starts[0]].Start) > int(toks[0].Start) {
		segs = append(segs, trimSQLSpace(item[toks[0].Start:toks[starts[0]].Start]))
	}
	for k, st := range starts {
		var end int
		if k+1 < len(starts) {
			end = int(toks[starts[k+1]].Start)
		} else {
			end = int(toks[len(toks)-1].End)
		}
		segs = append(segs, trimSQLSpace(item[toks[st].Start:end]))
	}
	return segs
}

// unpadded returns item with each run of blanks outside strings and comments reduced to one, and
// none before a comma: the length it has once a wrap has taken the alignment padding away.
func unpadded(item string) string {
	res, err := pg_query.Scan(item)
	if err != nil {
		return item
	}
	var sb strings.Builder
	prev := 0
	for i, t := range res.Tokens {
		gap := item[prev:t.Start]
		text := item[t.Start:t.End]
		if i > 0 && strings.TrimLeft(gap, " \t") == "" && gap != "" {
			if text == "," {
				gap = ""
			} else {
				gap = " "
			}
		}
		sb.WriteString(gap)
		sb.WriteString(text)
		prev = int(t.End)
	}
	sb.WriteString(item[prev:])
	return sb.String()
}
