package layout

import (
	"regexp"
	"strings"

	pg_query "github.com/pganalyze/pg_query_go/v6"
)

// alignItems pads the part of every item before its top-level separator ("as" for
// SELECT aliases, "=" for SET assignments) so the separators line up. Items that
// span lines, carry comments, contain CASE or have no separator are returned as
// they are; with fewer than two alignable items nothing changes.
func alignItems(items []string, sep string, indentLen, limit int, commaCompact, skipSubqueries bool) []string {
	// comma_spacing: compact takes the blank after a comma off in the token pass that follows,
	// so the width of a head is the one it will have then
	widthOf := func(head string) int {
		if commaCompact {
			head = unspaceCommas(head)
		}
		return runeLen(head)
	}
	type split struct{ head, tail string }
	parts := make([]split, len(items))
	ok := make([]bool, len(items))
	n, width := 0, 0
	for i, full := range items {
		_, it, _ := splitCommentLines(full)
		if strings.ContainsAny(it, "\n") || strings.Contains(it, "--") || strings.Contains(it, "/*") {
			continue
		}
		if skipSubqueries && alignSubqueryOpen.MatchString(it) {
			continue // (a subquery that is opened onto lines afterwards: the item has several lines then)
		}
		at := topLevelSeparator(it, sep)
		if at <= 0 {
			continue
		}
		head := strings.TrimRight(it[:at], " \t")
		if head == "" {
			continue
		}
		if limit > 0 && indentLen+runeLen(head)+1+runeLen(it[at:]) > limit {
			// line wrapping will break this item up; its width must not set the column of the
			// others (the next run finds it on several lines and leaves it out)
			continue
		}
		parts[i], ok[i] = split{head, it[at:]}, true
		n++
		if w := widthOf(head); w > width {
			width = w
		}
	}
	if n < 2 {
		return items
	}
	out := make([]string, len(items))
	for i, it := range items {
		if !ok[i] {
			out[i] = it
			continue
		}
		p := parts[i]
		lead, _, trail := splitCommentLines(it)
		out[i] = lead + p.head + strings.Repeat(" ", width-widthOf(p.head)+1) + p.tail + trail
	}
	return out
}

// unpadItems is the opposite of alignItems: the blanks before the separator of every
// eligible item shrink to a single space (items spanning lines, with comments or
// CASE, or without a separator stay as they are).
func unpadItems(items []string, sep string) []string {
	out := make([]string, len(items))
	for i, full := range items {
		out[i] = full
		lead, it, trail := splitCommentLines(full)
		if strings.ContainsAny(it, "\n") || strings.Contains(it, "--") || strings.Contains(it, "/*") {
			continue
		}
		at := topLevelSeparator(it, sep)
		if at <= 0 {
			continue
		}
		head := strings.TrimRight(it[:at], " \t")
		if head == "" {
			continue
		}
		out[i] = lead + head + " " + it[at:] + trail
	}
	return out
}

// topLevelSeparator returns the byte offset of the last parenthesis-depth-0 token
// equal to sep (case-insensitive; "=" for symbols), or -1. Items with CASE or
// brackets are rejected, as are items where the separator is a symbol that is
// also part of a longer operator.
func topLevelSeparator(item, sep string) int {
	res, err := pg_query.Scan(item)
	if err != nil {
		return -1
	}
	depth, found := 0, -1
	for _, t := range res.Tokens {
		text := item[t.Start:t.End]
		lower := strings.ToLower(text)
		switch {
		case lower == "case":
			return -1
		case text == "(" || text == "[":
			depth++
		case text == ")" || text == "]":
			depth--
		case depth == 0 && lower == sep:
			if sep == "=" {
				if found >= 0 {
					return -1 // a second top-level "=": the first one is the assignment, keep it simple
				}
			}
			found = int(t.Start)
		}
	}
	return found
}

// splitCommentLines splits the whole comment lines an item starts and ends with from the code
// between them: which item such a line belongs to depends on where the input put it (and on
// comma_position), so it must not decide whether the item takes part in the alignment.
func splitCommentLines(item string) (lead, body, trail string) {
	lead, rest := splitLeadingCommentLines(item)
	if trimSQLSpace(rest) == "" {
		return "", item, ""
	}
	body, trail = splitTrailingCommentLines(rest)
	if trail != "" {
		// splitTrailingCommentLines trims what it takes; the line break before it stays here
		trail = "\n" + strings.TrimSuffix(trail, "\n")
	}
	return lead, body, trail
}

// collapseSeparatorPadding shrinks the blanks before the top-level separator of an item that
// spans several lines to one space: it was aligned while it still fitted on one line and the
// line wrapping broke it up afterwards, leaving padding behind that the next run would not find.
func collapseSeparatorPadding(item, sep string) string {
	at := topLevelSeparator(item, sep)
	if at <= 0 {
		return item
	}
	head := strings.TrimRight(item[:at], " \t")
	if head == "" || strings.HasSuffix(head, "\n") {
		return item
	}
	return head + " " + item[at:]
}

var alignSubqueryOpen = regexp.MustCompile(`(?i)\(\s*(select|with|values)\b`)
