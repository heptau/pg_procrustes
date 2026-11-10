// Package layout implements clause-level SQL layout and indentation for pg_procrustes.
package layout

import (
	"regexp"
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
	for _, b := range cfg.DDL.DDLBreakModes() {
		if b != "" && b != config.BreakPreserve {
			return false
		}
	}
	if b := cfg.DDL.AlterTable.Constraints; b != "" && b != config.BreakPreserve {
		return false
	}
	return clauseBreak == config.BreakPreserve &&
		unionBL == config.UnionBlankLinePreserve &&
		contentBreak == config.BreakPreserve &&
		indentNorm == config.IndentNormalizePreserve &&
		caseBrk == config.BreakPreserve &&
		parenMode == config.ParenIndentPreserve
}

// Apply applies layout rules (clause breaking, indentation) to already-formatted SQL.
func Apply(sql string, cfg *config.LayoutConfig) (result string, err error) {
	if IsNoop(cfg) {
		return sql, nil
	}
	if strings.TrimSpace(sql) == "" {
		return sql, nil
	}
	indentUnit := indentUnitStr(cfg)

	// What lies inside a multi-line string constant is data: layout works around it.
	sql, unmask := maskStrings(sql)
	defer func() { result = unmask(result) }()

	result = applyAll(sql, cfg, indentUnit)

	// A statement that starts indented (inside BEGIN; … COMMIT;) is laid out as if it started in
	// column 0 and then moved back: the layout of its other lines does not depend on where the
	// first one stands, and a second run reads what the first wrote the same way.
	if cfg.KeepStatementIndent {
		if dedented, indents, ok := dedentStatements(sql, cfg.Clauses.Align == config.AlignRight); ok {
			if laid, ok := reindentStatements(applyAll(dedented, cfg, indentUnit), indents); ok {
				result = laid
			}
		}
	}
	return result, nil
}

// applyAll runs every layout step on sql.
func applyAll(sql string, cfg *config.LayoutConfig, indentUnit string) string {
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
	return result
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

	// river: a statement whose first keyword is shorter than the river starts with blanks of its
	// own; the blanks in front of it in the input (the same blanks of an earlier run) go.
	riverPad := func(gap, rebuilt string) string {
		if cfg.Clauses.Align == config.AlignRight && strings.HasPrefix(rebuilt, " ") {
			return strings.TrimRight(gap, " \t")
		}
		return gap
	}

	var parts []string
	for i, stmt := range stmts {
		rebuilt := rebuildStatement(sql, tokens, stmt, cfg, indentUnit, 0)
		// Preserve inter-statement gap (between end of previous stmt and start of this one).
		if i > 0 {
			prevEnd := stmtEndOffset(tokens, stmts[i-1], 0)
			curStart := tokens[stmt.startTok].start
			parts = append(parts, riverPad(sql[prevEnd:curStart], rebuilt))
		} else {
			leadingGap = riverPad(leadingGap, rebuilt)
		}
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
	clauseLocking // FOR UPDATE | NO KEY UPDATE | SHARE | KEY SHARE of a query
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
	case "for":
		switch n2 := next(); n2 {
		case "update", "share":
			return clauseLocking, 2
		case "no":
			if i+3 < n && strings.EqualFold(tokens[i+2].text, "key") && strings.EqualFold(tokens[i+3].text, "update") {
				return clauseLocking, 4
			}
		case "key":
			if i+2 < n && strings.EqualFold(tokens[i+2].text, "share") {
				return clauseLocking, 3
			}
		}
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

	var riverW []riverSlot // river slot per clause (clauses.align: right), nil otherwise
	if cfg.Clauses.Align == config.AlignRight {
		riverW = riverWidths(sql, tokens, stmt, cfg, stmtFlatLen)
	}

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
			startLen := sb.Len()
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
			if startLen == 0 {
				// nothing precedes the operator (a statement that opens with a parenthesis, `(…) INTERSECT …`,
				// has the set operation as its first clause): the line breaks "around" it would start the
				// statement, and the next run would add them again
				rest := strings.TrimLeft(sb.String(), "\n")
				sb.Reset()
				sb.WriteString(rest)
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

		// CREATE VIEW v (a, b) AS …: the column list of the header may be laid out.
		viewHdr, viewHdrOK := "", false
		if ci == 0 && (cl.kind == clauseSelect || cl.kind == clauseWith) && hasCodeTokens(tokens, kwStart, cl.kwTok) {
			viewHdr, viewHdrOK = viewHeader(sql, tokens, kwStart, cl.kwTok, cfg, indentUnit, depth)
		}

		if !doBreak {
			if viewHdrOK {
				sb.WriteString(viewHdr)
				sb.WriteString(gapBetween(sql, tokens, cl.kwTok-1, cl.kwTok))
				kwStart = cl.kwTok
			}
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
		// river: the keyword is padded so it ends on the common edge; the content hangs after it.
		riverKw, riverHang := "", ""
		riverWidth := 0
		if riverW != nil && riverW[ci].width > 0 {
			riverWidth = riverW[ci].width
			riverKw, _ = riverKeyword(sql, tokens, stmt, ci)
			if riverW[ci].hang {
				clauseIndent = baseIndent + strings.Repeat(" ", riverWidth+1)
				riverHang = clauseIndent + strings.Repeat(" ", runeLen(riverKw)+1)
			} else {
				clauseIndent = baseIndent + strings.Repeat(" ", riverWidth-runeLen(riverKw))
				riverHang = baseIndent + strings.Repeat(" ", riverWidth+1)
			}
		}

		// A leading SELECT/WITH preceded by a statement prefix (CREATE VIEW v AS,
		// CREATE TABLE t AS, EXPLAIN, …): keep the prefix on its own line and start
		// the query on the next one, instead of gluing "AS SELECT" together.
		prefixed := ci == 0 && (cl.kind == clauseSelect || cl.kind == clauseWith) && hasCodeTokens(tokens, kwStart, cl.kwTok)

		// comment lines in front of the query stay in front of the keyword on the river
		leadComment := ""
		if riverKw != "" && ci == 0 && !prefixed {
			leadComment = strings.TrimSpace(originalText(sql, tokens, kwStart, cl.kwTok))
		}

		if ci > 0 {
			if prevIsUnionBlankAfter {
				sb.WriteString("\n\n")
			} else {
				sb.WriteByte('\n')
			}
			sb.WriteString(clauseIndent)
		} else if leadComment != "" {
			sb.WriteString(baseIndent)
			sb.WriteString(leadComment)
			sb.WriteByte('\n')
			sb.WriteString(clauseIndent)
		} else if prefixed {
			sb.WriteString(baseIndent)
			if viewHdrOK {
				sb.WriteString(viewHdr)
			} else {
				sb.WriteString(strings.TrimSpace(originalText(sql, tokens, kwStart, cl.kwTok)))
			}
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
		if riverKw != "" {
			kwText = riverKw
		}
		sb.WriteString(kwText)
		if ci == 0 && !prefixed {
			// The first clause of a statement that does not start with it (CREATE INDEX … WHERE) is
			// written together with the text before it, as it stands: its content hangs from the line
			// the keyword is really on, not from column 0 (which made the next run read the
			// layout differently).
			if nl := strings.LastIndexByte(kwText, '\n'); nl >= 0 {
				last := kwText[nl+1:]
				clauseIndent = last[:len(last)-len(strings.TrimLeft(last, " \t"))]
			}
		}

		contentBreakMode, contentAlign, contentFirstItem := contentBreakAlign(cl.kind, cfg)
		// (decided before the alignment is read: a JOIN whose table comes onto the keyword line has
		// no first item on a line of its own to read the style from, whatever the input looked like)
		joinNewline := cl.kind == clauseJoin && resolveJoinNewline(cfg.Content.JoinOnPosition, sql, tokens, kwEnd, conEnd)
		if joinNewline {
			contentFirstItem = config.FirstItemInline // JOIN t stays on the keyword line
		} else if cl.kind == clauseJoin && cfg.Content.JoinOn.FirstItem == "" && hasJoinCondition(tokens, kwEnd, conEnd) {
			// JOIN t ON a = b: the table and its condition stay on the keyword line
			// unless join_on.first_item asks for the first item on a line of its own.
			contentFirstItem = config.FirstItemInline
		}
		if contentFirstItem == config.FirstItemInline && contentAlign == config.AlignPreserve {
			// With the first item on the keyword line there is no first item to read the style
			// from (and the first line that starts a line may be a continuation of that item):
			// detecting it would give another answer on the next run.
			contentAlign = config.AlignIndent
		} else {
			contentAlign = resolveContentAlign(contentAlign, sql, tokens, cl.kwTok, kwEnd, conEnd)
		}
		if riverHang != "" {
			contentFirstItem = config.FirstItemInline // the first item stands right after the keyword
			contentAlign = config.AlignIndent
		}
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
		if cl.kind == clauseOnConflict {
			doContentBreak = false // ON CONFLICT (target) DO UPDATE / DO NOTHING stays on one line; SET follows below
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
			caseBase := clauseIndent
			if riverHang != "" {
				caseBase = riverHang
			}
			content = reformatCaseExprs(content, caseBase, cfg.Case, indentUnit, cfg.LineLength)
			if i := strings.IndexByte(content, '\n'); i >= 0 && strings.HasPrefix(strings.TrimLeft(content, " \t"), "--") && !strings.Contains(content[i+1:], "\n") {
				// a line comment after the keyword ends its line: what follows hangs below the keyword
				// like the content of a broken clause, not at the margin
				hang := clauseIndent + indentUnit
				if riverHang != "" {
					hang = riverHang
				}
				content = content[:i+1] + hang + strings.TrimLeft(content[i+1:], " \t")
			}
			sb.WriteString(content)
			continue
		}

		contentIndent := clauseIndent + indentUnit
		if contentAlign == config.AlignSame {
			contentIndent = clauseIndent
		}
		if riverHang != "" {
			contentIndent = riverHang
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
				if riverHang != "" { // river: the columns stand below the first one, not below the keyword
					contentIndent = riverHang + " "
					if prefix != "" {
						contentIndent += strings.Repeat(" ", runeLen(prefix)+1)
					}
				}
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
							if colStart {
								// comment lines in front of the column stay above its comma line
								// (`, --` would put them behind the comma, where the next run reads them
								// as a trailing comment of the line)
								item = writeLeadingComments(&sb, item, contentIndent)
							}
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
					if riverHang != "" && !strings.Contains(colItems[len(colItems)-1], "--") {
						sb.WriteByte(')') // river: the list closes behind its last column
					} else {
						sb.WriteByte('\n')
						sb.WriteString(clauseIndent)
						sb.WriteByte(')')
					}
				} else {
					sb.WriteString("(\n")
					colStart := resolveCommaStart(cfg.Content.CommaPosition, sql, tokens, openIdx+1, closeIdx)
					for ii, item := range colItems {
						if colStart && ii > 0 {
							var lead strings.Builder
							item = writeLeadingComments(&lead, item, contentIndent)
							if lead.Len() > 0 { // (it writes a line break before each comment line)
								sb.WriteString(strings.TrimPrefix(lead.String(), "\n") + "\n")
							}
							sb.WriteString(leadCommaIndent(contentIndent) + ", ")
						} else {
							sb.WriteString(contentIndent)
						}
						item = reindentContinuation(item, contentIndent) // (not "\n" + indent: each run added another level)
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

		items := splitContent(sql, tokens, kwEnd, conEnd, cl.kind, joinNewline)
		useComma := usesCommaSeparator(cl.kind)
		commaStart := useComma && resolveCommaStart(cfg.Content.CommaPosition, sql, tokens, kwEnd, conEnd)
		if !useComma && resolveOperatorEnd(cfg.Content.LogicalOperatorPosition, sql, tokens, kwEnd, conEnd) {
			items = moveOperatorsToEnd(items)
		}
		if contentFirstItem != config.FirstItemInline {
			switch {
			case cl.kind == clauseSelect && cfg.Content.AlignAliases == config.AlignItemsAlign:
				items = alignItems(items, "as", runeLen(contentIndent), wrapLimit(cfg), cfg.CommaSpacing == config.CommaSpacingCompact)
			case cl.kind == clauseSelect && cfg.Content.AlignAliases == config.AlignItemsNone:
				items = unpadItems(items, "as")
			case cl.kind == clauseSet && cfg.Content.AlignSet == config.AlignItemsAlign:
				items = alignItems(items, "=", runeLen(contentIndent), wrapLimit(cfg), cfg.CommaSpacing == config.CommaSpacingCompact)
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
		// Comment lines in front of the item are not part of it: judged as its first line they
		// made every line below look unindented (their own first line has no indent yet), so each
		// run indented the rest once more.
		var reindentBody func(item string) string
		reindentItem := func(item string) string {
			lead, body := splitLeadingCommentLines(item)
			if lead == "" || strings.TrimSpace(body) == "" {
				return reindentBody(item)
			}
			// (the first line below the comments starts a line of its own: blanks it has left of the
			// item's indent, from the keyword line, are brought to it)
			if ws := body[:len(body)-len(strings.TrimLeft(body, " \t"))]; ws != "" && len(ws) < len(contentIndent) && strings.HasPrefix(contentIndent, ws) {
				body = contentIndent + body[len(ws):]
			}
			rest := reindentBody(body)
			if !strings.HasPrefix(body, " ") && !strings.HasPrefix(body, "\t") && (rest != body || !strings.Contains(body, "\n")) {
				// The comment lines took the item's indent off its other lines
				// (trimLeadingCommentIndent), so the first line of the body has none either: it starts
				// a line after the comments and moves in together with the lines below it.
				rest = contentIndent + rest
			}
			return lead + rest
		}
		reindentBody = func(item string) string {
			if !strings.ContainsRune(item, '\n') {
				return item
			}
			firstNewline := strings.IndexByte(item, '\n')
			if firstNewline < 0 {
				return item
			}
			// The arguments of a call (`f(` / `a,` / `b);`) that stand left of the item's indent are
			// first moved to it, so that the shift below puts them one level deeper (the same place
			// they have when they already stood at the indent: the second run must not move them).
			if first, ok := openParenBodyIndent(item); ok && len(first) < len(contentIndent) && strings.HasPrefix(contentIndent, first) {
				pad := contentIndent[len(first):]
				parts := strings.Split(item, "\n")
				for j := 1; j < len(parts); j++ {
					if strings.TrimSpace(parts[j]) != "" {
						parts[j] = pad + parts[j]
					}
				}
				item = strings.Join(parts, "\n")
			}
			// A parenthesis the item opens (a VALUES row, a CTE body: `v AS (` / `v AS` + `(`) closes on
			// a line of its own, and that `)` belongs at the indent of the item. When it stands further
			// left, the whole item is shifted right by the difference, body and parentheses together
			// (judging by the body alone left `(` at the item's indent and the `)` at the margin).
			if fixed, ok := shiftClosedParenItem(item, contentIndent, indentUnit); ok {
				return fixed
			}
			// `PERFORM f(` + arguments on the lines below: the call opens a parenthesis that closes on
			// the last argument line, and its arguments stand one level deeper than the call; when they
			// stand at the indent of the item itself (the item was moved right under its keyword),
			// they are shifted one level.
			if shift := openParenBodyShift(item, contentIndent, indentUnit); shift != "" {
				parts := strings.Split(item, "\n")
				for j := 1; j < len(parts); j++ {
					if strings.TrimSpace(parts[j]) != "" {
						// after the blanks it has (not before them): with tabs for the indent unit
						// and spaces right-aligned under the keyword, "<spaces><tab>" is the order
						// a second pass finds again.
						t := strings.TrimLeft(parts[j], " \t")
						parts[j] = parts[j][:len(parts[j])-len(t)] + shift + t
					}
				}
				return strings.Join(parts, "\n")
			}
			// Judge by the first non-blank continuation line; a blank line right after
			// the first newline must not make every pass add another level of indent.
			// Lone parentheses say little about the item's indent (paren_indent.open: own_line
			// puts "(" at the indent of its ")"), so a line with real content is preferred.
			// A line inside a parenthesis the item opened on an earlier line (a subquery body,
			// an argument list) is judged only when no line outside one exists: those lines get
			// laid out again by their own rules, and what is left of the item after them (a USING
			// clause, an alias) says more.
			judgeOf := func(onlyOuter, skipComments bool) string {
				depth, _ := parenScanLine(item[:firstNewline])
				first := ""
				for _, ln := range strings.Split(item[firstNewline+1:], "\n") {
					inner := depth > 0
					d, _ := parenScanLine(ln)
					if depth += d; depth < 0 {
						depth = 0
					}
					t := strings.TrimSpace(ln)
					if t == "" || onlyOuter && inner || skipComments && isCommentLine(ln) {
						continue
					}
					if first == "" {
						first = ln
					}
					if t != "(" && !strings.HasPrefix(t, ")") {
						return ln
					}
				}
				if onlyOuter {
					return "" // lone parentheses alone: judge by all the lines instead
				}
				return first
			}
			// (comment lines judge last: their indent says nothing about the item, and a comment
			// line moved to the left margin made every run indent the item once more)
			judge := judgeOf(true, true)
			if judge == "" {
				judge = judgeOf(false, true)
			}
			if judge == "" {
				judge = judgeOf(true, false)
			}
			if judge == "" {
				judge = judgeOf(false, false)
			}
			if judge != "" && strings.HasPrefix(judge, contentIndent) {
				return indentAfterComments(item, contentIndent)
			}
			if cfg.ParenIndent.Open == config.ParenOpenOwnLine && strings.HasPrefix(strings.TrimSpace(judge), "(") {
				// own_line puts such a "(" at the indent of the line it came from, not of the items
				return item
			}
			parts := strings.Split(item, "\n")
			for j := 1; j < len(parts); j++ {
				switch {
				case strings.TrimSpace(parts[j]) == "":
				case isCommentLine(parts[j]):
					// a comment line gets the indent itself, not one more level on top of what
					// it has: its old indent says nothing about the item, and adding to it made
					// it a level deeper on every run
					parts[j] = contentIndent + strings.TrimLeft(parts[j], " \t")
				default:
					parts[j] = contentIndent + parts[j]
				}
			}
			return strings.Join(parts, "\n")
		}

		if contentFirstItem == config.FirstItemInline {
			// first_item: inline — first item on the keyword line, subsequent items on new lines.
			for ii, item := range items {
				item = reindentItem(item)
				item = reformatCaseExprs(item, contentIndent, cfg.Case, indentUnit, cfg.LineLength)
				if cfg.Content.WrapExpressions == config.WrapWrap {
					item = wrapLongItem(item, contentIndent, indentUnit, cfg.LineLength-1)
				}
				item = collapseAlignedPadding(item, cl.kind, cfg)
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
			// (reindented first: the CASE rewrite below lays out its own lines absolutely, and
			// judging them afterwards by the line that holds CASE re-indents them once more
			// when that line's indent is spelled differently, e.g. with tabs)
			item = reindentItem(item)
			item = reformatCaseExprs(item, contentIndent, cfg.Case, indentUnit, cfg.LineLength)
			if cfg.Content.WrapExpressions == config.WrapWrap {
				item = wrapLongItem(item, contentIndent, indentUnit, cfg.LineLength-1)
			}
			item = collapseAlignedPadding(item, cl.kind, cfg)
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

// shiftClosedParenItem moves a multi-line item that opens a parenthesis (CTE body, VALUES row) so
// that the `)` that closes it — the first line starting with ")" at nesting level one — stands at
// indent, and the lines between the opening and the closing parenthesis stand deeper than that.
// ok is false when there is no such `)` line or it already stands at indent or further right.
func shiftClosedParenItem(item, indent, unit string) (string, bool) {
	lines := strings.Split(item, "\n")
	depth := 0
	closeAt := -1
	for j, ln := range lines {
		t := strings.TrimLeft(ln, " \t")
		if j > 0 && depth == 1 && strings.HasPrefix(t, ")") {
			have := ln[:len(ln)-len(t)]
			if strings.HasPrefix(have, indent) || !strings.HasPrefix(indent, have) {
				return item, false
			}
			closeAt = j
			break
		}
		d, _ := parenScanLine(ln)
		if depth += d; depth < 0 {
			depth = 0
		}
	}
	if closeAt < 0 {
		return item, false
	}
	lead := func(ln string) string { return ln[:len(ln)-len(strings.TrimLeft(ln, " \t"))] }
	have := lead(lines[closeAt])
	closeShift := indent[len(have):]
	// Only the shapes of a CTE or a VALUES row: the parenthesis opens on the first line (`v AS (`,
	// `(`) or stands alone on the second (`v AS` / `(`). A parenthesis opened further down
	// (`x` / `IN (`) is left to the generic indentation.
	bodyFrom := 1
	switch d, _ := parenScanLine(lines[0]); {
	case d > 0:
	case d == 0 && strings.TrimSpace(lines[1]) == "(":
		bodyFrom = 2
	default:
		return item, false
	}
	minInd, have2 := "", false
	for j := bodyFrom; j < closeAt; j++ {
		if strings.TrimSpace(lines[j]) == "" {
			continue
		}
		if ind := lead(lines[j]); !have2 || len(ind) < len(minInd) {
			minInd, have2 = ind, true
		}
	}
	allTabs := have2
	for j := bodyFrom; j < closeAt; j++ {
		if strings.TrimSpace(lines[j]) != "" && !strings.HasPrefix(lines[j], "\t") {
			allTabs = false
		}
	}
	if allTabs && strings.Trim(have, " ") == "" {
		// tabs on one side and blanks on the other say nothing about which is deeper: measured by
		// bytes, the shift put a blank in front of every tab and grew on the next run
		return item, false
	}
	want := indent + unit
	// A body that already stands deeper than the parentheses stays where it is: the lines a run is
	// given may have it at the indent of an earlier run and the `)` at the margin, and moving it by
	// the distance of the `)` would push it further on every run. Otherwise the lines below the
	// first move together.
	bodyDeep := have2 && strings.HasPrefix(minInd, want)
	for j := 1; j < len(lines); j++ {
		if strings.TrimSpace(lines[j]) == "" || bodyDeep && j >= bodyFrom && j < closeAt {
			continue
		}
		if strings.HasPrefix(lines[j], have) {
			// after the blanks the line has in common with the `)` (with tabs for the unit and
			// blanks in front of them, "<blanks><tab>" is the order a second run finds again)
			lines[j] = have + closeShift + lines[j][len(have):]
		} else {
			lines[j] = closeShift + lines[j]
		}
		if j > closeAt && !strings.HasPrefix(lead(lines[j]), indent) && strings.TrimSpace(lines[j]) != "" && !isCommentLine(lines[j]) {
			// what follows the closing parenthesis (`) AS f` / `USING (id)`) belongs to the item
			// and stands at its indent at least; a line left of it would be what the next run
			// judges the whole item by, and move the item once more
			lines[j] = indent + strings.TrimLeft(lines[j], " \t")
		}
	}
	if !bodyDeep && have2 {
		// (the body, moved with the rest, must stand deeper than the parentheses)
		min2 := closeShift + minInd
		if len(min2) < len(want) && strings.HasPrefix(want, min2) {
			extra := want[len(min2):]
			for j := bodyFrom; j < closeAt; j++ {
				if strings.TrimSpace(lines[j]) != "" {
					if strings.HasPrefix(lines[j], min2) {
						// (behind the blanks the body already has, not in front of them)
						lines[j] = min2 + extra + lines[j][len(min2):]
					} else {
						lines[j] = extra + lines[j]
					}
				}
			}
		}
	}
	return strings.Join(lines, "\n"), true
}

// indentAfterComments brings a line that follows a line ending in a `--` comment up to indent when
// it stands left of it: such a line starts after the line break a flattened item got behind the
// comment, with no indent of its own. The lines of the item are judged by the others, which are in
// place, so it would stay at the margin — while a run that finds them out of place gives it the
// indent of the item with the rest.
func indentAfterComments(item, indent string) string {
	if !strings.Contains(item, "--") {
		return item
	}
	lines := strings.Split(item, "\n")
	changed := false
	for j := 1; j < len(lines); j++ {
		if strings.TrimSpace(lines[j]) == "" || isCommentLine(lines[j]) || strings.HasPrefix(lines[j], indent) || !endsInLineComment(lines[j-1]) {
			continue
		}
		lines[j] = indent + strings.TrimLeft(lines[j], " \t")
		changed = true
	}
	if !changed {
		return item
	}
	return strings.Join(lines, "\n")
}

// endsInLineComment reports whether line has a `--` comment outside strings that runs to its end.
func endsInLineComment(line string) bool {
	inStr := false
	for i := 0; i+1 < len(line); i++ {
		switch {
		case line[i] == '\'':
			inStr = !inStr
		case !inStr && line[i] == '-' && line[i+1] == '-':
			return true
		}
	}
	return false
}

// openParenBodyShift returns indentUnit when the first line of a multi-line item leaves a
// parenthesis open, no later line starts with the `)` that closes it (the call ends on its last
// argument: `f(` / `a,` / `b);`; the parenthesis stays open up to the last line), and the first line below stands exactly at indent: the arguments
// of such a call belong one level deeper. Otherwise "".
func openParenBodyShift(item, indent, unit string) string {
	if first, ok := openParenBodyIndent(item); ok && first == indent {
		return unit
	}
	return ""
}

// openParenBodyIndent returns the indent of the first argument line of a multi-line call item
// (see openParenBodyShift for the shape) and whether the item has that shape.
func openParenBodyIndent(item string) (string, bool) {
	lines := strings.Split(item, "\n")
	depth, _ := parenScanLine(lines[0])
	if depth <= 0 || !strings.HasSuffix(strings.TrimRight(lines[0], " \t"), "(") || !endsWithCallOpen(lines[0]) {
		return "", false
	}
	if subqueryOpen.MatchString(item) {
		return "", false // a subquery in the arguments lays itself out (expandSubqueries) and is not moved
	}
	first := ""
	seen := false
	for _, ln := range lines[1:] {
		t := strings.TrimLeft(ln, " \t")
		if strings.TrimSpace(ln) == "" || isCommentLine(ln) {
			continue
		}
		if depth <= 0 || depth == 1 && strings.HasPrefix(t, ")") {
			return "", false // the parenthesis closed before this line: what follows is not its body
		}
		if !seen {
			first, seen = ln[:len(ln)-len(t)], true
		}
		d, _ := parenScanLine(ln)
		depth += d
	}
	return first, seen
}

var subqueryOpen = regexp.MustCompile(`(?i)\(\s*(select|with|values)\b`)

// endsWithCallOpen reports whether the last parenthesis of line that is still open follows a
// function name (`f(`, `schema.f(`, `PERFORM f(`), not a keyword: `JOIN (`, `IN (`, `AS (` open a
// subquery or a list that lays itself out.
func endsWithCallOpen(line string) bool {
	var opens []int
	inStr := false
	for i := 0; i < len(line); i++ {
		switch c := line[i]; {
		case c == '\'':
			inStr = !inStr
		case inStr:
		case c == '(':
			opens = append(opens, i)
		case c == ')' && len(opens) > 0:
			opens = opens[:len(opens)-1]
		}
	}
	if len(opens) == 0 {
		return false
	}
	end := opens[len(opens)-1]
	start := end
	for start > 0 {
		c := line[start-1]
		if c == '_' || c == '.' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' {
			start--
			continue
		}
		break
	}
	if start == end {
		return false
	}
	switch strings.ToLower(line[start:end]) {
	case "join", "from", "in", "as", "on", "and", "or", "not", "exists", "any", "all", "some", "using",
		"values", "where", "when", "then", "else", "case", "between", "lateral", "select", "set", "by",
		"over", "filter", "returning", "into", "union", "intersect", "except":
		return false
	}
	return true
}

// wrapLimit is the line length items are wrapped at (0 when they are not wrapped).
func wrapLimit(cfg *config.LayoutConfig) int {
	if cfg.Content.WrapExpressions == config.WrapWrap {
		return cfg.LineLength - 1
	}
	return 0
}

// collapseAlignedPadding undoes the alignment padding of an item that ended up on several lines.
func collapseAlignedPadding(item string, kind clauseKind, cfg *config.LayoutConfig) string {
	if !strings.Contains(item, "\n") {
		return item
	}
	switch {
	case kind == clauseSelect && cfg.Content.AlignAliases == config.AlignItemsAlign:
		return collapseSeparatorPadding(item, "as")
	case kind == clauseSet && cfg.Content.AlignSet == config.AlignItemsAlign:
		return collapseSeparatorPadding(item, "=")
	}
	return item
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
	carry := "" // comment lines above a leading comma, for the item after it
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
			// Comment lines standing above a leading comma ("-- note" / ", b") belong to the
			// item after it, as they do when the comma stands at the end of the line above them.
			cut := i
			if itemEnd == i {
				for cut-1 > itemStart && strings.HasPrefix(tokens[cut-1].text, "--") &&
					strings.Contains(sql[tokens[cut-2].end:tokens[cut-1].start], "\n") {
					cut--
				}
				if cut == i || !strings.Contains(sql[tokens[i-1].end:tokens[i].start], "\n") {
					cut = i
				} else {
					itemEnd = cut
				}
			}
			item := trimLeadingCommentIndent(strings.TrimSpace(originalText(sql, tokens, itemStart, itemEnd)))
			if strings.HasPrefix(tokens[itemEnd-1].text, "--") && itemEnd != i && cut == i {
				// originalText included the comma token; drop it from the item text
				item = trimLeadingCommentIndent(strings.TrimSpace(originalText(sql, tokens, itemStart, i))) + " " + tokens[itemEnd-1].text
			}
			if carry != "" {
				item = carry + "\n" + item
				carry = ""
			}
			if item != "" {
				items = append(items, item)
			}
			itemStart = next
			if cut != i {
				carry = trimLeadingCommentIndent(strings.TrimSpace(originalText(sql, tokens, cut, i)))
			}
		}
	}
	item := trimLeadingCommentIndent(strings.TrimSpace(originalText(sql, tokens, itemStart, to)))
	if carry != "" {
		item = carry + "\n" + item
		carry = ""
	}
	if item != "" {
		items = append(items, item)
	}
	return foldTrailingComments(items)
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
				item := valuesItemText(sql, tokens, itemStart, i+1)
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
	item := valuesItemText(sql, tokens, itemStart, to)
	if item != "" {
		items = append(items, item)
	}
	return foldTrailingComments(items)
}

// foldTrailingComments appends a last item that consists of comments only (a commented-out
// last row or column after the final comma) to the item before it: as an item of its own it
// would be given a separating comma and leave a dangling `,` before `;` or the next clause.
func foldTrailingComments(items []string) []string {
	n := len(items)
	if n < 2 || !commentsOnly(items[n-1]) {
		return items
	}
	// (without their old indentation: the lines are indented again below, and keeping the
	// original blanks made every run indent them once more)
	lines := strings.Split(items[n-1], "\n")
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}
	items[n-2] += "\n" + strings.Join(lines, "\n")
	return items[:n-1]
}

// commentsOnly reports whether text holds no code, only comments.
func commentsOnly(text string) bool {
	res, err := pg_query.Scan(text)
	if err != nil || len(res.Tokens) == 0 {
		return false
	}
	for _, t := range res.Tokens {
		if !isCommentTok(text[t.Start:t.End]) {
			return false
		}
	}
	return true
}

// valuesItemText is the text of tokens[from:to] without the separating comma that leads the
// row when comment lines come before it (`-- note` / `, (1, 2)` in leading-comma style): the
// comment lines stay with the row, the comma is written again by the layout.
func valuesItemText(sql string, tokens []rawTok, from, to int) string {
	k := from
	for k < to && isCommentTok(tokens[k].text) {
		k++
	}
	if k >= to || tokens[k].text != "," {
		return strings.TrimSpace(originalText(sql, tokens, from, to))
	}
	lead := strings.TrimSpace(originalText(sql, tokens, from, k))
	rest := strings.TrimSpace(originalText(sql, tokens, k+1, to))
	if lead == "" {
		return rest
	}
	return lead + "\n" + rest
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
		if caseHasLineComment(toks, i, endIdx) {
			// Neither flattening nor rebuilding WHEN … THEN lines may put code after a --
			// comment: the CASE is left as it is.
			sb.WriteString(item[tok.start:toks[endIdx].end])
		} else if doExpand {
			sb.WriteString(expandCaseExpr(item, toks, i, endIdx, baseIndent, indentUnit, cfg.Indent, cfg.ParenSpacing))
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
func expandCaseExpr(item string, toks []rawTok, caseIdx, endIdx int, baseIndent, indentUnit string, indentMode config.BodyIndentMode, caseParen config.ParenSpacingMode) string {
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
		// (on one line: lines of a parenthesised operand kept their old indent while the rest
		// moved, and the next run moved them again)
		expr := strings.TrimSpace(inlineContent(item, toks, caseIdx+1, firstWhen, caseParen))
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
			code := strings.TrimRight(item[:start], " \t\r\n")
			if gap := item[len(code):start]; strings.Contains(gap, "\n") {
				// the comment has a line of its own: the suffix ends the line of the code above it
				// (`)` / `, -- note` read back as a comma and a trailing comment, and moved on every run)
				return code + suffix + gap + item[start:]
			}
			return strings.TrimRight(item[:start], " \t") + suffix + " " + item[start:]
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
		// comment lines in front of the operator describe the condition after it: they stay
		// in front of that condition
		lead, body := splitLeadingCommentLines(out[i])
		lead = trimLines(lead)
		op, rest, ok := splitLeadingOperator(body)
		if !ok {
			continue
		}
		// whole comment lines at the end of the previous item belong to the condition below
		prev, trail := splitTrailingCommentLines(out[i-1])
		lead = trail + lead
		out[i-1] = appendBeforeComment(prev, " "+op)
		// A comment right after the operator on its line ("AND -- why") ends the line the
		// operator now ends, so it travels with it; one on a line of its own stays with the
		// condition below it.
		if strings.HasPrefix(rest, "--") && !strings.Contains(body[len(op):len(body)-len(rest)], "\n") {
			comment, remaining := rest, ""
			if nl := strings.IndexByte(rest, '\n'); nl >= 0 {
				comment, remaining = rest[:nl], strings.TrimLeft(rest[nl+1:], " \t\n")
			}
			out[i-1] += " " + strings.TrimRight(comment, " \t\r")
			rest = remaining
		}
		out[i] = lead + rest
	}
	return out
}

// isCommentLine reports whether the line holds nothing but a comment (a -- comment, or block
// comments that start and end on it).
func isCommentLine(line string) bool {
	t := strings.TrimSpace(line)
	if strings.HasPrefix(t, "--") {
		return true
	}
	for strings.HasPrefix(t, "/*") {
		end := strings.Index(t, "*/")
		if end < 0 {
			return false
		}
		t = strings.TrimSpace(t[end+2:])
	}
	return t == "" && strings.HasPrefix(strings.TrimSpace(line), "/*")
}

// trimLines trims the blanks around every line of s (each line keeps its line break).
func trimLines(s string) string {
	if s == "" {
		return s
	}
	var sb strings.Builder
	for _, ln := range strings.SplitAfter(s, "\n") {
		if ln == "" {
			continue
		}
		sb.WriteString(strings.TrimSpace(ln))
		if strings.HasSuffix(ln, "\n") {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// splitTrailingCommentLines splits the whole -- comment lines an item ends with (starting at the
// line break before them) from the rest.
func splitTrailingCommentLines(item string) (rest, trail string) {
	cut := len(item)
	for cut > 0 {
		nl := strings.LastIndexByte(item[:cut], '\n')
		if nl < 0 || !isCommentLine(item[nl+1:cut]) {
			break
		}
		cut = nl
	}
	if cut == len(item) {
		return item, ""
	}
	var sb strings.Builder
	for _, ln := range strings.Split(item[cut+1:], "\n") {
		sb.WriteString(strings.TrimSpace(ln)) // the indent of the line they leave is not theirs
		sb.WriteByte('\n')
	}
	return item[:cut], sb.String()
}

// splitLeadingCommentLines splits the whole -- comment lines an item starts with (kept with
// their line break) from the rest.
func splitLeadingCommentLines(item string) (lead, rest string) {
	pos := 0
	for pos < len(item) {
		nl := strings.IndexByte(item[pos:], '\n')
		if nl < 0 {
			break
		}
		if !isCommentLine(item[pos : pos+nl]) {
			break
		}
		pos += nl + 1
	}
	return item[:pos], item[pos:]
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
	case clauseValues:
		if prev == "for" {
			return false // CREATE TABLE … PARTITION OF p FOR VALUES …
		}
	case clauseOrderBy:
		if prev == "for" {
			return false // CREATE OPERATOR CLASS … OPERATOR 1 < FOR ORDER BY float_ops
		}
	case clauseFrom:
		if prev == "distinct" || prev == "values" {
			return false // IS DISTINCT FROM; FOR VALUES FROM (…) TO (…)
		}
		if first := firstCodeTok(tokens, stmt.startTok, i); first >= 0 && strings.EqualFold(tokens[first].text, "copy") {
			return false
		}
		if isCreateTrigger(tokens, stmt.startTok, i) {
			return false // CREATE CONSTRAINT TRIGGER … FROM referenced_table
		}
	}
	switch kind {
	case clauseLocking:
		// The locking clause of a query (SELECT … FOR UPDATE), not FOR UPDATE of CREATE POLICY or a
		// trigger: a SELECT stands before it.
		for _, c := range stmt.clauses {
			if c.kind == clauseSelect && c.kwEnd <= i {
				return true
			}
		}
		return false
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

// lineIndentBefore returns the blanks that start the line on which offset lies, or "" when
// something else comes before offset on that line.
func lineIndentBefore(sql string, offset int) string {
	ls := strings.LastIndexByte(sql[:offset], '\n') + 1
	if prefix := sql[ls:offset]; strings.TrimLeft(prefix, " \t") == "" {
		return prefix
	}
	return ""
}

// scanStatements tokenizes sql and splits it into statements.
func scanStatements(sql string) ([]rawTok, []statement, bool) {
	res, err := pg_query.Scan(sql)
	if err != nil {
		return nil, nil, false
	}
	tokens := make([]rawTok, len(res.Tokens))
	for i, t := range res.Tokens {
		tokens[i] = rawTok{start: int(t.Start), end: int(t.End), text: sql[t.Start:t.End]}
	}
	stmts := parseStatements(tokens)
	return tokens, stmts, len(stmts) > 0
}

// multilineTokenLineStarts returns the offsets of the lines of text that start inside a
// multi-line token (a dollar-quoted body, a block comment): their blanks are not indentation.
func multilineTokenLineStarts(text string) map[int]bool {
	inside := map[int]bool{}
	if res, err := pg_query.Scan(text); err == nil {
		for _, t := range res.Tokens {
			for k := int(t.Start); k < int(t.End); k++ {
				if text[k] == '\n' {
					inside[k+1] = true
				}
			}
		}
	}
	return inside
}

// statementSpan is the byte range of a statement in the text its tokens come from.
func statementSpan(tokens []rawTok, stmt statement) (int, int) {
	start := tokens[stmt.startTok].start
	return start, stmtEndOffset(tokens, stmt, tokens[stmt.startTok].end)
}

// dedentStatements removes the indentation of each statement that starts a line (also from the
// lines below its first line) and returns that indentation per statement. ok is false when
// no statement starts indented.
func dedentStatements(sql string, river bool) (string, []string, bool) {
	tokens, stmts, ok := scanStatements(sql)
	if !ok {
		return sql, nil, false
	}
	inside := multilineTokenLineStarts(sql)
	firstLen := make([]int, len(stmts)) // blanks that start the first line of each statement
	indents := make([]string, len(stmts))
	any := false
	for i, st := range stmts {
		indents[i] = lineIndentBefore(sql, tokens[st.startTok].start)
		firstLen[i] = len(indents[i])
		if river {
			// river: the first keyword may be padded to the edge of the river, so the indentation
			// of the statement is what all its lines have in common
			start, end := statementSpan(tokens, st)
			if start > len(sql) {
				start = len(sql)
			}
			if end > len(sql) {
				end = len(sql)
			}
			indents[i] = commonIndent(indents[i], sql[start:end], start, inside)
		}
		any = any || firstLen[i] > 0
	}
	if !any {
		return sql, nil, false
	}
	var b strings.Builder
	cur := 0
	for i, st := range stmts {
		start, end := statementSpan(tokens, st)
		if start > len(sql) {
			start = len(sql)
		}
		if end > len(sql) {
			end = len(sql)
		}
		b.WriteString(sql[cur : start-firstLen[i]]) // (the blanks that start the first line go too)
		b.WriteString(mapLinesBelowFirst(sql[start:end], start, inside, func(line string) string {
			return strings.TrimPrefix(line, indents[i])
		}))
		cur = end
	}
	b.WriteString(sql[cur:])
	return b.String(), indents, true
}

// commonIndent is the longest leading blank prefix shared by first (the indent of the first line)
// and the indent of every other line of text that does not start inside a multi-line token.
func commonIndent(first, text string, base int, inside map[int]bool) string {
	common := first
	mapLinesBelowFirst(text, base, inside, func(line string) string {
		if strings.TrimSpace(line) == "" {
			return line
		}
		ind := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		n := 0
		for n < len(common) && n < len(ind) && common[n] == ind[n] {
			n++
		}
		common = common[:n]
		return line
	})
	return common
}

// reindentStatements puts the indentation back on every line of each statement of laid-out text. ok is false when the statements are not those that were dedented.
func reindentStatements(sql string, indents []string) (string, bool) {
	tokens, stmts, ok := scanStatements(sql)
	if !ok || len(stmts) != len(indents) {
		return sql, false
	}
	inside := multilineTokenLineStarts(sql)
	var b strings.Builder
	cur := 0
	for i, st := range stmts {
		start, end := statementSpan(tokens, st)
		if start > len(sql) {
			start = len(sql)
		}
		if end > len(sql) {
			end = len(sql)
		}
		b.WriteString(sql[cur:start])
		b.WriteString(indents[i])
		b.WriteString(mapLinesBelowFirst(sql[start:end], start, inside, func(line string) string {
			if strings.TrimSpace(line) == "" {
				return line
			}
			return indents[i] + line
		}))
		cur = end
	}
	b.WriteString(sql[cur:])
	return b.String(), true
}

// mapLinesBelowFirst applies f to every line of text after the first that does not start
// inside a multi-line token; base is the offset of text in the string inside refers to.
func mapLinesBelowFirst(text string, base int, inside map[int]bool, f func(string) string) string {
	if !strings.Contains(text, "\n") {
		return text
	}
	var b strings.Builder
	start := 0
	for {
		end := strings.IndexByte(text[start:], '\n')
		line := text[start:]
		if end >= 0 {
			line = text[start : start+end]
		}
		if start > 0 && !inside[base+start] {
			line = f(line)
		}
		b.WriteString(line)
		if end < 0 {
			break
		}
		b.WriteByte('\n')
		start += end + 1
	}
	return b.String()
}
