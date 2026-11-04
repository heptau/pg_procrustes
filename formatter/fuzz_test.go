package formatter_test

import (
	"testing"

	pg_query "github.com/pganalyze/pg_query_go/v6"

	"github.com/heptau/pg_procrustes/config"
	"github.com/heptau/pg_procrustes/formatter"
)

// FuzzFormat checks, for inputs PostgreSQL can parse, that formatting never
// panics, keeps the SQL parseable with the same AST fingerprint, and is idempotent.
//
//	go test ./formatter -run '^$' -fuzz FuzzFormat -fuzztime 30s
func FuzzFormat(f *testing.F) {
	for _, s := range matrixInline {
		f.Add(s)
	}
	f.Add("select 1")
	f.Add("select a from t where b = 1 and c = 2 order by a limit 1")

	var variants []matrixVariant
	for _, v := range matrixVariants {
		switch v.name {
		case "clauses_content_always", "arith_normalize", "arith_compact", "operators_compact", "capitalize_with_layout":
			variants = append(variants, v)
		}
	}
	f.Fuzz(func(t *testing.T, src string) {
		if len(src) > 2000 {
			t.Skip()
		}
		if _, err := pg_query.Parse(src); err != nil {
			t.Skip() // only parseable SQL has a defined semantic to preserve
		}
		base, _ := config.Load("/nonexistent/.pg_procrustes.yaml")
		baseOut, err := formatter.Format(src, base)
		if err != nil {
			t.Skip()
		}
		baseFP, err := pg_query.Fingerprint(baseOut)
		if err != nil {
			t.Fatalf("baseline output does not parse: %v\n--- in ---\n%s\n--- out ---\n%s", err, src, baseOut)
		}
		for _, v := range variants {
			cfg, _ := config.Load("/nonexistent/.pg_procrustes.yaml")
			v.apply(cfg)
			out, err := formatter.Format(src, cfg)
			if err != nil {
				t.Fatalf("%s: Format: %v", v.name, err)
			}
			fp, err := pg_query.Fingerprint(out)
			if err != nil {
				t.Fatalf("%s: output does not parse: %v\n--- in ---\n%s\n--- out ---\n%s", v.name, err, src, out)
			}
			if fp != baseFP {
				t.Fatalf("%s: AST changed\n--- in ---\n%s\n--- out ---\n%s", v.name, src, out)
			}
			out2, err := formatter.Format(out, cfg)
			if err != nil || out2 != out {
				t.Fatalf("%s: not idempotent (err=%v)\n--- first ---\n%s\n--- second ---\n%s", v.name, err, out, out2)
			}
		}
	})
}
