package formatter_test

import (
	"testing"

	"github.com/heptau/pg_procrustes/config"
	"github.com/heptau/pg_procrustes/formatter"
)

// Shapes found by running real schema files through the shipped templates (and a few option
// combinations): each used to give a different text on the second run.
func TestStabilityOfRealWorldShapes(t *testing.T) {
	own := func(name string) *config.Config {
		c, err := config.Load(name)
		if err != nil {
			t.Fatal(err)
		}
		c.Layout.ParenIndent.Open = config.ParenOpenOwnLine
		return c
	}
	tweak := func(name string, f func(c *config.Config)) func() *config.Config {
		return func() *config.Config {
			c, err := config.Load(name)
			if err != nil {
				t.Fatal(err)
			}
			f(c)
			return c
		}
	}
	cases := []struct {
		name, tmpl string
		cfg        func() *config.Config
		sql        string
	}{
		{"where item with a multi-line subquery, first item inline", "postgresql", nil,
			"CREATE FUNCTION f(_a integer) RETURNS void\n    LANGUAGE sql\n    AS $_$\nUPDATE t\n  SET s = 7\n  WHERE \n  id IN\n  (\n    SELECT op.id\n    FROM t op\n    INNER JOIN e ON e.id = op.id_event\n    WHERE e.l = $1\n    AND op.b = 0\n    AND op.c = 3\n  );\n$_$;\n"},
		{"function name and ( on different lines", "ai", nil,
			"CREATE FUNCTION f() RETURNS void LANGUAGE plpgsql AS $$\nBEGIN\n  SELECT COALESCE\n  ( a->>_l\n  , b\n  ) INTO _x\n  FROM tx;\nEND;\n$$;\n"},
		{"SET (a, b) = (…) over several lines, compact operators", "ai", nil,
			"CREATE FUNCTION f() RETURNS trigger LANGUAGE plpgsql AS $$\nBEGIN\n  UPDATE d.t SET\n    (id, name)\n    =\n    (NEW.id, NEW.name)\n    WHERE id = NEW.id;\n  RETURN NEW;\nEND;\n$$;\n"},
		{"VALUES tuple with a subquery and wrapping", "modern", nil,
			"CREATE FUNCTION f() RETURNS void LANGUAGE plpgsql AS $$\nBEGIN\n  INSERT INTO d.conf\n  (id_sport_region, sport_region_order)\n  VALUES\n  (_newid, (SELECT COALESCE(MAX(sport_region_order), 0) + 1 FROM d.conf src INNER JOIN d.regions sr ON sr.id = src.id WHERE sr.id_sport = _id_sport));\nEND;\n$$;\n"},
		{"INSERT column list with leading commas and ( on its own line", "modern", func() *config.Config { return own("../templates/modern.yaml") },
			"CREATE FUNCTION f() RETURNS int LANGUAGE plpgsql AS $$\nBEGIN\n  INSERT INTO d.params\n    (\n      id_oppty_type,\n      shortcut,\n      name,\n      name_lang,\n      sort_order,\n      template_args_long_name\n    )\n    VALUES \n    (\n      _id_oppty_type,\n      _shortcut,\n      _name,\n      jsonb_object(ARRAY[_id_language], ARRAY[_name]),\n      _order,\n      _template_args\n    )\n  RETURNING id INTO _id;\n  RETURN _id;\nEND;\n$$;\n"},
		{"call with a multi-line argument list and ( on its own line", "modern", func() *config.Config { return own("../templates/modern.yaml") },
			"CREATE FUNCTION f() RETURNS trigger LANGUAGE plpgsql AS $$\nBEGIN\n    PERFORM clients.update_all\n    (\n        _id_client => NEW.id_client,\n        _eventtime => NOW(),\n        _open_ticket_incr => 0 -- no ticket increment\n    );\n    RETURN NEW;\nEND;\n$$;\n"},
		{"CASE after a JOIN … AND item, source indented with tabs", "modern", nil,
			"CREATE FUNCTION f() RETURNS void LANGUAGE plpgsql AS $$\nBEGIN\n\t\tSELECT se.id\n\t\tFROM g\n\t\tINNER JOIN live_scoreboard.lst_sport_events se ON se.id_sport_type = gp.id_sport_type AND se.id_event_type =\n\t\t\tCASE (d ->> 'type')::character varying\n\t\t\t\tWHEN 'goal' THEN 'gol'\n\t\t\tELSE\n\t\t\t\t(d ->> 'type')::character varying\n\t\t\tEND\n\t\tINTO STRICT _id;\nEND;\n$$;\n"},
		{"JOIN subquery followed by a block comment, source indented with tabs", "ai", nil,
			"CREATE FUNCTION f() RETURNS void LANGUAGE plpgsql AS $$\nBEGIN\n\t\tSELECT 1\n\t\tFROM a\n\t\t\tJOIN (SELECT id, count(x) AS n\n\t\t\t\tFROM b\n\t\t\t\tGROUP BY id) ti ON ti.id = a.id\n\t\t\t/*JOIN (SELECT 2\n\t\t\t\t\tFROM c) ti2 ON ti2.id = a.id\n\t\t\t*/\n\t\tINTO _x;\nEND;\n$$;\n"},
		{"join of a subquery with a -- comment inside, USING on its own line", "modern", nil,
			"SELECT a, b\nFROM t LEFT JOIN (\n  SELECT account, sum(a) m1\n  FROM (--account, col\n    SELECT account, sum(amount) a FROM y GROUP BY 1\n  ) t\n  GROUP BY 1\n) AS m\nUSING (account)\nWHERE p = 1;\n"},
		{"JOIN whose table starts on a line of its own in the input", "ai", nil,
			"CREATE FUNCTION f() RETURNS void LANGUAGE plpgsql AS $$\nBEGIN\n  FOR r IN SELECT a FROM t\n\nLEFT JOIN\n\n(SELECT credit_account, sum(d1) d1, sum(d2) d2, sum(d3) d3, sum(d4) d4, sum(d5) d5, sum(d6) d6,\n  sum(d7) d7, sum(d8) d8, sum(d9) d9, sum(d10) d10, sum(d11) d11, sum(d12) d12 FROM (\n\n    SELECT credit_account, sum(amount) d1, 0 d2, 0 d3, 0 d4, 0 d5, 0 d6, 0 d7, 0 d8, 0 d9, 0 d10, 0 d11, 0 d12\n    FROM accounting.records WHERE period_id = _period AND to_char(\"timestamp\", 'MM') = '01' GROUP BY credit_account\n\n  ) d GROUP BY credit_account) sd ON account = credit_account\n  LOOP NULL; END LOOP;\nEND;\n$$;\n"},
		{"AND at the end of the line with comment lines between the conditions", "postgresql",
			tweak("../templates/postgresql.yaml", func(c *config.Config) { c.Layout.Content.LogicalOperatorPosition = config.LogicalOperatorEnd }),
			"SELECT 1\nFROM t\nWHERE fo.id_client = _rec.id_client\n  -- zajima nas pouze rozhodujici den\n  AND fo.day = 1\n  AND fo.type IN (1, 11) -- sazky\n  AND fo.x = 2;\n"},
		{"aligned aliases with a comment line between the items", "modern",
			tweak("../templates/modern.yaml", func(c *config.Config) { c.Layout.Content.AlignAliases = config.AlignItemsAlign }),
			"SELECT c.gid,\n/*null manually_processed,*/\ncoalesce(a.subjekt, c.subjekt) AS subjekt,\nc.street,\ncoalesce(a.country_code, c.country_code) AS country_code\nFROM c;\n"},
		{"aligned alias of an item the line wrapping opens up", "modern",
			tweak("../templates/modern.yaml", func(c *config.Config) {
				c.Layout.Content.AlignAliases = config.AlignItemsAlign
				c.Layout.ParenIndent.Open = config.ParenOpenOwnLine
			}),
			"SELECT lt.id_tip,\n  COALESCE((lt.tip_name_lang ->> LOWER (_id_language))::character varying, (lt.tip_name_lang ->> _default_language)::character varying, lt.tip_name) AS tip_name,\n  COALESCE((lt.tip_shortname_lang ->> LOWER (_id_language))::character varying, (lt.tip_shortname_lang ->> _default_language)::character varying, lt.tip_shortname) AS tip_shortname\nFROM opportunities.lst_tips lt;\n"},
		{"alias column not set by an item the wrapping breaks up", "modern",
			tweak("../templates/modern.yaml", func(c *config.Config) { c.Layout.Content.AlignAliases = config.AlignItemsAlign }),
			"SELECT regexp_replace(COALESCE(u.user_fname || ' ', '') || COALESCE(u.user_lname || ' ', '') || '- ' || u.user_login, '^[- ]+', '') AS display_name,\n r.user_role_name,\n (d.department_code || ' - ' || d.department_name) AS description\nFROM u;\n"},
		{"JOIN LATERAL subquery with ( on its own line, space-indented body, tab indent", "modern",
			tweak("../templates/modern.yaml", func(c *config.Config) {
				c.Layout.ParenIndent.Open = config.ParenOpenOwnLine
				c.Layout.Content.JoinOnPosition = config.JoinOnNewline
				c.Layout.Content.AlignAliases = config.AlignItemsAlign
				c.Layout.Indent.Type = config.IndentTypeTab
			}),
			"CREATE FUNCTION f() RETURNS void LANGUAGE plpgsql AS $$\nBEGIN\n    UPDATE frontend_aggregs.tab_agg_matches m\n    SET (opportunities_stops, opportunities_stops_tz) = (s.agg_stop_ts, s.agg_stop_ts_tz)\n    FROM live_scoreboard.tab_games g\n    JOIN LATERAL (\n        SELECT json_agg(os.stop_ts ORDER BY os.stop_ts) AS agg_stop_ts\n        FROM live_opportunities.tab_opportunities_stops os\n        WHERE os.id_opportunity = g.id_opportunity\n    ) AS s ON TRUE\n    WHERE g.id_game = m.id_game;\nEND;\n$$;\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var cfg *config.Config
			if tc.cfg != nil {
				cfg = tc.cfg()
			} else {
				c, err := config.Load("../templates/" + tc.tmpl + ".yaml")
				if err != nil {
					t.Fatal(err)
				}
				cfg = c
			}
			once, err := formatter.Format(tc.sql, cfg)
			if err != nil {
				t.Fatal(err)
			}
			twice, err := formatter.Format(once, cfg)
			if err != nil {
				t.Fatal(err)
			}
			if once != twice {
				t.Errorf("not idempotent\n--- first ---\n%s\n--- second ---\n%s", once, twice)
			}
		})
	}
}
