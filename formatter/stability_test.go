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
