package layout_test

import (
	"strings"
	"testing"

	"github.com/heptau/pg_procrustes/config"
	"github.com/heptau/pg_procrustes/internal/layout"
)

func TestWrapExpressions(t *testing.T) {
	cfg := mkCfg(config.BreakAlways, config.BreakAlways, 40)
	cfg.Content.FirstItem = config.FirstItemBreak
	cfg.Content.WrapExpressions = config.WrapWrap
	in := "SELECT coalesce(first_name_column, last_name_column, 'unknown value') AS n, id FROM t WHERE x IN (1, 2) AND y IN ('aaaaaaaaaaaa', 'bbbbbbbbbbbb', 'cccccccccccc')"
	got, err := layout.Apply(in, cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"  coalesce(\n    first_name_column,\n    last_name_column,\n    'unknown value'\n  ) AS n,", "  AND y IN (\n    'aaaaaaaaaaaa',"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if !strings.Contains(got, "x IN (1, 2)") {
		t.Errorf("short item was wrapped:\n%s", got)
	}
	if again, _ := layout.Apply(got, cfg); again != got {
		t.Errorf("not idempotent:\n%s\n---\n%s", got, again)
	}
	cfg.Content.WrapExpressions = config.WrapPreserve
	if p, _ := layout.Apply(in, cfg); strings.Contains(p, "coalesce(\n") {
		t.Errorf("preserve wrapped:\n%s", p)
	}
	// Subqueries and CASE are never split.
	cfg.Content.WrapExpressions = config.WrapWrap
	sub := "SELECT 1 FROM t WHERE a IN (SELECT aaaaaaaaaaaaaaa, bbbbbbbbbbbbbbbbb, cccccccccccccc FROM u)"
	if got, _ := layout.Apply(sub, cfg); strings.Contains(got, "IN (\n") {
		t.Errorf("subquery wrapped:\n%s", got)
	}
}
