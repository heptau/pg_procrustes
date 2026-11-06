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
	if !strings.Contains(sql, "'") || strings.Contains(sql, litPrefix) {
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
	for _, t := range res.Tokens {
		text := sql[t.Start:t.End]
		if !quote(text) {
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
