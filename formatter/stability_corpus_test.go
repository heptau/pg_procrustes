package formatter_test

import (
	"testing"

	"github.com/heptau/pg_procrustes/config"
	"github.com/heptau/pg_procrustes/formatter"
)

// Shapes found by running ~111,000 real .sql files through the shipped templates
// (scripts/corpus_check.go): each used to give a different text on the second run, several
// added another level of indentation on every run. Each is formatted three times with every
// template; the runs must agree.
func TestStabilityOfCorpusShapes(t *testing.T) {
	inputs := map[string]string{
		"row with a multi-line constant after a comment line (leading commas)": "INSERT INTO q (a, b, c, d)\nVALUES (1, 'x', 'y'\n        , '\n            SELECT 1\n          '\n        )\n    -- insert:\n  , (10, 'x', 'y'\n        , '\n            SELECT 2\n          '\n        );\n",
		"last row followed by comments and blank lines":                        "INSERT INTO validator.lst_queries (id_query, queries_name, queries_description, queries_sql)\nVALUES ( 403, 'fail_if__already_have_verified_phone'   , 'Fail if client already have phone number verified.'\n        , '\n            SELECT 1\n          '\n        )\n\n-- param_required_to:\n\n;\n",
		"commented-out last row after leading commas":                          "INSERT INTO t (a, b)\nVALUES (1, 2)\n     , (3, 4)\n     -- @x: wrong entry\n     --, (5, 6)\n;\n",
		"empty comment line between insert columns":                            "insert into some_table (\n    column_number_one,\n    column_number_two,\n    column_number_three,\n    column_number_four,\n    --\n    column_number_five,\n    -- note\n    column_number_six\n)\nvalues (2000000 + 1, 1, 3, 1, 0, 'PLN', 2000000 * 1000 + 1, 0.01, 1, 0, 0, 0);\n",
		"comment between two columns, leading commas":                          "create table node (\n  nid serial,\n  created int not null default '0',\n  -- snip\n  other int\n);\n",
		"CREATE INDEX with WHERE on a line of its own":                         "create unique index concurrently ux on opportunities.tab_odds using btree (id_tip, id_opportunity)\n    where\n    (\n        id_odds > 836107228 -- prod\n        and\n        (id_tip_name_replace_type not in (6, 7) or id_tip_name_replace_type is null)\n    );\n",
		"statements indented inside begin/commit":                              "begin;\n\n    select\n    id_client\n    , id_ticket\n    from tickets\n    where id_client = 1;\n\n    select pos_api.add_contact_relations(\n        _id_a => 1,\n        _id_b => 2\n    );\n\ncommit;\n",
		"comment line between a subquery and the next join":                    "CREATE FUNCTION housing.xml_bill(_year integer, _owner public.typ_kod DEFAULT NULL::character varying) RETURNS text\n    AS $$\n  for _owner in select z.flat_code, z.owner, greatest(z.acquisition, _start) b, least(min(owners.acquisition)-1, ('31.12.'||_year)::date) e,\n    from (--vlastnici nastehovani pred pocatkem (roku)\n      select flat_code, owner, acquisition from housing.owners where (flat_code, acquisition)\n    ) z\n    --doplneni adresy vlastnika\n    left join common._contacts() on z.owner=firma\n    end loop;\nend;$$;\n",
		"operator on a line of its own inside CAST":                            "SELECT CAST(CAST(date 'today' + time with time zone '05:30'\n            + interval '02:01' AS time with time zone) AS time) AS \"07:31:00\";\n",
	}
	for _, tmpl := range []string{"postgresql", "ai", "modern"} {
		cfg, err := config.Load("../templates/" + tmpl + ".yaml")
		if err != nil {
			t.Fatal(err)
		}
		for name, src := range inputs {
			t.Run(tmpl+"/"+name, func(t *testing.T) {
				var runs [3]string
				in := src
				for i := range runs {
					out, err := formatter.Format(in, cfg)
					if err != nil {
						t.Fatalf("run %d: %v\n%s", i+1, err, in)
					}
					runs[i], in = out, out
				}
				if runs[0] != runs[1] || runs[1] != runs[2] {
					t.Errorf("runs differ\n--- 1 ---\n%s\n--- 2 ---\n%s\n--- 3 ---\n%s", runs[0], runs[1], runs[2])
				}
			})
		}
	}
}
