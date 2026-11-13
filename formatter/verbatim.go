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
// A line that only looks like a directive because it is a line of a multi-line string
// constant, quoted identifier or /* … */ comment is data and starts no region.

var directiveRE = regexp.MustCompile(`(?i)^[ \t]*(?:--|/\*)[ \t]*pg_procrustes:[ \t]*(off|on)[ \t]*(?:\*/)?[ \t]*$`)

const verbatimPrefix = "-- __pgp_verbatim_"

type verbatimRegion struct {
	text       string // the original lines, without the final line break
	maskedLine int    // 1-based line of the placeholder in the masked text
	maskedOff  int    // byte offset of the placeholder in the masked text
	lines      int    // number of lines the region had
}

func verbatimPlaceholder(i int) string { return fmt.Sprintf("%s%d__", verbatimPrefix, i) }

// maskVerbatimRegions replaces every off…on region of sql (LF line endings) by a
// placeholder line. It returns sql unchanged when there is none.
func maskVerbatimRegions(sql string) (string, []verbatimRegion) {
	if !strings.Contains(strings.ToLower(sql), "pg_procrustes:") || strings.Contains(sql, verbatimPrefix) {
		return sql, nil
	}
	lines := strings.Split(sql, "\n")
	var regions []verbatimRegion
	var out strings.Builder
	line, off := 1, 0
	for i := 0; i < len(lines); i++ {
		m := directiveRE.FindStringSubmatch(lines[i])
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
