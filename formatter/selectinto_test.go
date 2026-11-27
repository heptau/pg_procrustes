package formatter_test

import (
	"strings"
	"testing"

	"github.com/heptau/pg_procrustes/config"
	"github.com/heptau/pg_procrustes/formatter"
)

const selectIntoBody = `create function f(_year int) returns void language plpgsql as $$
declare
  _period record;
  a int;
  b int;
begin
  SELECT INTO _period * FROM accounting.periods WHERE period = _year :: varchar;
  select into r.x, b count(*), max(y) from t where z = 1 for update;
  select into strict a 1;
  select into a (select 1 from u), case when a = 1 then 2 end from t;
  select x, y into a, b from t;
  select into a -- why
    b from t;
  insert into t select 1;
  execute 'select into a 1';
end
$$;
select a :: text, b ::int, c:: int[], e /* c */ :: int, f
  :: text from t;
`

func intoCfg(t *testing.T, into config.PLpgSQLIntoMode, cast config.CastSpacingMode) *config.Config {
	t.Helper()
	c, err := config.Load("/nonexistent/.pg_procrustes.yaml")
	if err != nil {
		t.Fatal(err)
	}
	c.PLpgSQLInto = into
	c.CastSpacing = cast
	return c
}

func TestSelectIntoAndCastSpacingPreserveChangeNothing(t *testing.T) {
	got, err := formatter.Format(selectIntoBody, intoCfg(t, config.PLpgSQLIntoPreserve, config.CastSpacingPreserve))
	if err != nil {
		t.Fatal(err)
	}
	if got != selectIntoBody {
		t.Errorf("changed:\n%s", got)
	}
}

func TestPLpgSQLIntoAfterList(t *testing.T) {
	c := intoCfg(t, config.PLpgSQLIntoAfterList, config.CastSpacingPreserve)
	want := `create function f(_year int) returns void language plpgsql as $$
declare
  _period record;
  a int;
  b int;
begin
  SELECT * INTO _period FROM accounting.periods WHERE period = _year :: varchar;
  select count(*), max(y) into r.x, b from t where z = 1 for update;
  select 1 into strict a;
  select (select 1 from u), case when a = 1 then 2 end into a from t;
  select x, y into a, b from t;
  select into a -- why
    b from t;
  insert into t select 1;
  execute 'select into a 1';
end
$$;
select a :: text, b ::int, c:: int[], e /* c */ :: int, f
  :: text from t;
`
	got, err := formatter.Format(selectIntoBody, c)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	if again, _ := formatter.Format(got, c); again != got {
		t.Errorf("not idempotent:\n%s", again)
	}
}

func TestCastSpacingRemove(t *testing.T) {
	c := intoCfg(t, config.PLpgSQLIntoPreserve, config.CastSpacingRemove)
	got, err := formatter.Format(selectIntoBody, c)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		"period = _year::varchar;",
		"select a::text, b::int, c::int[], e /* c */ ::int, f\n  ::text from t;",
	} {
		if !strings.Contains(got, s) {
			t.Errorf("missing %q in:\n%s", s, got)
		}
	}
	if again, _ := formatter.Format(got, c); again != got {
		t.Errorf("not idempotent:\n%s", again)
	}
}

// The ai template with both options: the example from a real migration.
func TestAITemplateSelectIntoAndCast(t *testing.T) {
	c, err := config.Load("../templates/ai.yaml")
	if err != nil {
		t.Fatal(err)
	}
	in := "create function f(_year int) returns void language plpgsql as $$\nBEGIN\n  SELECT INTO _period * FROM accounting.obdobia WHERE obdobie = _year :: varchar;\n  SELECT INTO _company * FROM public.vlastna_firma WHERE plati_od <= date_cz() ORDER BY plati_od DESC LIMIT 1;\nEND\n$$;\n"
	want := "create function f(_year int) returns void language plpgsql as $$\nbegin\n  select *\n  into _period\n  from accounting.obdobia\n  where obdobie=_year::varchar;\n  select *\n  into _company\n  from vlastna_firma\n  where plati_od<=date_cz()\n  order by plati_od desc\n  limit 1;\nend\n$$;"
	got, err := formatter.Format(in, c)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}
