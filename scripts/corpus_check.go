//go:build ignore

// Runs the formatter over a folder of real .sql files with every template and checks that
// the output still parses and that formatting twice gives the same text. Nothing is written
// to the folder. Usage:
//
//	go run scripts/corpus_check.go [-t postgresql,ai,modern] [-o report.txt] <dir>
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	pg_query "github.com/pganalyze/pg_query_go/v6"

	"github.com/heptau/pg_procrustes/config"
	"github.com/heptau/pg_procrustes/formatter"
)

var fingerprint *bool

type problem struct {
	kind, template, file, detail string
}

func main() {
	tmpls := flag.String("t", "postgresql,ai,modern,none", "templates to run (names of files in templates/; \"none\" = empty config, which must not change anything)")
	report := flag.String("o", "", "write the list of problems here")
	fingerprint = flag.Bool("fp", false, "also require the same pg_query fingerprint (the parse tree without whitespace) before and after; only for configs that rewrite no tokens")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: corpus_check [-t names] [-o report] <dir>")
		os.Exit(2)
	}
	var files []string
	filepath.WalkDir(flag.Arg(0), func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(strings.ToLower(p), ".sql") {
			files = append(files, p)
		}
		return nil
	})
	sort.Strings(files)

	cfgs := map[string]*config.Config{}
	var names []string
	for _, n := range strings.Split(*tmpls, ",") {
		path := filepath.Join("templates", n+".yaml")
		if strings.HasSuffix(n, ".yaml") {
			path = n
		}
		if n == "none" {
			path = "/nonexistent/.pg_procrustes.yaml"
		}
		cfg, err := config.Load(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, n, err)
			os.Exit(1)
		}
		cfgs[n] = cfg
		names = append(names, n)
	}

	var mu sync.Mutex
	var problems []problem
	counts := map[string]int{}
	add := func(kind, tmpl, file, detail string) {
		mu.Lock()
		problems = append(problems, problem{kind, tmpl, file, detail})
		counts[kind+"/"+tmpl]++
		mu.Unlock()
	}

	start := time.Now()
	jobs := make(chan string)
	var wg sync.WaitGroup
	skippedInput := 0
	for w := 0; w < runtime.NumCPU(); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for f := range jobs {
				data, err := os.ReadFile(f)
				if err != nil {
					continue
				}
				src := string(data)
				if _, err := pg_query.Parse(src); err != nil {
					mu.Lock()
					skippedInput++
					mu.Unlock()
					continue
				}
				for _, n := range names {
					check(f, n, src, cfgs[n], add)
				}
			}
		}()
	}
	for _, f := range files {
		jobs <- f
	}
	close(jobs)
	wg.Wait()

	fmt.Printf("%d files, %d not parseable as given (skipped), templates %v, %s\n", len(files), skippedInput, names, time.Since(start).Round(time.Second))
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		fmt.Println("no problems")
	}
	for _, k := range keys {
		fmt.Printf("  %-28s %d\n", k, counts[k])
	}
	if *report != "" {
		sort.Slice(problems, func(i, j int) bool {
			if problems[i].kind != problems[j].kind {
				return problems[i].kind < problems[j].kind
			}
			return problems[i].file < problems[j].file
		})
		var sb strings.Builder
		for _, p := range problems {
			fmt.Fprintf(&sb, "%s\t%s\t%s\t%s\n", p.kind, p.template, p.file, strings.ReplaceAll(p.detail, "\n", "⏎"))
		}
		os.WriteFile(*report, []byte(sb.String()), 0o644)
	}
}

func check(file, tmpl, src string, cfg *config.Config, add func(kind, tmpl, file, detail string)) {
	defer func() {
		if r := recover(); r != nil {
			add("panic", tmpl, file, fmt.Sprint(r))
		}
	}()
	out, err := formatter.Format(src, cfg)
	if err != nil {
		add("format-error", tmpl, file, err.Error())
		return
	}
	if tmpl == "none" {
		if out != src {
			add("empty-config-changed-input", tmpl, file, firstDiff(src, out))
		}
		return
	}
	if _, err := pg_query.Parse(out); err != nil {
		add("output-unparseable", tmpl, file, err.Error())
		return
	}
	if *fingerprint {
		a, errA := pg_query.Fingerprint(src)
		b, errB := pg_query.Fingerprint(out)
		if errA == nil && errB == nil && a != b {
			add("ast-changed", tmpl, file, "")
		}
	}
	if cfg.LineEndings == "" || cfg.LineEndings == config.LineEndingPreserve {
		// a fingerprint ignores constant values, so compare the text of the string constants
		// themselves: no rule may change what is inside '…'
		if a, b := stringConstants(src), stringConstants(out); a != b {
			add("string-changed", tmpl, file, firstDiff(a, b))
		}
	}
	if n, m := badBodyQueries(src), badBodyQueries(out); m > n {
		add("function-body-broken", tmpl, file, fmt.Sprintf("%d SQL fragments in PL/pgSQL bodies do not parse (input: %d)", m, n))
	}
	again, err := formatter.Format(out, cfg)
	if err != nil {
		add("second-pass-error", tmpl, file, err.Error())
		return
	}
	if again != out {
		add("not-idempotent", tmpl, file, firstDiff(out, again))
	}
}

// stringConstants returns the '…' string constants of sql (dollar-quoted bodies are formatted
// on purpose and left out), sorted, one per line with line breaks written as ⏎.
func stringConstants(sql string) string {
	res, err := pg_query.Scan(sql)
	if err != nil {
		return ""
	}
	var lits []string
	for _, t := range res.Tokens {
		text := sql[t.Start:t.End]
		if strings.HasPrefix(text, "$") {
			continue
		}
		if i := strings.IndexByte(text, '\''); i >= 0 && i <= 2 && strings.TrimLeft(text[:i], "eEbBxXuU&nN") == "" {
			lits = append(lits, strings.ReplaceAll(text, "\n", "⏎"))
		}
	}
	sort.Strings(lits)
	return strings.Join(lits, "\n")
}

func firstDiff(a, b string) string {
	al, bl := strings.Split(a, "\n"), strings.Split(b, "\n")
	for i := 0; i < len(al) && i < len(bl); i++ {
		if al[i] != bl[i] {
			return fmt.Sprintf("line %d: %q → %q", i+1, al[i], bl[i])
		}
	}
	return fmt.Sprintf("length %d → %d lines", len(al), len(bl))
}

// badBodyQueries counts the SQL fragments inside PL/pgSQL function bodies of sql that do not
// parse on their own (a body is an opaque string to the SQL parser, so a statement that was
// commented out or cut in half by the formatter would otherwise go unnoticed).
func badBodyQueries(sql string) int {
	tree, err := pg_query.Parse(sql)
	if err != nil {
		return 0
	}
	bad := 0
	for _, stmt := range tree.Stmts {
		cf := stmt.Stmt.GetCreateFunctionStmt()
		if cf == nil {
			continue
		}
		lang, body := "", ""
		for _, o := range cf.Options {
			de := o.GetDefElem()
			if de == nil {
				continue
			}
			switch de.Defname {
			case "language":
				lang = strings.ToLower(de.Arg.GetString_().GetSval())
			case "as":
				if l := de.Arg.GetList(); l != nil && len(l.Items) > 0 {
					body = l.Items[0].GetString_().GetSval()
				}
			}
		}
		if lang != "plpgsql" || body == "" {
			continue
		}
		js, err := pg_query.ParsePlPgSqlToJSON("create function f() returns void language plpgsql as $b$" + body + "$b$")
		if err != nil {
			if os.Getenv("CBDEBUG") != "" {
				fmt.Fprintf(os.Stderr, "BAD BODY: %v\n", err)
			}
			bad++ // the body itself does not parse
			continue
		}
		var v any
		if json.Unmarshal([]byte(js), &v) != nil {
			continue
		}
		var walk func(any)
		walk = func(x any) {
			switch t := x.(type) {
			case map[string]any:
				if q, ok := t["query"].(string); ok {
					if _, err := pg_query.Parse(q); err != nil {
						// assignments are stored as "x := expr" (or "x = expr"), which is no SQL on its own
						_, err2 := pg_query.Parse("select " + q)
						if err2 != nil && strings.Contains(q, ":=") {
							_, err2 = pg_query.Parse("select " + strings.Replace(q, ":=", "=", 1))
						}
						if err2 != nil {
							if os.Getenv("CBDEBUG") != "" {
								fmt.Fprintf(os.Stderr, "BAD FRAGMENT: %q\n", q)
							}
							bad++
						}
					}
				}
				for _, c := range t {
					walk(c)
				}
			case []any:
				for _, c := range t {
					walk(c)
				}
			}
		}
		walk(v)
	}
	return bad
}
