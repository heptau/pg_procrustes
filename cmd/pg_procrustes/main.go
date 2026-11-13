package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/heptau/pg_procrustes/config"
	"github.com/heptau/pg_procrustes/formatter"
	"github.com/heptau/pg_procrustes/internal/lsp"
)

// version is set at build time via -ldflags "-X main.version=..."
// Falls back to "dev" when built without the flag (e.g. go run).
var version = "dev"

// reorderArgs moves all flag arguments (and their values) before positional
// arguments so that Go's flag package — which stops at the first non-flag arg —
// still parses flags regardless of where the user placed them on the command line.
func reorderArgs(args []string) []string {
	var flags, positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(a, "-") {
			positional = append(positional, a)
			continue
		}
		// -flag=value or --flag=value: single token, no next arg consumed.
		if strings.Contains(a, "=") {
			flags = append(flags, a)
			continue
		}
		// Boolean flags have no value; value flags consume the next token.
		// We recognise our own boolean flags so we don't accidentally consume
		// a positional argument as a flag value.
		name := strings.TrimLeft(a, "-")
		// --backup has an optional value (default .bak): the next token is its value only when
		// it looks like an extension and is not a file ("--backup .orig"), so that
		// "-w --backup query.sql" does not take the file for the extension.
		if name == "backup" {
			if i+1 < len(args) && isBackupExt(args[i+1]) {
				i++
				flags = append(flags, a+"="+args[i])
			} else {
				flags = append(flags, a+"=.bak")
			}
			continue
		}
		switch name {
		case "w", "check", "diff", "v", "version", "print-config", "list-templates", "install-templates", "lsp":
			flags = append(flags, a)
		default:
			// Assume it takes a value; grab the next token if present.
			flags = append(flags, a)
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				flags = append(flags, args[i])
			}
		}
	}
	return append(flags, positional...)
}

// isBackupExt reports whether s can be the value of --backup given as a separate argument:
// ".ext" that is not an existing file or directory.
func isBackupExt(s string) bool {
	if len(s) < 2 || s[0] != '.' || strings.ContainsAny(s[1:], `./\*?[`) {
		return false
	}
	_, err := os.Stat(s)
	return err != nil
}

// releaseVersion is the version of this build: the -ldflags value, else the module version
// `go install …@vX.Y.Z` records, else "dev".
func releaseVersion() string {
	if version != "dev" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && strings.HasPrefix(bi.Main.Version, "v") && !strings.Contains(bi.Main.Version, "-") {
		return bi.Main.Version
	}
	return "dev"
}

// installTemplates implements --install-templates.
func installTemplates(from string) {
	if from == "" {
		from = config.TemplatesURL(releaseVersion())
	}
	dest, err := config.UserTemplateDir()
	if err != nil {
		die("install-templates: %v", err)
	}
	fmt.Fprintf(os.Stderr, "installing templates from %s\n", from)
	got, err := config.InstallTemplates(from, dest)
	if err != nil {
		die("install-templates: %v", err)
	}
	for _, t := range got {
		note := "installed"
		if t.Replaced {
			note = "updated"
		}
		fmt.Printf("%-16s %s (%s)\n", t.Name, t.Path, note)
	}
}

// printTemplates lists the installed templates and where they come from.
func printTemplates() {
	found := config.ListTemplates()
	if len(found) == 0 {
		fmt.Println("no templates found; looked in:")
		for _, d := range config.TemplateDirs() {
			fmt.Println("  " + d)
		}
		return
	}
	for _, name := range config.SortedTemplateNames(found) {
		fmt.Printf("%-16s %s\n", name, found[name])
	}
}

// printEffectiveConfig prints every option with the value that is in effect, preceded by
// the files it came from.
func printEffectiveConfig(cfg *config.Config) {
	fmt.Printf("# pg_procrustes %s — effective configuration\n", releaseVersion())
	if len(cfg.Sources) == 0 {
		fmt.Println("# no config file found: built-in defaults (every rule is preserve)")
	} else {
		fmt.Println("# files, in the order they were applied (later ones win):")
		for _, f := range cfg.Sources {
			fmt.Println("#   " + f)
		}
	}
	out, err := cfg.YAML()
	if err != nil {
		die("print-config: %v", err)
	}
	fmt.Print(string(out))
}

func main() {
	var (
		writeInPlace  bool
		checkMode     bool
		diffMode      bool
		showVersion   bool
		configPath    string
		backupExt     string
		outDir        string
		syntaxErrors  string
		printConfig   bool
		listTemplates bool
		installTmpls  bool
		lspMode       bool
		templatesFrom string
		linesRange    string
		stdinFilepath string
		rangeStart    int
		rangeEnd      int
	)
	flag.BoolVar(&writeInPlace, "w", false, "write result back to source files (in-place)")
	flag.BoolVar(&checkMode, "check", false, "exit 1 if any file would be reformatted (CI mode)")
	flag.BoolVar(&diffMode, "diff", false, "print unified diff of changes without writing")
	flag.BoolVar(&showVersion, "version", false, "print version and exit")
	flag.BoolVar(&showVersion, "v", false, "print version and exit")
	flag.StringVar(&configPath, "c", "", "path to config file (default: the .pg_procrustes.yaml nearest to each file, then to the working directory)")
	flag.StringVar(&backupExt, "backup", "", "before overwriting, save original with this extension (e.g. .bak); requires -w")
	flag.StringVar(&outDir, "out-dir", "", "write formatted files into this directory instead of stdout")
	flag.BoolVar(&printConfig, "print-config", false, "print the effective configuration (templates resolved, every option) and exit")
	flag.BoolVar(&listTemplates, "list-templates", false, "list the installed templates and exit")
	flag.BoolVar(&installTmpls, "install-templates", false, "download the templates of this version from the GitHub release into your user templates directory and exit")
	flag.BoolVar(&lspMode, "lsp", false, "run as a Language Server (LSP over stdio): formatting, range formatting and syntax-error diagnostics")
	flag.StringVar(&templatesFrom, "templates-from", "", "with --install-templates: take the templates from this URL (zip), .zip file or directory instead")
	flag.StringVar(&syntaxErrors, "syntax-errors", "", "what to do with SQL that does not parse: fail | skip | ignore (default: syntax_errors from the config, else fail)")
	flag.StringVar(&linesRange, "lines", "", "format only the statements that touch lines START:END (or one line N) of a single input; the rest is copied as written")
	flag.StringVar(&stdinFilepath, "stdin-filepath", "", "when reading stdin: the path the text belongs to; the config is looked up from its directory (the file need not exist)")
	flag.Usage = usage
	os.Args = append(os.Args[:1], reorderArgs(os.Args[1:])...)
	flag.Parse()

	if showVersion {
		fmt.Printf("pg_procrustes %s\n", releaseVersion())
		return
	}

	if writeInPlace && checkMode {
		die("-w and --check are mutually exclusive")
	}
	if writeInPlace && diffMode {
		die("-w and --diff are mutually exclusive")
	}
	if outDir != "" && writeInPlace {
		die("-w and --out-dir are mutually exclusive")
	}
	if outDir != "" && checkMode {
		die("--check and --out-dir are mutually exclusive")
	}
	if backupExt != "" && !writeInPlace {
		die("--backup requires -w")
	}
	if backupExt != "" && !strings.HasPrefix(backupExt, ".") {
		backupExt = "." + backupExt
	}
	if outDir != "" {
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			die("out-dir: %v", err)
		}
	}

	// A config file named with -c must exist (a typo must not silently mean "no config").
	if configPath != "" {
		if info, err := os.Stat(configPath); err != nil {
			die("config: %v", err)
		} else if info.IsDir() {
			die("config: %s is a directory", configPath)
		}
	}

	if lspMode {
		os.Exit(lsp.Serve(os.Stdin, os.Stdout, lsp.Options{ConfigPath: configPath, Version: releaseVersion(), Log: os.Stderr}))
	}
	if listTemplates {
		printTemplates()
		return
	}
	if installTmpls {
		installTemplates(templatesFrom)
		return
	}

	// Expand globs from args.
	files, err := expandInputs(flag.Args())
	if err != nil {
		die("%v", err)
	}

	// The config is the one nearest to each file (so it does not depend on the directory the
	// program runs in); -c picks one for everything. Several files often share a config.
	cache := map[string]*config.Config{}
	configFor := func(file string) *config.Config {
		path := configPath
		if path == "" && file != "" {
			path = config.ConfigPathFor(file)
		}
		if c, ok := cache[path]; ok {
			return c
		}
		c, err := config.Load(path)
		if err != nil {
			die("config: %v", err)
		}
		if syntaxErrors != "" {
			c.SyntaxErrors = config.SyntaxErrorMode(syntaxErrors)
			if err := config.Validate(c); err != nil {
				die("--syntax-errors: %v", err)
			}
		}
		cache[path] = c
		return c
	}

	if printConfig {
		target := stdinFilepath
		if len(files) > 0 {
			target = files[0].path
		}
		printEffectiveConfig(configFor(target))
		return
	}
	if stdinFilepath != "" && len(files) > 0 {
		die("--stdin-filepath is for reading stdin; give either files or --stdin-filepath")
	}

	if linesRange != "" {
		rangeStart, rangeEnd, err = parseLines(linesRange)
		if err != nil {
			die("--lines: %v", err)
		}
		if len(files) > 1 {
			die("--lines works on a single file or on stdin")
		}
		if outDir != "" {
			die("--lines and --out-dir are mutually exclusive")
		}
		formatSource = func(name, src string, cfg *config.Config) (string, bool) {
			return formatRangeSource(name, src, cfg, rangeStart, rangeEnd)
		}
	}

	// Two inputs must not land on the same file below --out-dir (a/x.sql and b/x.sql given
	// as files both go to x.sql): the second would silently replace the first.
	if outDir != "" {
		if len(files) == 0 {
			die("--out-dir needs files or directories to format (stdin is written to stdout)")
		}
		if a, b, dest, clash := outDirClash(outDir, files); clash {
			die("--out-dir: %s and %s would both be written to %s; give their common directory instead of the files", a, b, dest)
		}
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
		out, ok := formatSource("stdin", src, configFor(stdinFilepath))
		if !ok {
			os.Exit(1)
		}
		switch {
		case checkMode:
			if out != src {
				if diffMode {
					fmt.Print(unifiedDiff("<stdin>", src, out))
				}
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
	for _, in := range files {
		path := in.path
		src, out, ok := readAndFormat(path, configFor(path))
		if !ok {
			exitCode = 1
			continue
		}

		switch {
		case checkMode:
			if out != src {
				if diffMode {
					fmt.Print(unifiedDiff(path, src, out))
				}
				fmt.Fprintf(os.Stderr, "pg_procrustes: %s: would be reformatted\n", path)
				exitCode = 1
			}
		case diffMode:
			if out != src {
				fmt.Print(unifiedDiff(path, src, out))
			}
		case outDir != "":
			dest := filepath.Join(outDir, in.rel)
			if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
				fmt.Fprintf(os.Stderr, "pg_procrustes: %s: %v\n", dest, err)
				exitCode = 1
				continue
			}
			if err := os.WriteFile(dest, []byte(out), 0o644); err != nil {
				fmt.Fprintf(os.Stderr, "pg_procrustes: %s: %v\n", dest, err)
				exitCode = 1
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
			if backupExt != "" {
				if err := os.WriteFile(path+backupExt, []byte(src), info.Mode()); err != nil {
					fmt.Fprintf(os.Stderr, "pg_procrustes: %s: backup: %v\n", path, err)
					exitCode = 1
					continue
				}
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

// outDirClash finds two inputs that would be written to the same file below outDir.
func outDirClash(outDir string, files []input) (first, second, dest string, clash bool) {
	dests := map[string]string{}
	for _, in := range files {
		d := filepath.Clean(filepath.Join(outDir, in.rel))
		if other, ok := dests[d]; ok {
			return other, in.path, d, true
		}
		dests[d] = in.path
	}
	return "", "", "", false
}

// readAndFormat reads path and returns (original, formatted, ok). Problems are
// reported on stderr; ok is false when the file must not be written or checked.
func readAndFormat(path string, cfg *config.Config) (src, out string, ok bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pg_procrustes: %s: %v\n", path, err)
		return "", "", false
	}
	src = string(data)
	out, ok = formatSource(path, src, cfg)
	return src, out, ok
}

// formatSource formats src and reports syntax problems on stderr in the usual
// "file:line:col: message" form with the offending line and a caret. With
// syntax_errors: fail the result is not ok; with skip the statements that do not
// parse are left as written and reported as warnings.
func defaultFormatSource(name, src string, cfg *config.Config) (string, bool) {
	return formatRangeSource(name, src, cfg, 0, 0)
}

// formatSource is what the CLI uses to format one input; --lines swaps in a range version.
var formatSource = defaultFormatSource

// parseLines reads "START:END" or "N" (1-based, inclusive).
func parseLines(s string) (int, int, error) {
	a, b, found := strings.Cut(s, ":")
	start, err := strconv.Atoi(strings.TrimSpace(a))
	if err != nil || start < 1 {
		return 0, 0, fmt.Errorf("want START:END or N (1-based line numbers), got %q", s)
	}
	end := start
	if found {
		if end, err = strconv.Atoi(strings.TrimSpace(b)); err != nil || end < start {
			return 0, 0, fmt.Errorf("want START:END with END >= START, got %q", s)
		}
	}
	return start, end, nil
}

// formatRangeSource formats src — as a whole when start is 0, else only the statements
// that touch lines start:end — and reports syntax problems like formatSource did before.
func formatRangeSource(name, src string, cfg *config.Config, start, end int) (string, bool) {
	var (
		out   string
		diags []formatter.SyntaxError
		err   error
	)
	if start > 0 {
		out, diags, err = formatter.FormatRange(src, cfg, start, end)
	} else {
		out, diags, err = formatter.FormatWithDiagnostics(src, cfg)
	}
	var fatal formatter.SyntaxErrors
	if errors.As(err, &fatal) {
		reportSyntax(name, src, fatal, false)
		return "", false
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "pg_procrustes: %s: %v\n", name, err)
		return "", false
	}
	if len(diags) > 0 {
		if cfg.SyntaxErrors == config.SyntaxErrorSkip {
			reportSyntax(name, src, diags, true)
			return out, true
		}
		reportSyntax(name, src, diags, false)
		return "", false
	}
	return out, true
}

// reportSyntax prints each diagnostic as "name:line:col: [warning: ]message", followed
// by the source line and a caret under the error column.
func reportSyntax(name, src string, diags []formatter.SyntaxError, warn bool) {
	// A byte order mark has no width: without it the caret stands under the right column.
	lines := strings.Split(strings.TrimPrefix(src, "\uFEFF"), "\n")
	kind := "error"
	if warn {
		kind = "warning: statement left unformatted"
	}
	for _, d := range diags {
		fmt.Fprintf(os.Stderr, "%s:%d:%d: %s: %s\n", name, d.Line, d.Col, kind, d.Message)
		if d.Line >= 1 && d.Line <= len(lines) {
			line := strings.TrimRight(lines[d.Line-1], "\r")
			fmt.Fprintf(os.Stderr, "    %s\n", line)
			// Keep tabs in the padding so the caret lines up in the terminal.
			pad := []rune{}
			for i, r := range []rune(line) {
				if i >= d.Col-1 {
					break
				}
				if r == '\t' {
					pad = append(pad, '\t')
				} else {
					pad = append(pad, ' ')
				}
			}
			fmt.Fprintf(os.Stderr, "    %s^\n", string(pad))
		}
	}
}

// unifiedDiff produces a unified diff between original and formatted: hunks with 3 lines
// of context before and after each change (changes closer than that share a hunk), which
// patch(1) and git apply accept.
func unifiedDiff(name, original, formatted string) string {
	edits := lcsEdits(splitLines(original), splitLines(formatted))

	var b strings.Builder
	fmt.Fprintf(&b, "--- %s\n", name)
	fmt.Fprintf(&b, "+++ %s\n", name)

	const ctxLines = 3
	// aBefore[i] / bBefore[i]: the number of lines of each side before edit i.
	aBefore := make([]int, len(edits)+1)
	bBefore := make([]int, len(edits)+1)
	for i, e := range edits {
		aBefore[i+1], bBefore[i+1] = aBefore[i], bBefore[i]
		if e.op != opInsert {
			aBefore[i+1]++
		}
		if e.op != opDelete {
			bBefore[i+1]++
		}
	}
	for i := 0; i < len(edits); {
		if edits[i].op == opEqual {
			i++
			continue
		}
		// The hunk runs from ctxLines before this change to ctxLines after the last change
		// that is followed by more than 2*ctxLines unchanged lines (or by the end).
		lo := max(i-ctxLines, 0)
		last := i // last changed edit of the hunk
		for j := i + 1; j < len(edits) && j-last <= 2*ctxLines; j++ {
			if edits[j].op != opEqual {
				last = j
			}
		}
		hi := min(last+1+ctxLines, len(edits))

		aCount, bCount := aBefore[hi]-aBefore[lo], bBefore[hi]-bBefore[lo]
		aStart, bStart := aBefore[lo]+1, bBefore[lo]+1
		if aCount == 0 { // an empty range names the line before it
			aStart--
		}
		if bCount == 0 {
			bStart--
		}
		fmt.Fprintf(&b, "@@ -%d,%d +%d,%d @@\n", aStart, aCount, bStart, bCount)
		for _, e := range edits[lo:hi] {
			b.WriteByte(" -+"[e.op])
			b.WriteString(e.text)
			if !strings.HasSuffix(e.text, "\n") {
				b.WriteString("\n\\ No newline at end of file\n")
			}
		}
		i = hi
	}
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
	// The unchanged lines at both ends need no table (it has len(a)*len(b) cells).
	pre := 0
	for pre < len(a) && pre < len(b) && a[pre] == b[pre] {
		pre++
	}
	suf := 0
	for suf < len(a)-pre && suf < len(b)-pre && a[len(a)-1-suf] == b[len(b)-1-suf] {
		suf++
	}
	head, tail := a[:pre], a[len(a)-suf:]
	a, b = a[pre:len(a)-suf], b[pre:len(b)-suf]

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
	all := make([]edit, 0, len(head)+len(edits)+len(tail))
	for _, l := range head {
		all = append(all, edit{opEqual, l})
	}
	all = append(all, edits...)
	for _, l := range tail {
		all = append(all, edit{opEqual, l})
	}
	return all
}

func usage() {
	fmt.Fprintln(os.Stderr, `pg_procrustes — PostgreSQL SQL formatter

Usage:
  pg_procrustes [flags] [file ...]

Reads SQL files and normalizes them according to the configuration file.
With no file arguments reads from stdin. A directory is searched recursively
for *.sql files, glob patterns are expanded ("migrations/*.sql") and ** matches
any number of directories ("migrations/**/*.sql", quote it so the shell leaves
it alone). Hidden directories and node_modules are not entered.

Flags:
  -v, --version      print version and exit
  -c <path>          path to config file (default: the .pg_procrustes.yaml nearest to each file, looking in
                     the file's directory and its parents; then in the working directory and its parents)
  -w                 write result back to source files (in-place)
  --backup[=.ext]    save original before overwriting (requires -w); default extension is .bak
  --out-dir <dir>    write formatted files into this directory, keeping their path below the searched
                     directory (cannot be combined with -w)
  --check            exit 1 if any file would be reformatted (CI / pre-commit use)
                     (with --diff the changes are printed as well)
  --diff             print unified diff of changes without writing
  --syntax-errors <m> what to do with SQL that does not parse: fail (default), skip, ignore
                     fail:   report every bad statement (file:line:col) and exit 1
                     skip:   format the valid statements, leave bad ones as written, warn
                     ignore: format everything at token level without checking
  --lines <a:b>      format only the statements that touch lines a..b (or one line N) of a single
                     file or of stdin; everything else is copied as written (for editors' range formatting)
  --stdin-filepath <path>  when reading stdin: the path the text belongs to; the config is looked up from
                     that file's directory (the file need not exist), not from the working directory
  --lsp              run as a Language Server (LSP over stdio) for VS Code, Zed, Neovim, Helix, …:
                     document and range formatting, syntax errors as diagnostics; the config is looked up
                     per document (-c fixes one for all)
  --print-config     print the effective configuration (templates resolved, every option) and exit
  --install-templates  download the templates of this version from the GitHub release into
                     your user templates directory (for go install; Homebrew and the archives already carry them)
  --templates-from <src>  with --install-templates: a URL (zip), .zip file or directory to take them from
  --list-templates   list the installed templates (use one with "template: name" in the config) and exit

Examples:
  pg_procrustes query.sql
  pg_procrustes -w *.sql
  pg_procrustes -w sql/
  pg_procrustes --check 'migrations/**/*.sql'
  pg_procrustes -w --backup=.orig migrations/**/*.sql
  pg_procrustes --out-dir formatted/ *.sql
  pg_procrustes --check *.sql
  pg_procrustes --diff query.sql
  pg_procrustes --lines 10:25 -w query.sql
  cat query.sql | pg_procrustes --stdin-filepath db/migrations/001.sql`)
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "pg_procrustes: "+format+"\n", args...)
	os.Exit(1)
}
