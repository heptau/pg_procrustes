//go:build ignore

// Differential check: each statement of the files below <dir> is formatted at the top level and
// wrapped in DO $$ BEGIN … END $$; the statement inside the body must read the same
// (whitespace aside). Usage: go run scripts/body_diff.go <config.yaml> <dir> [max-reports]
package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	pg_query "github.com/pganalyze/pg_query_go/v6"

	"github.com/heptau/pg_procrustes/config"
	"github.com/heptau/pg_procrustes/formatter"
)

func squash(s string) string { return strings.Join(strings.Fields(s), " ") }

func main() {
	cfg, err := config.Load(os.Args[1])
	if err != nil {
		panic(err)
	}
	max := 60
	if len(os.Args) > 3 {
		max, _ = strconv.Atoi(os.Args[3])
	}
	var files []string
	filepath.WalkDir(os.Args[2], func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(strings.ToLower(p), ".sql") {
			files = append(files, p)
		}
		return nil
	})
	sort.Strings(files)
	seen := map[string]bool{}
	total, bad := 0, 0
	for _, f := range files {
		b, _ := os.ReadFile(f)
		src := string(b)
		stmts, err := pg_query.SplitWithParser(src, true)
		if err != nil {
			continue
		}
		for _, st := range stmts {
			text := strings.TrimSpace(st)
			if text == "" || strings.Contains(text, "$$") || strings.Contains(text, "$_$") || strings.Contains(text, "--") || strings.Contains(text, "/*") {
				continue
			}
			if _, err := pg_query.Parse(text); err != nil {
				continue
			}
			key := squash(text)
			if seen[key] || len(text) > 1500 {
				continue
			}
			seen[key] = true
			top, err := formatter.Format(text+";", cfg)
			if err != nil {
				continue
			}
			wrapped, err := formatter.Format("DO $$\nBEGIN\n"+text+";\nEND\n$$;", cfg)
			if err != nil {
				continue
			}
			i := strings.Index(wrapped, "BEGIN")
			j := strings.LastIndex(wrapped, "END")
			if i < 0 || j < i {
				continue
			}
			inner := wrapped[i+5 : j]
			total++
			a, c := squash(strings.TrimSuffix(strings.TrimSpace(top), ";")), squash(strings.TrimSuffix(strings.TrimSpace(inner), ";"))
			if a != c {
				bad++
				if bad <= max {
					fmt.Printf("--- %s\nTOP : %s\nBODY: %s\n", f, a, c)
				}
			}
		}
	}
	fmt.Printf("\n%d statements compared, %d differ\n", total, bad)
}
