package formatter

import "strings"

// isGetDiagnosticsWord reports whether tokens[i] is a word of the PL/pgSQL statement
// GET [CURRENT | STACKED] DIAGNOSTICS (comments between the words are skipped).
func isGetDiagnosticsWord(tokens []rawToken, i int) bool {
	word := func(k int) string {
		if k < 0 || k >= len(tokens) {
			return ""
		}
		return strings.ToLower(tokens[k].text)
	}
	next := func(k int) int {
		for k++; k < len(tokens) && isCommentText(tokens[k].text); k++ {
		}
		return k
	}
	prev := func(k int) int {
		for k--; k >= 0 && isCommentText(tokens[k].text); k-- {
		}
		return k
	}
	switch word(i) {
	case "get":
		n := next(i)
		if w := word(n); w == "stacked" || w == "current" {
			n = next(n)
		}
		return word(n) == "diagnostics"
	case "stacked", "current":
		return word(prev(i)) == "get" && word(next(i)) == "diagnostics"
	case "diagnostics":
		p := prev(i)
		if w := word(p); w == "stacked" || w == "current" {
			p = prev(p)
		}
		return word(p) == "get"
	}
	return false
}
