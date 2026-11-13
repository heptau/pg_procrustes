package layout

import "strings"

// sqlSpace is the white space of SQL: PostgreSQL's scanner knows these six and nothing else.
// A character such as U+00A0 or U+0085, white space to strings.TrimSpace, is part of an
// identifier in SQL (`select \u0085` selects a column of that name) and must not be trimmed.
const sqlSpace = " \t\n\r\f\v"

// trimSQLSpace is strings.TrimSpace for SQL white space only.
func trimSQLSpace(s string) string { return strings.Trim(s, sqlSpace) }

// sqlFields is strings.Fields for SQL white space only.
func sqlFields(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return strings.ContainsRune(sqlSpace, r) })
}

// blockCommentLen is the length of the /* … */ comment s starts with, nested comments included
// (`/* a /* b */ c */`: PostgreSQL nests them), or -1 when it does not end.
func blockCommentLen(s string) int {
	depth := 0
	for i := 0; i+1 < len(s); i++ {
		switch {
		case s[i] == '/' && s[i+1] == '*':
			depth++
			i++
		case s[i] == '*' && s[i+1] == '/':
			depth--
			i++
			if depth == 0 {
				return i + 1
			}
		}
	}
	return -1
}
