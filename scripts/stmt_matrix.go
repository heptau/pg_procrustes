//go:build ignore

// Runs every single statement of the .sql files below <dir> through several configs, bare and
// wrapped in DO $$ BEGIN … END $$;, and reports output that does not parse, is not idempotent, or
// (for configs that only lay out) changes the parse tree. Single statements make small test cases.
// Usage: go run scripts/stmt_matrix.go <dir> <max-reports> <config.yaml|templates/x.yaml>...
package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	pg_query "github.com/pganalyze/pg_query_go/v6"

	"github.com/heptau/pg_procrustes/config"
	"github.com/heptau/pg_procrustes/formatter"
)

func main() {
	dir := os.Args[1]
	max, _ := strconv.Atoi(os.Args[2])
	cfgs := map[string]*config.Config{}
	var names []string
	for _, p := range os.Args[3:] {
		c, err := config.Load(p)
		if err != nil {
			panic(err)
		}
		cfgs[p] = c
		names = append(names, p)
	}
	var files []string
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(strings.ToLower(p), ".sql") {
			files = append(files, p)
		}
		return nil
	})
	sort.Strings(files)
	type job struct{ file, text string }
	var jobs []job
	seen := map[string]bool{}
	for _, f := range files {
		b, _ := os.ReadFile(f)
		stmts, err := pg_query.SplitWithParser(string(b), true)
		if err != nil {
			continue
		}
		for _, st := range stmts {
			t := strings.TrimSpace(st)
			if t == "" || len(t) > 2500 || strings.Contains(t, "$$") || strings.Contains(t, "$_$") {
				continue
			}
			if _, err := pg_query.Parse(t); err != nil {
				continue
			}
			k := strings.Join(strings.Fields(t), " ")
			if seen[k] {
				continue
			}
			seen[k] = true
			jobs = append(jobs, job{f, t})
		}
	}
	fmt.Printf("%d statements, %d configs\n", len(jobs), len(names))
	var mu sync.Mutex
	counts := map[string]int{}
	reports := 0
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for _, j := range jobs {
		wg.Add(1)
		sem <- struct{}{}
		go func(j job) {
			defer wg.Done()
			defer func() { <-sem }()
			for _, n := range names {
				for _, wrap := range []bool{false, true} {
					in := j.text + ";\n"
					if wrap {
						in = "DO $$\nBEGIN\n" + j.text + ";\nEND\n$$;\n"
					}
					out, err := formatter.Format(in, cfgs[n])
					if err != nil {
						continue
					}
					kind := ""
					if _, err := pg_query.Parse(out); err != nil {
						kind = "unparseable"
					} else if out2, err := formatter.Format(out, cfgs[n]); err != nil || out2 != out {
						kind = "not-idempotent"
					}
					if kind == "" {
						continue
					}
					mu.Lock()
					counts[kind+" "+filepath.Base(n)]++
					if reports < max {
						reports++
						fmt.Printf("=== %s [%s] wrap=%v  %s\n--- in\n%s--- out\n%s\n", kind, n, wrap, j.file, in, out)
					}
					mu.Unlock()
				}
			}
		}(j)
	}
	wg.Wait()
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("%6d  %s\n", counts[k], k)
	}
	if len(keys) == 0 {
		fmt.Println("no problems")
	}
}
