package formatter

import (
	"strings"

	pg_query "github.com/pganalyze/pg_query_go/v6"

	"github.com/heptau/pg_procrustes/config"
)

// moveSelectInto puts the INTO of a PL/pgSQL SELECT after the select list
// (plpgsql_into: after_list):
//
//	SELECT INTO _period * FROM t WHERE …       →  SELECT * INTO _period FROM t WHERE …
//	SELECT INTO STRICT a, b x, y FROM t        →  SELECT x, y INTO STRICT a, b FROM t
//
// The target list is read as PL/pgSQL reads it: a name (or record.field), then more names as
// long as a comma follows. A statement with a comment between INTO and the end of the select
// list, or with a second INTO, is left as it is.
func moveSelectInto(body string, mode config.PLpgSQLIntoMode) string {
	if mode != config.PLpgSQLIntoAfterList || !strings.Contains(strings.ToLower(body), "into") {
		return body
	}
	res, err := pg_query.Scan(body)
	if err != nil {
		return body
	}
	type tk struct {
		start, end int
		text, low  string
		comment    bool
	}
	var toks []tk
	for _, t := range res.Tokens {
		text := body[t.Start:t.End]
		toks = append(toks, tk{int(t.Start), int(t.End), text, strings.ToLower(text),
			strings.HasPrefix(text, "--") || strings.HasPrefix(text, "/*")})
	}
	// next returns the index of the first token from k on that is no comment, or len(toks).
	next := func(k int) int {
		for k < len(toks) && toks[k].comment {
			k++
		}
		return k
	}
	isName := func(k int) bool {
		if k >= len(toks) || toks[k].comment || toks[k].text == "" {
			return false
		}
		c := toks[k].text[0]
		if c == '"' {
			return true
		}
		if !(c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= 0x80) {
			return false
		}
		return !selectListEnd[toks[k].low]
	}

	type edit struct {
		start, end int
		text       string
	}
	var edits []edit
	for i := 0; i < len(toks); i++ {
		if toks[i].low != "select" {
			continue
		}
		into := next(i + 1)
		if into >= len(toks) || toks[into].low != "into" {
			continue
		}
		// INTO [STRICT] target [, target …]
		k := next(into + 1)
		if k < len(toks) && toks[k].low == "strict" {
			k = next(k + 1)
		}
		lastTarget := -1
		for {
			if !isName(k) {
				break
			}
			lastTarget = k
			k = next(k + 1)
			for k+1 < len(toks) && toks[k].text == "." && isName(k+1) {
				lastTarget = k + 1
				k = next(k + 2)
			}
			if k < len(toks) && toks[k].text == "," {
				k = next(k + 1)
				continue
			}
			break
		}
		if lastTarget < 0 {
			continue
		}
		listStart := lastTarget + 1
		if listStart >= len(toks) || toks[listStart].comment {
			continue
		}
		// the select list runs to the first clause keyword (or ';') outside parentheses
		depth, caseDepth := 0, 0
		stop := listStart
		ok := true
		for ; stop < len(toks); stop++ {
			t := toks[stop]
			if t.comment {
				ok = false
				break
			}
			switch t.text {
			case "(", "[":
				depth++
				continue
			case ")", "]":
				depth--
				if depth < 0 {
					stop = len(toks) // a ")" that closes something outside: not our statement shape
					ok = false
				}
				continue
			}
			if !ok || depth > 0 {
				continue
			}
			switch t.low {
			case "case":
				caseDepth++
				continue
			case "end":
				if caseDepth > 0 {
					caseDepth--
				}
				continue
			}
			if caseDepth == 0 && (t.text == ";" || selectListEnd[t.low]) {
				break
			}
		}
		if !ok || stop == listStart || (stop < len(toks) && toks[stop].low == "into") {
			continue
		}
		listEnd := toks[stop-1].end
		edits = append(edits, edit{
			start: toks[into].start,
			end:   listEnd,
			text:  body[toks[listStart].start:listEnd] + " " + body[toks[into].start:toks[lastTarget].end],
		})
		i = stop - 1
	}
	if len(edits) == 0 {
		return body
	}
	var b strings.Builder
	cur := 0
	for _, e := range edits {
		b.WriteString(body[cur:e.start])
		b.WriteString(e.text)
		cur = e.end
	}
	b.WriteString(body[cur:])
	return b.String()
}

// selectListEnd holds the words that end the select list of a SELECT.
var selectListEnd = map[string]bool{
	"from": true, "where": true, "group": true, "having": true, "window": true, "order": true,
	"limit": true, "offset": true, "fetch": true, "for": true, "union": true, "intersect": true,
	"except": true, "into": true,
}
