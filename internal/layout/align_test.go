package layout_test

import (
	"testing"

	"github.com/heptau/pg_procrustes/config"
	"github.com/heptau/pg_procrustes/internal/layout"
)

func TestAlignAliasesAndSet(t *testing.T) {
	cfg := mkCfg(config.BreakAlways, config.BreakAlways, 80)
	cfg.Content.FirstItem = config.FirstItemBreak
	cfg.Content.AlignAliases = config.AlignItemsAlign
	cfg.Content.AlignSet = config.AlignItemsAlign
	in := "SELECT a, long_col AS x, count(*) AS c, CASE WHEN a THEN 1 END AS k, cast(b AS int) AS bb FROM t; UPDATE t SET a = 1, long_name = 2, z = f(a, b = 1);"
	got, err := layout.Apply(in, cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"  long_col       AS x,", "  count(*)       AS c,", "  cast(b AS int) AS bb", "  CASE WHEN a THEN 1 END AS k,", "  a         = 1,", "  long_name = 2,", "  z         = f(a, b = 1);"} {
		if !containsLine(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	again, _ := layout.Apply(got, cfg)
	if again != got {
		t.Errorf("not idempotent:\n%s\n---\n%s", got, again)
	}
	// preserve leaves the spacing alone.
	cfg.Content.AlignAliases, cfg.Content.AlignSet = config.AlignItemsPreserve, config.AlignItemsPreserve
	p, _ := layout.Apply("SELECT a AS x, long_col AS y FROM t", cfg)
	if !containsLine(p, "  a AS x,") {
		t.Errorf("preserve changed spacing:\n%s", p)
	}
}

func containsLine(s, line string) bool {
	for _, l := range splitLines(s) {
		if l == line || l == line+"," {
			return true
		}
	}
	return false
}

func splitLines(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == '\n' {
			out = append(out, cur)
			cur = ""
			continue
		}
		cur += string(r)
	}
	return append(out, cur)
}
