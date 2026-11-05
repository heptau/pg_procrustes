package layout_test

import (
	"testing"

	"github.com/heptau/pg_procrustes/config"
	"github.com/heptau/pg_procrustes/internal/layout"
)

// brokenCfg breaks every clause and every list; the position / align options keep
// their defaults (preserve).
func brokenCfg(mod func(*config.LayoutConfig)) *config.LayoutConfig {
	cfg := config.DefaultLayout()
	cfg.Clauses.Break = config.BreakAlways
	cfg.Content.Break = config.BreakAlways
	if mod != nil {
		mod(&cfg)
	}
	return &cfg
}

func applyTwice(t *testing.T, sql string, cfg *config.LayoutConfig) string {
	t.Helper()
	got, err := layout.Apply(sql, cfg)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	again, err := layout.Apply(got, cfg)
	if err != nil {
		t.Fatalf("Apply (second pass): %v", err)
	}
	if again != got {
		t.Errorf("not idempotent\n--- first ---\n%s\n--- second ---\n%s", got, again)
	}
	return got
}

func TestPositionDefaultsArePreserve(t *testing.T) {
	c := config.DefaultLayout().Content
	if c.CommaPosition != config.CommaPreserve || c.LogicalOperatorPosition != config.LogicalOperatorPreserve ||
		c.JoinOnPosition != config.JoinOnPreserve || c.Align != config.AlignPreserve {
		t.Errorf("defaults are not preserve: %+v", c)
	}
}

func TestCommaPositionPreserve(t *testing.T) {
	cfg := brokenCfg(nil)
	if got := applyTwice(t, "select\n   a\n , b\n , c\nfrom t;", cfg); got != "select\n   a\n , b\n , c\nfrom\n   t;" {
		t.Errorf("leading commas not kept:\n%s", got)
	}
	if got := applyTwice(t, "select a, b, c from t;", cfg); got != "select\n   a,\n   b,\n   c\nfrom\n   t;" {
		t.Errorf("trailing commas not used when input has none leading:\n%s", got)
	}
	// an explicit value still wins over the input
	cfg = brokenCfg(func(c *config.LayoutConfig) { c.Content.CommaPosition = config.CommaEnd })
	if got := applyTwice(t, "select\n   a\n , b\nfrom t;", cfg); got != "select\n   a,\n   b\nfrom\n   t;" {
		t.Errorf("end not forced:\n%s", got)
	}
}

func TestLogicalOperatorPositionPreserve(t *testing.T) {
	cfg := brokenCfg(nil)
	if got := applyTwice(t, "select 1 from t where a = 1 and\n  b = 2 or\n  c = 3;", cfg); got != "select\n   1\nfrom\n   t\nwhere\n   a = 1 and\n   b = 2 or\n   c = 3;" {
		t.Errorf("trailing operators not kept:\n%s", got)
	}
	if got := applyTwice(t, "select 1 from t where a = 1 and b = 2;", cfg); got != "select\n   1\nfrom\n   t\nwhere\n   a = 1\n   and b = 2;" {
		t.Errorf("leading operators expected without a signal:\n%s", got)
	}
}

func TestJoinOnPositionPreserve(t *testing.T) {
	cfg := brokenCfg(nil)
	got := applyTwice(t, "select 1 from a\njoin b\n   on b.id = a.id\n   and b.ok;", cfg)
	if want := "select\n   1\nfrom\n   a\njoin b\n   on b.id = a.id\n   and b.ok;"; got != want {
		t.Errorf("ON on its own line not kept:\n%s", got)
	}
	got = applyTwice(t, "select 1 from a join b on b.id = a.id and b.ok;", cfg)
	if want := "select\n   1\nfrom\n   a\njoin\n   b on b.id = a.id\n   and b.ok;"; got != want {
		t.Errorf("ON inline expected without a signal:\n%s", got)
	}
}

func TestContentAlignPreserve(t *testing.T) {
	cfg := brokenCfg(nil)
	if got := applyTwice(t, "select\na,\nb\nfrom\nt\nwhere\nx = 1;", cfg); got != "select\na,\nb\nfrom\nt\nwhere\nx = 1;" {
		t.Errorf("items at keyword level not kept:\n%s", got)
	}
	if got := applyTwice(t, "select a, b\nfrom t\nwhere\n   x = 1;", cfg); got != "select\n   a,\n   b\nfrom\n   t\nwhere\n   x = 1;" {
		t.Errorf("indented items not kept:\n%s", got)
	}
}

func TestWhenActionsPreserve(t *testing.T) {
	cfg := brokenCfg(nil)
	cfg.Content.Break = config.BreakPreserve
	cfg.Clauses.Break = config.BreakAlways
	got := applyTwice(t, "merge into t using s on t.id = s.id when matched then\n   delete when not matched then insert (id) values (s.id);", cfg)
	want := "merge into t\nusing s\non t.id = s.id\nwhen matched then\n   delete\nwhen not matched then insert (id) values (s.id);"
	if got != want {
		t.Errorf("WHEN actions not preserved:\n%s", got)
	}
}

func TestAlignNoneRemovesPadding(t *testing.T) {
	cfg := brokenCfg(func(c *config.LayoutConfig) {
		c.Content.AlignAliases = config.AlignItemsNone
		c.Content.AlignSet = config.AlignItemsNone
	})
	if got := applyTwice(t, "select\n   a        as x,\n   long_col as y\nfrom t;", cfg); got != "select\n   a as x,\n   long_col as y\nfrom\n   t;" {
		t.Errorf("alias padding not removed:\n%s", got)
	}
	if got := applyTwice(t, "update t set\n   a         = 1,\n   long_name = 2;", cfg); got != "update\n   t\nset\n   a = 1,\n   long_name = 2;" {
		t.Errorf("SET padding not removed:\n%s", got)
	}
}
