package layout

import (
	"strings"

	pg_query "github.com/pganalyze/pg_query_go/v6"
)

// alignItems pads the part of every item before its top-level separator ("as" for
// SELECT aliases, "=" for SET assignments) so the separators line up. Items that
// span lines, carry comments, contain CASE or have no separator are returned as
// they are; with fewer than two alignable items nothing changes.
func alignItems(items []string, sep string) []string {
	type split struct{ head, tail string }
	parts := make([]split, len(items))
	ok := make([]bool, len(items))
	n, width := 0, 0
	for i, it := range items {
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
		parts[i], ok[i] = split{head, it[at:]}, true
		n++
		if w := runeLen(head); w > width {
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
		out[i] = p.head + strings.Repeat(" ", width-runeLen(p.head)+1) + p.tail
	}
	return out
}

// unpadItems is the opposite of alignItems: the blanks before the separator of every
// eligible item shrink to a single space (items spanning lines, with comments or
// CASE, or without a separator stay as they are).
func unpadItems(items []string, sep string) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it
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
		out[i] = head + " " + it[at:]
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
