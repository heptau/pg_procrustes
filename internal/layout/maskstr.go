package layout

import (
	"strconv"
	"strings"

	pg_query "github.com/pganalyze/pg_query_go/v6"
)

const litPrefix = "__pgp_lit_"

// maskStrings replaces every string constant of sql that spans several lines ('…', E'…',
// U&'…', B'…', X'…') by a one-line placeholder constant, so that no layout step — which
// indents and joins lines without knowing what is data — can change its text. The returned
// function puts the originals back. Dollar-quoted strings are not touched: their bodies are
// laid out on purpose. When sql cannot be scanned, or already uses the placeholder name,
// it is returned as it is.
func maskStrings(sql string) (string, func(string) string) {
	identity := func(s string) string { return s }
	if !strings.ContainsAny(sql, "'$") || strings.Contains(sql, litPrefix) {
		return sql, identity
	}
	res, err := pg_query.Scan(sql)
	if err != nil {
		return sql, identity
	}
	var (
		b     strings.Builder
		lits  []string
		cur   int
		quote = func(text string) bool {
			if strings.HasPrefix(text, "$") || !strings.Contains(text, "\n") {
				return false
			}
			i := strings.IndexByte(text, '\'')
			return i >= 0 && i <= 2 && strings.TrimLeft(text[:i], "eEbBxXuU&nN") == ""
		}
	)
	for k, t := range res.Tokens {
		text := sql[t.Start:t.End]
		if strings.HasPrefix(text, "$") {
			// a dollar-quoted text that is no function or DO body (COMMENT ON … IS $$…$$) is data
			if !strings.Contains(text, "\n") || dollarBodyIsCode(sql, res.Tokens, k) {
				continue
			}
		} else if !quote(text) {
			continue
		}
		b.WriteString(sql[cur:t.Start])
		b.WriteString("'" + litPrefix + strconv.Itoa(len(lits)) + "__'")
		lits = append(lits, text)
		cur = int(t.End)
	}
	if len(lits) == 0 {
		return sql, identity
	}
	b.WriteString(sql[cur:])
	return b.String(), func(s string) string {
		for i := len(lits) - 1; i >= 0; i-- {
			s = strings.Replace(s, "'"+litPrefix+strconv.Itoa(i)+"__'", lits[i], 1)
		}
		return s
	}
}

// dollarBodyIsCode reports whether the dollar-quoted token k is the code of a function or DO block
// (it follows AS or DO, and its language is SQL or PL/pgSQL), not a text value.
func dollarBodyIsCode(sql string, toks []*pg_query.ScanToken, k int) bool {
	word := func(i int) string {
		if i < 0 || i >= len(toks) {
			return ""
		}
		return strings.ToLower(sql[toks[i].Start:toks[i].End])
	}
	prev := word(k - 1)
	doLang := k >= 3 && word(k-2) == "language" && word(k-3) == "do"
	if prev != "as" && prev != "do" && !doLang {
		return false
	}
	lo, hi := k, k
	for lo > 0 && word(lo-1) != ";" {
		lo--
	}
	for hi < len(toks) && word(hi) != ";" {
		hi++
	}
	for i := lo; i+1 < hi; i++ {
		if word(i) == "language" {
			lang := strings.Trim(word(i+1), `'"`)
			return lang == "sql" || lang == "plpgsql"
		}
	}
	return true
}

// stringDataLines marks the lines of text (by index) that start inside a string constant that spans
// lines, or inside a dollar-quoted text that is no function body; the strings inside the code of
// a function body count too. Their blanks are data: no rule that moves indentation may touch them.
func stringDataLines(text string) map[int]bool {
	marked := map[int]bool{}
	var walk func(t string, base int)
	walk = func(t string, base int) {
		if !strings.ContainsAny(t, "'$") || !strings.Contains(t, "\n") {
			return
		}
		res, err := pg_query.Scan(t)
		if err != nil {
			return
		}
		mark := func(from, to int) { // the line breaks inside [from, to) of t
			for k := from; k < to; k++ {
				if t[k] == '\n' {
					marked[strings.Count(text[:base+k+1], "\n")] = true
				}
			}
		}
		for k, tok := range res.Tokens {
			s := t[tok.Start:tok.End]
			switch {
			case strings.HasPrefix(s, "$"):
				if dollarBodyIsCode(t, res.Tokens, k) {
					if i := strings.IndexByte(s[1:], '$'); i >= 0 {
						open := i + 2
						if end := len(s) - open; end > open {
							walk(s[open:end], base+int(tok.Start)+open)
						}
					}
				} else {
					mark(int(tok.Start), int(tok.End))
				}
			case strings.Contains(s, "\n"):
				if i := strings.IndexByte(s, '\''); i >= 0 && i <= 2 && strings.TrimLeft(s[:i], "eEbBxXuU&nN") == "" {
					mark(int(tok.Start), int(tok.End))
				}
			}
		}
	}
	walk(text, 0)
	return marked
}
