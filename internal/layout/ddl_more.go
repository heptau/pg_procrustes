package layout

import (
	"strings"

	pg_query "github.com/pganalyze/pg_query_go/v6"

	"github.com/heptau/pg_procrustes/config"
)

// rebuildSegments lays out a statement whose parts begin at the token indexes in starts
// (ascending): everything before starts[0] stays on the first line, every part goes on its
// own line one level deeper (always / auto when too long), or all parts are joined onto
// one line (never). ok is false when nothing would change. Statements with a `--`
// comment are left alone.
func rebuildSegments(sql string, tokens []rawTok, stmt statement, starts []int, mode config.BreakMode, cfg *config.LayoutConfig, indentUnit string, depth int) (string, bool) {
	if mode == "" || mode == config.BreakPreserve || len(starts) == 0 {
		return "", false
	}
	if hasLineComment(tokens, codeStart(tokens, stmt), stmt.endTok) {
		return "", false
	}
	flat := flatLength(sql, tokens, codeStart(tokens, stmt), stmt.endTok)
	parenMode := cfg.ParenSpacing
	var sb strings.Builder
	if !shouldBreak(flat, mode, cfg.LineLength) {
		if mode != config.BreakNever {
			return "", false
		}
		sb.WriteString(ddlHead(sql, tokens, stmt, starts[0]-1))
		for k, s := range starts {
			end := stmt.endTok
			if k+1 < len(starts) {
				end = starts[k+1]
			}
			sb.WriteByte(' ')
			sb.WriteString(inlineContent(sql, tokens, s, end, parenMode))
		}
		finishDDL(&sb, tokens, stmt)
		return sb.String(), true
	}
	indent := strings.Repeat(indentUnit, depth+1)
	sb.WriteString(ddlHead(sql, tokens, stmt, starts[0]-1))
	for k, s := range starts {
		end := stmt.endTok
		if k+1 < len(starts) {
			end = starts[k+1]
		}
		sb.WriteByte('\n')
		sb.WriteString(indent)
		sb.WriteString(inlineContent(sql, tokens, s, end, parenMode))
	}
	finishDDL(&sb, tokens, stmt)
	return sb.String(), true
}

// lowerAt returns the lower-cased token text at i, or "" outside [0, end).
func lowerAt(tokens []rawTok, i, end int) string {
	if i < 0 || i >= end || i >= len(tokens) {
		return ""
	}
	return strings.ToLower(tokens[i].text)
}

// segmentStarts returns the indexes in [from, end) of depth-0 tokens for which starts
// says they open a new part; starts sees the lower-cased word at i and may look around
// with w. A word that opens a part for the first time only is reported by once.
func segmentStarts(tokens []rawTok, from, end int, starts func(w func(int) string, i int) bool) []int {
	w := func(i int) string { return lowerAt(tokens, i, end) }
	var out []int
	depth := 0
	for i := from; i < end; i++ {
		switch tokens[i].text {
		case "(", "[":
			depth++
			continue
		case ")", "]":
			depth--
			continue
		}
		if depth == 0 && !isCommentTok(tokens[i].text) && starts(w, i) {
			out = append(out, i)
		}
	}
	return out
}

// rebuildMoreCreate handles the CREATE statements besides FUNCTION, INDEX and TABLE.
func rebuildMoreCreate(sql string, tokens []rawTok, stmt statement, first int, cfg *config.LayoutConfig, indentUnit string, depth int) (string, bool) {
	w := func(i int) string { return lowerAt(tokens, i, stmt.endTok) }
	// CREATE [OR REPLACE] [CONSTRAINT | EVENT | TEMP | …] <object> …
	i := first + 1
	if w(i) == "or" && w(i+1) == "replace" {
		i += 2
	}
	for _, m := range []string{"constraint", "event", "temp", "temporary", "unlogged", "global", "local", "recursive", "materialized"} {
		if w(i) == m {
			i++
			break
		}
	}
	switch w(i) {
	case "trigger":
		return rebuildTrigger(sql, tokens, stmt, i, w(i-1) == "event", cfg, indentUnit, depth)
	case "policy":
		return rebuildPolicy(sql, tokens, stmt, i, cfg, indentUnit, depth)
	case "sequence":
		return rebuildSequence(sql, tokens, stmt, first, cfg, indentUnit, depth)
	case "type":
		return rebuildCreateType(sql, tokens, stmt, i, cfg, indentUnit, depth)
	case "domain":
		return rebuildDomain(sql, tokens, stmt, i, cfg, indentUnit, depth)
	case "extension":
		return rebuildExtension(sql, tokens, stmt, i, cfg, indentUnit, depth)
	case "table":
		if nameEnd := afterName(tokens, stmt, i+1); nameEnd >= 0 && w(nameEnd) == "partition" && w(nameEnd+1) == "of" {
			return rebuildPartition(sql, tokens, stmt, nameEnd, cfg, indentUnit, depth)
		}
	}
	return "", false
}

// afterName returns the index after `[IF [NOT] EXISTS] name` starting at i, or -1.
func afterName(tokens []rawTok, stmt statement, i int) int {
	w := func(i int) string { return lowerAt(tokens, i, stmt.endTok) }
	if w(i) == "if" && w(i+1) == "not" && w(i+2) == "exists" {
		i += 3
	} else if w(i) == "if" && w(i+1) == "exists" {
		i += 2
	}
	return skipQualifiedName(tokens, i, stmt.endTok)
}

// rebuildTrigger puts the timing and events, ON, FROM, REFERENCING, FOR EACH, WHEN,
// [NOT] DEFERRABLE and EXECUTE parts of CREATE TRIGGER on lines of their own.
func rebuildTrigger(sql string, tokens []rawTok, stmt statement, kw int, event bool, cfg *config.LayoutConfig, indentUnit string, depth int) (string, bool) {
	mode := cfg.DDL.CreateTrigger.Break
	nameEnd := afterName(tokens, stmt, kw+1)
	if nameEnd < 0 {
		return "", false
	}
	seenTiming := false
	seen := map[string]bool{}
	starts := segmentStarts(tokens, nameEnd, stmt.endTok, func(w func(int) string, i int) bool {
		switch x := w(i); x {
		case "before", "after", "instead":
			if !seenTiming && !event {
				seenTiming = true
				return true
			}
		case "on", "from", "referencing", "for", "when", "execute":
			if !seen[x] {
				seen[x] = true
				return true
			}
		case "deferrable":
			return w(i-1) != "not"
		case "not":
			return w(i+1) == "deferrable"
		}
		return false
	})
	return rebuildSegments(sql, tokens, stmt, starts, mode, cfg, indentUnit, depth)
}

// rebuildPolicy puts the ON, AS, FOR, TO, USING and WITH CHECK parts of CREATE POLICY on
// lines of their own.
func rebuildPolicy(sql string, tokens []rawTok, stmt statement, kw int, cfg *config.LayoutConfig, indentUnit string, depth int) (string, bool) {
	nameEnd := skipQualifiedName(tokens, kw+1, stmt.endTok)
	if nameEnd < 0 {
		return "", false
	}
	seen := map[string]bool{}
	starts := segmentStarts(tokens, nameEnd, stmt.endTok, func(w func(int) string, i int) bool {
		switch x := w(i); x {
		case "on", "as", "for", "to", "using":
			if !seen[x] {
				seen[x] = true
				return true
			}
		case "with":
			return w(i+1) == "check"
		}
		return false
	})
	return rebuildSegments(sql, tokens, stmt, starts, cfg.DDL.CreatePolicy.Break, cfg, indentUnit, depth)
}

// rebuildGrant puts the ON, TO / FROM, WITH …, GRANTED BY and CASCADE / RESTRICT parts of
// GRANT and REVOKE on lines of their own. The privilege and object lists stay as written.
func rebuildGrant(sql string, tokens []rawTok, stmt statement, first int, cfg *config.LayoutConfig, indentUnit string, depth int) (string, bool) {
	seen := map[string]bool{}
	starts := segmentStarts(tokens, first+1, stmt.endTok, func(w func(int) string, i int) bool {
		switch x := w(i); x {
		case "on", "to", "from", "granted":
			if !seen[x] {
				seen[x] = true
				return true
			}
		case "with":
			return seen["to"] || seen["from"]
		case "cascade", "restrict":
			return true
		}
		return false
	})
	// A part has to follow something: the statement's first part must not be its keyword.
	for len(starts) > 0 && starts[0] <= first+1 {
		starts = starts[1:]
	}
	return rebuildSegments(sql, tokens, stmt, starts, cfg.DDL.Grant.Break, cfg, indentUnit, depth)
}

// rebuildSequence puts every option of CREATE / ALTER SEQUENCE (AS, INCREMENT, MINVALUE,
// MAXVALUE, START, RESTART, CACHE, CYCLE, OWNED BY, …) on a line of its own.
func rebuildSequence(sql string, tokens []rawTok, stmt statement, first int, cfg *config.LayoutConfig, indentUnit string, depth int) (string, bool) {
	kw := first + 1
	for kw < stmt.endTok && lowerAt(tokens, kw, stmt.endTok) != "sequence" {
		kw++
	}
	nameEnd := afterName(tokens, stmt, kw+1)
	if kw >= stmt.endTok || nameEnd < 0 {
		return "", false
	}
	starts := segmentStarts(tokens, nameEnd, stmt.endTok, func(w func(int) string, i int) bool {
		switch w(i) {
		case "as", "increment", "start", "restart", "cache", "owned", "logged", "unlogged", "no":
			return true
		case "minvalue", "maxvalue", "cycle":
			return w(i-1) != "no"
		}
		return false
	})
	return rebuildSegments(sql, tokens, stmt, starts, cfg.DDL.CreateSequence.Break, cfg, indentUnit, depth)
}

// rebuildCreateType lays out the parenthesised list of CREATE TYPE name AS ENUM (…),
// AS (attribute type, …) and AS RANGE / base type definitions: one entry per line.
func rebuildCreateType(sql string, tokens []rawTok, stmt statement, kw int, cfg *config.LayoutConfig, indentUnit string, depth int) (string, bool) {
	i := afterName(tokens, stmt, kw+1)
	if i < 0 {
		return "", false
	}
	w := func(i int) string { return lowerAt(tokens, i, stmt.endTok) }
	if w(i) == "as" {
		i++
		if w(i) == "enum" || w(i) == "range" {
			i++
		}
	}
	if w(i) != "(" {
		return "", false
	}
	return rebuildParenList(sql, tokens, stmt, i, cfg.DDL.CreateType.Break, config.ColumnAlignPreserve, cfg, indentUnit, depth)
}

// viewHeader lays out the column list of CREATE [OR REPLACE] [TEMP] [RECURSIVE]
// [MATERIALIZED] VIEW name (a, b, …) [WITH (…)] AS — the tokens [from, to) before the
// query — according to layout.ddl.create_view.columns. ok is false when the header is
// not one, has a `--` comment or the setting leaves it as written.
func viewHeader(sql string, tokens []rawTok, from, to int, cfg *config.LayoutConfig, indentUnit string, depth int) (string, bool) {
	mode := cfg.DDL.CreateView.Columns
	if mode == "" || mode == config.BreakPreserve || from >= to {
		return "", false
	}
	first := firstCodeTok(tokens, from, to)
	if first < 0 || lowerAt(tokens, first, to) != "create" || hasLineComment(tokens, from, to) {
		return "", false
	}
	w := func(i int) string { return lowerAt(tokens, i, to) }
	i := first + 1
	if w(i) == "or" && w(i+1) == "replace" {
		i += 2
	}
	for _, m := range []string{"temp", "temporary", "recursive", "materialized"} {
		if w(i) == m {
			i++
		}
	}
	if w(i) != "view" {
		return "", false
	}
	i++
	if w(i) == "if" && w(i+1) == "not" && w(i+2) == "exists" {
		i += 3
	}
	if i = skipQualifiedName(tokens, i, to); i < 0 || w(i) != "(" {
		return "", false
	}
	open := i
	closeIdx := matchParenTok(tokens, open, to)
	if closeIdx < 0 {
		return "", false
	}
	items := splitAtComma(sql, tokens, open+1, closeIdx)
	if len(items) == 0 {
		return "", false
	}
	flat := flatLength(sql, tokens, from, to)
	rest := strings.TrimSpace(originalText(sql, tokens, closeIdx+1, to))
	var sb strings.Builder
	sb.WriteString(sql[tokens[from].start:tokens[open].end])
	if !shouldBreak(flat, mode, cfg.LineLength) {
		if mode != config.BreakNever {
			return "", false
		}
		for k, it := range items {
			if k > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(collapseGaps(it, 0, len(it)))
		}
	} else {
		baseIndent := strings.Repeat(indentUnit, depth)
		emitDDLItems(&sb, items, resolveCommaStart(cfg.Content.CommaPosition, sql, tokens, open+1, closeIdx), baseIndent+indentUnit)
		sb.WriteByte('\n')
		sb.WriteString(baseIndent)
	}
	sb.WriteByte(')')
	if rest != "" {
		sb.WriteByte(' ')
		sb.WriteString(rest)
	}
	return sb.String(), true
}

// isCreateTrigger reports whether the statement starting at from is CREATE [OR REPLACE]
// [CONSTRAINT | EVENT] TRIGGER (looking at the tokens before i only).
func isCreateTrigger(tokens []rawTok, from, i int) bool {
	first := firstCodeTok(tokens, from, i)
	if first < 0 || !strings.EqualFold(tokens[first].text, "create") {
		return false
	}
	for k, n := first+1, 0; k < i && n < 4; k++ {
		if isCommentTok(tokens[k].text) {
			continue
		}
		n++
		switch strings.ToLower(tokens[k].text) {
		case "trigger":
			return true
		case "or", "replace", "constraint", "event":
		default:
			return false
		}
	}
	return false
}

// scanRaw tokenizes text for the DDL helpers that work on a single item.
func scanRaw(text string) []rawTok {
	res, err := pg_query.Scan(text)
	if err != nil {
		return nil
	}
	toks := make([]rawTok, len(res.Tokens))
	for i, t := range res.Tokens {
		toks[i] = rawTok{start: int(t.Start), end: int(t.End), text: text[t.Start:t.End]}
	}
	return toks
}

// constraintAction reports whether an ALTER TABLE action item is ADD [CONSTRAINT name]
// PRIMARY KEY | FOREIGN KEY | UNIQUE | CHECK | EXCLUDE and returns its tokens.
func constraintAction(item string) ([]rawTok, bool) {
	toks := scanRaw(item)
	w := func(i int) string { return lowerAt(toks, i, len(toks)) }
	if len(toks) < 3 || w(0) != "add" {
		return nil, false
	}
	i := 1
	if w(1) == "constraint" {
		i = 3
	}
	switch w(i) {
	case "primary", "foreign", "unique", "check", "exclude":
		return toks, true
	}
	return nil, false
}

// breakConstraint puts the parts of an ADD CONSTRAINT action — the constraint type
// (FOREIGN KEY …), REFERENCES, MATCH, ON DELETE / UPDATE, [NOT] DEFERRABLE, NOT VALID —
// on lines of their own, indented by indent. Items with a `--` comment are returned as is.
func breakConstraint(item string, toks []rawTok, indent string, parenMode config.ParenSpacingMode) string {
	if hasLineComment(toks, 0, len(toks)) {
		return item
	}
	named := lowerAt(toks, 1, len(toks)) == "constraint"
	starts := segmentStarts(toks, 1, len(toks), func(w func(int) string, i int) bool {
		switch w(i) {
		case "primary", "foreign", "unique", "check", "exclude":
			return named && i == 3
		case "references", "match":
			return true
		case "on":
			return w(i+1) == "delete" || w(i+1) == "update"
		case "deferrable":
			return w(i-1) != "not"
		case "not":
			return w(i+1) == "deferrable" || w(i+1) == "valid"
		}
		return false
	})
	if len(starts) == 0 {
		return item
	}
	var sb strings.Builder
	sb.WriteString(inlineContent(item, toks, 0, starts[0], parenMode))
	for k, s := range starts {
		end := len(toks)
		if k+1 < len(starts) {
			end = starts[k+1]
		}
		sb.WriteByte('\n')
		sb.WriteString(indent)
		sb.WriteString(inlineContent(item, toks, s, end, parenMode))
	}
	return sb.String()
}

// rebuildPartition puts the parts of CREATE TABLE name PARTITION OF parent — FOR VALUES …
// (or DEFAULT), PARTITION BY, WITH, TABLESPACE — on lines of their own.
func rebuildPartition(sql string, tokens []rawTok, stmt statement, nameEnd int, cfg *config.LayoutConfig, indentUnit string, depth int) (string, bool) {
	seenBy := false
	seen := map[string]bool{}
	starts := segmentStarts(tokens, nameEnd, stmt.endTok, func(w func(int) string, i int) bool {
		switch x := w(i); x {
		case "partition":
			if i == nameEnd {
				return true
			}
			if w(i+1) == "by" && !seenBy {
				seenBy = true
				return true
			}
		case "for", "default", "tablespace", "using", "inherits":
			if !seen[x] {
				seen[x] = true
				return true
			}
		case "with":
			return w(i-1) != "values"
		case "on":
			return w(i+1) == "commit"
		}
		return false
	})
	return rebuildSegments(sql, tokens, stmt, starts, cfg.DDL.CreatePartition.Break, cfg, indentUnit, depth)
}

// rebuildDrop puts the objects of DROP TABLE a, b, c [CASCADE] one per line with CASCADE /
// RESTRICT on a line of its own; always and auto need more than one object.
func rebuildDrop(sql string, tokens []rawTok, stmt statement, first int, cfg *config.LayoutConfig, indentUnit string, depth int) (string, bool) {
	mode := cfg.DDL.Drop.Break
	if mode == "" || mode == config.BreakPreserve || hasLineComment(tokens, codeStart(tokens, stmt), stmt.endTok) {
		return "", false
	}
	w := func(i int) string { return lowerAt(tokens, i, stmt.endTok) }
	i := first + 1
	switch w(i) {
	case "table", "view", "index", "sequence", "schema", "type", "domain", "extension", "collation", "role", "user", "group", "database":
		i++
	case "materialized", "foreign":
		if w(i+1) != "view" && w(i+1) != "table" {
			return "", false
		}
		i += 2
	default:
		return "", false
	}
	if w(i) == "concurrently" {
		i++
	}
	if w(i) == "if" && w(i+1) == "exists" {
		i += 2
	}
	listStart := i
	listEnd := stmt.endTok
	tail := -1
	depthP := 0
	for k := listStart; k < stmt.endTok; k++ {
		switch tokens[k].text {
		case "(":
			depthP++
		case ")":
			depthP--
		}
		if depthP == 0 && (w(k) == "cascade" || w(k) == "restrict") {
			listEnd, tail = k, k
			break
		}
	}
	if listStart >= listEnd {
		return "", false
	}
	items := splitAtComma(sql, tokens, listStart, listEnd)
	if len(items) == 0 {
		return "", false
	}
	flat := flatLength(sql, tokens, codeStart(tokens, stmt), stmt.endTok)
	var sb strings.Builder
	sb.WriteString(ddlHead(sql, tokens, stmt, listStart-1))
	if len(items) < 2 && mode != config.BreakNever || !shouldBreak(flat, mode, cfg.LineLength) && mode != config.BreakNever {
		return "", false
	}
	if mode == config.BreakNever {
		for k, it := range items {
			if k == 0 {
				sb.WriteByte(' ')
			} else {
				sb.WriteString(", ")
			}
			sb.WriteString(collapseGaps(it, 0, len(it)))
		}
		if tail >= 0 {
			sb.WriteByte(' ')
			sb.WriteString(inlineContent(sql, tokens, tail, stmt.endTok, cfg.ParenSpacing))
		}
		finishDDL(&sb, tokens, stmt)
		return sb.String(), true
	}
	baseIndent := strings.Repeat(indentUnit, depth)
	emitDDLItems(&sb, items, resolveCommaStart(cfg.Content.CommaPosition, sql, tokens, listStart, listEnd), baseIndent+indentUnit)
	if tail >= 0 {
		sb.WriteByte('\n')
		sb.WriteString(baseIndent)
		sb.WriteString(inlineContent(sql, tokens, tail, stmt.endTok, cfg.ParenSpacing))
	}
	finishDDL(&sb, tokens, stmt)
	return sb.String(), true
}

// rebuildDomain puts the DEFAULT, COLLATE, NOT NULL / NULL and CONSTRAINT / CHECK parts of
// CREATE DOMAIN name AS type on lines of their own.
func rebuildDomain(sql string, tokens []rawTok, stmt statement, kw int, cfg *config.LayoutConfig, indentUnit string, depth int) (string, bool) {
	nameEnd := skipQualifiedName(tokens, kw+1, stmt.endTok)
	if nameEnd < 0 {
		return "", false
	}
	starts := segmentStarts(tokens, nameEnd, stmt.endTok, func(w func(int) string, i int) bool {
		switch w(i) {
		case "default", "collate", "constraint":
			return true
		case "not":
			return w(i+1) == "null"
		case "null":
			return w(i-1) != "not"
		case "check":
			return w(i-2) != "constraint"
		}
		return false
	})
	return rebuildSegments(sql, tokens, stmt, starts, cfg.DDL.CreateDomain.Break, cfg, indentUnit, depth)
}

// rebuildExtension puts SCHEMA, VERSION and CASCADE of CREATE EXTENSION on lines of their own.
func rebuildExtension(sql string, tokens []rawTok, stmt statement, kw int, cfg *config.LayoutConfig, indentUnit string, depth int) (string, bool) {
	nameEnd := afterName(tokens, stmt, kw+1)
	if nameEnd < 0 {
		return "", false
	}
	starts := segmentStarts(tokens, nameEnd, stmt.endTok, func(w func(int) string, i int) bool {
		switch w(i) {
		case "with":
			return i == nameEnd
		case "schema", "version", "from":
			return w(i-1) != "with" || i-1 != nameEnd
		case "cascade":
			return true
		}
		return false
	})
	return rebuildSegments(sql, tokens, stmt, starts, cfg.DDL.CreateExtension.Break, cfg, indentUnit, depth)
}
