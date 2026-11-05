package formatter

import (
	"fmt"
	"strings"
	"unicode/utf8"

	pg_query "github.com/pganalyze/pg_query_go/v6"
	"github.com/pganalyze/pg_query_go/v6/parser"

	"github.com/heptau/pg_procrustes/config"
)

// SyntaxError describes one statement PostgreSQL could not parse.
type SyntaxError struct {
	Offset  int    // byte offset of the error (in the input with CRLF read as LF)
	Line    int    // 1-based line of the error
	Col     int    // 1-based column (in characters) of the error
	Message string // parser message, e.g. `syntax error at or near "from"`
}

func (e SyntaxError) Error() string {
	return fmt.Sprintf("line %d, column %d: %s", e.Line, e.Col, e.Message)
}

// SyntaxErrors is returned by Format when syntax_errors is "fail" and the input
// contains statements that do not parse. It lists every offending statement.
type SyntaxErrors []SyntaxError

func (e SyntaxErrors) Error() string {
	switch len(e) {
	case 0:
		return "syntax error"
	case 1:
		return e[0].Error()
	}
	return fmt.Sprintf("%s (and %d more syntax errors)", e[0].Error(), len(e)-1)
}

// Format normalizes sql according to cfg. How input that is not valid PostgreSQL is
// treated depends on cfg.SyntaxErrors: fail (default) returns a SyntaxErrors error,
// skip formats the valid statements and leaves the others untouched, ignore applies
// the token-level rules to everything. Use FormatWithDiagnostics to also get the
// statements that were skipped.
func Format(sql string, cfg *config.Config) (string, error) {
	out, diags, err := FormatWithDiagnostics(sql, cfg)
	if err != nil {
		return "", err
	}
	if len(diags) > 0 && syntaxMode(cfg) == config.SyntaxErrorFail {
		return "", SyntaxErrors(diags)
	}
	return out, nil
}

func syntaxMode(cfg *config.Config) config.SyntaxErrorMode {
	if cfg.SyntaxErrors == "" {
		return config.SyntaxErrorFail
	}
	return cfg.SyntaxErrors
}

// FormatWithDiagnostics is Format that also reports the statements which do not
// parse. With syntax_errors: fail the output is empty whenever diagnostics exist;
// with skip the output contains those statements verbatim; with ignore nothing is
// checked. A non-nil error means the input could not even be tokenised.
func FormatWithDiagnostics(sql string, cfg *config.Config) (string, []SyntaxError, error) {
	if strings.TrimSpace(sql) == "" {
		return sql, nil, nil
	}
	// The formatter works on LF only; the requested line ending is applied at the end.
	eol := detectEOL(sql)
	masked, regions := maskVerbatimRegions(normalizeEOL(sql))
	out, diags, err := formatDiagnostics(masked, cfg)
	if err != nil || out == "" {
		return out, shiftDiagnostics(diags, regions), err
	}
	if len(regions) > 0 {
		if out, err = restoreVerbatimRegions(out, regions); err != nil {
			return "", nil, err
		}
		diags = shiftDiagnostics(diags, regions)
	}
	return applyEOL(out, cfg.LineEndings, eol), diags, nil
}

// formatDiagnostics is FormatWithDiagnostics for LF-only input and output.
func formatDiagnostics(sql string, cfg *config.Config) (string, []SyntaxError, error) {
	mode := syntaxMode(cfg)
	if mode == config.SyntaxErrorIgnore {
		out, err := formatTolerant(sql, cfg)
		return out, nil, err
	}

	// Fast path: a script that parses as a whole is formatted as a whole.
	parsed, perr := pg_query.Parse(sql)
	if perr == nil {
		out, err := formatParsed(sql, cfg, parsed)
		return out, nil, err
	}

	chunks, err := splitStatements(sql)
	if err != nil {
		// Unterminated string/comment/dollar quote: statement boundaries are unknowable,
		// so even skip mode cannot isolate the damage.
		return "", nil, SyntaxErrors{syntaxErrorAt(sql, 0, perr)}
	}

	var diags []SyntaxError
	valid := make([]bool, len(chunks))
	for i, ch := range chunks {
		if ch.meta {
			valid[i] = true
			continue
		}
		res, err := pg_query.Parse(ch.text)
		if err != nil {
			diags = append(diags, syntaxErrorAt(ch.text, ch.start, err, sql))
			continue
		}
		chunks[i].parsed = res
		valid[i] = true
	}
	if len(diags) == 0 {
		// Only psql meta-commands made the whole script unparsable.
		return formatChunks(chunks, valid, cfg), nil, nil
	}
	if mode == config.SyntaxErrorFail {
		return "", diags, nil
	}
	return formatChunks(chunks, valid, cfg), diags, nil
}

// chunk is one statement (with its leading whitespace/comments and its terminating
// semicolon) or one psql meta-command line of the input.
type chunk struct {
	start, end int // byte range in the input
	text       string
	meta       bool                  // psql backslash command: left verbatim, never an error
	parsed     *pg_query.ParseResult // set once the chunk is known to parse
}

// splitStatements splits sql into chunks at top-level semicolons. psql meta-commands
// (\set, \connect, …) at the start of a statement become their own verbatim chunks.
func splitStatements(sql string) ([]chunk, error) {
	res, err := pg_query.Scan(sql)
	if err != nil {
		return nil, err
	}
	toks := res.Tokens
	var chunks []chunk
	pos := 0
	depth := 0
	atomic := 0 // inside BEGIN ATOMIC … END: semicolons do not end the statement
	caseDepth := 0
	startOfStmt := true
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		text := sql[t.Start:t.End]
		lower := strings.ToLower(text)
		isComment := strings.HasPrefix(text, "--") || strings.HasPrefix(text, "/*")

		if startOfStmt && !isComment && text == "\\" && atLineStart(sql, int(t.Start)) {
			// psql meta-command: runs to the end of the line.
			end := strings.IndexByte(sql[t.Start:], '\n')
			if end < 0 {
				end = len(sql)
			} else {
				end += int(t.Start) + 1
			}
			// Statements before the command (comments) stay with the command chunk.
			chunks = append(chunks, chunk{start: pos, end: end, text: sql[pos:end], meta: true})
			pos = end
			for i+1 < len(toks) && int(toks[i+1].Start) < end {
				i++
			}
			continue
		}
		if !isComment {
			startOfStmt = false
		}
		switch {
		case text == "(" || text == "[":
			depth++
		case text == ")" || text == "]":
			if depth > 0 {
				depth--
			}
		case lower == "atomic" && i > 0 && strings.EqualFold(sql[toks[i-1].Start:toks[i-1].End], "begin"):
			atomic++
			caseDepth = 0
		case atomic > 0 && lower == "case":
			caseDepth++
		case atomic > 0 && lower == "end":
			if caseDepth > 0 {
				caseDepth--
			} else {
				atomic--
			}
		case text == ";" && depth == 0 && atomic == 0:
			end := int(t.End)
			chunks = append(chunks, chunk{start: pos, end: end, text: sql[pos:end]})
			pos = end
			startOfStmt = true
		}
	}
	if pos < len(sql) {
		chunks = append(chunks, chunk{start: pos, end: len(sql), text: sql[pos:]})
	}
	return chunks, nil
}

// atLineStart reports whether only blanks precede offset on its line.
func atLineStart(sql string, offset int) bool {
	for i := offset - 1; i >= 0; i-- {
		switch sql[i] {
		case '\n':
			return true
		case ' ', '\t', '\r':
		default:
			return false
		}
	}
	return true
}

// syntaxErrorAt converts a parser error for text (which starts at byte base of the
// full input) into a SyntaxError with line and column in the full input. The optional
// full argument is the whole input; it defaults to text.
func syntaxErrorAt(text string, base int, err error, full ...string) SyntaxError {
	whole := text
	if len(full) > 0 {
		whole = full[0]
	}
	msg := err.Error()
	off := 0
	if pe, ok := err.(*parser.Error); ok {
		msg = pe.Message
		if pe.Cursorpos > 0 {
			// Cursorpos is a 1-based character index into text.
			n := 0
			for i := range text {
				if n == pe.Cursorpos-1 {
					off = i
					break
				}
				n++
				off = len(text)
			}
		}
	}
	abs := base + off
	if abs > len(whole) {
		abs = len(whole)
	}
	line := 1 + strings.Count(whole[:abs], "\n")
	col := 1 + utf8.RuneCountInString(whole[strings.LastIndexByte(whole[:abs], '\n')+1:abs])
	return SyntaxError{Offset: abs, Line: line, Col: col, Message: msg}
}

// formatChunks formats the valid chunks one by one, copies the others verbatim and
// applies the whole-text settings to the result.
func formatChunks(chunks []chunk, valid []bool, cfg *config.Config) string {
	per := *cfg
	per.BlankLines = config.BlankLinesPreserve
	per.Semicolons = config.SemicolonPreserve
	per.TrailingNewline = config.TrailingNewlinePreserve

	var b strings.Builder
	lastValid := true
	for i, ch := range chunks {
		lastValid = valid[i]
		if !valid[i] || ch.meta {
			b.WriteString(ch.text)
			continue
		}
		out, err := formatParsed(ch.text, &per, ch.parsed)
		if err != nil {
			out = ch.text
		}
		b.WriteString(out)
	}
	return finishWholeText(b.String(), cfg, lastValid)
}
