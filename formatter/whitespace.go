package formatter

import (
	"strings"
	"unicode"
	"unicode/utf8"

	pg_query "github.com/pganalyze/pg_query_go/v6"

	"github.com/heptau/pg_procrustes/config"
)

// detectEOL returns "\r\n" when most line breaks of sql are CRLF, else "\n".
func detectEOL(sql string) string {
	crlf := strings.Count(sql, "\r\n")
	lf := strings.Count(sql, "\n") - crlf
	if crlf > lf {
		return "\r\n"
	}
	return "\n"
}

// normalizeEOL converts every CRLF to LF; the formatter works on LF only.
func normalizeEOL(sql string) string { return strings.ReplaceAll(sql, "\r\n", "\n") }

// applyEOL converts LF-only text to the requested line ending. detected is the style
// of the input, used by line_endings: preserve.
func applyEOL(text string, mode config.LineEndingMode, detected string) string {
	eol := detected
	switch mode {
	case config.LineEndingLF:
		eol = "\n"
	case config.LineEndingCRLF:
		eol = "\r\n"
	}
	if eol == "\r\n" {
		return strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\n", "\r\n")
	}
	return text
}

// spaceAfterLineCommentMarker turns "--note" into "-- note". Separator lines
// ("-----"), markers such as "--+" or "--!" and comments that already start with a
// blank are left alone.
func spaceAfterLineCommentMarker(text string) string {
	if !strings.HasPrefix(text, "--") || len(text) == 2 {
		return text
	}
	r, _ := utf8.DecodeRuneInString(text[2:])
	if unicode.IsLetter(r) || unicode.IsDigit(r) {
		return "-- " + text[2:]
	}
	return text
}

// alignTrailingComments pads the code in front of trailing "--" comments so that the
// comments of consecutive lines start in the same column. A run needs at least two
// lines; runs whose code contains a tab are left alone (their width is ambiguous).
func alignTrailingComments(text string) string {
	if !strings.Contains(text, "--") {
		return text
	}
	res, err := pg_query.Scan(text)
	if err != nil {
		return text
	}
	type trailing struct {
		line      int
		codeEnd   int // byte offset just after the code
		cmtStart  int
		width     int
		hasTab    bool
		lineStart int
	}
	var found []trailing
	lineStarts := []int{0}
	for i := 0; i < len(text); i++ {
		if text[i] == '\n' {
			lineStarts = append(lineStarts, i+1)
		}
	}
	lineOf := func(off int) int {
		lo, hi := 0, len(lineStarts)-1
		for lo < hi {
			mid := (lo + hi + 1) / 2
			if lineStarts[mid] <= off {
				lo = mid
			} else {
				hi = mid - 1
			}
		}
		return lo
	}
	for _, t := range res.Tokens {
		start := int(t.Start)
		if !strings.HasPrefix(text[start:t.End], "--") {
			continue
		}
		ln := lineOf(start)
		ls := lineStarts[ln]
		code := strings.TrimRight(text[ls:start], " \t")
		if strings.TrimSpace(code) == "" {
			continue // comment on a line of its own
		}
		found = append(found, trailing{
			line: ln, codeEnd: ls + len(code), cmtStart: start,
			width: utf8.RuneCountInString(code), hasTab: strings.Contains(code, "\t"), lineStart: ls,
		})
	}
	if len(found) < 2 {
		return text
	}
	var b strings.Builder
	cur := 0
	for i := 0; i < len(found); {
		j := i + 1
		for j < len(found) && found[j].line == found[j-1].line+1 {
			j++
		}
		run := found[i:j]
		i = j
		if len(run) < 2 {
			continue
		}
		target, tabs := 0, false
		for _, f := range run {
			if f.width > target {
				target = f.width
			}
			tabs = tabs || f.hasTab
		}
		if tabs {
			continue
		}
		target++ // one space between the longest code and its comment
		for _, f := range run {
			b.WriteString(text[cur:f.codeEnd])
			b.WriteString(strings.Repeat(" ", target-f.width))
			cur = f.cmtStart
		}
	}
	b.WriteString(text[cur:])
	return b.String()
}

// separateStatements puts exactly n blank lines between statements: after the ';' of a
// statement (and a comment on the same line), the line breaks before whatever follows
// are replaced by n+1 line breaks. psql meta-commands and the text after the last
// statement are left as written.
func separateStatements(text string, n int) string {
	chunks, err := splitStatements(text)
	if err != nil || len(chunks) < 2 {
		return text
	}
	var b strings.Builder
	for i, ch := range chunks {
		s := ch.text
		if i > 0 && !chunks[i-1].meta && strings.HasSuffix(strings.TrimRight(chunks[i-1].text, " \t\r\n"), ";") {
			s = normalizeLead(s, n)
		}
		b.WriteString(s)
	}
	return b.String()
}

// breakSameLineStatements starts every statement that shares a line with the previous
// one on a line of its own. A comment on the same line as the previous ';' stays with
// that statement; blank lines and statements that already start a line are untouched.
func breakSameLineStatements(text string) string {
	chunks, err := splitStatements(text)
	if err != nil || len(chunks) < 2 {
		return text
	}
	var b strings.Builder
	for i, ch := range chunks {
		s := ch.text
		if i > 0 && !chunks[i-1].meta && strings.HasSuffix(strings.TrimRight(chunks[i-1].text, " \t\r\n"), ";") {
			s = breakLead(s)
		}
		b.WriteString(s)
	}
	return b.String()
}

func breakLead(s string) string {
	if strings.TrimSpace(s) == "" {
		return s
	}
	p := 0
	for p < len(s) && (s[p] == ' ' || s[p] == '\t') {
		p++
	}
	keep := ""
	if strings.HasPrefix(s[p:], "--") {
		return s // the comment ends the line, whatever follows is on the next one
	} else if strings.HasPrefix(s[p:], "/*") {
		end := strings.Index(s[p:], "*/")
		if end >= 0 && !strings.Contains(s[p:p+end], "\n") {
			keep = s[:p+end+2]
			s = s[p+end+2:]
		}
	}
	q := 0
	for q < len(s) && (s[q] == ' ' || s[q] == '\t') {
		q++
	}
	if q < len(s) && s[q] != '\n' && s[q] != '\r' {
		return keep + "\n" + s[q:]
	}
	return keep + s
}

// normalizeLead rewrites the whitespace at the start of a statement chunk (after the
// previous statement's ';') so that exactly n blank lines precede the statement.
func normalizeLead(s string, n int) string {
	if strings.TrimSpace(s) == "" {
		return s // nothing follows: trailing whitespace is trailing_newline's business
	}
	p := 0
	for p < len(s) && (s[p] == ' ' || s[p] == '\t') {
		p++
	}
	keep := ""
	// A comment on the same line as the previous ';' stays attached to it.
	if strings.HasPrefix(s[p:], "--") {
		end := strings.IndexByte(s[p:], '\n')
		if end < 0 {
			return s
		}
		keep = s[:p+end]
		s = s[p+end:]
	} else if strings.HasPrefix(s[p:], "/*") {
		end := strings.Index(s[p:], "*/")
		if end >= 0 && !strings.Contains(s[p:p+end], "\n") {
			keep = s[:p+end+2]
			s = s[p+end+2:]
		}
	}
	q := 0
	for q < len(s) && (s[q] == ' ' || s[q] == '\t' || s[q] == '\n') {
		q++
	}
	if q == len(s) {
		return keep + s
	}
	indent := s[strings.LastIndexByte(s[:q], '\n')+1 : q]
	if !strings.Contains(s[:q], "\n") {
		indent = "" // statements that shared a line start at column 0
	}
	return keep + strings.Repeat("\n", n+1) + indent + s[q:]
}

// collapseTrailingCommentGap leaves exactly one space between code and a trailing
// "--" comment (comments on a line of their own are untouched).
func collapseTrailingCommentGap(text string) string {
	if !strings.Contains(text, "--") {
		return text
	}
	res, err := pg_query.Scan(text)
	if err != nil {
		return text
	}
	var b strings.Builder
	cur := 0
	for _, t := range res.Tokens {
		start := int(t.Start)
		if !strings.HasPrefix(text[start:t.End], "--") {
			continue
		}
		ls := strings.LastIndexByte(text[:start], '\n') + 1
		code := strings.TrimRight(text[ls:start], " \t")
		if strings.TrimSpace(code) == "" {
			continue // comment on a line of its own
		}
		codeEnd := ls + len(code)
		if start-codeEnd == 1 && text[codeEnd] == ' ' {
			continue
		}
		b.WriteString(text[cur:codeEnd])
		b.WriteByte(' ')
		cur = start
	}
	b.WriteString(text[cur:])
	return b.String()
}

// compactGaps re-applies the "compact" spacing rules to gaps that layout created when it
// re-joined lines ("a,\n b" → "a, b", "x\n=\ny" → "x = y"): comma_spacing / operator_spacing:
// compact must hold for the final text, not just for the token pass that ran before the
// layout. A comma at the end of a line is left alone; only blanks (never line breaks) go.
func compactGaps(text string, commas, operators bool) string {
	if !(commas && strings.Contains(text, ",")) && !operators {
		return text
	}
	res, err := pg_query.Scan(text)
	if err != nil {
		return text
	}
	var b strings.Builder
	cur := 0
	toks := res.Tokens
	for i := 0; i+1 < len(toks); i++ {
		t, next := toks[i], toks[i+1]
		tt, nt := text[t.Start:t.End], text[next.Start:next.End]
		drop := commas && tt == ","
		if operators && (isSymbolicOp(tt) || isSymbolicOp(nt)) && tt != "(" && nt != ")" {
			drop = true
		}
		if !drop {
			continue
		}
		gapStart, gapEnd := int(t.End), int(next.Start)
		if gapEnd <= gapStart || !isOnlyHorizontalWS(text[gapStart:gapEnd]) || gapStart < cur {
			continue
		}
		b.WriteString(text[cur:gapStart])
		cur = gapEnd
	}
	b.WriteString(text[cur:])
	return b.String()
}
