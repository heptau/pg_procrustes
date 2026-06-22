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
	return clauseBreak == config.BreakPreserve &&
		unionBL == config.UnionBlankLinePreserve &&
		contentBreak == config.BreakPreserve &&
		indentNorm == config.IndentNormalizePreserve &&
		caseBrk == config.BreakPreserve
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

		if !doBreak {
			if ci > 0 {
				// Include gap between previous clause content and this clause keyword.
				prevConEnd := stmt.clauses[ci-1].conEnd
				if prevConEnd > 0 {
					sb.WriteString(gapBetween(sql, tokens, prevConEnd-1, kwStart))
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
			sb.WriteByte('\n')
			sb.WriteString(clauseIndent)
		} else {
			sb.WriteString(clauseIndent)
		}

		kwText := strings.TrimSpace(originalText(sql, tokens, kwStart, kwEnd))
		sb.WriteString(kwText)

		contentBreakMode, contentAlign := contentBreakAlign(cl.kind, cfg)
		contentFlatLen := flatLength(sql, tokens, kwEnd, conEnd)
		doContentBreak := shouldBreak(contentFlatLen, contentBreakMode, cfg.LineLength)

		if kwEnd >= conEnd {
			continue
		}

		if !doContentBreak {
			sb.WriteByte(' ')
			content := strings.TrimSpace(originalText(sql, tokens, kwEnd, conEnd))
			content = reformatCaseExprs(content, clauseIndent, cfg.Case, indentUnit, cfg.LineLength)
			sb.WriteString(content)
			continue
		}

		contentIndent := clauseIndent + indentUnit
		if contentAlign == config.AlignSame {
			contentIndent = clauseIndent
		}

		items := splitContent(sql, tokens, kwEnd, conEnd, cl.kind)
		useComma := usesCommaSeparator(cl.kind)
		for ii, item := range items {
			sb.WriteByte('\n')
			sb.WriteString(contentIndent)
			item = reformatCaseExprs(item, contentIndent, cfg.Case, indentUnit, cfg.LineLength)
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

func contentBreakAlign(kind clauseKind, cfg *config.LayoutConfig) (config.BreakMode, config.AlignMode) {
	c := &cfg.Content
	get := func(r config.ContentRule) (config.BreakMode, config.AlignMode) {
		return effectiveBreak(r.Break, c.Break), effectiveAlign(r.Align, c.Align)
	}
	switch kind {
	case clauseSelect, clausePerform:
		return get(c.SelectList)
	case clauseWhere:
		return get(c.WhereConds)
	case clauseJoin:
		return get(c.JoinOn)
	case clauseGroupBy:
		return get(c.GroupList)
	case clauseOrderBy:
		return get(c.OrderList)
	case clauseSet:
		return get(c.SetList)
	case clauseValues:
		return get(c.ValuesList)
	default:
		return c.Break, c.Align
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
	for i, line := range lines {
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
				if remainder >= size/2 {
					tabs++
					remainder = 0
				} else {
					remainder = 0
				}
			}
			newLeading = strings.Repeat("\t", tabs) + strings.Repeat(" ", remainder)
		}
		lines[i] = newLeading + rest
	}
	return strings.Join(lines, "\n")
}
