package formatter

import (
	"fmt"
	"strings"

	pg_query "github.com/pganalyze/pg_query_go/v6"

	"github.com/heptau/pg_procrustes/config"
)

// FormatRange formats only the statements that touch lines startLine…endLine (1-based,
// inclusive) of sql and copies everything else exactly as written. The unit is the whole
// statement — a statement that is only partly inside the range is formatted completely,
// because a half-formatted statement would not be idempotent.
//
// Statements outside the range are not even parsed, so a syntax error elsewhere in the
// file does not matter. For the selected statements syntax_errors applies as in Format.
// Settings that work on the whole text (blank_lines, blank_lines_between_statements,
// statement_break, semicolons: add, trailing_newline, line_endings) are not applied.
func FormatRange(sql string, cfg *config.Config, startLine, endLine int) (string, []SyntaxError, error) {
	if startLine < 1 || endLine < startLine {
		return "", nil, fmt.Errorf("invalid line range %d:%d", startLine, endLine)
	}
	if strings.TrimSpace(sql) == "" {
		return sql, nil, nil
	}
	eol := detectEOL(sql)
	masked, regions := maskVerbatimRegions(normalizeEOL(sql))
	startLine, endLine = maskedLine(startLine, regions), maskedLine(endLine, regions)

	chunks, err := splitStatements(masked)
	if err != nil {
		return "", nil, SyntaxErrors{syntaxErrorAt(masked, 0, err)}
	}

	per := *cfg
	per.BlankLines = config.BlankLinesPreserve
	per.Semicolons = config.SemicolonPreserve
	per.TrailingNewline = config.TrailingNewlinePreserve
	per.StatementBreak = config.StatementBreakPreserve
	per.BlankLinesBetweenStatements = ""

	mode := syntaxMode(cfg)
	var (
		b     strings.Builder
		diags []SyntaxError
	)
	for _, ch := range chunks {
		if ch.meta || !chunkTouches(masked, ch, startLine, endLine) {
			b.WriteString(ch.text)
			continue
		}
		if mode == config.SyntaxErrorIgnore {
			out, err := formatTolerant(ch.text, &per)
			if err != nil {
				out = ch.text
			}
			b.WriteString(out)
			continue
		}
		parsed, perr := pg_query.Parse(ch.text)
		if perr != nil {
			diags = append(diags, syntaxErrorAt(ch.text, ch.start, perr, masked))
			b.WriteString(ch.text)
			continue
		}
		out, err := formatParsed(ch.text, &per, parsed)
		if err != nil {
			out = ch.text
		}
		b.WriteString(out)
	}
	diags = shiftDiagnostics(diags, regions)
	if len(diags) > 0 && mode == config.SyntaxErrorFail {
		return "", diags, nil
	}
	out := b.String()
	if len(regions) > 0 {
		if out, err = restoreVerbatimRegions(out, regions); err != nil {
			return "", nil, err
		}
	}
	return applyEOL(out, config.LineEndingPreserve, eol), diags, nil
}

// chunkTouches reports whether the code of the chunk (leading blanks excluded) shares a
// line with startLine…endLine of the text.
func chunkTouches(text string, ch chunk, startLine, endLine int) bool {
	body := ch.text
	lead := len(body) - len(strings.TrimLeft(body, " \t\r\n"))
	trimmed := strings.TrimRight(body, " \t\r\n")
	if lead >= len(trimmed) {
		return false // only whitespace
	}
	first := 1 + strings.Count(text[:ch.start+lead], "\n")
	last := 1 + strings.Count(text[:ch.start+len(trimmed)-1], "\n")
	return first <= endLine && last >= startLine
}

// maskedLine maps a 1-based line of the original text to the matching line of the text
// in which every off…on region is a single placeholder line.
func maskedLine(line int, regions []verbatimRegion) int {
	shift := 0 // lines the earlier regions saved
	for _, r := range regions {
		origStart := r.maskedLine + shift
		if line < origStart {
			break
		}
		if line < origStart+r.lines {
			return r.maskedLine
		}
		shift += r.lines - 1
	}
	return line - shift
}
