package layout_test

import (
	"strings"
	"testing"

	"github.com/heptau/pg_procrustes/config"
	"github.com/heptau/pg_procrustes/internal/layout"
)

func parenItemCfg() *config.LayoutConfig {
	cfg := caseCfg(config.BreakPreserve, config.BodyIndentPreserve, 128)
	cfg.Indent.Size = 4
	cfg.Clauses.Break = config.BreakAlways
	cfg.Content.Break = config.BreakAlways
	cfg.Content.FirstItem = config.FirstItemBreak
	cfg.ParenIndent.Open = config.ParenOpenOwnLine
	return cfg
}

// A multi-line item that opens a parenthesis (CTE body, VALUES row) is shifted as a whole:
// the `)` stands at the indent of the item, the body one level deeper.
func TestParenItemClosingParenAtItemIndent(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{
			"cte",
			"WITH v AS\n(\n    SELECT 1\n)\nSELECT * FROM v;",
			"WITH\n    v AS\n    (\n        SELECT 1\n    )\nSELECT\n    *\nFROM\n    v;",
		},
		{
			"values rows",
			"INSERT INTO t (a, b)\nVALUES\n(\n    'x',\n    1\n),\n(\n    'y',\n    2\n);",
			"INSERT INTO t\n(\n    a,\n    b\n)\nVALUES\n    (\n        'x',\n        1\n    ),\n    (\n        'y',\n        2\n    );",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := parenItemCfg()
			if c.name == "values rows" {
				cfg.Content.ValuesList = config.ContentRule{Break: config.BreakAlways, Align: config.AlignIndent, FirstItem: config.FirstItemBreak}
			}
			got, err := layout.Apply(c.in, cfg)
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Errorf("\ngot:  %q\nwant: %q", got, c.want)
			}
			again, err := layout.Apply(got, cfg)
			if err != nil {
				t.Fatal(err)
			}
			if again != got {
				t.Errorf("not idempotent:\nfirst:  %q\nsecond: %q", got, again)
			}
		})
	}
}

// A body that stands further left than its closing parenthesis in the input still ends up deeper
// than the parentheses, and a second run changes nothing.
func TestParenItemBodyLeftOfClosingParen(t *testing.T) {
	cfg := parenItemCfg()
	in := "SELECT 1\nFROM t\nWHERE a = 1\n  AND NOT ( /* c */\nx = 1\nAND y = 2\n  );"
	got := applyIdem(t, in, cfg)
	lines := strings.Split(got, "\n")
	var body, closing string
	for _, ln := range lines {
		switch {
		case strings.HasPrefix(strings.TrimSpace(ln), "x = 1"):
			body = ln[:len(ln)-len(strings.TrimLeft(ln, " "))]
		case strings.HasPrefix(strings.TrimSpace(ln), ")"):
			closing = ln[:len(ln)-len(strings.TrimLeft(ln, " "))]
		}
	}
	if len(body) <= len(closing) {
		t.Errorf("body not deeper than the closing parenthesis:\n%s", got)
	}
}

// A subquery among the arguments is left to the subquery layout: no extra shift.
func TestCallArgumentsWithSubqueryNotShifted(t *testing.T) {
	cfg := parenItemCfg()
	in := "SELECT COALESCE(\n    (SELECT 1 FROM t),\n    0) AS x;"
	applyIdem(t, in, cfg)
}
