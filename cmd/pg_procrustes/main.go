package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/heptau/pg_procrustes/internal/config"
	"github.com/heptau/pg_procrustes/internal/formatter"
)

// version is set at build time via -ldflags "-X main.version=..."
// Falls back to "dev" when built without the flag (e.g. go run).
var version = "dev"

func main() {
	var (
		writeInPlace bool
		checkMode    bool
		diffMode     bool
		showVersion  bool
		configPath   string
	)
	flag.BoolVar(&writeInPlace, "w", false, "write result back to source files (in-place)")
	flag.BoolVar(&checkMode, "check", false, "exit 1 if any file would be reformatted (CI mode)")
	flag.BoolVar(&diffMode, "diff", false, "print unified diff of changes without writing")
	flag.BoolVar(&showVersion, "version", false, "print version and exit")
	flag.BoolVar(&showVersion, "v", false, "print version and exit")
	flag.StringVar(&configPath, "c", "", "path to config file (default: auto-detect .procrustes.yaml)")
	flag.Usage = usage
	flag.Parse()

	if showVersion {
		fmt.Printf("pg_procrustes %s\n", version)
		return
	}

	if writeInPlace && checkMode {
		die("-w and --check are mutually exclusive")
	}
	if writeInPlace && diffMode {
		die("-w and --diff are mutually exclusive")
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		die("config: %v", err)
	}

	// Expand globs from args.
	files, err := expandGlobs(flag.Args())
	if err != nil {
		die("%v", err)
	}

	// No files → read from stdin.
	if len(files) == 0 {
		if writeInPlace {
			fmt.Fprintln(os.Stderr, "pg_procrustes: -w has no effect when reading from stdin")
		}
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			die("read stdin: %v", err)
		}
		src := string(data)
		out, err := formatter.Format(src, cfg)
		if err != nil {
			die("%v", err)
		}
		switch {
		case checkMode:
			if out != src {
				fmt.Fprintln(os.Stderr, "pg_procrustes: stdin would be reformatted")
				os.Exit(1)
			}
		case diffMode:
			if out != src {
				fmt.Print(unifiedDiff("<stdin>", src, out))
			}
		default:
			fmt.Print(out)
		}
		return
	}

	exitCode := 0
	for _, path := range files {
		src, out, err := readAndFormat(path, cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "pg_procrustes: %s: %v\n", path, err)
			exitCode = 1
			continue
		}

		switch {
		case checkMode:
			if out != src {
				fmt.Fprintf(os.Stderr, "pg_procrustes: %s: would be reformatted\n", path)
				exitCode = 1
			}
		case diffMode:
			if out != src {
				fmt.Print(unifiedDiff(path, src, out))
			}
		case writeInPlace:
			if out == src {
				continue
			}
			info, err := os.Stat(path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "pg_procrustes: %s: %v\n", path, err)
				exitCode = 1
				continue
			}
			if err := os.WriteFile(path, []byte(out), info.Mode()); err != nil {
				fmt.Fprintf(os.Stderr, "pg_procrustes: %s: %v\n", path, err)
				exitCode = 1
				continue
			}
		default:
			fmt.Print(out)
		}
	}
	os.Exit(exitCode)
}

// expandGlobs expands glob patterns and returns deduplicated matched paths.
// Patterns that match nothing are kept as-is so the open error surfaces naturally.
func expandGlobs(args []string) ([]string, error) {
	var out []string
	seen := make(map[string]bool)
	for _, arg := range args {
		matches, err := filepath.Glob(arg)
		if err != nil {
			return nil, fmt.Errorf("invalid glob %q: %w", arg, err)
		}
		if len(matches) == 0 {
			if !seen[arg] {
				seen[arg] = true
				out = append(out, arg)
			}
			continue
		}
		for _, m := range matches {
			if !seen[m] {
				seen[m] = true
				out = append(out, m)
			}
		}
	}
	return out, nil
}

// readAndFormat reads path and returns (original, formatted, err).
func readAndFormat(path string, cfg *config.Config) (src, out string, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	src = string(data)
	out, err = formatter.Format(src, cfg)
	return src, out, err
}

// unifiedDiff produces a minimal unified diff between original and formatted.
func unifiedDiff(name, original, formatted string) string {
	origLines := splitLines(original)
	fmtLines := splitLines(formatted)

	var b strings.Builder
	fmt.Fprintf(&b, "--- %s\n", name)
	fmt.Fprintf(&b, "+++ %s\n", name)

	edits := lcsEdits(origLines, fmtLines)

	// Walk edits and emit hunks with 3 lines of context.
	const ctxLines = 3
	aLine, bLine := 1, 1
	// Collect pending hunk lines before emitting the header.
	type pending struct {
		aStart, bStart int
		lines          []string
		aCount, bCount int
	}
	var cur *pending

	flush := func() {
		if cur == nil {
			return
		}
		fmt.Fprintf(&b, "@@ -%d,%d +%d,%d @@\n", cur.aStart, cur.aCount, cur.bStart, cur.bCount)
		for _, l := range cur.lines {
			b.WriteString(l)
		}
		cur = nil
	}

	// trailing context lines buffered after last change
	trailingCtx := 0

	for _, e := range edits {
		switch e.op {
		case opEqual:
			if cur != nil {
				if trailingCtx < ctxLines {
					cur.lines = append(cur.lines, " "+e.text)
					cur.aCount++
					cur.bCount++
					trailingCtx++
				} else {
					flush()
				}
			}
			aLine++
			bLine++
		case opDelete:
			if cur == nil {
				startA := max(aLine-ctxLines, 1)
				startB := max(bLine-ctxLines, 1)
				cur = &pending{aStart: startA, bStart: startB}
			}
			cur.lines = append(cur.lines, "-"+e.text)
			cur.aCount++
			trailingCtx = 0
			aLine++
		case opInsert:
			if cur == nil {
				startA := max(aLine-ctxLines, 1)
				startB := max(bLine-ctxLines, 1)
				cur = &pending{aStart: startA, bStart: startB}
			}
			cur.lines = append(cur.lines, "+"+e.text)
			cur.bCount++
			trailingCtx = 0
			bLine++
		}
	}
	flush()
	return b.String()
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.SplitAfter(s, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

type opKind int

const (
	opEqual  opKind = iota
	opDelete        // in original, not in formatted
	opInsert        // in formatted, not in original
)

type edit struct {
	op   opKind
	text string
}

// lcsEdits computes an edit script between a and b using LCS.
func lcsEdits(a, b []string) []edit {
	m, n := len(a), len(b)
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if a[i-1] == b[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else if dp[i-1][j] >= dp[i][j-1] {
				dp[i][j] = dp[i-1][j]
			} else {
				dp[i][j] = dp[i][j-1]
			}
		}
	}
	var edits []edit
	i, j := m, n
	for i > 0 || j > 0 {
		switch {
		case i > 0 && j > 0 && a[i-1] == b[j-1]:
			edits = append(edits, edit{opEqual, a[i-1]})
			i--
			j--
		case j > 0 && (i == 0 || dp[i][j-1] >= dp[i-1][j]):
			edits = append(edits, edit{opInsert, b[j-1]})
			j--
		default:
			edits = append(edits, edit{opDelete, a[i-1]})
			i--
		}
	}
	for lo, hi := 0, len(edits)-1; lo < hi; lo, hi = lo+1, hi-1 {
		edits[lo], edits[hi] = edits[hi], edits[lo]
	}
	return edits
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func usage() {
	fmt.Fprintln(os.Stderr, `pg_procrustes — PostgreSQL SQL formatter

Usage:
  pg_procrustes [flags] [file ...]

Reads SQL files and normalizes them according to the configuration file.
With no file arguments reads from stdin. Glob patterns are expanded
(e.g. "migrations/*.sql").

Flags:
  -v, --version  print version and exit
  -c <path>      path to config file (default: auto-detect .procrustes.yaml)
  -w             write result back to source files (in-place)
  --check        exit 1 if any file would be reformatted (CI / pre-commit use)
  --diff         print unified diff of changes without writing

Examples:
  pg_procrustes query.sql
  pg_procrustes -w *.sql
  pg_procrustes -w migrations/**/*.sql
  pg_procrustes --check *.sql
  pg_procrustes --diff query.sql
  cat query.sql | pg_procrustes`)
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "pg_procrustes: "+format+"\n", args...)
	os.Exit(1)
}
