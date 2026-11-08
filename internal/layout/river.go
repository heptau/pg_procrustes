package layout

import (
	"strings"

	"github.com/heptau/pg_procrustes/config"
)

// River layout (clauses.align: right): the keywords of the clauses of a statement are
// right-aligned on a common edge and their content hangs after them, so the gap between
// keywords and content forms a "river" down the query:
//
//	SELECT a.id,
//	       a.name
//	  FROM accounts AS a
//	 WHERE a.id > 1
//	 ORDER BY a.name
//
// Only clauses that are broken onto a line of their own take part; the rest stays as written.
// A set operator (UNION, …) ends a river, each query of it gets its own width.

// riverEligible reports whether clauses of this kind are placed on the river.
func riverEligible(k clauseKind) bool {
	switch k {
	case clauseUnknown, clauseUnion, clauseException, clauseSemi, clauseWithOption:
		return false
	}
	return true
}

// riverKeyword returns the keyword of clause ci with its words separated by single blanks,
// and whether the clause can sit on the river at all (a comment between the clauses or text
// in front of a first clause that is not the query itself keep the clause out of it).
func riverKeyword(sql string, tokens []rawTok, stmt statement, ci int) (string, bool) {
	cl := stmt.clauses[ci]
	if !riverEligible(cl.kind) {
		return "", false
	}
	kwStart := stmt.startTok
	if ci > 0 {
		kwStart = stmt.clauses[ci-1].conEnd
	}
	kwFrom := kwStart
	if ci == 0 && hasCodeTokens(tokens, kwStart, cl.kwTok) {
		if cl.kind != clauseSelect && cl.kind != clauseWith {
			return "", false
		}
		kwFrom = cl.kwTok // CREATE VIEW v AS / EXPLAIN …: the query starts after the prefix
	}
	text := originalText(sql, tokens, kwFrom, cl.kwEnd)
	if strings.Contains(text, "--") || strings.Contains(text, "/*") {
		return "", false
	}
	kw := strings.Join(strings.Fields(text), " ")
	return kw, kw != ""
}

// riverSlot is where a clause sits: width is the river's width (0: the clause is not on one);
// hang marks a JOIN, which does not widen the river but starts in the column of the content
// (as in the SQL style guide: JOIN lines hang under the table of FROM).
type riverSlot struct {
	width int
	hang  bool
}

// riverWidths returns the riverSlot of every clause of stmt.
func riverWidths(sql string, tokens []rawTok, stmt statement, cfg *config.LayoutConfig, stmtFlatLen int) []riverSlot {
	slots := make([]riverSlot, len(stmt.clauses))
	var members []int
	width := 0
	flush := func() {
		if width > 0 {
			for _, m := range members {
				slots[m].width = width
				slots[m].hang = stmt.clauses[m].kind == clauseJoin
			}
		}
		members, width = nil, 0
	}
	for ci, cl := range stmt.clauses {
		if cl.kind == clauseUnion {
			flush()
			continue
		}
		kw, ok := riverKeyword(sql, tokens, stmt, ci)
		if !ok {
			continue
		}
		brk, _ := clauseBreakAlign(cl.kind, cfg)
		if !shouldBreak(stmtFlatLen, brk, cfg.LineLength) {
			continue
		}
		members = append(members, ci)
		if n := runeLen(kw); n > width && cl.kind != clauseJoin {
			width = n
		}
	}
	flush()
	return slots
}
