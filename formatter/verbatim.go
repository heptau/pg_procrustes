package formatter

import (
	"fmt"
	"regexp"
	"strings"

	pg_query "github.com/pganalyze/pg_query_go/v6"
)

// Regions between
//
//	-- pg_procrustes: off
//	-- pg_procrustes: on
//
// (or the same in /* … */), each directive alone on its line, are copied to the output as
// they are — directives included. Without a closing `on` the region runs to the end of the
// input. A stray `on` is an ordinary comment.
//
// The region is swapped for a one-line placeholder comment before formatting and put back
// afterwards, so the rest of the text is formatted as if the region were a comment: put
// the directives between statements (or between the statements of a function body).
//
// The data of COPY … FROM stdin (the lines after the statement up to the `\.` line, or to
// the end of the input) is no SQL: it is treated as such a region too, so a dump made by
// pg_dump can be formatted.
//
// A line that only looks like a directive because it is a line of a multi-line string
// constant, quoted identifier or /* … */ comment is data and starts no region.

var directiveRE = regexp.MustCompile(`(?i)^[ \t]*(?:--|/\*)[ \t]*pg_procrustes:[ \t]*(off|on)[ \t]*(?:\*/)?[ \t]*$`)

const verbatimPrefix = "-- __pgp_verbatim_"

type verbatimRegion struct {
	text       string // the original lines, without the final line break
	maskedLine int    // 1-based line of the placeholder in the masked text
	maskedOff  int    // byte offset of the placeholder in the masked text
	lines      int    // number of lines the region had
	copyData   bool   // data of COPY … FROM stdin: must follow its statement directly
}

func verbatimPlaceholder(i int) string { return fmt.Sprintf("%s%d__", verbatimPrefix, i) }

// maskVerbatimRegions replaces every off…on region of sql (LF line endings) by a
// placeholder line. It returns sql unchanged when there is none.
func maskVerbatimRegions(sql string) (string, []verbatimRegion) {
	lower := strings.ToLower(sql)
	if !(strings.Contains(lower, "pg_procrustes:") || strings.Contains(lower, "stdin")) || strings.Contains(sql, verbatimPrefix) {
		return sql, nil
	}
	lines := strings.Split(sql, "\n")
	copyData := copyDataStarts(sql, lines)
	var regions []verbatimRegion
	var out strings.Builder
	line, off := 1, 0
	for i := 0; i < len(lines); i++ {
		m := directiveRE.FindStringSubmatch(lines[i])
		if last, ok := copyData[i]; ok && !insideOpenToken(out.String()) {
			// Data of COPY … FROM stdin: lines i..last.
			ph := verbatimPlaceholder(len(regions))
			regions = append(regions, verbatimRegion{
				text:       strings.Join(lines[i:last+1], "\n"),
				maskedLine: line,
				maskedOff:  out.Len(),
				lines:      last - i + 1,
				copyData:   true,
			})
			out.WriteString(ph)
			if last < len(lines)-1 {
				out.WriteByte('\n')
			}
			line++
			off += len(ph) + 1
			i = last
			continue
		}
		if m == nil || !strings.EqualFold(m[1], "off") || insideOpenToken(out.String()) {
			out.WriteString(lines[i])
			if i < len(lines)-1 {
				out.WriteByte('\n')
			}
			line++
			off += len(lines[i]) + 1
			continue
		}
		end := len(lines) - 1
		for j := i + 1; j < len(lines); j++ {
			if m2 := directiveRE.FindStringSubmatch(lines[j]); m2 != nil && strings.EqualFold(m2[1], "on") {
				end = j
				break
			}
		}
		ph := verbatimPlaceholder(len(regions))
		regions = append(regions, verbatimRegion{
			text:       strings.Join(lines[i:end+1], "\n"),
			maskedLine: line,
			maskedOff:  out.Len(),
			lines:      end - i + 1,
		})
		out.WriteString(ph)
		if end < len(lines)-1 {
			out.WriteByte('\n')
		}
		line++
		off += len(ph) + 1
		i = end
	}
	return out.String(), regions
}

var copyFromStdinRE = regexp.MustCompile(`(?im)^[ \t]*copy\b[^;]*?\bfrom[ \t\r\n]+stdin\b[^;]*;[ \t]*(?:--[^\n]*)?\n`)

// copyDataStarts finds the COPY … FROM stdin statements of sql (split into lines) and returns,
// for the index of the first data line of each, the index of its last line: the `\.` line, or
// the last line of the text.
func copyDataStarts(sql string, lines []string) map[int]int {
	var res map[int]int
	for _, m := range copyFromStdinRE.FindAllStringIndex(sql, -1) {
		first := strings.Count(sql[:m[1]], "\n") // index of the line after the statement
		if first >= len(lines) {
			continue
		}
		last := len(lines) - 1
		if lines[last] == "" && last > first {
			last-- // the text's final line break is no data
		}
		for j := first; j < len(lines); j++ {
			if strings.TrimRight(lines[j], " \t") == `\.` {
				last = j
				break
			}
		}
		if res == nil {
			res = map[int]int{}
		}
		res[first] = last
	}
	return res
}

// insideOpenToken reports whether the text ends inside a string constant, a quoted
// identifier or a /* … */ comment, i.e. whether the line that follows it is part of that
// token. The inside of a dollar-quoted text does not count: it may be a function body, whose
// directives are honoured.
func insideOpenToken(prefix string) bool {
	if !strings.ContainsAny(prefix, `'"*`) {
		return false
	}
	_, err := pg_query.Scan(prefix)
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "unterminated") && !strings.Contains(msg, "dollar-quoted")
}

// restoreVerbatimRegions puts the regions back where their placeholder lines ended up.
func restoreVerbatimRegions(out string, regions []verbatimRegion) (string, error) {
	for i, r := range regions {
		ph := verbatimPlaceholder(i)
		if r.copyData {
			// The data starts on the line after the statement: a blank line the layout put
			// in front of the placeholder would become a data row.
			out = regexp.MustCompile(`\n(?:[ \t]*\n)+([ \t]*`+regexp.QuoteMeta(ph)+`)`).ReplaceAllString(out, "\n$1")
		}
		re := regexp.MustCompile(`(?m)^[ \t]*` + regexp.QuoteMeta(ph) + `[ \t]*$`)
		if re.MatchString(out) {
			out = re.ReplaceAllLiteralString(out, r.text)
			continue
		}
		if strings.Contains(out, ph) {
			out = strings.Replace(out, ph, r.text, 1)
			continue
		}
		return "", fmt.Errorf("pg_procrustes: off/on region %d was lost while formatting; please report this", i+1)
	}
	return out, nil
}

// shiftDiagnostics maps error positions in the masked text back to the original input.
func shiftDiagnostics(diags []SyntaxError, regions []verbatimRegion) []SyntaxError {
	for k := range diags {
		// The masked position decides which regions lie before the error; the shifts of the
		// earlier regions must not take part in the comparison.
		line, off := diags[k].Line, diags[k].Offset
		for i, r := range regions {
			if r.maskedLine < line {
				diags[k].Line += r.lines - 1
			}
			if r.maskedOff < off {
				diags[k].Offset += len(r.text) - len(verbatimPlaceholder(i))
			}
		}
	}
	return diags
}
