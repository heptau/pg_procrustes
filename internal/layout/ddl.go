package layout

import (
	"strings"

	pg_query "github.com/pganalyze/pg_query_go/v6"

	"github.com/heptau/pg_procrustes/config"
)

// isCommentTok reports whether the token text is a line or block comment.
func isCommentTok(text string) bool {
	return strings.HasPrefix(text, "--") || strings.HasPrefix(text, "/*")
}

// firstCodeTok returns the index of the first non-comment token in [from, to), or -1.
func firstCodeTok(tokens []rawTok, from, to int) int {
	for i := from; i < to && i < len(tokens); i++ {
		if !isCommentTok(tokens[i].text) {
			return i
		}
	}
	return -1
}

// isDDLStart reports whether the statement tokens[from:to] begin with CREATE or ALTER.
func isDDLStart(tokens []rawTok, from, to int) bool {
	i := firstCodeTok(tokens, from, to)
	if i < 0 {
		return false
	}
	switch strings.ToLower(tokens[i].text) {
	case "create", "alter", "drop", "grant", "revoke":
		return true
	}
	return false
}

// rebuildDDL formats CREATE TABLE column lists and ALTER TABLE action lists according to
// layout.ddl. ok is false when the statement is not handled or the settings leave it as
// written, in which case the caller keeps the original text.
func rebuildDDL(sql string, tokens []rawTok, stmt statement, cfg *config.LayoutConfig, indentUnit string, depth int) (string, bool) {
	if stmt.endTok <= stmt.startTok {
		return "", false
	}
	first := firstCodeTok(tokens, stmt.startTok, stmt.endTok)
	if first < 0 {
		return "", false
	}
	w := func(i int) string {
		if i < 0 || i >= stmt.endTok {
			return ""
		}
		return strings.ToLower(tokens[i].text)
	}
	switch w(first) {
	case "create":
		if open := createFunctionOpenParen(tokens, stmt, first); open >= 0 {
			return rebuildCreateFunction(sql, tokens, stmt, open, cfg, indentUnit, depth)
		}
		if idx := createIndexToken(tokens, stmt, first); idx >= 0 {
			return rebuildCreateIndex(sql, tokens, stmt, idx, cfg, indentUnit, depth)
		}
		if open := createTableOpenParen(tokens, stmt, first); open >= 0 {
			return rebuildCreateTable(sql, tokens, stmt, open, cfg, indentUnit, depth)
		}
		if out, ok := rebuildMoreCreate(sql, tokens, stmt, first, cfg, indentUnit, depth); ok {
			return out, true
		}
	case "alter":
		if w(first+1) == "table" {
			if start := alterTableActionsStart(tokens, stmt, first); start >= 0 {
				return rebuildAlterTable(sql, tokens, stmt, start, cfg, indentUnit, depth)
			}
		}
		if w(first+1) == "sequence" {
			return rebuildSequence(sql, tokens, stmt, first, cfg, indentUnit, depth)
		}
	case "drop":
		return rebuildDrop(sql, tokens, stmt, first, cfg, indentUnit, depth)
	case "grant", "revoke":
		return rebuildGrant(sql, tokens, stmt, first, cfg, indentUnit, depth)
	}
	return "", false
}

// createTableOpenParen returns the index of the "(" that opens the column list of
// CREATE [GLOBAL|LOCAL] [TEMP|TEMPORARY|UNLOGGED] TABLE [IF NOT EXISTS] name (, or -1.
func createTableOpenParen(tokens []rawTok, stmt statement, first int) int {
	i := first + 1
	lower := func(i int) string {
		if i >= stmt.endTok {
			return ""
		}
		return strings.ToLower(tokens[i].text)
	}
	if t := lower(i); t == "global" || t == "local" {
		i++
	}
	if t := lower(i); t == "temp" || t == "temporary" || t == "unlogged" {
		i++
	}
	if lower(i) != "table" {
		return -1
	}
	i++
	if lower(i) == "if" && lower(i+1) == "not" && lower(i+2) == "exists" {
		i += 3
	}
	i = skipQualifiedName(tokens, i, stmt.endTok)
	if i >= 0 && i < stmt.endTok && tokens[i].text == "(" {
		return i
	}
	return -1
}

// skipQualifiedName returns the index after ident(.ident)* starting at i, or -1.
func skipQualifiedName(tokens []rawTok, i, end int) int {
	if i >= end || isCommentTok(tokens[i].text) {
		return -1
	}
	c := tokens[i].text[0]
	if !(c == '"' || c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= 0x80) {
		return -1
	}
	i++
	for i+1 < end && tokens[i].text == "." {
		i += 2
	}
	return i
}

// alterTableActionsStart returns the index of the first action of an ALTER TABLE
// [IF EXISTS] [ONLY] name [*] statement, or -1 when it is not a plain table alteration.
func alterTableActionsStart(tokens []rawTok, stmt statement, first int) int {
	lower := func(i int) string {
		if i >= stmt.endTok {
			return ""
		}
		return strings.ToLower(tokens[i].text)
	}
	i := first + 2
	if lower(i) == "all" {
		return -1 // ALTER TABLE ALL IN TABLESPACE …
	}
	if lower(i) == "if" && lower(i+1) == "exists" {
		i += 2
	}
	if lower(i) == "only" {
		i++
	}
	i = skipQualifiedName(tokens, i, stmt.endTok)
	if i < 0 {
		return -1
	}
	if i < stmt.endTok && tokens[i].text == "*" {
		i++
	}
	if i >= stmt.endTok {
		return -1
	}
	return i
}

// ddlHead returns the text from the start of the statement (leading comments included)
// through the end of token last.
func ddlHead(sql string, tokens []rawTok, stmt statement, last int) string {
	return sql[tokens[stmt.startTok].start:tokens[last].end]
}

// finishDDL appends the terminating ';' (on its own line after a trailing line comment).
func finishDDL(sb *strings.Builder, tokens []rawTok, stmt statement) {
	if stmt.semiIdx < 0 {
		return
	}
	if last := stmt.endTok - 1; last >= 0 && strings.HasPrefix(tokens[last].text, "--") {
		sb.WriteByte('\n')
	}
	sb.WriteString(tokens[stmt.semiIdx].text)
}

func hasLineComment(tokens []rawTok, from, to int) bool {
	for i := from; i < to; i++ {
		if strings.HasPrefix(tokens[i].text, "--") {
			return true
		}
	}
	return false
}

// emitDDLItems writes items one per line after the current text, honouring
// content.comma_position, and returns nothing; sb ends right after the last item.
func emitDDLItems(sb *strings.Builder, items []string, commaStart bool, indent string) {
	for i, item := range items {
		if commaStart && i > 0 {
			item = writeLeadingComments(sb, item, indent)
		}
		sb.WriteByte('\n')
		if commaStart && i > 0 {
			sb.WriteString(leadCommaIndent(indent) + ", ")
		} else {
			sb.WriteString(indent)
		}
		item = reindentContinuation(item, indent)
		if !commaStart && i < len(items)-1 {
			item = withTrailingComma(item)
		}
		sb.WriteString(item)
	}
}

// reindentContinuation indents the continuation lines of a multi-line item to indent
// unless they already are (so repeated runs do not keep adding indentation).
func reindentContinuation(item, indent string) string {
	nl := strings.IndexByte(item, '\n')
	if nl < 0 {
		return item
	}
	for _, ln := range strings.Split(item[nl+1:], "\n") {
		if strings.TrimSpace(ln) == "" {
			continue
		}
		if strings.HasPrefix(ln, indent) {
			return item
		}
		break
	}
	parts := strings.Split(item, "\n")
	for j := 1; j < len(parts); j++ {
		if strings.TrimSpace(parts[j]) != "" {
			parts[j] = indent + strings.TrimLeft(parts[j], " \t")
		}
	}
	return strings.Join(parts, "\n")
}

func rebuildCreateTable(sql string, tokens []rawTok, stmt statement, open int, cfg *config.LayoutConfig, indentUnit string, depth int) (string, bool) {
	return rebuildParenList(sql, tokens, stmt, open, cfg.DDL.CreateTable.Break, cfg.DDL.CreateTable.Align, cfg, indentUnit, depth)
}

// rebuildParenList lays out the comma-separated list inside the "(" at open — the columns of
// CREATE TABLE, the labels of CREATE TYPE … AS ENUM — one entry per line with the closing
// parenthesis on a line of its own, or joined onto one line (never).
func rebuildParenList(sql string, tokens []rawTok, stmt statement, open int, mode config.BreakMode, align config.ColumnAlignMode, cfg *config.LayoutConfig, indentUnit string, depth int) (string, bool) {
	if mode == "" || mode == config.BreakPreserve {
		return "", false
	}
	closeIdx := matchParenTok(tokens, open, stmt.endTok)
	if closeIdx < 0 {
		return "", false
	}
	items := splitAtComma(sql, tokens, open+1, closeIdx)
	if len(items) == 0 {
		return "", false
	}
	flat := flatLength(sql, tokens, stmt.startTok, stmt.endTok)
	tail := strings.TrimSpace(originalText(sql, tokens, closeIdx+1, stmt.endTok))

	var sb strings.Builder
	if !shouldBreak(flat, mode, cfg.LineLength) {
		if mode != config.BreakNever || hasLineComment(tokens, open, closeIdx+1) {
			return "", false
		}
		// never: collapse to one line (not possible with a line comment inside).
		sb.WriteString(ddlHead(sql, tokens, stmt, open))
		for i, it := range items {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(collapseGaps(it, 0, len(it)))
		}
		sb.WriteByte(')')
		if tail != "" {
			sb.WriteByte(' ')
			sb.WriteString(tail)
		}
		finishDDL(&sb, tokens, stmt)
		return sb.String(), true
	}

	baseIndent := strings.Repeat(indentUnit, depth)
	itemIndent := baseIndent + indentUnit
	items = alignColumns(items, align)
	sb.WriteString(ddlHead(sql, tokens, stmt, open))
	emitDDLItems(&sb, items, resolveCommaStart(cfg.Content.CommaPosition, sql, tokens, open+1, closeIdx), itemIndent)
	sb.WriteByte('\n')
	sb.WriteString(baseIndent)
	sb.WriteByte(')')
	if tail != "" {
		sb.WriteByte(' ')
		sb.WriteString(tail)
	}
	finishDDL(&sb, tokens, stmt)
	return sb.String(), true
}

func rebuildAlterTable(sql string, tokens []rawTok, stmt statement, start int, cfg *config.LayoutConfig, indentUnit string, depth int) (string, bool) {
	mode := cfg.DDL.AlterTable.Break
	cmode := cfg.DDL.AlterTable.Constraints
	modeSet := mode != "" && mode != config.BreakPreserve
	consSet := cmode != "" && cmode != config.BreakPreserve
	if !modeSet && !consSet {
		return "", false
	}
	items := splitAtComma(sql, tokens, start, stmt.endTok)
	if len(items) == 0 {
		return "", false
	}
	flat := flatLength(sql, tokens, stmt.startTok, stmt.endTok)
	doBreak := false
	if modeSet {
		switch mode {
		case config.BreakAlways:
			doBreak = true
		case config.BreakAuto:
			doBreak = len(items) > 1 || flat > cfg.LineLength
		}
	}
	// ADD CONSTRAINT actions whose parts go on lines of their own (they need the action on a line of its own too).
	consToks := make([][]rawTok, len(items))
	consBreak := false
	if consSet && !hasLineComment(tokens, start, stmt.endTok) {
		for k, it := range items {
			if t, ok := constraintAction(it); ok {
				consToks[k] = t
				consBreak = true
			}
		}
		consBreak = consBreak && shouldBreak(flat, cmode, cfg.LineLength)
	}
	if consBreak {
		doBreak = true
	}
	if !doBreak {
		if !modeSet || mode != config.BreakNever || hasLineComment(tokens, start, stmt.endTok) {
			return "", false
		}
		var sb strings.Builder
		sb.WriteString(ddlHead(sql, tokens, stmt, start-1))
		for i, it := range items {
			if i == 0 {
				sb.WriteByte(' ')
			} else {
				sb.WriteString(", ")
			}
			sb.WriteString(collapseGaps(it, 0, len(it)))
		}
		finishDDL(&sb, tokens, stmt)
		return sb.String(), true
	}
	baseIndent := strings.Repeat(indentUnit, depth)
	if consBreak {
		partIndent := baseIndent + indentUnit + indentUnit
		for k, t := range consToks {
			if t != nil {
				items[k] = breakConstraint(items[k], t, partIndent, cfg.ParenSpacing)
			}
		}
	}
	var sb strings.Builder
	sb.WriteString(ddlHead(sql, tokens, stmt, start-1))
	emitDDLItems(&sb, items, resolveCommaStart(cfg.Content.CommaPosition, sql, tokens, start, stmt.endTok), baseIndent+indentUnit)
	finishDDL(&sb, tokens, stmt)
	return sb.String(), true
}

// matchParenTok returns the index of the ")" that closes the "(" at open, or -1.
func matchParenTok(tokens []rawTok, open, end int) int {
	depth := 0
	for i := open; i < end; i++ {
		switch tokens[i].text {
		case "(":
			depth++
		case ")":
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// columnConstraintStarters begin the constraint part of a column definition.
var columnConstraintStarters = map[string]bool{
	"not": true, "null": true, "default": true, "primary": true, "references": true,
	"check": true, "unique": true, "generated": true, "constraint": true, "collate": true,
	"compression": true, "storage": true,
}

// tableConstraintStarters begin a table-level constraint (not a column definition).
var tableConstraintStarters = map[string]bool{
	"constraint": true, "primary": true, "unique": true, "foreign": true,
	"check": true, "exclude": true, "like": true,
}

// columnParts is a column definition split into name, type and the rest.
type columnParts struct {
	lead    string // comment lines above the definition
	name    string
	typ     string
	rest    string // constraints, defaults, … (may be empty)
	comment string // trailing line comment
}

// splitColumn splits a column definition; ok is false for table constraints and for
// anything that does not look like "name type …" (those are left as written).
func splitColumn(item string) (columnParts, bool) {
	var cp columnParts
	res, err := pg_query.Scan(item)
	if err != nil || len(res.Tokens) == 0 {
		return cp, false
	}
	type tk struct {
		start, end int
		text       string
	}
	var toks []tk
	for _, t := range res.Tokens {
		toks = append(toks, tk{int(t.Start), int(t.End), item[t.Start:t.End]})
	}
	// Leading comments.
	i := 0
	for i < len(toks) && isCommentTok(toks[i].text) {
		i++
	}
	if i == len(toks) {
		return cp, false
	}
	if i > 0 {
		cp.lead = strings.TrimSpace(item[:toks[i].start])
	}
	body := toks[i:]
	// Trailing line comment.
	if n := len(body); n > 0 && strings.HasPrefix(body[n-1].text, "--") {
		if n > 1 && strings.Contains(item[body[n-2].end:body[n-1].start], "\n") {
			// a comment line of its own below the definition (it precedes the next entry):
			// pulling it up behind the definition would move it on the next run
			return cp, false
		}
		cp.comment = body[n-1].text
		body = body[:n-1]
	}
	for _, t := range body {
		if isCommentTok(t.text) {
			return cp, false // a comment inside the definition: leave it alone
		}
	}
	if len(body) < 2 || tableConstraintStarters[strings.ToLower(body[0].text)] {
		return cp, false
	}
	cp.name = body[0].text
	depth := 0
	restAt := -1
	for k := 1; k < len(body); k++ {
		t := body[k].text
		switch t {
		case "(", "[":
			depth++
		case ")", "]":
			if depth > 0 {
				depth--
			}
		}
		if depth == 0 && k > 1 && columnConstraintStarters[strings.ToLower(t)] {
			restAt = k
			break
		}
	}
	typeEnd := len(body)
	if restAt >= 0 {
		typeEnd = restAt
	}
	cp.typ = collapseGaps(item, body[1].start, body[typeEnd-1].end)
	if restAt >= 0 {
		cp.rest = collapseGaps(item, body[restAt].start, body[len(body)-1].end)
	}
	if cp.typ == "" {
		return cp, false
	}
	return cp, true
}

// collapseGaps returns item[from:to] with every run of whitespace replaced by one space,
// except inside string literals and quoted identifiers.
func collapseGaps(item string, from, to int) string {
	res, err := pg_query.Scan(item[from:to])
	if err != nil {
		return strings.TrimSpace(item[from:to])
	}
	text := item[from:to]
	var b strings.Builder
	cur := 0
	for _, t := range res.Tokens {
		gap := text[cur:t.Start]
		if gap != "" {
			b.WriteByte(' ')
		}
		b.WriteString(text[t.Start:t.End])
		cur = int(t.End)
	}
	return b.String()
}

// alignColumns applies create_table.align to the column definitions in items; table
// constraints and definitions that cannot be split are returned unchanged.
func alignColumns(items []string, mode config.ColumnAlignMode) []string {
	if mode == "" || mode == config.ColumnAlignPreserve {
		return items
	}
	parts := make([]columnParts, len(items))
	ok := make([]bool, len(items))
	maxName, maxType := 0, 0
	for i, it := range items {
		parts[i], ok[i] = splitColumn(it)
		if !ok[i] {
			continue
		}
		if n := runeLen(parts[i].name); n > maxName {
			maxName = n
		}
		if parts[i].rest != "" {
			if n := runeLen(parts[i].typ); n > maxType {
				maxType = n
			}
		}
	}
	out := make([]string, len(items))
	for i, it := range items {
		if !ok[i] {
			out[i] = it
			continue
		}
		p := parts[i]
		var b strings.Builder
		if p.lead != "" {
			b.WriteString(p.lead)
			b.WriteByte('\n')
		}
		b.WriteString(p.name)
		switch mode {
		case config.ColumnAlignNone:
			b.WriteByte(' ')
		default:
			b.WriteString(strings.Repeat(" ", maxName-runeLen(p.name)+1))
		}
		b.WriteString(p.typ)
		if p.rest != "" {
			if mode == config.ColumnAlignAll {
				b.WriteString(strings.Repeat(" ", maxType-runeLen(p.typ)+1))
			} else {
				b.WriteByte(' ')
			}
			b.WriteString(p.rest)
		}
		if p.comment != "" {
			b.WriteByte(' ')
			b.WriteString(p.comment)
		}
		out[i] = b.String()
	}
	return out
}

func runeLen(s string) int { return len([]rune(s)) }

// createIndexToken returns the index of the INDEX token of CREATE [UNIQUE] INDEX …, or -1.
func createIndexToken(tokens []rawTok, stmt statement, first int) int {
	i := first + 1
	if i < stmt.endTok && strings.EqualFold(tokens[i].text, "unique") {
		i++
	}
	if i < stmt.endTok && strings.EqualFold(tokens[i].text, "index") {
		return i
	}
	return -1
}

// rebuildCreateIndex puts the ON, USING, INCLUDE, NULLS [NOT] DISTINCT, WITH (…),
// TABLESPACE and WHERE parts of CREATE INDEX on their own indented lines.
func rebuildCreateIndex(sql string, tokens []rawTok, stmt statement, idx int, cfg *config.LayoutConfig, indentUnit string, depth int) (string, bool) {
	mode := cfg.DDL.CreateIndex.Break
	if mode == "" || mode == config.BreakPreserve {
		return "", false
	}
	if hasLineComment(tokens, stmt.startTok, stmt.endTok) {
		return "", false
	}
	// Segment starts: the first ON after INDEX, then USING / INCLUDE / TABLESPACE / WHERE /
	// WITH ( / NULLS [NOT] DISTINCT at parenthesis depth 0.
	var starts []int
	parens, seenOn := 0, false
	lw := func(i int) string {
		if i >= stmt.endTok {
			return ""
		}
		return strings.ToLower(tokens[i].text)
	}
	for i := idx + 1; i < stmt.endTok; i++ {
		switch tokens[i].text {
		case "(":
			parens++
			continue
		case ")":
			parens--
			continue
		}
		if parens != 0 {
			continue
		}
		switch lw(i) {
		case "on":
			if !seenOn {
				seenOn = true
				starts = append(starts, i)
			}
		case "using", "include", "tablespace", "where":
			if seenOn {
				starts = append(starts, i)
			}
		case "with":
			if seenOn && lw(i+1) == "(" {
				starts = append(starts, i)
			}
		case "nulls":
			if seenOn && (lw(i+1) == "distinct" || lw(i+1) == "not" && lw(i+2) == "distinct") {
				starts = append(starts, i)
			}
		}
	}
	if len(starts) == 0 {
		return "", false
	}
	return rebuildSegments(sql, tokens, stmt, starts, mode, cfg, indentUnit, depth)
}

// createFunctionOpenParen returns the index of the "(" opening the parameter list of
// CREATE [OR REPLACE] FUNCTION | PROCEDURE name (, or -1.
func createFunctionOpenParen(tokens []rawTok, stmt statement, first int) int {
	lower := func(i int) string {
		if i >= stmt.endTok {
			return ""
		}
		return strings.ToLower(tokens[i].text)
	}
	i := first + 1
	if lower(i) == "or" && lower(i+1) == "replace" {
		i += 2
	}
	if w := lower(i); w != "function" && w != "procedure" {
		return -1
	}
	i = skipQualifiedName(tokens, i+1, stmt.endTok)
	if i >= 0 && i < stmt.endTok && tokens[i].text == "(" {
		return i
	}
	return -1
}

// functionOptionStarts returns the indexes of the tokens in (from, end) that begin a
// function option (RETURNS, LANGUAGE, IMMUTABLE, SECURITY DEFINER, SET …, AS body, …).
func functionOptionStarts(tokens []rawTok, from, end int) []int {
	lw := func(i int) string {
		if i < 0 || i >= end {
			return ""
		}
		return strings.ToLower(tokens[i].text)
	}
	var starts []int
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
		if depth != 0 {
			continue
		}
		switch lw(i) {
		case "returns", "language", "window", "immutable", "stable", "volatile", "leakproof",
			"strict", "external", "parallel", "cost", "rows", "support", "set", "transform", "as", "called":
			starts = append(starts, i)
		case "security":
			if lw(i-1) != "external" {
				starts = append(starts, i)
			}
		case "not":
			if lw(i+1) == "leakproof" {
				starts = append(starts, i)
			}
		}
	}
	return starts
}

// rebuildCreateFunction lays out the header of CREATE FUNCTION / PROCEDURE: the parameter
// list one parameter per line and/or every option (RETURNS, LANGUAGE, …) on its own line.
func rebuildCreateFunction(sql string, tokens []rawTok, stmt statement, open int, cfg *config.LayoutConfig, indentUnit string, depth int) (string, bool) {
	pm, om := cfg.DDL.CreateFunction.Params, cfg.DDL.CreateFunction.Options
	isSet := func(m config.BreakMode) bool { return m != "" && m != config.BreakPreserve }
	if !isSet(pm) && !isSet(om) {
		return "", false
	}
	closeIdx := matchParenTok(tokens, open, stmt.endTok)
	if closeIdx < 0 {
		return "", false
	}
	for i := stmt.startTok; i < stmt.endTok; i++ {
		// BEGIN ATOMIC bodies and comments in the header are left to the generic code.
		if strings.HasPrefix(tokens[i].text, "--") || strings.EqualFold(tokens[i].text, "begin") {
			return "", false
		}
	}
	starts := functionOptionStarts(tokens, closeIdx+1, stmt.endTok)
	if closeIdx+1 < stmt.endTok && (len(starts) == 0 || starts[0] != closeIdx+1) {
		return "", false // something unexpected follows the parameter list
	}
	// Flat length of the header, up to the body (a string / dollar-quoted token).
	headEnd := stmt.endTok
	for i := closeIdx + 1; i < stmt.endTok; i++ {
		if t := tokens[i].text; strings.HasPrefix(t, "$") || strings.HasPrefix(t, "'") || strings.HasPrefix(t, "E'") {
			headEnd = i
			break
		}
	}
	flat := flatLength(sql, tokens, stmt.startTok, headEnd)
	baseIndent := strings.Repeat(indentUnit, depth)

	var sb strings.Builder
	sb.WriteString(ddlHead(sql, tokens, stmt, open))
	// Parameters.
	if closeIdx > open+1 {
		items := splitAtComma(sql, tokens, open+1, closeIdx)
		switch {
		case !isSet(pm):
			sb.WriteString(originalText(sql, tokens, open+1, closeIdx))
		case shouldBreak(flat, pm, cfg.LineLength):
			emitDDLItems(&sb, items, resolveCommaStart(cfg.Content.CommaPosition, sql, tokens, open+1, closeIdx), baseIndent+indentUnit)
			sb.WriteByte('\n')
			sb.WriteString(baseIndent)
		default:
			if pm != config.BreakNever {
				sb.WriteString(originalText(sql, tokens, open+1, closeIdx))
				break
			}
			for i, it := range items {
				if i > 0 {
					sb.WriteString(", ")
				}
				sb.WriteString(collapseGaps(it, 0, len(it)))
			}
		}
	}
	sb.WriteByte(')')
	// Options.
	if len(starts) > 0 {
		switch {
		case !isSet(om) || (!shouldBreak(flat, om, cfg.LineLength) && om != config.BreakNever):
			sb.WriteString(originalText(sql, tokens, closeIdx+1, stmt.endTok))
		default:
			for k, s := range starts {
				end := stmt.endTok
				if k+1 < len(starts) {
					end = starts[k+1]
				}
				if shouldBreak(flat, om, cfg.LineLength) {
					sb.WriteByte('\n')
					sb.WriteString(baseIndent)
				} else {
					sb.WriteByte(' ')
				}
				sb.WriteString(inlineContent(sql, tokens, s, end, cfg.ParenSpacing))
			}
		}
	}
	finishDDL(&sb, tokens, stmt)
	return sb.String(), true
}
