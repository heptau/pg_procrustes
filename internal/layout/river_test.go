package layout_test

import (
	"testing"

	"github.com/heptau/pg_procrustes/config"
	"github.com/heptau/pg_procrustes/internal/layout"
)

func riverCfg() *config.LayoutConfig {
	cfg := caseCfg(config.BreakPreserve, config.BodyIndentIndent, 128)
	cfg.Clauses.Break = config.BreakAlways
	cfg.Clauses.Align = config.AlignRight
	cfg.Content = config.ContentCfg{Break: config.BreakAlways, Align: config.AlignSame, FirstItem: config.FirstItemInline}
	return cfg
}

func TestRiverSelect(t *testing.T) {
	sql := "SELECT a.id, a.name FROM accounts a WHERE a.id > 1 AND a.active GROUP BY a.id, a.name ORDER BY a.name"
	// the longest keyword (GROUP BY, ORDER BY: 8) sets the edge of the river
	want := "  SELECT a.id,\n" +
		"         a.name\n" +
		"    FROM accounts a\n" +
		"   WHERE a.id > 1\n" +
		"         AND a.active\n" +
		"GROUP BY a.id,\n" +
		"         a.name\n" +
		"ORDER BY a.name"
	assertLayoutFormat(t, sql, riverCfg(), want)
}

func TestRiverJoinHangsInContentColumn(t *testing.T) {
	sql := "SELECT a.id FROM accounts a LEFT JOIN orders o ON o.account_id = a.id WHERE a.id > 1"
	want := "SELECT a.id\n" +
		"  FROM accounts a\n" +
		"       LEFT JOIN orders o ON o.account_id = a.id\n" +
		" WHERE a.id > 1"
	assertLayoutFormat(t, sql, riverCfg(), want)
}

func TestRiverUpdateInsert(t *testing.T) {
	assertLayoutFormat(t, "UPDATE t SET a = 1, b = 2 WHERE id = 3", riverCfg(),
		"UPDATE t\n   SET a = 1,\n       b = 2\n WHERE id = 3")
	assertLayoutFormat(t, "INSERT INTO t (a, b) VALUES (1, 2)", riverCfg(),
		"INSERT INTO t (a,\n               b)\n     VALUES (1, 2)")
}

func TestRiverEachQueryOfAUnionHasItsOwnWidth(t *testing.T) {
	assertLayoutFormat(t, "SELECT 1 UNION SELECT 2 ORDER BY 1", riverCfg(),
		"SELECT 1 UNION\n  SELECT 2\nORDER BY 1")
}

func TestRiverIsIdempotent(t *testing.T) {
	cases := []string{
		"SELECT a.id, a.name FROM accounts a LEFT JOIN o ON o.a = a.id AND o.x WHERE a.id > 1 GROUP BY a.id ORDER BY a.name;\nSELECT 1 FROM t;\nUPDATE t SET a = 1 WHERE b = 2;",
		"BEGIN;\n    SELECT a, b FROM t WHERE x = 1 ORDER BY a;\n    INSERT INTO t (a, b) VALUES (1, 2);\nCOMMIT;",
		"SELECT CASE WHEN a THEN 1 ELSE 2 END AS c, b FROM t",
	}
	for _, sql := range cases {
		cfg := riverCfg()
		cfg.KeepStatementIndent = true
		one, err := layout.Apply(sql, cfg)
		if err != nil {
			t.Fatal(err)
		}
		two, err := layout.Apply(one, cfg)
		if err != nil {
			t.Fatal(err)
		}
		if one != two {
			t.Errorf("not idempotent:\nfirst:\n%s\nsecond:\n%s", one, two)
		}
	}
}

func TestRiverLeavesUnbrokenClausesAlone(t *testing.T) {
	cfg := riverCfg()
	cfg.Clauses.Break = config.BreakNever
	sql := "SELECT a FROM t WHERE a > 1"
	assertLayoutFormat(t, sql, cfg, sql)
}

// A comment in front of the query stands on a line of its own above the first keyword, which is
// on the river like those of a query without one (the comment kept it off the river before, and
// a subquery body that starts with a comment was laid out differently on the next run).
func TestRiverQueryAfterLeadingComment(t *testing.T) {
	sql := "-- why\nSELECT a.id, a.name FROM accounts a WHERE a.id > 1"
	want := "-- why\n" +
		"SELECT a.id,\n" +
		"       a.name\n" +
		"  FROM accounts a\n" +
		" WHERE a.id > 1"
	assertLayoutFormat(t, sql, riverCfg(), want)
	cfg := riverCfg()
	cfg.Subquery.Break = config.BreakAlways
	cfg.ParenIndent.Open = config.ParenOpenOwnLine
	applyIdem(t, "WITH v AS (\n  -- why\n  SELECT a.id, a.name FROM accounts a WHERE a.id > 1\n)\nSELECT * FROM v", cfg)
}
