//go:build ignore

// Runs the formatter over a folder of real .sql files with every template and checks that
// the output still parses and that formatting twice gives the same text. Nothing is written
// to the folder. Usage:
//
//	go run scripts/corpus_check.go [-t postgresql,ai,modern] [-o report.txt] <dir>
package main

import (
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
	again, err := formatter.Format(out, cfg)
	if err != nil {
		add("second-pass-error", tmpl, file, err.Error())
		return
	}
	if again != out {
		add("not-idempotent", tmpl, file, firstDiff(out, again))
	}
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
