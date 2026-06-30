// Package layout implements clause-level SQL layout and indentation for pg_procrustes.
package layout

import (
	"strings"

	pg_query "github.com/pganalyze/pg_query_go/v6"

	"github.com/heptau/pg_procrustes/internal/config"
)

// IsNoop returns true if all layout settings are effectively "preserve" / no-op.
func IsNoop(cfg *config.LayoutConfig) bool {
	if cfg == nil {
		return true
	}
	clauseBreak := cfg.Clauses.Break
	if clauseBreak == "" {
		clauseBreak = config.BreakPreserve
	}
	contentBreak := cfg.Content.Break
	if contentBreak == "" {
		contentBreak = config.BreakPreserve
	}
	unionBL := cfg.Union.BlankLine
	if unionBL == "" {
		unionBL = config.UnionBlankLinePreserve
	}
	indentNorm := cfg.Indent.Normalize
	if indentNorm == "" {
		indentNorm = config.IndentNormalizePreserve
	}
	caseBrk := cfg.Case.Break
	if caseBrk == "" {
		caseBrk = config.BreakPreserve
	}
	parenMode := cfg.ParenIndent.Mode
	if parenMode == "" {
		parenMode = config.ParenIndentPreserve
	}
	return clauseBreak == config.BreakPreserve &&
		unionBL == config.UnionBlankLinePreserve &&
		contentBreak == config.BreakPreserve &&
		indentNorm == config.IndentNormalizePreserve &&
		caseBrk == config.BreakPreserve &&
		parenMode == config.ParenIndentPreserve
}

// Apply applies layout rules (clause breaking, indentation) to already-formatted SQL.
func Apply(sql string, cfg *config.LayoutConfig) (string, error) {
	if IsNoop(cfg) {
		return sql, nil
	}
	if strings.TrimSpace(sql) == "" {
		return sql, nil
	}

	scanResult, err := pg_query.Scan(sql)
	if err != nil {
		return sql, nil
	}

	tokens := make([]rawTok, len(scanResult.Tokens))
	for i, t := range scanResult.Tokens {
		tokens[i] = rawTok{
			start: int(t.Start),
			end:   int(t.End),
			text:  sql[t.Start:t.End],
		}
	}

	stmts := parseStatements(tokens)
	if len(stmts) == 0 {
		return sql, nil
	}
	indentUnit := indentUnitStr(cfg)

	// Preserve the leading gap (whitespace before the first token) and trailing
	// gap (whitespace after the last token) so dollar-quoted body indentation survives.
	firstTokStart := tokens[stmts[0].startTok].start
	leadingGap := sql[:firstTokStart]

	lastStmt := stmts[len(stmts)-1]
	lastTokEnd := firstTokStart
	if lastStmt.semiIdx >= 0 {
		lastTokEnd = tokens[lastStmt.semiIdx].end
	} else if len(lastStmt.clauses) > 0 {
		lastTokEnd = tokens[lastStmt.clauses[len(lastStmt.clauses)-1].conEnd-1].end
	}
	trailingGap := sql[lastTokEnd:]

	var parts []string
	for i, stmt := range stmts {
		// Preserve inter-statement gap (between end of previous stmt and start of this one).
		if i > 0 {
			prevStmt := stmts[i-1]
			var prevEnd int
			if prevStmt.semiIdx >= 0 {
				prevEnd = tokens[prevStmt.semiIdx].end
			} else if len(prevStmt.clauses) > 0 {
				prevEnd = tokens[prevStmt.clauses[len(prevStmt.clauses)-1].conEnd-1].end
			}
			curStart := tokens[stmt.startTok].start
			parts = append(parts, sql[prevEnd:curStart])
		}
		rebuilt := rebuildStatement(sql, tokens, stmt, cfg, indentUnit, 0)
		parts = append(parts, rebuilt)
	}

	result := leadingGap + strings.Join(parts, "") + trailingGap

	if cfg.Indent.Normalize == config.IndentNormalizeChange {
		result = normalizeIndent(result, cfg)
	}

	if cfg.ParenIndent.Mode != config.ParenIndentPreserve && cfg.ParenIndent.Mode != "" {
		result = applyParenIndent(result, indentUnit, cfg.ParenIndent.Mode, cfg.ParenIndent.CloseFirst)
	}

	return result, nil
}

type rawTok struct {
	start, end int
	text       string
}

type clauseKind int

const (
	clauseUnknown clauseKind = iota
	clauseWith
	clauseSelect
	clausePerform
	clauseInto
	clauseFrom
	clauseJoin
	clauseWhere
	clauseGroupBy
	clauseHaving
	clauseOrderBy
	clauseLimit
	clauseOffset
	clauseFetch
	clauseValues
	clauseOnConflict
	clauseSet
	clauseUsing
	clauseReturning
	clauseInsert
	clauseUpdate
	clauseDelete
	clauseUnion
	clauseException
	clauseSemi
)

type stmtClause struct {
	kind   clauseKind
	kwEnd  int
	conEnd int
}

type statement struct {
	startTok int
	clauses  []stmtClause
	semiIdx  int
}

func parseStatements(tokens []rawTok) []statement {
	var stmts []statement
	n := len(tokens)
	if n == 0 {
		return nil
	}

	i := 0
	for i < n {
		stmt := statement{startTok: i, semiIdx: -1}
		depth := 0
		for i < n {
			tok := tokens[i]

			if tok.text == "(" || tok.text == "[" {
				depth++
				i++
				continue
			}
			if tok.text == ")" || tok.text == "]" {
				if depth > 0 {
					depth--
				}
				i++
				continue
			}
			if tok.text == ";" && depth == 0 {
				stmt.semiIdx = i
				i++
				break
			}

			if depth == 0 {
				kind, kwLen := detectClause(tokens, i)
				if kind != clauseUnknown {
					if len(stmt.clauses) > 0 {
						last := &stmt.clauses[len(stmt.clauses)-1]
						last.conEnd = i
					}
					cl := stmtClause{
						kind:  kind,
						kwEnd: i + kwLen,
					}
					stmt.clauses = append(stmt.clauses, cl)
					i += kwLen
					continue
				}
			}
			i++
		}
		if len(stmt.clauses) > 0 {
			last := &stmt.clauses[len(stmt.clauses)-1]
			if last.conEnd == 0 {
				end := i
				if stmt.semiIdx >= 0 {
					end = stmt.semiIdx
				}
				last.conEnd = end
			}
		}
		if len(stmt.clauses) > 0 || stmt.semiIdx >= 0 {
			stmts = append(stmts, stmt)
		}
	}
	return stmts
}

func detectClause(tokens []rawTok, i int) (clauseKind, int) {
	n := len(tokens)
	lower := strings.ToLower(tokens[i].text)
	next := func() string {
		if i+1 < n {
			return strings.ToLower(tokens[i+1].text)
		}
		return ""
	}

	switch lower {
	case "with":
		return clauseWith, 1
	case "select":
		return clauseSelect, 1
	case "perform":
		return clausePerform, 1
	case "into":
		return clauseInto, 1
	case "from":
		return clauseFrom, 1
	case "join":
		return clauseJoin, 1
	case "inner":
		if next() == "join" {
			return clauseJoin, 2
		}
	case "left":
		n2 := next()
		if n2 == "join" {
			return clauseJoin, 2
		}
		if n2 == "outer" && i+2 < len(tokens) && strings.ToLower(tokens[i+2].text) == "join" {
			return clauseJoin, 3
		}
	case "right":
		n2 := next()
		if n2 == "join" {
			return clauseJoin, 2
		}
		if n2 == "outer" && i+2 < len(tokens) && strings.ToLower(tokens[i+2].text) == "join" {
			return clauseJoin, 3
		}
	case "full":
		n2 := next()
		if n2 == "join" {
			return clauseJoin, 2
		}
		if n2 == "outer" && i+2 < len(tokens) && strings.ToLower(tokens[i+2].text) == "join" {
			return clauseJoin, 3
		}
	case "cross":
		if next() == "join" {
			return clauseJoin, 2
		}
	case "natural":
		if next() == "join" {
			return clauseJoin, 2
		}
	case "where":
		return clauseWhere, 1
	case "group":
		if next() == "by" {
			return clauseGroupBy, 2
		}
	case "having":
		return clauseHaving, 1
	case "order":
		if next() == "by" {
			return clauseOrderBy, 2
		}
	case "limit":
		return clauseLimit, 1
	case "offset":
		return clauseOffset, 1
	case "fetch":
		return clauseFetch, 1
	case "values":
		return clauseValues, 1
	case "on":
		if next() == "conflict" {
			return clauseOnConflict, 2
		}
	case "set":
		return clauseSet, 1
	case "using":
		return clauseUsing, 1
	case "returning":
		return clauseReturning, 1
	case "insert":
		if next() == "into" {
			return clauseInsert, 2
		}
	case "update":
		return clauseUpdate, 1
	case "delete":
		if next() == "from" {
			return clauseDelete, 2
		}
	case "union", "intersect", "except":
		if next() == "all" {
			return clauseUnion, 2
		}
		return clauseUnion, 1
	case "exception":
		return clauseException, 1
	}
	return clauseUnknown, 0
}

func indentUnitStr(cfg *config.LayoutConfig) string {
	if cfg.Indent.Type == config.IndentTypeTab {
		return "\t"
	}
	size := cfg.Indent.Size
	if size <= 0 {
		size = 3
	}
	return strings.Repeat(" ", size)
}

func flatLength(sql string, tokens []rawTok, from, to int) int {
	if from >= to {
		return 0
	}
	length := 0
	for i := from; i < to; i++ {
		if i > from {
			length++
		}
		length += len(tokens[i].text)
	}
	return length
}

func originalText(sql string, tokens []rawTok, from, to int) string {
	if from >= to {
		return ""
	}
	start := tokens[from].start
	end := tokens[to-1].end
	if start > len(sql) {
		start = len(sql)
	}
	if end > len(sql) {
		end = len(sql)
	}
	return sql[start:end]
}

// gapBetween returns the whitespace/gap in sql between the end of token at index
// prev and the start of token at index next.
func gapBetween(sql string, tokens []rawTok, prev, next int) string {
	if prev < 0 || next >= len(tokens) {
		return ""
	}
	start := tokens[prev].end
	end := tokens[next].start
	if start > end || start > len(sql) || end > len(sql) {
		return ""
	}
	return sql[start:end]
}

func shouldBreak(flatLen int, mode config.BreakMode, lineLength int) bool {
	switch mode {
	case config.BreakAlways:
		return true
	case config.BreakNever:
		return false
	case config.BreakAuto:
		return flatLen > lineLength
	default:
		return false
	}
}

func effectiveBreak(specific config.BreakMode, parent config.BreakMode) config.BreakMode {
	if specific == "" {
		return parent
	}
	return specific
}

func effectiveAlign(specific config.AlignMode, parent config.AlignMode) config.AlignMode {
	if specific == "" {
		return parent
	}
	return specific
}

func clauseBreakAlign(kind clauseKind, cfg *config.LayoutConfig) (config.BreakMode, config.AlignMode) {
	c := &cfg.Clauses
	get := func(r config.ClauseRule) (config.BreakMode, config.AlignMode) {
		return effectiveBreak(r.Break, c.Break), effectiveAlign(r.Align, c.Align)
	}
	switch kind {
	case clauseWith:
		return get(c.With)
	case clauseInto:
		return get(c.Into)
	case clauseFrom:
		return get(c.From)
	case clauseJoin:
		return get(c.Join)
	case clauseWhere:
		return get(c.Where)
	case clauseGroupBy:
		return get(c.GroupBy)
	case clauseHaving:
		return get(c.Having)
	case clauseOrderBy:
		return get(c.OrderBy)
	case clauseLimit:
		return get(c.Limit)
	case clauseOffset:
		return get(c.Offset)
	case clauseValues:
		return get(c.Values)
	case clauseOnConflict:
		return get(c.OnConflict)
	case clauseSet:
		return get(c.Set)
	case clauseUsing:
		return get(c.Using)
	case clauseReturning:
		return get(c.Returning)
	case clauseException:
		return get(c.Exception)
	default:
		return c.Break, c.Align
	}
}

// inlineContent reconstructs the tokens from [from, to) as a single-line string.
// Unlike originalText, it replaces gaps that contain a newline with a single space,
// so content originally formatted across multiple lines is collapsed to one line.
func inlineContent(sql string, tokens []rawTok, from, to int) string {
	if from >= to {
		return ""
	}
	var sb strings.Builder
	for i := from; i < to; i++ {
		if i > from {
			gap := gapBetween(sql, tokens, i-1, i)
			if strings.ContainsRune(gap, '\n') {
				sb.WriteByte(' ')
			} else {
				sb.WriteString(gap)
			}
		}
		sb.WriteString(tokens[i].text)
	}
	return sb.String()
}

func rebuildStatement(sql string, tokens []rawTok, stmt statement, cfg *config.LayoutConfig, indentUnit string, depth int) string {
	if len(stmt.clauses) == 0 {
		start := tokens[stmt.startTok].start
		end := tokens[stmt.startTok].end
		if stmt.semiIdx >= 0 {
			end = tokens[stmt.semiIdx].end
		}
		if start > len(sql) {
			start = len(sql)
		}
		if end > len(sql) {
			end = len(sql)
		}
		return sql[start:end]
	}

	stmtStart := stmt.startTok
	stmtEnd := stmt.clauses[len(stmt.clauses)-1].conEnd
	if stmtEnd > len(tokens) {
		stmtEnd = len(tokens)
	}
	stmtFlatLen := flatLength(sql, tokens, stmtStart, stmtEnd)

	baseIndent := strings.Repeat(indentUnit, depth)

	unionBL := cfg.Union.BlankLine
	if unionBL == "" {
		unionBL = config.UnionBlankLinePreserve
	}

	var sb strings.Builder

	for ci, cl := range stmt.clauses {
		kwStart := stmt.startTok
		if ci > 0 {
			kwStart = stmt.clauses[ci-1].conEnd
		}
		kwEnd := cl.kwEnd
		conEnd := cl.conEnd

		if cl.kind == clauseUnion {
			unionText := originalText(sql, tokens, kwStart, kwEnd)
			contentText := originalText(sql, tokens, kwEnd, conEnd)
			blankLine := cfg.Union.BlankLine
			if blankLine == "" {
				blankLine = config.UnionBlankLinePreserve
			}
			switch blankLine {
			case config.UnionBlankLineNone:
				sb.WriteByte('\n')
				sb.WriteString(baseIndent)
				sb.WriteString(strings.TrimSpace(unionText))
				if strings.TrimSpace(contentText) != "" {
					sb.WriteByte('\n')
					sb.WriteString(baseIndent)
					sb.WriteString(strings.TrimSpace(contentText))
				}
			case config.UnionBlankLineBefore:
				sb.WriteString("\n\n")
				sb.WriteString(baseIndent)
				sb.WriteString(strings.TrimSpace(unionText))
				if strings.TrimSpace(contentText) != "" {
					sb.WriteByte('\n')
					sb.WriteString(baseIndent)
					sb.WriteString(strings.TrimSpace(contentText))
				}
			case config.UnionBlankLineAfter:
				sb.WriteByte('\n')
				sb.WriteString(baseIndent)
				sb.WriteString(strings.TrimSpace(unionText))
				if strings.TrimSpace(contentText) != "" {
					sb.WriteString("\n\n")
					sb.WriteString(baseIndent)
					sb.WriteString(strings.TrimSpace(contentText))
				}
			case config.UnionBlankLineBoth:
				sb.WriteString("\n\n")
				sb.WriteString(baseIndent)
				sb.WriteString(strings.TrimSpace(unionText))
				if strings.TrimSpace(contentText) != "" {
					sb.WriteString("\n\n")
					sb.WriteString(baseIndent)
					sb.WriteString(strings.TrimSpace(contentText))
				}
			default:
				sb.WriteString(originalText(sql, tokens, kwStart, conEnd))
			}
			continue
		}

		breakMode, alignMode := clauseBreakAlign(cl.kind, cfg)
		doBreak := shouldBreak(stmtFlatLen, breakMode, cfg.LineLength)

		// If the previous clause was a UNION with blank-line "after" or "both", insert
		// a blank line before this clause regardless of the source gap.
		prevIsUnionBlankAfter := ci > 0 &&
			stmt.clauses[ci-1].kind == clauseUnion &&
			(unionBL == config.UnionBlankLineAfter || unionBL == config.UnionBlankLineBoth)

		if !doBreak {
			if ci > 0 {
				if prevIsUnionBlankAfter {
					sb.WriteString("\n\n")
					sb.WriteString(baseIndent)
				} else {
					// Include gap between previous clause content and this clause keyword.
					prevConEnd := stmt.clauses[ci-1].conEnd
					if prevConEnd > 0 {
						sb.WriteString(gapBetween(sql, tokens, prevConEnd-1, kwStart))
					}
				}
			}
			needsCaseReformat := cfg.Case.Break != "" && cfg.Case.Break != config.BreakPreserve
			if !needsCaseReformat || kwEnd >= conEnd {
				sb.WriteString(originalText(sql, tokens, kwStart, conEnd))
			} else {
				// Write clause keyword verbatim, reformat CASE within the content.
				sb.WriteString(strings.TrimSpace(originalText(sql, tokens, kwStart, kwEnd)))
				content := strings.TrimSpace(originalText(sql, tokens, kwEnd, conEnd))
				if content != "" {
					sb.WriteByte(' ')
					content = reformatCaseExprs(content, baseIndent, cfg.Case, indentUnit, cfg.LineLength)
					sb.WriteString(content)
				}
			}
			continue
		}

		clauseIndent := baseIndent
		if alignMode == config.AlignIndent {
			clauseIndent = baseIndent + indentUnit
		}

		if ci > 0 {
			if prevIsUnionBlankAfter {
				sb.WriteString("\n\n")
			} else {
				sb.WriteByte('\n')
			}
			sb.WriteString(clauseIndent)
		} else {
			sb.WriteString(clauseIndent)
		}

		kwText := strings.TrimSpace(originalText(sql, tokens, kwStart, kwEnd))
		sb.WriteString(kwText)

		contentBreakMode, contentAlign, contentFirstItem := contentBreakAlign(cl.kind, cfg)
		contentFlatLen := flatLength(sql, tokens, kwEnd, conEnd)
		doContentBreak := shouldBreak(contentFlatLen, contentBreakMode, cfg.LineLength)

		if kwEnd >= conEnd {
			continue
		}

		if !doContentBreak {
			sb.WriteByte(' ')
			// Use inlineContent instead of originalText: the original may span multiple
			// lines (e.g. ORDER BY items each on their own line), and placing that verbatim
			// on the same line as the keyword would embed raw newlines in the output.
			content := inlineContent(sql, tokens, kwEnd, conEnd)
			content = reformatCaseExprs(content, clauseIndent, cfg.Case, indentUnit, cfg.LineLength)
			sb.WriteString(content)
			continue
		}

		contentIndent := clauseIndent + indentUnit
		if contentAlign == config.AlignSame {
			contentIndent = clauseIndent
		}

		// Special case: INSERT INTO table_name (col1, col2, ...) column list.
		if cl.kind == clauseInsert {
			openIdx, closeIdx := findInsertColumnParen(tokens, kwEnd, conEnd)
			if openIdx >= 0 {
				prefix := strings.TrimSpace(inlineContent(sql, tokens, kwEnd, openIdx))
				colItems := splitAtComma(sql, tokens, openIdx+1, closeIdx)
				sb.WriteByte(' ')
				if prefix != "" {
					sb.WriteString(prefix)
					sb.WriteByte(' ')
				}
				if len(colItems) <= 1 {
					sb.WriteByte('(')
					if len(colItems) == 1 {
						sb.WriteString(colItems[0])
					}
					sb.WriteByte(')')
				} else if contentFirstItem == config.FirstItemInline {
					sb.WriteByte('(')
					for ii, item := range colItems {
						if ii == 0 {
							sb.WriteString(item)
						} else {
							sb.WriteByte(',')
							sb.WriteByte('\n')
							sb.WriteString(contentIndent)
							sb.WriteString(item)
						}
					}
					sb.WriteByte('\n')
					sb.WriteString(clauseIndent)
					sb.WriteByte(')')
				} else {
					sb.WriteString("(\n")
					for ii, item := range colItems {
						sb.WriteString(contentIndent)
						sb.WriteString(item)
						if ii < len(colItems)-1 {
							sb.WriteByte(',')
						}
						sb.WriteByte('\n')
					}
					sb.WriteString(clauseIndent)
					sb.WriteByte(')')
				}
				if closeIdx+1 < conEnd {
					trailing := strings.TrimSpace(inlineContent(sql, tokens, closeIdx+1, conEnd))
					if trailing != "" {
						sb.WriteByte(' ')
						sb.WriteString(trailing)
					}
				}
				continue
			}
		}

		items := splitContent(sql, tokens, kwEnd, conEnd, cl.kind)
		useComma := usesCommaSeparator(cl.kind)

		// reindentItem ensures continuation lines of a multi-line item are indented correctly.
		reindentItem := func(item string) string {
			if !strings.ContainsRune(item, '\n') {
				return item
			}
			firstNewline := strings.IndexByte(item, '\n')
			if firstNewline < 0 || strings.HasPrefix(item[firstNewline+1:], contentIndent) {
				return item
			}
			parts := strings.Split(item, "\n")
			for j := 1; j < len(parts); j++ {
				if strings.TrimSpace(parts[j]) != "" {
					parts[j] = contentIndent + parts[j]
				}
			}
			return strings.Join(parts, "\n")
		}

		if contentFirstItem == config.FirstItemInline {
			// first_item: inline — first item on the keyword line, subsequent items on new lines.
			for ii, item := range items {
				item = reformatCaseExprs(item, contentIndent, cfg.Case, indentUnit, cfg.LineLength)
				item = reindentItem(item)
				if ii == 0 {
					sb.WriteByte(' ')
				} else {
					sb.WriteByte('\n')
					sb.WriteString(contentIndent)
				}
				sb.WriteString(item)
				if useComma && ii < len(items)-1 {
					sb.WriteByte(',')
				}
			}
			continue
		}

		for ii, item := range items {
			sb.WriteByte('\n')
			sb.WriteString(contentIndent)
			item = reformatCaseExprs(item, contentIndent, cfg.Case, indentUnit, cfg.LineLength)
			// When an item spans multiple lines and its continuation lines are NOT
			// already indented to contentIndent level (e.g. case.break=preserve keeps
			// WHEN/END at zero indent, or the source had the expression broken across
			// lines), add contentIndent to each continuation line so that reindentLines
			// (applied by applyLayoutToSQL) produces the correct absolute indentation
			// for the whole item without disrupting the relative structure.
			item = reindentItem(item)
			sb.WriteString(item)
			if useComma && ii < len(items)-1 {
				sb.WriteByte(',')
			}
		}
	}

	if stmt.semiIdx >= 0 {
		sb.WriteString(tokens[stmt.semiIdx].text)
	}

	return sb.String()
}

func contentBreakAlign(kind clauseKind, cfg *config.LayoutConfig) (config.BreakMode, config.AlignMode, config.FirstItemMode) {
	c := &cfg.Content
	effectiveFirstItem := func(r config.FirstItemMode) config.FirstItemMode {
		if r != "" {
			return r
		}
		return c.FirstItem
	}
	get := func(r config.ContentRule) (config.BreakMode, config.AlignMode, config.FirstItemMode) {
		return effectiveBreak(r.Break, c.Break), effectiveAlign(r.Align, c.Align), effectiveFirstItem(r.FirstItem)
	}
	switch kind {
	case clauseSelect, clausePerform:
		return get(c.SelectList)
	case clauseWhere:
		return get(c.WhereConds)
	case clauseHaving:
		return get(c.HavingConds)
	case clauseJoin:
		return get(c.JoinOn)
	case clauseGroupBy:
		return get(c.GroupList)
	case clauseOrderBy:
		return get(c.OrderList)
	case clauseSet:
		return get(c.SetList)
	case clauseInsert:
		return get(c.InsertColumns)
	case clauseValues:
		return get(c.ValuesList)
	case clauseReturning:
		return get(c.ReturningList)
	case clauseWith:
		return get(c.WithList)
	default:
		return c.Break, c.Align, c.FirstItem
	}
}

func splitContent(sql string, tokens []rawTok, from, to int, kind clauseKind) []string {
	switch kind {
	case clauseWhere, clauseHaving:
		return splitAtAndOr(sql, tokens, from, to)
	case clauseJoin:
		onIdx := from
		for i := from; i < to; i++ {
			if strings.ToLower(tokens[i].text) == "on" {
				onIdx = i + 1
				break
			}
		}
		prefix := ""
		if onIdx > from {
			prefix = strings.TrimSpace(originalText(sql, tokens, from, onIdx))
		}
		conds := splitAtAndOr(sql, tokens, onIdx, to)
		if prefix != "" && len(conds) > 0 {
			conds[0] = prefix + " " + conds[0]
		}
		return conds
	case clauseValues:
		return splitValues(sql, tokens, from, to)
	default:
		return splitAtComma(sql, tokens, from, to)
	}
}

func splitAtComma(sql string, tokens []rawTok, from, to int) []string {
	var items []string
	depth := 0
	itemStart := from
	for i := from; i < to; i++ {
		t := tokens[i].text
		if t == "(" || t == "[" {
			depth++
		} else if t == ")" || t == "]" {
			if depth > 0 {
				depth--
			}
		} else if t == "," && depth == 0 {
			item := strings.TrimSpace(originalText(sql, tokens, itemStart, i))
			if item != "" {
				items = append(items, item)
			}
			itemStart = i + 1
		}
	}
	item := strings.TrimSpace(originalText(sql, tokens, itemStart, to))
	if item != "" {
		items = append(items, item)
	}
	return items
}

func splitAtAndOr(sql string, tokens []rawTok, from, to int) []string {
	var items []string
	parenDepth := 0
	caseDepth := 0
	itemStart := from
	for i := from; i < to; i++ {
		t := tokens[i].text
		lower := strings.ToLower(t)
		if t == "(" || t == "[" {
			parenDepth++
		} else if t == ")" || t == "]" {
			if parenDepth > 0 {
				parenDepth--
			}
		} else if lower == "case" {
			caseDepth++
		} else if lower == "end" && caseDepth > 0 {
			caseDepth--
		} else if (lower == "and" || lower == "or") && parenDepth == 0 && caseDepth == 0 {
			item := strings.TrimSpace(originalText(sql, tokens, itemStart, i))
			if item != "" {
				items = append(items, item)
			}
			itemStart = i
		}
	}
	item := strings.TrimSpace(originalText(sql, tokens, itemStart, to))
	if item != "" {
		items = append(items, item)
	}
	return items
}

func splitValues(sql string, tokens []rawTok, from, to int) []string {
	var items []string
	depth := 0
	itemStart := from
	for i := from; i < to; i++ {
		t := tokens[i].text
		if t == "(" || t == "[" {
			depth++
		} else if t == ")" || t == "]" {
			if depth > 0 {
				depth--
			}
			if depth == 0 {
				item := strings.TrimSpace(originalText(sql, tokens, itemStart, i+1))
				if item != "" {
					items = append(items, item)
				}
				if i+1 < to && tokens[i+1].text == "," {
					i++
				}
				itemStart = i + 1
			}
		}
	}
	item := strings.TrimSpace(originalText(sql, tokens, itemStart, to))
	if item != "" {
		items = append(items, item)
	}
	return items
}

func usesCommaSeparator(kind clauseKind) bool {
	switch kind {
	case clauseWhere, clauseHaving, clauseJoin:
		return false
	default:
		return true
	}
}

// ── SQL CASE expression reformatter ──────────────────────────────────────────

// reformatCaseExprs rewrites CASE...END expressions at depth 0 in item.
// baseIndent is the leading whitespace of the line where item starts.
func reformatCaseExprs(item string, baseIndent string, cfg config.SQLCaseCfg, indentUnit string, lineLen int) string {
	if cfg.Break == config.BreakPreserve || cfg.Break == "" {
		return item
	}

	scanResult, err := pg_query.Scan(item)
	if err != nil || len(scanResult.Tokens) == 0 {
		return item
	}

	toks := make([]rawTok, len(scanResult.Tokens))
	for i, t := range scanResult.Tokens {
		toks[i] = rawTok{
			start: int(t.Start),
			end:   int(t.End),
			text:  item[t.Start:t.End],
		}
	}

	var sb strings.Builder
	pos := 0
	parenDepth := 0

	for i := 0; i < len(toks); i++ {
		tok := toks[i]
		lower := strings.ToLower(tok.text)

		if tok.text == "(" || tok.text == "[" {
			parenDepth++
			continue
		}
		if tok.text == ")" || tok.text == "]" {
			if parenDepth > 0 {
				parenDepth--
			}
			continue
		}

		if lower != "case" || parenDepth != 0 {
			continue
		}

		endIdx := findCaseEnd(toks, i)
		if endIdx < 0 {
			break
		}

		// Measure flat length: count chars between token texts joined by spaces.
		flatLen := 0
		for j := i; j <= endIdx; j++ {
			if j > i {
				flatLen++
			}
			flatLen += len(toks[j].text)
		}

		doExpand := shouldBreak(flatLen, cfg.Break, lineLen)

		sb.WriteString(item[pos:tok.start])
		if doExpand {
			sb.WriteString(expandCaseExpr(item, toks, i, endIdx, baseIndent, indentUnit, cfg.Indent))
		} else {
			sb.WriteString(flattenCaseExpr(toks, i, endIdx))
		}
		pos = toks[endIdx].end
		i = endIdx
	}

	sb.WriteString(item[pos:])
	return sb.String()
}

// findCaseEnd returns the index of the END token that closes the CASE at caseIdx.
func findCaseEnd(toks []rawTok, caseIdx int) int {
	depth := 1
	for i := caseIdx + 1; i < len(toks); i++ {
		lower := strings.ToLower(toks[i].text)
		if lower == "case" {
			depth++
		} else if lower == "end" {
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// flattenCaseExpr returns a single-line representation of CASE toks[caseIdx..endIdx].
func flattenCaseExpr(toks []rawTok, caseIdx, endIdx int) string {
	parts := make([]string, 0, endIdx-caseIdx+1)
	for i := caseIdx; i <= endIdx; i++ {
		parts = append(parts, toks[i].text)
	}
	return strings.Join(parts, " ")
}

// expandCaseExpr returns a multi-line representation of CASE toks[caseIdx..endIdx].
// The WHEN/ELSE lines are indented by one indentUnit relative to the CASE line.
// END is placed at baseIndent level.
func expandCaseExpr(item string, toks []rawTok, caseIdx, endIdx int, baseIndent, indentUnit string, indentMode config.BodyIndentMode) string {
	// Compute the indent unit to use for WHEN/ELSE.
	whenIndent := baseIndent + indentUnit
	if indentMode == config.BodyIndentNone {
		whenIndent = baseIndent
	}

	var sb strings.Builder
	sb.WriteString(toks[caseIdx].text) // CASE

	// Find first WHEN at depth-0-relative-to-CASE to detect simple vs searched.
	caseDepth := 0
	firstWhen := -1
	for i := caseIdx + 1; i < endIdx; i++ {
		lower := strings.ToLower(toks[i].text)
		t := toks[i].text
		if t == "(" || t == "[" {
			caseDepth++
		} else if t == ")" || t == "]" {
			if caseDepth > 0 {
				caseDepth--
			}
		} else if lower == "case" {
			caseDepth++
		} else if lower == "end" && caseDepth > 0 {
			caseDepth--
		} else if lower == "when" && caseDepth == 0 {
			firstWhen = i
			break
		}
	}

	if firstWhen < 0 {
		// Malformed or empty CASE — return original text.
		return originalText(item, toks, caseIdx, endIdx+1)
	}

	// Simple CASE: there is content between CASE and first WHEN.
	if firstWhen > caseIdx+1 {
		expr := strings.TrimSpace(originalText(item, toks, caseIdx+1, firstWhen))
		if expr != "" {
			sb.WriteByte(' ')
			sb.WriteString(expr)
		}
	}

	// Walk the remaining tokens to collect WHEN/ELSE clauses.
	i := firstWhen
	for i < endIdx {
		lower := strings.ToLower(toks[i].text)
		t := toks[i].text

		if t == "(" || t == "[" {
			caseDepth++
			i++
			continue
		}
		if t == ")" || t == "]" {
			if caseDepth > 0 {
				caseDepth--
			}
			i++
			continue
		}
		if lower == "case" {
			caseDepth++
			i++
			continue
		}
		if lower == "end" && caseDepth > 0 {
			caseDepth--
			i++
			continue
		}

		if caseDepth != 0 {
			i++
			continue
		}

		if lower == "when" {
			// Find THEN at case-depth 0.
			thenIdx := findCaseKeyword(toks, i+1, endIdx, "then")
			if thenIdx < 0 {
				i++
				continue
			}
			// Find boundary of the result (next WHEN/ELSE/END at case-depth 0).
			nextIdx := findCaseKeywordAny(toks, thenIdx+1, endIdx)

			cond := strings.TrimSpace(originalText(item, toks, i+1, thenIdx))
			body := strings.TrimSpace(originalText(item, toks, thenIdx+1, nextIdx))

			sb.WriteByte('\n')
			sb.WriteString(whenIndent)
			sb.WriteString(strings.ToUpper(toks[i].text[:1]))
			sb.WriteString(toks[i].text[1:]) // preserve casing of WHEN
			sb.WriteByte(' ')
			sb.WriteString(cond)
			sb.WriteByte(' ')
			sb.WriteString(strings.ToUpper(toks[thenIdx].text[:1]))
			sb.WriteString(toks[thenIdx].text[1:]) // THEN
			sb.WriteByte(' ')
			sb.WriteString(body)

			i = nextIdx
			continue
		}

		if lower == "else" {
			body := strings.TrimSpace(originalText(item, toks, i+1, endIdx))
			sb.WriteByte('\n')
			sb.WriteString(whenIndent)
			sb.WriteString(strings.ToUpper(toks[i].text[:1]))
			sb.WriteString(toks[i].text[1:]) // ELSE
			sb.WriteByte(' ')
			sb.WriteString(body)
			i = endIdx
			continue
		}

		i++
	}

	sb.WriteByte('\n')
	sb.WriteString(baseIndent)
	sb.WriteString(strings.ToUpper(toks[endIdx].text[:1]))
	sb.WriteString(toks[endIdx].text[1:]) // END

	return sb.String()
}

// findCaseKeyword returns the index of the next occurrence of keyword at case-depth 0,
// searching toks[from..to). Returns -1 if not found.
func findCaseKeyword(toks []rawTok, from, to int, keyword string) int {
	depth := 0
	for i := from; i < to; i++ {
		lower := strings.ToLower(toks[i].text)
		t := toks[i].text
		if t == "(" || t == "[" {
			depth++
		} else if t == ")" || t == "]" {
			if depth > 0 {
				depth--
			}
		} else if lower == "case" {
			depth++
		} else if lower == "end" && depth > 0 {
			depth--
		} else if lower == keyword && depth == 0 {
			return i
		}
	}
	return -1
}

// findCaseKeywordAny returns the index of the next WHEN, ELSE, or END at case-depth 0,
// searching toks[from..to). Returns to if not found.
func findCaseKeywordAny(toks []rawTok, from, to int) int {
	depth := 0
	for i := from; i < to; i++ {
		lower := strings.ToLower(toks[i].text)
		t := toks[i].text
		if t == "(" || t == "[" {
			depth++
		} else if t == ")" || t == "]" {
			if depth > 0 {
				depth--
			}
		} else if lower == "case" {
			depth++
		} else if lower == "end" && depth > 0 {
			depth--
		} else if depth == 0 && (lower == "when" || lower == "else" || lower == "end") {
			return i
		}
	}
	return to
}

func normalizeIndent(s string, cfg *config.LayoutConfig) string {
	lines := strings.Split(s, "\n")
	dqDelim := "" // current dollar-quote delimiter; non-empty when inside a dollar-quoted body
	for i, line := range lines {
		inDollarBody := dqDelim != ""
		if dqDelim != "" {
			if idx := strings.Index(line, dqDelim); idx >= 0 {
				// Closing delimiter found; check the rest of the line for a new opening.
				_, dqDelim = parenScanLine(line[idx+len(dqDelim):])
			}
		} else {
			_, dqDelim = parenScanLine(line)
		}

		// Inside a dollar-quoted body, skip normalization for tab-leading lines.
		// These lines were placed by applyLayoutPerStatement's reindentLines and are
		// already in the correct relative position. Re-normalizing them would change
		// the byte sequence of "baseIndent+contentIndent" in a way that breaks
		// stripLeadingIndent on subsequent passes (non-idempotent).
		// Lines with pure space indentation (e.g. from a 3-space-indented source body)
		// are still normalized: they haven't been through reindentLines.
		if inDollarBody && len(line) > 0 && line[0] == '\t' {
			continue
		}

		j := 0
		for j < len(line) && (line[j] == ' ' || line[j] == '\t') {
			j++
		}
		leading := line[:j]
		rest := line[j:]
		if leading == "" {
			continue
		}

		var newLeading string
		if cfg.Indent.Type == config.IndentTypeSpaces {
			newLeading = strings.ReplaceAll(leading, "\t", strings.Repeat(" ", cfg.Indent.Size))
		} else {
			size := cfg.Indent.Size
			if size <= 0 {
				size = 1
			}
			spaceCount := 0
			tabCount := 0
			for _, ch := range leading {
				if ch == '\t' {
					tabCount++
				} else if ch == ' ' {
					spaceCount++
				}
			}
			totalSpaces := tabCount*size + spaceCount
			tabs := totalSpaces / size
			remainder := totalSpaces % size
			switch cfg.Indent.Remainder {
			case config.IndentRemainderAdd:
				if remainder > 0 {
					tabs++
					remainder = 0
				}
			case config.IndentRemainderRemove:
				remainder = 0
			case config.IndentRemainderRound:
				// Standard "round half up": round up when remainder >= half the size.
				// Use remainder*2 >= size to avoid integer-division truncation
				// (e.g. size=3: 3/2=1 would round up for ANY remainder, but 1*2=2 < 3 → truncate).
				if remainder*2 >= size {
					tabs++
				}
				remainder = 0
			}
			newLeading = strings.Repeat("\t", tabs) + strings.Repeat(" ", remainder)
		}
		lines[i] = newLeading + rest
	}
	return strings.Join(lines, "\n")
}

// applyParenIndent re-indents content inside multi-line parenthesised blocks.
// It replaces the leading whitespace of each non-skipped line with exactly
// depth * indentUnit, where depth is the number of unclosed parentheses at
// the start of that line. Lines inside dollar-quoted strings are left unchanged.
func applyParenIndent(text, indentUnit string, mode config.ParenIndentMode, closeFirst config.ParenCloseMode) string {
	lines := strings.Split(text, "\n")
	n := len(lines)
	if n == 0 {
		return text
	}

	depths := make([]int, n)
	skip := make([]bool, n)

	depth := 0
	dqDelim := "" // current dollar-quote delimiter, or "" if not in one

	for i, line := range lines {
		depths[i] = depth

		if dqDelim != "" {
			skip[i] = true
			// Look for the closing delimiter on this line.
			if idx := strings.Index(line, dqDelim); idx >= 0 {
				rest := line[idx+len(dqDelim):]
				d, newDelim := parenScanLine(rest)
				depth += d
				if depth < 0 {
					depth = 0
				}
				dqDelim = newDelim
			}
			continue
		}

		d, newDelim := parenScanLine(line)
		depth += d
		if depth < 0 {
			depth = 0
		}
		dqDelim = newDelim
	}

	result := make([]string, n)
	for i, line := range lines {
		if skip[i] {
			result[i] = line
			continue
		}

		stripped := strings.TrimLeft(line, " \t")
		if stripped == "" {
			result[i] = line // preserve blank lines as-is
			continue
		}

		d := depths[i]
		// For closing-paren-first lines: dedent before the paren.
		if strings.HasPrefix(stripped, ")") && closeFirst != config.ParenCloseAfter {
			d = max(0, d-1)
		}

		switch mode {
		case config.ParenIndentIndent:
			if d == 0 {
				result[i] = line // preserve clause-level indentation added by rebuildStatement
			} else {
				result[i] = strings.Repeat(indentUnit, d) + stripped
			}
		case config.ParenIndentNone:
			result[i] = stripped
		default:
			result[i] = line
		}
	}

	return strings.Join(result, "\n")
}

// parenScanLine counts the net parentheses on a single line, respecting
// single-quoted strings, double-quoted identifiers, line comments (--), and
// block comments (/* … */). Returns (netParens, dollarQuoteDelimiter).
// dollarQuoteDelimiter is non-empty when the line opened a dollar-quoted block
// that was not closed on the same line.
func parenScanLine(line string) (int, string) {
	net := 0
	i := 0
	for i < len(line) {
		ch := line[i]

		// Line comment: stop counting.
		if ch == '-' && i+1 < len(line) && line[i+1] == '-' {
			break
		}

		// Block comment: skip to closing */.
		if ch == '/' && i+1 < len(line) && line[i+1] == '*' {
			end := strings.Index(line[i+2:], "*/")
			if end < 0 {
				break // comment runs to end of line (or next line)
			}
			i += 2 + end + 2
			continue
		}

		// Single-quoted string (SQL uses '' for escape, not \').
		if ch == '\'' {
			i++
			for i < len(line) {
				if line[i] == '\'' {
					if i+1 < len(line) && line[i+1] == '\'' {
						i += 2
						continue
					}
					break
				}
				i++
			}
			i++ // skip closing '
			continue
		}

		// Double-quoted identifier.
		if ch == '"' {
			i++
			for i < len(line) && line[i] != '"' {
				i++
			}
			i++ // skip closing "
			continue
		}

		// Dollar-quoted string.
		if ch == '$' {
			if delim, dlen := parenParseDollarDelim(line, i); dlen > 0 {
				rest := line[i+dlen:]
				closeIdx := strings.Index(rest, delim)
				if closeIdx >= 0 {
					// Opened and closed on same line — skip body.
					i += dlen + closeIdx + len(delim)
					continue
				}
				// Not closed on this line — signal to caller.
				return net, delim
			}
		}

		switch ch {
		case '(':
			net++
		case ')':
			net--
		}
		i++
	}
	return net, ""
}

// findInsertColumnParen returns the indices of the opening and closing parentheses
// of an INSERT column list: INSERT INTO table_name (col1, col2, ...).
// Returns (-1, -1) when no column list is present (e.g. INSERT INTO t SELECT ...).
func findInsertColumnParen(tokens []rawTok, from, to int) (int, int) {
	depth := 0
	for i := from; i < to; i++ {
		t := tokens[i].text
		if t == "(" {
			if depth == 0 {
				// Found the column list opening paren; now find its matching close.
				for j := i + 1; j < to; j++ {
					switch tokens[j].text {
					case "(":
						depth++
					case ")":
						if depth == 0 {
							return i, j
						}
						depth--
					}
				}
				return -1, -1
			}
			depth++
		} else if t == ")" && depth > 0 {
			depth--
		}
	}
	return -1, -1
}

// parenParseDollarDelim tries to parse a dollar-quote delimiter starting at
// position pos in s. Returns (delimiter, length) or ("", 0) on failure.
// Delimiter format: $[tag]$ where [tag] is zero or more letters/digits/underscores.
func parenParseDollarDelim(s string, pos int) (string, int) {
	if pos >= len(s) || s[pos] != '$' {
		return "", 0
	}
	i := pos + 1
	for i < len(s) {
		c := s[i]
		if c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			i++
			continue
		}
		break
	}
	if i >= len(s) || s[i] != '$' {
		return "", 0
	}
	return s[pos : i+1], i + 1 - pos
}
