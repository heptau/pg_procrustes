package formatter

import "strings"

// sqlSpace is the white space of SQL: PostgreSQL's scanner knows these six and nothing else.
// A character such as U+00A0 or U+0085, white space to strings.TrimSpace, is part of an
// identifier in SQL (`select \u0085` selects a column of that name) and must not be trimmed.
const sqlSpace = " \t\n\r\f\v"

// trimSQLSpace is strings.TrimSpace for SQL white space only.
func trimSQLSpace(s string) string { return strings.Trim(s, sqlSpace) }
