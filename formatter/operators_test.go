package formatter_test

import (
	"testing"

	pg_query "github.com/pganalyze/pg_query_go/v6"

	"github.com/heptau/pg_procrustes/config"
	"github.com/heptau/pg_procrustes/formatter"
)

// "<>" followed directly by + or - is two tokens (a trailing sign belongs to an operator only
// if it has one of ~!@#%^&|`), but "!=+" is one operator: the C spelling must not glue them.
func TestInequalityCKeepsGluedSignSeparate(t *testing.T) {
	c := verbatimCfg(t)
	c.InequalityOp = config.InequalityC
	for _, in := range []string{"select a<>+1 from t;\n", "select a<>-1 from t;\n"} {
		got, err := formatter.Format(in, c)
		if err != nil {
			t.Fatal(err)
		}
		a, _ := pg_query.Fingerprint(in)
		b, errB := pg_query.Fingerprint(got)
		if errB != nil || a != b {
			t.Errorf("%q → %q: the parse tree changed (%v)", in, got, errB)
		}
	}
}
