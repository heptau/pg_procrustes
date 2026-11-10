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

// check mode: stmt_matrix -check <config> <file.sql> prints what is wrong with the file formatted
// with the config (unparseable, not-idempotent, data-changed, tree-changed) or "ok".
func check(cfgPath, file string) {
	c, err := config.Load(cfgPath)
	if err != nil {
		fmt.Println("config-error")
		return
	}
	b, _ := os.ReadFile(file)
	in := string(b)
	out, err := formatter.Format(in, c)
	switch {
	case err != nil:
		fmt.Println("input-error")
	case func() bool { _, e := pg_query.Parse(out); return e != nil }():
		fmt.Println("unparseable")
	case func() bool { o2, e := formatter.Format(out, c); return e != nil || o2 != out }():
		fmt.Println("not-idempotent")
	case !sameConstants(in, out):
		fmt.Println("data-changed")
	case !strings.Contains(in, "$") && func() bool {
		x, e1 := pg_query.Fingerprint(in)
		y, e2 := pg_query.Fingerprint(out)
		return e1 == nil && e2 == nil && x != y
	}():
		fmt.Println("tree-changed")
	default:
		fmt.Println("ok")
	}
}

func main() {
	if os.Args[1] == "-check" {
		check(os.Args[2], os.Args[3])
		return
	}
	dir := os.Args[1]
	max, _ := strconv.Atoi(os.Args[2])
	cfgs := map[string]*config.Config{}
	fpCfg := map[string]bool{} // "fp:file.yaml": the output must keep the parse tree (configs that rewrite no tokens)
	var names []string
	for _, p := range os.Args[3:] {
		if strings.HasPrefix(p, "fp:") {
			p = strings.TrimPrefix(p, "fp:")
			fpCfg[p] = true
		}
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
			if t == "" || len(t) > 6000 || strings.HasPrefix(strings.ToUpper(t), "DO ") {
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
						tag := "$$"
						if strings.Contains(j.text, "$$") {
							tag = "$w$" // (the statement has dollar-quoted text of its own)
						}
						in = "DO " + tag + "\nBEGIN\n" + j.text + ";\nEND\n" + tag + ";\n"
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
					} else if !sameConstants(in, out) {
						kind = "data-changed"
					} else if fpCfg[n] && !wrap && !strings.Contains(in, "$") { // (a body of code is formatted: its text changes)
						a, e1 := pg_query.Fingerprint(in)
						b, e2 := pg_query.Fingerprint(out)
						if e1 == nil && e2 == nil && a != b {
							kind = "tree-changed"
						}
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

// constants lists the string constants of sql as written (single-quoted strings, dollar-quoted
// texts that are no function body; the strings inside function bodies, which are code).
func constants(sql string) []string {
	res, err := pg_query.Scan(sql)
	if err != nil {
		return nil
	}
	word := func(i int) string {
		if i < 0 || i >= len(res.Tokens) {
			return ""
		}
		return strings.ToLower(sql[res.Tokens[i].Start:res.Tokens[i].End])
	}
	var out []string
	for k, t := range res.Tokens {
		text := sql[t.Start:t.End]
		switch {
		case strings.HasPrefix(text, "$"):
			prev := word(k - 1)
			doLang := k >= 3 && word(k-2) == "language" && word(k-3) == "do"
			code := prev == "as" || prev == "do" || doLang
			if code {
				lo, hi := k, k
				for lo > 0 && word(lo-1) != ";" {
					lo--
				}
				for hi < len(res.Tokens) && word(hi) != ";" {
					hi++
				}
				for i := lo; i+1 < hi; i++ {
					if word(i) == "language" {
						l := strings.Trim(word(i+1), `'"`)
						code = l == "sql" || l == "plpgsql"
						break
					}
				}
			}
			if code {
				if i := strings.IndexByte(text[1:], '$'); i >= 0 {
					open := i + 2
					if end := len(text) - open; end > open {
						out = append(out, constants(text[open:end])...)
					}
				}
			} else {
				out = append(out, text)
			}
		case strings.HasPrefix(text, "'") || len(text) > 1 && strings.ContainsRune("eEbBxXnN", rune(text[0])) && text[1] == '\'' || strings.HasPrefix(text, "U&'") || strings.HasPrefix(text, "u&'"):
			out = append(out, text)
		}
	}
	return out
}

func sameConstants(a, b string) bool {
	x, y := constants(a), constants(b)
	if len(x) != len(y) {
		return false
	}
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}
