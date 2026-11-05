// Package layout implements clause-level SQL layout and indentation for pg_procrustes.
package layout

import (
	"strings"

	pg_query "github.com/pganalyze/pg_query_go/v6"

	"github.com/heptau/pg_procrustes/config"
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
	if sb := cfg.Subquery.Break; sb != "" && sb != config.BreakPreserve {
		return false
	}
	if cfg.ParenIndent.Open == config.ParenOpenOwnLine {
		return false
	}
	parenMode := cfg.ParenIndent.Mode
	if parenMode == "" {
		parenMode = config.ParenIndentPreserve
	}
	// Per-clause / per-section overrides can enable breaking even when the global
	// setting is preserve.
	for _, nr := range cfg.Clauses.ClauseRules() {
		if nr.Rule.Break != "" && nr.Rule.Break != config.BreakPreserve {
			return false
		}
	}
	for _, nr := range cfg.Content.ContentRules() {
		if nr.Rule.Break != "" && nr.Rule.Break != config.BreakPreserve {
			return false
		}
	}
	for _, b := range []config.BreakMode{cfg.DDL.CreateTable.Break, cfg.DDL.AlterTable.Break, cfg.DDL.CreateIndex.Break, cfg.DDL.CreateFunction.Params, cfg.DDL.CreateFunction.Options} {
		if b != "" && b != config.BreakPreserve {
			return false
		}
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
	indentUnit := indentUnitStr(cfg)

	result := applyCore(sql, cfg, indentUnit)

	if cfg.Indent.Normalize == config.IndentNormalizeChange {
		result = normalizeIndent(result, cfg)
	}

	if cfg.ParenIndent.Mode != config.ParenIndentPreserve && cfg.ParenIndent.Mode != "" {
		result = applyParenIndent(result, indentUnit, cfg.ParenIndent.Mode, cfg.ParenIndent.CloseFirst)
	}

	if cfg.ParenIndent.Open == config.ParenOpenOwnLine {
		result = moveOpenParens(result)
	}

	return result, nil
}

// applyCore lays out the statements of sql (clauses, content, subqueries) without the
// whole-text passes (indent normalisation, paren indent), so it can also run on the body
// of a subquery.
func applyCore(sql string, cfg *config.LayoutConfig, indentUnit string) string {
	sql = flattenSubqueries(sql, cfg)

	scanResult, err := pg_query.Scan(sql)
	if err != nil {
		return sql
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
		return sql
	}

	// Preserve the leading gap (whitespace before the first token) and trailing
	// gap (whitespace after the last token) so dollar-quoted body indentation survives.
	firstTokStart := tokens[stmts[0].startTok].start
	leadingGap := sql[:firstTokStart]

	lastStmt := stmts[len(stmts)-1]
	lastTokEnd := stmtEndOffset(tokens, lastStmt, firstTokStart)
	trailingGap := sql[lastTokEnd:]

	var parts []string
	for i, stmt := range stmts {
		// Preserve inter-statement gap (between end of previous stmt and start of this one).
		if i > 0 {
			prevEnd := stmtEndOffset(tokens, stmts[i-1], 0)
			curStart := tokens[stmt.startTok].start
			parts = append(parts, sql[prevEnd:curStart])
		}
		rebuilt := rebuildStatement(sql, tokens, stmt, cfg, indentUnit, 0)
		parts = append(parts, rebuilt)
	}

	result := leadingGap + strings.Join(parts, "") + trailingGap
	return expandSubqueries(result, cfg, indentUnit)
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
	clauseMerge   // MERGE INTO target [AS alias]
	clauseMergeOn // ON join_condition of a MERGE
	clauseWhen    // WHEN [NOT] MATCHED … THEN action
	clauseWindow  // WINDOW name AS (…) of a SELECT
	// clauseWithOption is the trailer of CREATE [MATERIALIZED] VIEW / CREATE TABLE AS:
	// WITH [NO] DATA and WITH [CASCADED | LOCAL] CHECK OPTION.
	clauseWithOption
)

type stmtClause struct {
	kind   clauseKind
	kwTok  int // index of the clause keyword's first token
	kwEnd  int
	conEnd int
}

type statement struct {
	startTok int
	endTok   int // exclusive end of the statement's tokens, before the ';'
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
				stmt.endTok = i
				i++
				break
			}

			if depth == 0 {
				kind, kwLen := detectClause(tokens, i)
				if kind != clauseUnknown && !clauseAllowed(kind, tokens, i, &stmt) {
					kind = clauseUnknown
				}
				if kind != clauseUnknown {
					if len(stmt.clauses) > 0 {
						last := &stmt.clauses[len(stmt.clauses)-1]
						last.conEnd = i
					}
					cl := stmtClause{
						kind:  kind,
						kwTok: i,
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
		if stmt.semiIdx < 0 {
			stmt.endTok = i
		}
		if len(stmt.clauses) > 0 || stmt.semiIdx >= 0 || isDDLStart(tokens, stmt.startTok, stmt.endTok) {
			stmts = append(stmts, stmt)
		}
	}
	return stmts
}

// withOptionLen returns the token count of a WITH [NO] DATA or WITH [CASCADED|LOCAL]
// CHECK OPTION trailer starting at i, or 0 when the WITH starts a CTE list.
func withOptionLen(tokens []rawTok, i int) int {
	lw := func(k int) string {
		if k >= len(tokens) {
			return ""
		}
		return strings.ToLower(tokens[k].text)
	}
	atEnd := func(k int) bool { return k >= len(tokens) || tokens[k].text == ";" }
	switch lw(i + 1) {
	case "no":
		if lw(i+2) == "data" && atEnd(i+3) {
			return 3
		}
	case "data":
		if atEnd(i + 2) {
			return 2
		}
	case "check":
		if lw(i+2) == "option" {
			return 3
		}
	case "local", "cascaded":
		if lw(i+2) == "check" && lw(i+3) == "option" {
			return 4
		}
	}
	return 0
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
		if n := withOptionLen(tokens, i); n > 0 {
			return clauseWithOption, n
		}
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
	case "window":
		return clauseWindow, 1 // only accepted in a query (see clauseAllowed)
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
		return clauseMergeOn, 1 // only accepted inside MERGE (see clauseAllowed)
	case "merge":
		if next() == "into" {
			return clauseMerge, 2
		}
	case "when":
		return clauseWhen, 1 // only accepted inside MERGE (see clauseAllowed)
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
		size = 2
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
	case clauseMerge:
		return get(c.Merge)
	case clauseMergeOn:
		return get(c.MergeOn)
	case clauseWhen:
		return get(c.When)
	case clauseWhere:
		return get(c.Where)
	case clauseGroupBy:
		return get(c.GroupBy)
	case clauseHaving:
		return get(c.Having)
	case clauseWindow:
		return get(c.Window)
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
func inlineContent(sql string, tokens []rawTok, from, to int, parenMode config.ParenSpacingMode) string {
	if from >= to {
		return ""
	}
	var sb strings.Builder
	for i := from; i < to; i++ {
		if i > from {
			gap := gapBetween(sql, tokens, i-1, i)
			if strings.ContainsRune(gap, '\n') {
				// A line comment runs to the end of the line: joining what follows
				// onto it would comment that code out, so keep the line break.
				switch {
				case strings.HasPrefix(tokens[i-1].text, "--"):
					sb.WriteByte('\n')
				case tokens[i].text == ",":
					// a line break before a leading comma joins to "a, b", not "a , b".
				case parenMode == config.ParenSpacingRemove && (tokens[i-1].text == "(" || tokens[i].text == ")"):
					// paren_spacing: remove — no space inside parentheses.
				default:
					sb.WriteByte(' ')
				}
			} else {
				sb.WriteString(gap)
			}
		}
		sb.WriteString(tokens[i].text)
	}
	return sb.String()
}

func rebuildStatement(sql string, tokens []rawTok, stmt statement, cfg *config.LayoutConfig, indentUnit string, depth int) string {
	// CREATE INDEX … WHERE has clauses but is laid out by the DDL code when asked to.
	if len(stmt.clauses) > 0 {
		if first := firstCodeTok(tokens, stmt.startTok, stmt.endTok); first >= 0 &&
			strings.EqualFold(tokens[first].text, "create") && createIndexToken(tokens, stmt, first) >= 0 {
			if out, ok := rebuildDDL(sql, tokens, stmt, cfg, indentUnit, depth); ok {
				return out
			}
		}
	}
	if len(stmt.clauses) == 0 {
		if out, ok := rebuildDDL(sql, tokens, stmt, cfg, indentUnit, depth); ok {
			return out
		}
		start := tokens[stmt.startTok].start
		end := stmtEndOffset(tokens, stmt, tokens[stmt.startTok].end)
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
				// Preserve the source gap before the set operator; without it the
				// keyword is glued to the previous token ("uUNION"). When earlier
				// clauses were broken onto separate lines, keep the operator on its own line.
				if ci > 0 && stmt.clauses[ci-1].conEnd > 0 {
					gap := gapBetween(sql, tokens, stmt.clauses[ci-1].conEnd-1, kwStart)
					if !strings.Contains(gap, "\n") && strings.Contains(sb.String(), "\n") {
						gap = "\n" + baseIndent
					}
					if gap == "" {
						gap = " "
					}
					sb.WriteString(gap)
				}
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
		// align: indent places the other clauses one level deeper than the keyword
		// that opens the statement, which itself stays at the base indent.
		if alignMode == config.AlignIndent && ci > 0 && !opensStatement(cl.kind) {
			clauseIndent = baseIndent + indentUnit
		}

		// A leading SELECT/WITH preceded by a statement prefix (CREATE VIEW v AS,
		// CREATE TABLE t AS, EXPLAIN, …): keep the prefix on its own line and start
		// the query on the next one, instead of gluing "AS SELECT" together.
		prefixed := ci == 0 && (cl.kind == clauseSelect || cl.kind == clauseWith) && hasCodeTokens(tokens, kwStart, cl.kwTok)

		if ci > 0 {
			if prevIsUnionBlankAfter {
				sb.WriteString("\n\n")
			} else {
				sb.WriteByte('\n')
			}
			sb.WriteString(clauseIndent)
		} else if prefixed {
			sb.WriteString(baseIndent)
			sb.WriteString(strings.TrimSpace(originalText(sql, tokens, kwStart, cl.kwTok)))
			sb.WriteByte('\n')
			sb.WriteString(clauseIndent)
		} else {
			sb.WriteString(clauseIndent)
		}

		kwFrom := kwStart
		if prefixed {
			kwFrom = cl.kwTok
		}
		kwText := strings.TrimSpace(originalText(sql, tokens, kwFrom, kwEnd))
		sb.WriteString(kwText)

		contentBreakMode, contentAlign, contentFirstItem := contentBreakAlign(cl.kind, cfg)
		contentAlign = resolveContentAlign(contentAlign, sql, tokens, cl.kwTok, kwEnd, conEnd)
		contentFlatLen := flatLength(sql, tokens, kwEnd, conEnd)
		doContentBreak := shouldBreak(contentFlatLen, contentBreakMode, cfg.LineLength)
		if contentBreakMode == config.BreakAuto && subqueryMode(cfg) == config.BreakAlways && cl.kind != clauseFrom && cl.kind != clauseJoin && hasSubquery(tokens, kwEnd, conEnd) {
			// an item that opens into several lines makes the list read best one item per line
			doContentBreak = true
		}
		if cl.kind == clauseWhen && contentBreakMode == config.BreakPreserve {
			// preserve: the action stays where the input has it (below THEN or on its line)
			if thenIdx := findWhenThen(tokens, kwEnd, conEnd); thenIdx >= 0 {
				doContentBreak = hasNewlineAfter(sql, tokens, thenIdx, conEnd)
			}
		}
		if cl.kind == clauseMerge {
			doContentBreak = false // a single target name reads best on the keyword line
		}

		if kwEnd >= conEnd {
			continue
		}

		if !doContentBreak {
			sb.WriteByte(' ')
			// Use inlineContent instead of originalText: the original may span multiple
			// lines (e.g. ORDER BY items each on their own line), and placing that verbatim
			// on the same line as the keyword would embed raw newlines in the output.
			content := inlineContent(sql, tokens, kwEnd, conEnd, cfg.ParenSpacing)
			content = reformatCaseExprs(content, clauseIndent, cfg.Case, indentUnit, cfg.LineLength)
			sb.WriteString(content)
			continue
		}

		contentIndent := clauseIndent + indentUnit
		if contentAlign == config.AlignSame {
			contentIndent = clauseIndent
		}

		// MERGE WHEN [NOT] MATCHED … THEN action: the action goes on its own indented line.
		if cl.kind == clauseWhen {
			if thenIdx := findWhenThen(tokens, kwEnd, conEnd); thenIdx >= 0 && thenIdx+1 < conEnd {
				head := reformatCaseExprs(inlineContent(sql, tokens, kwEnd, thenIdx+1, cfg.ParenSpacing), clauseIndent, cfg.Case, indentUnit, cfg.LineLength)
				action := reformatCaseExprs(inlineContent(sql, tokens, thenIdx+1, conEnd, cfg.ParenSpacing), contentIndent, cfg.Case, indentUnit, cfg.LineLength)
				sb.WriteByte(' ')
				sb.WriteString(head)
				sb.WriteByte('\n')
				sb.WriteString(contentIndent)
				sb.WriteString(action)
				continue
			}
		}

		// Special case: INSERT INTO table_name (col1, col2, ...) column list.
		if cl.kind == clauseInsert {
			openIdx, closeIdx := findInsertColumnParen(tokens, kwEnd, conEnd)
			if openIdx >= 0 {
				prefix := strings.TrimSpace(inlineContent(sql, tokens, kwEnd, openIdx, cfg.ParenSpacing))
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
					colStart := resolveCommaStart(cfg.Content.CommaPosition, sql, tokens, openIdx+1, closeIdx)
					for ii, item := range colItems {
						if ii == 0 {
							if !colStart && ii < len(colItems)-1 {
								item = withTrailingComma(item)
							}
							sb.WriteString(item)
						} else {
							sb.WriteByte('\n')
							if colStart {
								sb.WriteString(leadCommaIndent(contentIndent) + ", ")
							} else {
								sb.WriteString(contentIndent)
							}
							if !colStart && ii < len(colItems)-1 {
								item = withTrailingComma(item)
							}
							sb.WriteString(item)
						}
					}
					sb.WriteByte('\n')
					sb.WriteString(clauseIndent)
					sb.WriteByte(')')
				} else {
					sb.WriteString("(\n")
					colStart := resolveCommaStart(cfg.Content.CommaPosition, sql, tokens, openIdx+1, closeIdx)
					for ii, item := range colItems {
						if colStart && ii > 0 {
							sb.WriteString(leadCommaIndent(contentIndent) + ", ")
						} else {
							sb.WriteString(contentIndent)
						}
						item = strings.ReplaceAll(item, "\n", "\n"+contentIndent)
						if !colStart && ii < len(colItems)-1 {
							item = withTrailingComma(item)
						}
						sb.WriteString(item)
						sb.WriteByte('\n')
					}
					sb.WriteString(clauseIndent)
					sb.WriteByte(')')
				}
				if closeIdx+1 < conEnd {
					trailing := strings.TrimSpace(inlineContent(sql, tokens, closeIdx+1, conEnd, cfg.ParenSpacing))
					if trailing != "" {
						sb.WriteByte(' ')
						sb.WriteString(trailing)
					}
				}
				continue
			}
		}

		joinNewline := cl.kind == clauseJoin && resolveJoinNewline(cfg.Content.JoinOnPosition, sql, tokens, kwEnd, conEnd)
		if joinNewline {
			contentFirstItem = config.FirstItemInline // JOIN t stays on the keyword line
		} else if cl.kind == clauseJoin && cfg.Content.JoinOn.FirstItem == "" && hasJoinCondition(tokens, kwEnd, conEnd) {
			// JOIN t ON a = b: the table and its condition stay on the keyword line
			// unless join_on.first_item asks for the first item on a line of its own.
			contentFirstItem = config.FirstItemInline
		}
		items := splitContent(sql, tokens, kwEnd, conEnd, cl.kind, joinNewline)
		useComma := usesCommaSeparator(cl.kind)
		commaStart := useComma && resolveCommaStart(cfg.Content.CommaPosition, sql, tokens, kwEnd, conEnd)
		if !useComma && resolveOperatorEnd(cfg.Content.LogicalOperatorPosition, sql, tokens, kwEnd, conEnd) {
			items = moveOperatorsToEnd(items)
		}
		if contentFirstItem != config.FirstItemInline {
			switch {
			case cl.kind == clauseSelect && cfg.Content.AlignAliases == config.AlignItemsAlign:
				items = alignItems(items, "as")
			case cl.kind == clauseSelect && cfg.Content.AlignAliases == config.AlignItemsNone:
				items = unpadItems(items, "as")
			case cl.kind == clauseSet && cfg.Content.AlignSet == config.AlignItemsAlign:
				items = alignItems(items, "=")
			case cl.kind == clauseSet && cfg.Content.AlignSet == config.AlignItemsNone:
				items = unpadItems(items, "=")
			}
		}
		// lineIndent is the indent of line ii: with comma_position: start the comma sits
		// two columns to the left so the items themselves stay aligned.
		lineLead := func(ii int) string {
			if !commaStart || ii == 0 {
				return contentIndent
			}
			return leadCommaIndent(contentIndent) + ", "
		}

		// reindentItem ensures continuation lines of a multi-line item are indented correctly.
		reindentItem := func(item string) string {
			if !strings.ContainsRune(item, '\n') {
				return item
			}
			firstNewline := strings.IndexByte(item, '\n')
			if firstNewline < 0 {
				return item
			}
			// Judge by the first non-blank continuation line; a blank line right after
			// the first newline must not make every pass add another level of indent.
			for _, ln := range strings.Split(item[firstNewline+1:], "\n") {
				if strings.TrimSpace(ln) == "" {
					continue
				}
				if strings.HasPrefix(ln, contentIndent) {
					return item
				}
				break
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
				if cfg.Content.WrapExpressions == config.WrapWrap {
					item = wrapLongItem(item, contentIndent, indentUnit, cfg.LineLength-1)
				}
				item = reindentItem(item)
				if ii == 0 {
					sb.WriteByte(' ')
				} else {
					if commaStart {
						item = writeLeadingComments(&sb, item, contentIndent)
					}
					sb.WriteByte('\n')
					sb.WriteString(lineLead(ii))
				}
				if useComma && !commaStart && ii < len(items)-1 {
					item = withTrailingComma(item)
				}
				sb.WriteString(item)
			}
			continue
		}

		for ii, item := range items {
			if commaStart && ii > 0 {
				item = writeLeadingComments(&sb, item, contentIndent)
			}
			sb.WriteByte('\n')
			sb.WriteString(lineLead(ii))
			item = reformatCaseExprs(item, contentIndent, cfg.Case, indentUnit, cfg.LineLength)
			if cfg.Content.WrapExpressions == config.WrapWrap {
				item = wrapLongItem(item, contentIndent, indentUnit, cfg.LineLength-1)
			}
			// When an item spans multiple lines and its continuation lines are NOT
			// already indented to contentIndent level (e.g. case.break=preserve keeps
			// WHEN/END at zero indent, or the source had the expression broken across
			// lines), add contentIndent to each continuation line so that reindentLines
			// (applied by applyLayoutToSQL) produces the correct absolute indentation
			// for the whole item without disrupting the relative structure.
			item = reindentItem(item)
			if useComma && !commaStart && ii < len(items)-1 {
				item = withTrailingComma(item)
			}
			sb.WriteString(item)
		}
	}

	if stmt.semiIdx >= 0 {
		// A ';' written after a trailing -- comment would end up inside the comment.
		if last := stmt.clauses[len(stmt.clauses)-1].conEnd - 1; last >= 0 && last < len(tokens) &&
			strings.HasPrefix(tokens[last].text, "--") {
			sb.WriteByte('\n')
		}
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
	case clauseMergeOn:
		return get(c.MergeOn)
	case clauseWhen:
		// WHEN … THEN action stays on one line unless when_actions asks otherwise;
		// deliberately not inherited from content.break.
		brk, align, first := get(c.WhenActions)
		if c.WhenActions.Break == "" {
			brk = config.BreakPreserve
		}
		return brk, align, first
	case clauseGroupBy:
		return get(c.GroupList)
	case clauseOrderBy:
		return get(c.OrderList)
	case clauseWindow:
		return get(c.WindowList)
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

func splitContent(sql string, tokens []rawTok, from, to int, kind clauseKind, joinOnNewline bool) []string {
	switch kind {
	case clauseWhere, clauseHaving, clauseMergeOn:
		return splitAtAndOr(sql, tokens, from, to)
	case clauseJoin:
		onTok := findTopLevelOn(tokens, from, to)
		onIdx := from
		if onTok >= 0 {
			onIdx = onTok + 1
		}
		prefix := ""
		if onIdx > from {
			prefix = strings.TrimSpace(originalText(sql, tokens, from, onIdx))
			// inline: ON follows the table on the same line, whatever the input did
			// (a line comment right before ON must keep its newline, or it would swallow ON)
			if onTok > from && !strings.HasPrefix(tokens[onTok-1].text, "--") {
				prefix = strings.TrimSpace(originalText(sql, tokens, from, onTok)) + " " + tokens[onTok].text
			}
		}
		conds := splitAtAndOr(sql, tokens, onIdx, to)
		if joinOnNewline && onTok >= 0 && len(conds) > 0 {
			// JOIN t / ON a = b / AND c — the table stays on the keyword line.
			table := strings.TrimSpace(originalText(sql, tokens, from, onTok))
			conds[0] = tokens[onTok].text + " " + conds[0]
			return append([]string{table}, conds...)
		}
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
			itemEnd := i
			next := i + 1
			// A line comment on the same line as the comma belongs to the item
			// before it ("a, -- note"); keep it there so it is stable across passes.
			if i+1 < to && strings.HasPrefix(tokens[i+1].text, "--") &&
				!strings.Contains(sql[tokens[i].end:tokens[i+1].start], "\n") {
				itemEnd = i + 2
				next = i + 2
			}
			item := trimLeadingCommentIndent(strings.TrimSpace(originalText(sql, tokens, itemStart, itemEnd)))
			if strings.HasPrefix(tokens[itemEnd-1].text, "--") && itemEnd != i {
				// originalText included the comma token; drop it from the item text
				item = trimLeadingCommentIndent(strings.TrimSpace(originalText(sql, tokens, itemStart, i))) + " " + tokens[itemEnd-1].text
			}
			if item != "" {
				items = append(items, item)
			}
			itemStart = next
		}
	}
	item := trimLeadingCommentIndent(strings.TrimSpace(originalText(sql, tokens, itemStart, to)))
	if item != "" {
		items = append(items, item)
	}
	return items
}

func splitAtAndOr(sql string, tokens []rawTok, from, to int) []string {
	var items []string
	parenDepth := 0
	caseDepth := 0
	between := 0 // pending BETWEEN … AND
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
		} else if lower == "between" && parenDepth == 0 && caseDepth == 0 {
			between++
		} else if lower == "and" && between > 0 && parenDepth == 0 && caseDepth == 0 {
			between-- // the AND of "x BETWEEN a AND b" is part of the predicate
		} else if (lower == "and" || lower == "or") && parenDepth == 0 && caseDepth == 0 {
			if item := strings.TrimSpace(originalText(sql, tokens, itemStart, i)); item != "" {
				items = append(items, joinLeadingOperator(sql, tokens, itemStart, i))
			}
			itemStart = i
		}
	}
	if item := strings.TrimSpace(originalText(sql, tokens, itemStart, to)); item != "" {
		items = append(items, joinLeadingOperator(sql, tokens, itemStart, to))
	}
	return items
}

// joinLeadingOperator returns tokens[from:to] as an item. When the item starts with
// AND/OR and a line break separates it from its condition ("AND\n x = 1", as written in
// the operator-at-end style) the break is replaced by a space.
func joinLeadingOperator(sql string, tokens []rawTok, from, to int) string {
	item := strings.TrimSpace(originalText(sql, tokens, from, to))
	if from+1 < to {
		if l := strings.ToLower(tokens[from].text); (l == "and" || l == "or") && !strings.HasPrefix(tokens[from+1].text, "--") &&
			strings.Contains(gapBetween(sql, tokens, from, from+1), "\n") {
			item = tokens[from].text + " " + strings.TrimSpace(originalText(sql, tokens, from+1, to))
		}
	}
	return item
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
				// Row text ends at the closing paren. Then consume, in source order: the
				// separating comma, a line comment on the same line (it belongs to the row),
				// and a comma that follows that comment (leading-comma style).
				j := i
				sawComma := false
				if j+1 < to && tokens[j+1].text == "," {
					j++
					sawComma = true
				}
				comment := ""
				if j+1 < to && strings.HasPrefix(tokens[j+1].text, "--") &&
					!strings.Contains(sql[tokens[j].end:tokens[j+1].start], "\n") {
					j++
					comment = tokens[j].text
					if !sawComma && j+1 < to && tokens[j+1].text == "," {
						j++
					}
				}
				item := strings.TrimSpace(originalText(sql, tokens, itemStart, i+1))
				if comment != "" {
					item += " " + comment
				}
				if item != "" {
					items = append(items, item)
				}
				i = j
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
	case clauseWhere, clauseHaving, clauseJoin, clauseMergeOn:
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
		} else if caseHasLineComment(toks, i, endIdx) {
			// Flattening would put everything after a -- comment inside the comment.
			sb.WriteString(item[tok.start:toks[endIdx].end])
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
			sb.WriteString(toks[i].text) // preserve casing of WHEN
			sb.WriteByte(' ')
			sb.WriteString(cond)
			sb.WriteByte(' ')
			sb.WriteString(toks[thenIdx].text) // THEN
			sb.WriteByte(' ')
			sb.WriteString(body)

			i = nextIdx
			continue
		}

		if lower == "else" {
			body := strings.TrimSpace(originalText(item, toks, i+1, endIdx))
			sb.WriteByte('\n')
			sb.WriteString(whenIndent)
			sb.WriteString(toks[i].text) // ELSE
			sb.WriteByte(' ')
			sb.WriteString(body)
			i = endIdx
			continue
		}

		i++
	}

	sb.WriteByte('\n')
	sb.WriteString(baseIndent)
	sb.WriteString(toks[endIdx].text) // END

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

// withTrailingComma appends a comma to item. When the item ends with a line
// comment (-- ...), the comma is inserted before the comment; appending it
// after would put the comma inside the comment and silently change the SQL.
func withTrailingComma(item string) string { return appendBeforeComment(item, ",") }

// appendBeforeComment appends suffix (",", " AND", …) to item. When the item ends with
// a line comment the suffix goes before the comment, otherwise it would be swallowed.
func appendBeforeComment(item, suffix string) string {
	res, err := pg_query.Scan(item)
	if err == nil && len(res.Tokens) > 0 {
		last := res.Tokens[len(res.Tokens)-1]
		start, end := int(last.Start), int(last.End)
		if start >= 0 && end <= len(item) && strings.HasPrefix(item[start:end], "--") {
			before := strings.TrimRight(item[:start], " \t")
			return before + suffix + " " + item[start:]
		}
	}
	return item + suffix
}

// splitLeadingOperator splits "AND x = 1" into ("AND", "x = 1"); ok is false when item
// does not start with AND/OR followed by more text.
func splitLeadingOperator(item string) (op, rest string, ok bool) {
	for _, w := range []string{"and", "or"} {
		if len(item) > len(w) && strings.EqualFold(item[:len(w)], w) && (item[len(w)] == ' ' || item[len(w)] == '\t' || item[len(w)] == '\n') {
			return item[:len(w)], strings.TrimLeft(item[len(w):], " \t\n"), true
		}
	}
	return "", item, false
}

// moveOperatorsToEnd turns ["a = 1", "AND b = 2", "OR c"] into ["a = 1 AND", "b = 2 OR", "c"].
func moveOperatorsToEnd(items []string) []string {
	out := append([]string(nil), items...)
	for i := 1; i < len(out); i++ {
		if op, rest, ok := splitLeadingOperator(out[i]); ok {
			out[i-1] = appendBeforeComment(out[i-1], " "+op)
			// A comment right after the operator ("AND -- why") ends the line the operator
			// now ends, so it travels with it.
			if strings.HasPrefix(rest, "--") {
				comment, remaining := rest, ""
				if nl := strings.IndexByte(rest, '\n'); nl >= 0 {
					comment, remaining = rest[:nl], strings.TrimLeft(rest[nl+1:], " \t\n")
				}
				out[i-1] += " " + strings.TrimRight(comment, " \t\r")
				rest = remaining
			}
			out[i] = rest
		}
	}
	return out
}

// findTopLevelOn returns the index of the first ON outside parentheses in tokens[from:to], or -1.
// hasJoinCondition reports whether the JOIN content has a top-level ON or USING.
func hasJoinCondition(tokens []rawTok, from, to int) bool {
	depth := 0
	for i := from; i < to; i++ {
		switch t := strings.ToLower(tokens[i].text); {
		case t == "(" || t == "[":
			depth++
		case t == ")" || t == "]":
			if depth > 0 {
				depth--
			}
		case depth == 0 && (t == "on" || t == "using"):
			return true
		}
	}
	return false
}

func findTopLevelOn(tokens []rawTok, from, to int) int {
	depth := 0
	for i := from; i < to; i++ {
		switch t := tokens[i].text; {
		case t == "(" || t == "[":
			depth++
		case t == ")" || t == "]":
			if depth > 0 {
				depth--
			}
		case depth == 0 && strings.EqualFold(t, "on"):
			return i
		}
	}
	return -1
}

// trimLeadingCommentIndent removes the common source indentation of the lines that
// follow a leading line comment, so that the item is re-indented consistently
// (and identically on every pass) while relative indentation is kept.
func trimLeadingCommentIndent(item string) string {
	if !strings.HasPrefix(item, "--") {
		return item
	}
	lines := strings.Split(item, "\n")
	minIndent := -1
	for _, ln := range lines[1:] {
		if strings.TrimSpace(ln) == "" {
			continue
		}
		n := len(ln) - len(strings.TrimLeft(ln, " \t"))
		if minIndent < 0 || n < minIndent {
			minIndent = n
		}
	}
	if minIndent <= 0 {
		return item
	}
	for i := 1; i < len(lines); i++ {
		if len(lines[i]) >= minIndent {
			lines[i] = lines[i][minIndent:]
		}
	}
	return strings.Join(lines, "\n")
}

// opensStatement reports whether a clause keyword begins a statement (SELECT,
// WITH, INSERT INTO, UPDATE, DELETE FROM, PERFORM) rather than continuing one.
func opensStatement(k clauseKind) bool {
	switch k {
	case clauseWith, clauseSelect, clausePerform, clauseInsert, clauseUpdate, clauseDelete, clauseMerge:
		return true
	}
	return false
}

// hasCodeTokens reports whether tokens[from:to] contain anything besides comments.
func hasCodeTokens(tokens []rawTok, from, to int) bool {
	for i := from; i < to && i < len(tokens); i++ {
		t := tokens[i].text
		if !strings.HasPrefix(t, "--") && !strings.HasPrefix(t, "/*") {
			return true
		}
	}
	return false
}

// clauseAllowed rejects clause keywords that appear in a different role than the
// query clause they are named after: SET in ALTER TABLE … SET / ALTER ROLE … SET,
// USING in CREATE INDEX … USING gin or ALTER COLUMN … TYPE … USING, WITH (…) storage
// parameters, and UPDATE in FOR UPDATE, ON UPDATE CASCADE, DO UPDATE and trigger
// events (BEFORE INSERT OR UPDATE). Those stay part of the surrounding clause.
func clauseAllowed(kind clauseKind, tokens []rawTok, i int, stmt *statement) bool {
	prev := ""
	if i > stmt.startTok {
		for k := i - 1; k >= stmt.startTok; k-- {
			if t := tokens[k].text; !strings.HasPrefix(t, "--") && !strings.HasPrefix(t, "/*") {
				prev = strings.ToLower(t)
				break
			}
		}
	}
	has := func(kinds ...clauseKind) bool {
		for _, c := range stmt.clauses {
			for _, k := range kinds {
				if c.kind == k {
					return true
				}
			}
		}
		return false
	}
	// GRANT / REVOKE (also inside ALTER DEFAULT PRIVILEGES) list privileges that are spelled
	// like clauses (grant select, update on t …; revoke … from role); COPY t FROM stdin and
	// IS [NOT] DISTINCT FROM are not the FROM of a query either; CREATE POLICY … FOR SELECT
	// names a command.
	switch kind {
	case clauseSelect, clauseUpdate, clauseInsert, clauseDelete, clauseFrom:
		for k := stmt.startTok; k < i && k < len(tokens); k++ {
			switch strings.ToLower(tokens[k].text) {
			case "grant", "revoke":
				return false
			}
		}
	}
	switch kind {
	case clauseSelect:
		if prev == "for" {
			return false
		}
	case clauseFrom:
		if prev == "distinct" {
			return false
		}
		if first := firstCodeTok(tokens, stmt.startTok, i); first >= 0 && strings.EqualFold(tokens[first].text, "copy") {
			return false
		}
	}
	switch kind {
	case clauseWindow:
		// WINDOW definitions of a query; the word can only follow its FROM/WHERE/HAVING.
		return has(clauseSelect, clauseFrom)
	case clauseSet:
		// UPDATE … SET and ON CONFLICT … DO UPDATE SET only.
		return has(clauseUpdate, clauseOnConflict)
	case clauseUsing:
		// DELETE … USING tables and MERGE … USING source only (JOIN … USING stays in the join).
		if has(clauseMerge) {
			return !has(clauseUsing, clauseWhen)
		}
		// A USING right after a JOIN belongs to that join (JOIN b USING (id)) and stays
		// part of its content; only DELETE … USING opens a clause of its own.
		if n := len(stmt.clauses); n > 0 && stmt.clauses[n-1].kind == clauseJoin {
			return false
		}
		return has(clauseDelete)
	case clauseMergeOn:
		// The ON of a MERGE (after USING, before the first WHEN); an ON that belongs to a
		// JOIN in the source stays part of that join.
		return has(clauseMerge, clauseUsing) && !has(clauseWhen, clauseMergeOn, clauseJoin)
	case clauseWhen:
		// WHEN [NOT] MATCHED of a MERGE, not CASE WHEN.
		if !has(clauseMerge) {
			return false
		}
		depth := 0
		from := stmt.startTok
		if n := len(stmt.clauses); n > 0 {
			from = stmt.clauses[n-1].kwEnd
		}
		for k := from; k < i; k++ {
			switch strings.ToLower(tokens[k].text) {
			case "case":
				depth++
			case "end":
				if depth > 0 {
					depth--
				}
			}
		}
		return depth == 0
	case clauseValues:
		// MERGE … WHEN NOT MATCHED THEN INSERT (…) VALUES (…) is part of the WHEN action.
		return !has(clauseWhen)
	case clauseWith:
		// A CTE list opens a statement or follows AS (CREATE VIEW v AS WITH …).
		return prev == "" || prev == "as" || prev == ";"
	case clauseUpdate:
		if has(clauseWhen) {
			return false // THEN UPDATE SET … is the action of a MERGE WHEN
		}
		switch prev {
		case "do", "for", "on", "or", "before", "after", "of", "key", "instead":
			return false
		}
	}
	return true
}

// findWhenThen returns the index of the THEN that ends the condition of a MERGE
// WHEN clause: the first THEN outside parentheses and CASE expressions, or -1.
func findWhenThen(tokens []rawTok, from, to int) int {
	paren, cs := 0, 0
	for i := from; i < to; i++ {
		switch t := strings.ToLower(tokens[i].text); {
		case t == "(" || t == "[":
			paren++
		case t == ")" || t == "]":
			if paren > 0 {
				paren--
			}
		case t == "case":
			cs++
		case t == "end" && cs > 0:
			cs--
		case t == "then" && paren == 0 && cs == 0:
			return i
		}
	}
	return -1
}

// caseHasLineComment reports whether toks[from..to] contain a -- line comment.
func caseHasLineComment(toks []rawTok, from, to int) bool {
	for i := from; i <= to && i < len(toks); i++ {
		if strings.HasPrefix(toks[i].text, "--") {
			return true
		}
	}
	return false
}

// leadCommaIndent shortens indent by two columns (the width of ", ") so that a line
// starting with a comma keeps its item aligned with the others; indents that are too
// short (or tabs) are returned unchanged and the comma is simply placed after them.
func leadCommaIndent(indent string) string {
	if strings.HasSuffix(indent, "  ") {
		return indent[:len(indent)-2]
	}
	return indent
}

// writeLeadingComments writes the comments that open item on lines of their own (each
// indented to indent) and returns the rest of the item. With comma_position: start a
// comment after the comma would be read back as a trailing comment of the previous item,
// so comments that precede an item go above its comma line instead.
func writeLeadingComments(sb *strings.Builder, item, indent string) string {
	res, err := pg_query.Scan(item)
	if err != nil || len(res.Tokens) == 0 {
		return item
	}
	cut := -1
	for _, t := range res.Tokens {
		text := item[t.Start:t.End]
		if strings.HasPrefix(text, "--") || strings.HasPrefix(text, "/*") {
			continue
		}
		cut = int(t.Start)
		break
	}
	if cut <= 0 {
		return item
	}
	lead := strings.TrimSpace(item[:cut])
	if lead == "" {
		return item
	}
	for _, ln := range strings.Split(lead, "\n") {
		if strings.TrimSpace(ln) == "" {
			continue
		}
		sb.WriteByte('\n')
		sb.WriteString(indent)
		sb.WriteString(strings.TrimSpace(ln))
	}
	return item[cut:]
}

// stmtEndOffset returns the byte offset just past the statement (including its ';').
// fallback is returned for a statement without tokens.
func stmtEndOffset(tokens []rawTok, stmt statement, fallback int) int {
	if stmt.semiIdx >= 0 {
		return tokens[stmt.semiIdx].end
	}
	if stmt.endTok > stmt.startTok && stmt.endTok <= len(tokens) {
		return tokens[stmt.endTok-1].end
	}
	return fallback
}
