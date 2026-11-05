package formatter_test

import (
	"strings"
	"testing"

	"github.com/heptau/pg_procrustes/config"
	"github.com/heptau/pg_procrustes/formatter"
)

const assignBody = `create function f(a int) returns int language plpgsql as $$
declare
  v int := 0;
  w text = 'x';
  c cursor for select * from t where a = 1;
begin
  v := 1;
  w = 'y';
  r.field := 2;
  arr[1] = 3;
  if v = 1 then
    v := v + 1;
  else
    v = 5;
  end if;
  v := case when a = 1 then 1 else 2 end;
  update t set x = 1 where y = 2;
  perform g(p := 1, q = 2);
  raise exception 'x' using errcode = 'P0001';
  return v;
end
$$;
`

func assignCfg(t *testing.T, m config.PLpgSQLAssignment) *config.Config {
	t.Helper()
	c, err := config.Load("/nonexistent/.pg_procrustes.yaml")
	if err != nil {
		t.Fatal(err)
	}
	c.PLpgSQLAssignment = m
	return c
}

func TestPLpgSQLAssignmentPreserveChangesNothing(t *testing.T) {
	got, err := formatter.Format(assignBody, assignCfg(t, config.AssignPreserve))
	if err != nil {
		t.Fatal(err)
	}
	if got != assignBody {
		t.Errorf("changed:\n%s", got)
	}
}

func TestPLpgSQLAssignmentColonEquals(t *testing.T) {
	c := assignCfg(t, config.AssignColonEquals)
	got, err := formatter.Format(assignBody, c)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"v int := 0;", "w text := 'x';", "v := 1;", "w := 'y';", "r.field := 2;", "arr[1] := 3;",
		"    v := 5;", "v := case when a = 1 then 1 else 2 end;",
		// untouched: cursor query, comparison, UPDATE SET, named argument, USING option
		"where a = 1;", "if v = 1 then", "set x = 1 where y = 2;", "g(p := 1, q = 2)", "errcode = 'P0001'",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if again, _ := formatter.Format(got, c); again != got {
		t.Errorf("not idempotent:\n%s\n---\n%s", got, again)
	}
}

func TestPLpgSQLAssignmentEquals(t *testing.T) {
	c := assignCfg(t, config.AssignEquals)
	got, err := formatter.Format(assignBody, c)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"v int = 0;", "v = 1;", "r.field = 2;", "v = v + 1;", "v = case when", "g(p := 1, q = 2)"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "v :=") || strings.Contains(got, "int :=") {
		t.Errorf("a := is left in:\n%s", got)
	}
	if again, _ := formatter.Format(got, c); again != got {
		t.Errorf("not idempotent:\n%s\n---\n%s", got, again)
	}
}
