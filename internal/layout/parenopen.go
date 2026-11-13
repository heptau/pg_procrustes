package layout

import (
	"strings"

	pg_query "github.com/pganalyze/pg_query_go/v6"
)

// moveOpenParens implements layout.paren_indent.open: own_line. A "(" that ends its line
// (with something before it on that line) and whose matching ")" starts a later line is
// moved onto a line of its own, in the column of that ")":
//
//	from (                  from
//	  select 1       →      (
//	) s                       select 1
//	                        ) s
//
// Parentheses that are already first on their line, share their line with their ")", are
// followed by a comment, or follow a comma, AND / OR or another opening bracket are left alone.
func moveOpenParens(text string) string {
	res, err := pg_query.Scan(text)
	if err != nil {
		return text
	}
	toks := make([]rawTok, len(res.Tokens))
	for i, t := range res.Tokens {
		toks[i] = rawTok{start: int(t.Start), end: int(t.End), text: text[t.Start:t.End]}
	}

	// matching ")" of every "("
	match := map[int]int{}
	var stack []int
	for i, t := range toks {
		switch t.text {
		case "(":
			stack = append(stack, i)
		case ")":
			if n := len(stack); n > 0 {
				match[stack[n-1]] = i
				stack = stack[:n-1]
			}
		}
	}

	type edit struct {
		from, to int
		insert   string
	}
	var edits []edit
	for i, t := range toks {
		if t.text != "(" || i == 0 || i+1 >= len(toks) {
			continue
		}
		cl, ok := match[i]
		if !ok {
			continue
		}
		// the "(" ends its line
		if !strings.Contains(text[t.end:toks[i+1].start], "\n") {
			continue
		}
		// something precedes it on its line
		lineStart := strings.LastIndexByte(text[:t.start], '\n') + 1
		before := trimSQLSpace(text[lineStart:t.start])
		if before == "" || strings.HasSuffix(before, ",") || strings.HasSuffix(before, "(") || strings.HasSuffix(before, "[") {
			continue
		}
		// AND / OR left alone at the end of a line would flip the operator-position detection
		// (and = at line end vs. start) on the next run
		if prev := strings.ToLower(toks[i-1].text); prev == "and" || prev == "or" {
			continue
		}
		// the ")" starts its line
		gap := text[toks[cl-1].end:toks[cl].start]
		nl := strings.LastIndexByte(gap, '\n')
		if nl < 0 || strings.TrimLeft(gap[nl+1:], " \t") != "" {
			continue
		}
		indent := gap[nl+1:]
		// a ")" dedented below the line that opens the block (a verbatim multi-line item) is
		// no column to align with
		if own := len(text[lineStart:t.start]) - len(strings.TrimLeft(text[lineStart:t.start], " \t")); len(indent) < own {
			continue
		}
		edits = append(edits, edit{from: toks[i-1].end, to: t.start, insert: "\n" + indent})
	}
	if len(edits) == 0 {
		return text
	}
	var sb strings.Builder
	pos := 0
	for _, e := range edits {
		sb.WriteString(text[pos:e.from])
		sb.WriteString(e.insert)
		pos = e.to
	}
	sb.WriteString(text[pos:])
	return sb.String()
}
