package formatter_test

import (
	"testing"

	"github.com/heptau/pg_procrustes/config"
	"github.com/heptau/pg_procrustes/formatter"
)

func fnCfg(t *testing.T, mutate func(*config.Config)) *config.Config {
	t.Helper()
	c, _ := loadStyled("")
	mutate(c)
	return c
}

func TestCallParenSpacing(t *testing.T) {
	const in = "select count (*), my_func(b), coalesce (a, b), (a + b) * c from t where x in (1, 2);\n"
	cases := map[config.ParenSpacingMode]string{
		config.ParenSpacingPreserve: "SELECT count (*), my_func(b), COALESCE (a, b), (a + b) * c FROM t WHERE x IN (1, 2);\n",
		config.ParenSpacingAdd:      "SELECT count (*), my_func (b), COALESCE (a, b), (a + b) * c FROM t WHERE x IN (1, 2);\n",
		config.ParenSpacingRemove:   "SELECT count(*), my_func(b), COALESCE(a, b), (a + b) * c FROM t WHERE x IN (1, 2);\n",
	}
	for mode, want := range cases {
		c := fnCfg(t, func(c *config.Config) { c.CallParenSpacing = mode })
		got, err := formatter.Format(in, c)
		if err != nil || got != want {
			t.Errorf("%s:\ngot  %q (%v)\nwant %q", mode, got, err, want)
		}
		if again, _ := formatter.Format(got, c); again != got {
			t.Errorf("%s not idempotent: %q", mode, again)
		}
	}
}

func TestBuiltinFunctionsCase(t *testing.T) {
	const in = "select Count(*), my_func(b), date_trunc('day', t) from t;\n"
	c := fnCfg(t, func(c *config.Config) { c.BuiltinFunctions.Case = config.CaseUpper })
	got, _ := formatter.Format(in, c)
	if want := "SELECT COUNT(*), my_func(b), DATE_TRUNC('day', t) FROM t;\n"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
	c = fnCfg(t, func(c *config.Config) {
		c.BuiltinFunctions.Case = config.CaseUpper
		c.BuiltinFunctions.Exceptions = []string{"date_trunc"}
	})
	got, _ = formatter.Format(in, c)
	if want := "SELECT COUNT(*), my_func(b), date_trunc('day', t) FROM t;\n"; got != want {
		t.Errorf("exceptions: got %q want %q", got, want)
	}
	// Default: built-ins follow functions.case.
	d := fnCfg(t, func(*config.Config) {})
	if got, _ := formatter.Format(in, d); got != "SELECT count(*), my_func(b), date_trunc('day', t) FROM t;\n" {
		t.Errorf("default changed: %q", got)
	}
	bad := fnCfg(t, func(c *config.Config) { c.BuiltinFunctions.Case = "shout"; c.CallParenSpacing = "x" })
	if config.Validate(bad) == nil {
		t.Error("expected validation error")
	}
}
