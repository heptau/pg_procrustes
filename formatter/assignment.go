package formatter

import (
	"strings"

	pg_query "github.com/pganalyze/pg_query_go/v6"

	"github.com/heptau/pg_procrustes/config"
)

// normalizeAssignments rewrites the assignment operator of a PL/pgSQL body:
//
//	plpgsql_assignment: colon_equals   x = 1;  →  x := 1;     v int = 0;  →  v int := 0;
//	plpgsql_assignment: equals         x := 1; →  x = 1;      v int := 0; →  v int = 0;
//
// Only real assignments are touched: a statement that starts with a variable (or
// record.field, arr[i]) followed by the operator, and the default of a DECLARE entry. Named
// arguments (f(a := 1)), SET / UPDATE lists, comparisons and cursor declarations stay as they are.
func normalizeAssignments(body string, mode config.PLpgSQLAssignment) string {
	if mode != config.AssignColonEquals && mode != config.AssignEquals {
		return body
	}
	res, err := pg_query.Scan(body)
	if err != nil {
		return body
	}
	type tk struct {
		start, end int
		text, low  string
	}
	var toks []tk
	for _, t := range res.Tokens {
		text := body[t.Start:t.End]
		if strings.HasPrefix(text, "--") || strings.HasPrefix(text, "/*") {
			continue
		}
		toks = append(toks, tk{int(t.Start), int(t.End), text, strings.ToLower(text)})
	}

	from, to := "=", ":="
	if mode == config.AssignEquals {
		from, to = ":=", "="
	}
	isOp := func(s string) bool { return s == "=" || s == ":=" }

	var edits []int // token indexes whose operator is rewritten
	depth := 0
	inDecl := false
	stmtStart := true
	stmtFirst := ""     // first token of the current statement
	declOpSeen := false // DECLARE entry: the default operator was already looked at
	declCursor := false

	for i := 0; i < len(toks); i++ {
		t := toks[i]
		switch t.text {
		case "(", "[":
			depth++
			continue
		case ")", "]":
			if depth > 0 {
				depth--
			}
			continue
		}
		if depth > 0 {
			continue
		}

		if inDecl && t.low != "begin" {
			switch {
			case t.text == ";":
				declOpSeen, declCursor = false, false
			case t.low == "cursor":
				declCursor = true
			case isOp(t.text) && !declOpSeen:
				declOpSeen = true
				if !declCursor && t.text == from {
					edits = append(edits, i)
				}
			}
			continue
		}

		switch {
		case t.low == "declare" && stmtStart:
			inDecl, declOpSeen, declCursor = true, false, false
			continue
		case t.low == "begin":
			inDecl = false
			stmtStart, stmtFirst = true, ""
			continue
		case t.text == ";":
			stmtStart, stmtFirst = true, ""
			continue
		case t.text == "<<": // <<label>>: the statement starts after it
			continue
		case t.text == ">>":
			stmtStart, stmtFirst = true, ""
			continue
		}

		if stmtStart {
			stmtStart, stmtFirst = false, t.low
			// target: name { .name | [ … ] } followed by the operator
			j := i + 1
			for j < len(toks) {
				if toks[j].text == "." && j+1 < len(toks) {
					j += 2
					continue
				}
				if toks[j].text == "[" {
					d := 0
					for ; j < len(toks); j++ {
						if toks[j].text == "[" {
							d++
						} else if toks[j].text == "]" {
							d--
							if d == 0 {
								j++
								break
							}
						}
					}
					continue
				}
				break
			}
			if j < len(toks) && toks[j].text == from && t.low != "update" && t.low != "set" {
				edits = append(edits, j)
			}
		}

		switch t.low {
		case "then", "else", "loop":
			switch stmtFirst {
			case "if", "elsif", "elseif", "case", "when", "exception", "while", "for", "foreach", "loop", "else":
				stmtStart, stmtFirst = true, ""
			}
		}
	}

	if len(edits) == 0 {
		return body
	}
	var sb strings.Builder
	pos := 0
	for _, i := range edits {
		sb.WriteString(body[pos:toks[i].start])
		sb.WriteString(to)
		pos = toks[i].end
	}
	sb.WriteString(body[pos:])
	return sb.String()
}
