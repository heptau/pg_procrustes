package formatter_test

import (
	"path/filepath"
	"testing"

	pg_query "github.com/pganalyze/pg_query_go/v6"

	"github.com/heptau/pg_procrustes/config"
	"github.com/heptau/pg_procrustes/formatter"
)

// Every shipped template must turn valid SQL into valid SQL, idempotently. (The AST is not
// compared: some options — short type names, dropping public., ASC — legitimately change it.)
func TestShippedTemplatesFormatSafely(t *testing.T) {
	files, _ := filepath.Glob("../templates/*.yaml")
	if len(files) == 0 {
		t.Fatal("no templates found in templates/")
	}
	inputs := matrixInputs(t)
	for _, f := range files {
		cfg, err := config.Load(f)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		for name, src := range inputs {
			if _, err := pg_query.Parse(src); err != nil {
				continue
			}
			t.Run(filepath.Base(f)+"/"+name, func(t *testing.T) {
				out, err := formatter.Format(src, cfg)
				if err != nil {
					t.Fatalf("Format: %v", err)
				}
				if _, err := pg_query.Parse(out); err != nil {
					t.Fatalf("output does not parse: %v\n--- input ---\n%s\n--- output ---\n%s", err, src, out)
				}
				again, err := formatter.Format(out, cfg)
				if err != nil {
					t.Fatalf("second pass: %v", err)
				}
				if again != out {
					t.Errorf("not idempotent\n--- first ---\n%s\n--- second ---\n%s", out, again)
				}
			})
		}
	}
}
